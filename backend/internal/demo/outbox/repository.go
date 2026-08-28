package outbox

import (
	"context"
	"database/sql"
	"fmt"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"event-hunter/backend/internal/demo/event"
)

type EventRepository struct{}

func (EventRepository) Append(ctx context.Context, tx *sql.Tx, envelope event.Envelope, topic, serviceVersion string) error {
	envelope, traceParent, traceState := applyTraceContext(ctx, envelope)
	payload, err := envelope.JSON()
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}
	const statement = `
INSERT INTO outbox_events
    (id, aggregate_type, aggregate_id, event_type, topic_name, correlation_id,
     trace_id, trace_parent, trace_state, service_version, payload, occurred_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, now())`
	if _, err := tx.ExecContext(ctx, statement,
		envelope.EventID, envelope.AggregateType, envelope.AggregateID, envelope.EventType,
		topic, envelope.CorrelationID, envelope.TraceID, nullable(traceParent), nullable(traceState),
		serviceVersion, payload, envelope.OccurredAt,
	); err != nil {
		return fmt.Errorf("append outbox event %s: %w", envelope.EventID, err)
	}
	return nil
}

func applyTraceContext(ctx context.Context, envelope event.Envelope) (event.Envelope, string, string) {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		traceID := spanContext.TraceID().String()
		envelope.TraceID = &traceID
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return envelope, carrier.Get("traceparent"), carrier.Get("tracestate")
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
