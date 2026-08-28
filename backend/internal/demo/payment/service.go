package payment

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

var tracer = otel.Tracer("event-hunter/backend/internal/demo/payment")

func NewService(db *sql.DB, serviceVersion string, eventDelay time.Duration) *Service {
	return &Service{db: db, serviceVersion: serviceVersion, eventDelay: eventDelay}
}

// HandleOrderCreated applies the payment side effect and emits PaymentCompleted
// in the same local transaction. A repeated OrderCreated is acknowledged without
// creating another payment or outbox event.
func (service *Service) HandleOrderCreated(ctx context.Context, orderEvent event.Envelope) (err error) {
	ctx, span := tracer.Start(ctx, "payment.HandleOrderCreated")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	span.SetAttributes(
		attribute.String("event.id", orderEvent.EventID),
		attribute.String("event.type", orderEvent.EventType),
		attribute.String("correlation.id", orderEvent.CorrelationID),
	)
	if orderEvent.EventType != "OrderCreated" {
		return fmt.Errorf("unsupported event type %q", orderEvent.EventType)
	}
	orderID, customerAmount, currency, profile, err := decodeOrderPayload(orderEvent.Payload)
	if err != nil {
		return err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin payment transaction: %w", err)
	}
	defer tx.Rollback()

	var existingID string
	err = tx.QueryRowContext(ctx, "SELECT id::text FROM payments WHERE order_id = $1", orderID).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing payment: %w", err)
	}

	paymentID, err := newPaymentID()
	if err != nil {
		return err
	}
	span.SetAttributes(attribute.String("payment.id", paymentID), attribute.String("order.id", orderID))
	status := "COMPLETED"
	eventType := "PaymentCompleted"
	payload := map[string]any{
		"paymentId": paymentID, "orderId": orderID, "amount": customerAmount,
		"currency": currency, "simulationProfile": profile,
	}
	if profile == event.ProfilePaymentFailed {
		status = "FAILED"
		eventType = "PaymentFailed"
		payload = map[string]any{
			"paymentId": paymentID, "orderId": orderID, "reasonCode": "DEMO_DECLINED",
			"retryable": false, "status": "FAILED",
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO payments (id, payment_id, order_id, correlation_id, amount, currency, status) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)",
		paymentID, orderID, orderEvent.CorrelationID, customerAmount, currency, status,
	); err != nil {
		return fmt.Errorf("insert payment: %w", err)
	}
	envelope, err := event.NewEnvelope(
		eventType, "payment-service", orderEvent.CorrelationID, "Payment", paymentID, 1,
		&orderEvent.EventID, orderEvent.TraceID,
		payload,
	)
	if err != nil {
		return err
	}
	telemetry.PreparingOutboxEvent(ctx, envelope, "payment.events", service.eventDelay)
	if err := emission.Wait(ctx, service.eventDelay); err != nil {
		wrapped := fmt.Errorf("wait before %s emission: %w", eventType, err)
		telemetry.FailedOutboxEvent(ctx, envelope, "payment.events", telemetry.EmissionFailureDelay, wrapped)
		return wrapped
	}
	if err := service.outbox.Append(ctx, tx, envelope, "payment.events", service.serviceVersion); err != nil {
		telemetry.FailedOutboxEvent(ctx, envelope, "payment.events", telemetry.EmissionFailureOutboxAppend, err)
		return err
	}
	if err := tx.Commit(); err != nil {
		wrapped := fmt.Errorf("commit payment transaction: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "payment.events", telemetry.EmissionFailureTransactionCommit, wrapped)
		return wrapped
	}
	telemetry.CommittedOutboxEvent(ctx, envelope, "payment.events")
	return nil
}

func (service *Service) HandleReturnReceived(ctx context.Context, returnEvent event.Envelope) (err error) {
	ctx, span := tracer.Start(ctx, "payment.HandleReturnReceived")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	if returnEvent.EventType != "ReturnReceived" {
		return fmt.Errorf("unsupported event type %q", returnEvent.EventType)
	}
	orderID, ok := returnEvent.Payload["orderId"].(string)
	if !ok || strings.TrimSpace(orderID) == "" {
		return fmt.Errorf("ReturnReceived payload orderId is required")
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin refund transaction: %w", err)
	}
	defer tx.Rollback()
	var paymentID, status string
	var amount int
	if err := tx.QueryRowContext(ctx, "SELECT payment_id,amount,status FROM payments WHERE order_id=$1 FOR UPDATE", orderID).Scan(&paymentID, &amount, &status); err != nil {
		return fmt.Errorf("load payment for refund: %w", err)
	}
	if status == "REFUNDED" {
		return nil
	}
	if status != "COMPLETED" {
		return fmt.Errorf("payment status %s cannot be refunded", status)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE payments SET status='REFUNDED' WHERE order_id=$1", orderID); err != nil {
		return fmt.Errorf("mark payment refunded: %w", err)
	}
	envelope, err := event.NewEnvelope(
		"PaymentRefunded", "payment-service", returnEvent.CorrelationID, "Payment", paymentID, 2,
		&returnEvent.EventID, returnEvent.TraceID,
		map[string]any{"paymentId": paymentID, "orderId": orderID, "amount": amount, "reason": "demo return received"},
	)
	if err != nil {
		return err
	}
	telemetry.PreparingOutboxEvent(ctx, envelope, "payment.events", service.eventDelay)
	if err := emission.Wait(ctx, service.eventDelay); err != nil {
		wrapped := fmt.Errorf("wait before PaymentRefunded emission: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "payment.events", telemetry.EmissionFailureDelay, wrapped)
		return wrapped
	}
	if err := service.outbox.Append(ctx, tx, envelope, "payment.events", service.serviceVersion); err != nil {
		telemetry.FailedOutboxEvent(ctx, envelope, "payment.events", telemetry.EmissionFailureOutboxAppend, err)
		return err
	}
	if err := tx.Commit(); err != nil {
		wrapped := fmt.Errorf("commit refund transaction: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "payment.events", telemetry.EmissionFailureTransactionCommit, wrapped)
		return wrapped
	}
	telemetry.CommittedOutboxEvent(ctx, envelope, "payment.events")
	return nil
}

func decodeOrderPayload(payload map[string]any) (string, int, string, string, error) {
	orderID, ok := payload["orderId"].(string)
	if !ok || strings.TrimSpace(orderID) == "" {
		return "", 0, "", "", fmt.Errorf("OrderCreated payload orderId is required")
	}
	currency, ok := payload["currency"].(string)
	if !ok || len(currency) != 3 {
		return "", 0, "", "", fmt.Errorf("OrderCreated payload currency is invalid")
	}
	amount, ok := payload["totalAmount"].(float64)
	if !ok || amount <= 0 || amount != float64(int(amount)) {
		return "", 0, "", "", fmt.Errorf("OrderCreated payload totalAmount is invalid")
	}
	profile, _ := payload["simulationProfile"].(string)
	profile, err := event.NormalizeSimulationProfile(profile)
	if err != nil {
		return "", 0, "", "", err
	}
	return orderID, int(amount), strings.ToUpper(currency), profile, nil
}

func DecodeOrderCreated(data []byte) (event.Envelope, error) {
	envelope, err := event.Decode(data)
	if err != nil {
		return event.Envelope{}, err
	}
	if envelope.EventType != "OrderCreated" {
		return event.Envelope{}, fmt.Errorf("expected OrderCreated, got %q", envelope.EventType)
	}
	return envelope, nil
}

func DecodeConsumableEvent(data []byte) (event.Envelope, error) {
	return event.Decode(data)
}

func newPaymentID() (string, error) {
	return newBusinessID("PAYMENT")
}

func newBusinessID(prefix string) (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return prefix + "-" + fmt.Sprintf("%X", bytes), nil
}
