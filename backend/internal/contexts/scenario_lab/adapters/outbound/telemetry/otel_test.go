package telemetry

import (
	"context"
	"event-hunter/backend/internal/contexts/scenario_lab/domain"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestStartKeepsRequestedTraceID(t *testing.T) {
	want := "0123456789abcdef0123456789abcdef"
	ctx, end, err := new(Adapter).Start(context.Background(), want, domain.RunRecord{ScenarioID: "S3"})
	if err != nil {
		t.Fatal(err)
	}
	defer end()
	if got := trace.SpanContextFromContext(ctx).TraceID().String(); got != want {
		t.Fatalf("trace ID = %s", got)
	}
}
