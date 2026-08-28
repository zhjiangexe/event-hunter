package outbox

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"event-hunter/backend/internal/demo/event"
)

func TestApplyTraceContextUsesActiveSpanForEnvelopeAndOutboxHeader(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("11111111111111111111111111111111")
	spanID, _ := trace.SpanIDFromHex("2222222222222222")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	envelope := event.Envelope{TraceID: nil}

	updated, traceParent, traceState := applyTraceContext(ctx, envelope)
	if updated.TraceID == nil || *updated.TraceID != traceID.String() {
		t.Fatalf("trace ID = %v, want %s", updated.TraceID, traceID.String())
	}
	if traceParent != "00-11111111111111111111111111111111-2222222222222222-01" {
		t.Fatalf("traceparent = %q", traceParent)
	}
	if traceState != "" {
		t.Fatalf("tracestate = %q, want empty", traceState)
	}
}
