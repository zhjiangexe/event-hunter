package shipping

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"event-hunter/backend/internal/demo/emission"
	"event-hunter/backend/internal/demo/event"
	"event-hunter/backend/internal/demo/outbox"
	"event-hunter/backend/internal/demo/telemetry"
)

type Service struct {
	db             *sql.DB
	outbox         outbox.EventRepository
	serviceVersion string
	eventDelay     time.Duration
}

var tracer = otel.Tracer("event-hunter/backend/internal/demo/shipping")

func NewService(db *sql.DB, serviceVersion string, eventDelay time.Duration) *Service {
	return &Service{db: db, serviceVersion: serviceVersion, eventDelay: eventDelay}
}

// HandlePaymentCompleted creates a shipment and its ShipmentCreated event in
// one local transaction. A redelivered PaymentCompleted is acknowledged by the
// existing order_id unique key and produces no duplicate side effect.
func (service *Service) HandlePaymentCompleted(ctx context.Context, paymentEvent event.Envelope) (err error) {
	ctx, span := tracer.Start(ctx, "shipping.HandlePaymentCompleted")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	span.SetAttributes(
		attribute.String("event.id", paymentEvent.EventID),
		attribute.String("event.type", paymentEvent.EventType),
		attribute.String("correlation.id", paymentEvent.CorrelationID),
	)
	if paymentEvent.EventType != "PaymentCompleted" {
		return fmt.Errorf("unsupported event type %q", paymentEvent.EventType)
	}
	orderID, profile, err := decodePaymentPayload(paymentEvent.Payload)
	if err != nil {
		return err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin shipping transaction: %w", err)
	}
	defer tx.Rollback()

	var existingID string
	err = tx.QueryRowContext(ctx, "SELECT id::text FROM shipments WHERE order_id = $1", orderID).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing shipment: %w", err)
	}

	shipmentID, err := newShipmentID()
	if err != nil {
		return err
	}
	span.SetAttributes(attribute.String("shipment.id", shipmentID), attribute.String("order.id", orderID))
	finalStatus := "CREATED"
	if profile == event.ProfileShipmentDelivered || profile == event.ProfileReturnRefund {
		finalStatus = "DELIVERED"
	}
	if profile == event.ProfileReturnRefund {
		finalStatus = "RETURN_RECEIVED"
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO shipments (id, order_id, correlation_id, provider, status) VALUES (gen_random_uuid(), $1, $2, 'demo-logistics', $3)",
		orderID, paymentEvent.CorrelationID,
		finalStatus,
	); err != nil {
		return fmt.Errorf("insert shipment: %w", err)
	}
	created, err := event.NewEnvelope(
		"ShipmentCreated", "shipping-service", paymentEvent.CorrelationID, "Shipment", shipmentID, 1,
		&paymentEvent.EventID, paymentEvent.TraceID,
		map[string]any{"shipmentId": shipmentID, "orderId": orderID, "provider": "demo-logistics", "status": "CREATED"},
	)
	if err != nil {
		return err
	}
	telemetry.PreparingOutboxEvent(ctx, created, "shipping.events", service.eventDelay)
	if err := emission.Wait(ctx, service.eventDelay); err != nil {
		wrapped := fmt.Errorf("wait before ShipmentCreated emission: %w", err)
		telemetry.FailedOutboxEvent(ctx, created, "shipping.events", telemetry.EmissionFailureDelay, wrapped)
		return wrapped
	}
	if err := service.outbox.Append(ctx, tx, created, "shipping.events", service.serviceVersion); err != nil {
		telemetry.FailedOutboxEvent(ctx, created, "shipping.events", telemetry.EmissionFailureOutboxAppend, err)
		return err
	}
	events := []event.Envelope{created}
	previousID := created.EventID
	sequence := uint64(2)
	appendShipmentEvent := func(eventType string, payload map[string]any) error {
		envelope, buildErr := event.NewEnvelope(
			eventType, "shipping-service", paymentEvent.CorrelationID, "Shipment", shipmentID, sequence,
			&previousID, paymentEvent.TraceID, payload,
		)
		if buildErr != nil {
			return buildErr
		}
		telemetry.PreparingOutboxEvent(ctx, envelope, "shipping.events", service.eventDelay)
		if waitErr := emission.Wait(ctx, service.eventDelay); waitErr != nil {
			wrapped := fmt.Errorf("wait before %s emission: %w", eventType, waitErr)
			telemetry.FailedOutboxEvent(ctx, envelope, "shipping.events", telemetry.EmissionFailureDelay, wrapped)
			return wrapped
		}
		if appendErr := service.outbox.Append(ctx, tx, envelope, "shipping.events", service.serviceVersion); appendErr != nil {
			telemetry.FailedOutboxEvent(ctx, envelope, "shipping.events", telemetry.EmissionFailureOutboxAppend, appendErr)
			return appendErr
		}
		events = append(events, envelope)
		previousID = envelope.EventID
		sequence++
		return nil
	}
	if profile == event.ProfileShipmentDelivered || profile == event.ProfileReturnRefund {
		trackingNumber := "TRACK-" + strings.TrimPrefix(shipmentID, "SHIPMENT-")
		if err := appendShipmentEvent("ShipmentDispatched", map[string]any{
			"shipmentId": shipmentID, "orderId": orderID, "provider": "demo-logistics",
			"trackingNumber": trackingNumber, "status": "DISPATCHED",
		}); err != nil {
			return err
		}
		if profile == event.ProfileShipmentDelivered {
			if err := appendShipmentEvent("ShipmentInTransit", map[string]any{
				"shipmentId": shipmentID, "orderId": orderID, "location": "Taipei Hub", "status": "IN_TRANSIT",
			}); err != nil {
				return err
			}
		}
		if err := appendShipmentEvent("ShipmentDelivered", map[string]any{
			"shipmentId": shipmentID, "orderId": orderID, "deliveredAt": time.Now().UTC().Format(time.RFC3339Nano),
			"recipient": "Demo Recipient", "status": "DELIVERED",
		}); err != nil {
			return err
		}
	}
	if profile == event.ProfileReturnRefund {
		returnID, idErr := newBusinessID("RETURN")
		if idErr != nil {
			return idErr
		}
		requested, buildErr := event.NewEnvelope(
			"ReturnRequested", "shipping-service", paymentEvent.CorrelationID, "Return", returnID, 1,
			&previousID, paymentEvent.TraceID,
			map[string]any{"returnId": returnID, "orderId": orderID, "shipmentId": shipmentID, "reason": "demo customer return", "status": "REQUESTED"},
		)
		if buildErr != nil {
			return buildErr
		}
		telemetry.PreparingOutboxEvent(ctx, requested, "shipping.events", service.eventDelay)
		if err := emission.Wait(ctx, service.eventDelay); err != nil {
			wrapped := fmt.Errorf("wait before ReturnRequested emission: %w", err)
			telemetry.FailedOutboxEvent(ctx, requested, "shipping.events", telemetry.EmissionFailureDelay, wrapped)
			return wrapped
		}
		if err := service.outbox.Append(ctx, tx, requested, "shipping.events", service.serviceVersion); err != nil {
			telemetry.FailedOutboxEvent(ctx, requested, "shipping.events", telemetry.EmissionFailureOutboxAppend, err)
			return err
		}
		received, buildErr := event.NewEnvelope(
			"ReturnReceived", "shipping-service", paymentEvent.CorrelationID, "Return", returnID, 2,
			&requested.EventID, paymentEvent.TraceID,
			map[string]any{"returnId": returnID, "orderId": orderID, "receivedAt": time.Now().UTC().Format(time.RFC3339Nano), "condition": "GOOD", "status": "RECEIVED"},
		)
		if buildErr != nil {
			return buildErr
		}
		telemetry.PreparingOutboxEvent(ctx, received, "shipping.events", service.eventDelay)
		if err := emission.Wait(ctx, service.eventDelay); err != nil {
			wrapped := fmt.Errorf("wait before ReturnReceived emission: %w", err)
			telemetry.FailedOutboxEvent(ctx, received, "shipping.events", telemetry.EmissionFailureDelay, wrapped)
			return wrapped
		}
		if err := service.outbox.Append(ctx, tx, received, "shipping.events", service.serviceVersion); err != nil {
			telemetry.FailedOutboxEvent(ctx, received, "shipping.events", telemetry.EmissionFailureOutboxAppend, err)
			return err
		}
		events = append(events, requested, received)
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO returns (return_id,order_id,shipment_id,correlation_id,status) VALUES ($1,$2,$3,$4,'RECEIVED')",
			returnID, orderID, shipmentID, paymentEvent.CorrelationID,
		); err != nil {
			return fmt.Errorf("insert return: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		wrapped := fmt.Errorf("commit shipping transaction: %w", err)
		for _, envelope := range events {
			telemetry.FailedOutboxEvent(ctx, envelope, "shipping.events", telemetry.EmissionFailureTransactionCommit, wrapped)
		}
		return wrapped
	}
	for _, envelope := range events {
		telemetry.CommittedOutboxEvent(ctx, envelope, "shipping.events")
	}
	return nil
}

func decodePaymentPayload(payload map[string]any) (string, string, error) {
	orderID, ok := payload["orderId"].(string)
	if !ok || strings.TrimSpace(orderID) == "" {
		return "", "", fmt.Errorf("PaymentCompleted payload orderId is required")
	}
	profile, _ := payload["simulationProfile"].(string)
	profile, err := event.NormalizeSimulationProfile(profile)
	if err != nil {
		return "", "", err
	}
	return orderID, profile, nil
}

func DecodePaymentCompleted(data []byte) (event.Envelope, error) {
	envelope, err := event.Decode(data)
	if err != nil {
		return event.Envelope{}, err
	}
	if envelope.EventType != "PaymentCompleted" {
		return event.Envelope{}, fmt.Errorf("expected PaymentCompleted, got %q", envelope.EventType)
	}
	return envelope, nil
}

func DecodeConsumableEvent(data []byte) (event.Envelope, error) {
	return event.Decode(data)
}

func newShipmentID() (string, error) {
	return newBusinessID("SHIPMENT")
}

func newBusinessID(prefix string) (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return fmt.Sprintf("%s-%X", prefix, bytes), nil
}
