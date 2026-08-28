package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"event-hunter/backend/internal/demo/event"
)

func TestEventLifecycleLogsExposeEventTypeAndOutcome(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	envelope := event.Envelope{
		EventID:       "evt-1",
		EventType:     "OrderCreated",
		Producer:      "order-service",
		CorrelationID: "ORDER-1",
		AggregateType: "Order",
		AggregateID:   "ORDER-1",
		Sequence:      1,
	}

	PreparingOutboxEvent(context.Background(), envelope, "order.events", 2*time.Second)
	CommittedOutboxEvent(context.Background(), envelope, "order.events")
	FailedOutboxEvent(context.Background(), envelope, "order.events", EmissionFailureOutboxAppend, errors.New("database unavailable"))

	logs := output.String()
	for _, want := range []string{
		`msg="domain event emission starting: OrderCreated"`,
		`msg="domain event committed to outbox: OrderCreated"`,
		`msg="domain event emission failed: OrderCreated"`,
		"event.id=evt-1",
		"event.type=OrderCreated",
		"correlation.id=ORDER-1",
		"event.emission.phase=PREPARING",
		"event.emission.phase=COMMITTED",
		"event.emission.phase=FAILED",
		"event.emission.failure_stage=OUTBOX_APPEND",
		"event.emission.delay_ms=2000",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("lifecycle logs missing %q:\n%s", want, logs)
		}
	}
}
