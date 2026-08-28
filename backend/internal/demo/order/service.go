package order

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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

var ErrIdempotencyConflict = errors.New("idempotency key was used with a different request")
var tracer = otel.Tracer("event-hunter/backend/internal/demo/order")

type CreateOrderRequest struct {
	CustomerID        string `json:"customer_id"`
	TotalAmount       int    `json:"total_amount"`
	Currency          string `json:"currency"`
	SimulationProfile string `json:"simulation_profile,omitempty"`
}

type AcceptedOrder struct {
	OrderID       string    `json:"order_id"`
	CorrelationID string    `json:"correlation_id"`
	Status        string    `json:"status"`
	AcceptedAt    time.Time `json:"accepted_at"`
}

type Service struct {
	db             *sql.DB
	outbox         outbox.EventRepository
	serviceVersion string
	eventDelay     time.Duration
	now            func() time.Time
}

func NewService(db *sql.DB, serviceVersion string, eventDelay time.Duration) *Service {
	return &Service{db: db, serviceVersion: serviceVersion, eventDelay: eventDelay, now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) CreateOrder(ctx context.Context, idempotencyKey string, request CreateOrderRequest) (accepted AcceptedOrder, err error) {
	ctx, span := tracer.Start(ctx, "order.CreateOrder")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	span.SetAttributes(
		attribute.String("order.customer_id", request.CustomerID),
		attribute.String("order.currency", strings.ToUpper(request.Currency)),
		attribute.Int("order.total_amount", request.TotalAmount),
	)
	if strings.TrimSpace(idempotencyKey) == "" {
		return AcceptedOrder{}, fmt.Errorf("Idempotency-Key is required")
	}
	if strings.TrimSpace(request.CustomerID) == "" || request.TotalAmount <= 0 || !validCurrency(request.Currency) {
		return AcceptedOrder{}, fmt.Errorf("customer_id, positive total_amount and three-letter currency are required")
	}
	profile, err := event.NormalizeSimulationProfile(request.SimulationProfile)
	if err != nil {
		return AcceptedOrder{}, err
	}
	request.SimulationProfile = profile

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return AcceptedOrder{}, fmt.Errorf("marshal order request: %w", err)
	}
	requestHash := sha256.Sum256(requestBytes)

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return AcceptedOrder{}, fmt.Errorf("begin order transaction: %w", err)
	}
	defer tx.Rollback()

	// Serialize only requests sharing this idempotency key. The lock lives for
	// the transaction, so different keys remain fully concurrent without a
	// process-local mutex that would fail after scaling the service.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", idempotencyKey); err != nil {
		return AcceptedOrder{}, fmt.Errorf("lock idempotency key: %w", err)
	}

	var previousHash string
	var previousResponse []byte
	err = tx.QueryRowContext(ctx,
		"SELECT request_hash, accepted_response FROM idempotency_keys WHERE key = $1 FOR UPDATE",
		idempotencyKey,
	).Scan(&previousHash, &previousResponse)
	if err == nil {
		if previousHash != hex.EncodeToString(requestHash[:]) {
			return AcceptedOrder{}, ErrIdempotencyConflict
		}
		var accepted AcceptedOrder
		if err := json.Unmarshal(previousResponse, &accepted); err != nil {
			return AcceptedOrder{}, fmt.Errorf("decode stored idempotent response: %w", err)
		}
		return accepted, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AcceptedOrder{}, fmt.Errorf("read idempotency key: %w", err)
	}

	orderID, err := newBusinessID("ORDER")
	if err != nil {
		return AcceptedOrder{}, err
	}
	accepted = AcceptedOrder{OrderID: orderID, CorrelationID: orderID, Status: "ACCEPTED", AcceptedAt: service.now()}
	span.SetAttributes(attribute.String("order.id", orderID), attribute.String("correlation.id", accepted.CorrelationID))
	acceptedBytes, err := json.Marshal(accepted)
	if err != nil {
		return AcceptedOrder{}, fmt.Errorf("marshal accepted order: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO orders (id, customer_id, total_amount, currency, correlation_id, created_at) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)",
		request.CustomerID, request.TotalAmount, strings.ToUpper(request.Currency), accepted.CorrelationID, accepted.AcceptedAt,
	); err != nil {
		return AcceptedOrder{}, fmt.Errorf("insert order: %w", err)
	}

	envelope, err := event.NewEnvelope(
		"OrderCreated", "order-service", accepted.CorrelationID, "Order", accepted.OrderID, 1, nil, nil,
		map[string]any{
			"orderId": accepted.OrderID, "customerId": request.CustomerID,
			"totalAmount": request.TotalAmount, "currency": strings.ToUpper(request.Currency),
			"simulationProfile": profile,
		},
	)
	if err != nil {
		return AcceptedOrder{}, err
	}
	telemetry.PreparingOutboxEvent(ctx, envelope, "order.events", service.eventDelay)
	if err := emission.Wait(ctx, service.eventDelay); err != nil {
		wrapped := fmt.Errorf("wait before OrderCreated emission: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureDelay, wrapped)
		return AcceptedOrder{}, wrapped
	}
	if err := service.outbox.Append(ctx, tx, envelope, "order.events", service.serviceVersion); err != nil {
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureOutboxAppend, err)
		return AcceptedOrder{}, err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO idempotency_keys (key, request_hash, order_id, accepted_response) SELECT $1, $2, id, $3::jsonb FROM orders WHERE correlation_id = $4",
		idempotencyKey, hex.EncodeToString(requestHash[:]), acceptedBytes, accepted.CorrelationID,
	); err != nil {
		wrapped := fmt.Errorf("insert idempotency key: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureTransactionCommit, wrapped)
		return AcceptedOrder{}, wrapped
	}
	if err := tx.Commit(); err != nil {
		wrapped := fmt.Errorf("commit order transaction: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureTransactionCommit, wrapped)
		return AcceptedOrder{}, wrapped
	}
	telemetry.CommittedOutboxEvent(ctx, envelope, "order.events")
	return accepted, nil
}

func (service *Service) HandlePaymentFailed(ctx context.Context, paymentEvent event.Envelope) (err error) {
	ctx, span := tracer.Start(ctx, "order.HandlePaymentFailed")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	if paymentEvent.EventType != "PaymentFailed" {
		return fmt.Errorf("unsupported event type %q", paymentEvent.EventType)
	}
	orderID, ok := paymentEvent.Payload["orderId"].(string)
	if !ok || strings.TrimSpace(orderID) == "" {
		return fmt.Errorf("PaymentFailed payload orderId is required")
	}
	reason, ok := paymentEvent.Payload["reasonCode"].(string)
	if !ok || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("PaymentFailed payload reasonCode is required")
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin order cancellation transaction: %w", err)
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, "SELECT status FROM orders WHERE correlation_id=$1 FOR UPDATE", paymentEvent.CorrelationID).Scan(&status); err != nil {
		return fmt.Errorf("load order for cancellation: %w", err)
	}
	if status == "CANCELLED" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE orders SET status='CANCELLED' WHERE correlation_id=$1", paymentEvent.CorrelationID); err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}
	envelope, err := event.NewEnvelope(
		"OrderCancelled", "order-service", paymentEvent.CorrelationID, "Order", orderID, 2,
		&paymentEvent.EventID, paymentEvent.TraceID,
		map[string]any{"orderId": orderID, "reason": "payment failed: " + reason},
	)
	if err != nil {
		return err
	}
	telemetry.PreparingOutboxEvent(ctx, envelope, "order.events", service.eventDelay)
	if err := emission.Wait(ctx, service.eventDelay); err != nil {
		wrapped := fmt.Errorf("wait before OrderCancelled emission: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureDelay, wrapped)
		return wrapped
	}
	if err := service.outbox.Append(ctx, tx, envelope, "order.events", service.serviceVersion); err != nil {
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureOutboxAppend, err)
		return err
	}
	if err := tx.Commit(); err != nil {
		wrapped := fmt.Errorf("commit order cancellation: %w", err)
		telemetry.FailedOutboxEvent(ctx, envelope, "order.events", telemetry.EmissionFailureTransactionCommit, wrapped)
		return wrapped
	}
	telemetry.CommittedOutboxEvent(ctx, envelope, "order.events")
	return nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, character := range currency {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func newBusinessID(prefix string) (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return prefix + "-" + strings.ToUpper(hex.EncodeToString(bytes)), nil
}
