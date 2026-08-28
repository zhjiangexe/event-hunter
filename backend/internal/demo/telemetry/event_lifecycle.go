package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"event-hunter/backend/internal/demo/event"
)

const (
	EmissionFailureDelay             = "PRE_EMISSION_DELAY"
	EmissionFailureOutboxAppend      = "OUTBOX_APPEND"
	EmissionFailureTransactionCommit = "TRANSACTION_COMMIT"
)

// PreparingOutboxEvent records the exact event identity before the artificial
// demo delay and outbox append. The context-aware log is correlated to the
// active trace by otelslog, while the span event makes the same point visible
// directly in Tempo.
func PreparingOutboxEvent(ctx context.Context, envelope event.Envelope, topic string, delay time.Duration) {
	attrs := append(ProducerLogAttrs(envelope, topic),
		"event.emission.phase", "PREPARING",
		"event.emission.delay_ms", delay.Milliseconds(),
	)
	slog.InfoContext(ctx, fmt.Sprintf("domain event emission starting: %s", envelope.EventType), attrs...)
	trace.SpanFromContext(ctx).AddEvent("domain.event.preparing", trace.WithAttributes(
		outboxSpanAttributes(envelope, topic, "PREPARING", delay.Milliseconds())...,
	))
}

// CommittedOutboxEvent is called only after the local business transaction and
// its outbox row commit successfully. Absence of this marker after PREPARING is
// therefore useful evidence of a transaction or append failure.
func CommittedOutboxEvent(ctx context.Context, envelope event.Envelope, topic string) {
	attrs := append(ProducerLogAttrs(envelope, topic),
		"event.emission.phase", "COMMITTED",
	)
	slog.InfoContext(ctx, fmt.Sprintf("domain event committed to outbox: %s", envelope.EventType), attrs...)
	trace.SpanFromContext(ctx).AddEvent("domain.event.committed", trace.WithAttributes(
		outboxSpanAttributes(envelope, topic, "COMMITTED", 0)...,
	))
}

// FailedOutboxEvent records why a prepared event never reached a committed
// outbox row. The stage distinguishes an interrupted demo delay, an outbox
// append failure, and a business transaction commit failure.
func FailedOutboxEvent(ctx context.Context, envelope event.Envelope, topic, stage string, err error) {
	attrs := append(ProducerLogAttrs(envelope, topic),
		"event.emission.phase", "FAILED",
		"event.emission.failure_stage", stage,
		"error", err,
	)
	slog.ErrorContext(ctx, fmt.Sprintf("domain event emission failed: %s", envelope.EventType), attrs...)
	spanAttrs := append(outboxSpanAttributes(envelope, topic, "FAILED", 0),
		attribute.String("event.emission.failure_stage", stage),
		attribute.String("error.message", err.Error()),
	)
	trace.SpanFromContext(ctx).AddEvent("domain.event.failed", trace.WithAttributes(spanAttrs...))
}

func outboxSpanAttributes(envelope event.Envelope, topic, phase string, delayMilliseconds int64) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("event.id", envelope.EventID),
		attribute.String("event.type", envelope.EventType),
		attribute.String("event.producer", envelope.Producer),
		attribute.String("correlation.id", envelope.CorrelationID),
		attribute.String("aggregate.type", envelope.AggregateType),
		attribute.String("aggregate.id", envelope.AggregateID),
		attribute.Int64("event.sequence", int64(envelope.Sequence)),
		attribute.String("kafka.topic", topic),
		attribute.String("event.emission.phase", phase),
	}
	if delayMilliseconds > 0 {
		attrs = append(attrs, attribute.Int64("event.emission.delay_ms", delayMilliseconds))
	}
	if envelope.CausationID != nil {
		attrs = append(attrs, attribute.String("causation.id", *envelope.CausationID))
	}
	return attrs
}
