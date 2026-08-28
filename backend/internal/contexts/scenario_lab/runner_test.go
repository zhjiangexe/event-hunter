package scenariolab

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestEvaluateUsesActualObservedValues(t *testing.T) {
	actual := EmptyActual()
	actual.EventTypes = []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}
	actual.EventCount = 4
	actual.DuplicateEventIDs = []string{"evt-payment"}
	checks := Evaluate("S3", []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}, actual)
	if !checksPassed(checks) {
		t.Fatalf("checks = %#v", checks)
	}

	actual.DuplicateEventIDs = []string{}
	if checksPassed(Evaluate("S3", []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}, actual)) {
		t.Fatal("duplicate scenario passed without an observed duplicate")
	}
}

func TestScenarioTraceContextKeepsEnvelopeTraceID(t *testing.T) {
	want := "0123456789abcdef0123456789abcdef"
	ctx, err := scenarioTraceContext(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if got := trace.SpanContextFromContext(ctx).TraceID().String(); got != want {
		t.Fatalf("trace ID = %s, want %s", got, want)
	}
}

func TestLinksUseRunnableGrafanaExplorePanes(t *testing.T) {
	fixed := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	runner := Runner{
		EventHunterURL: "http://localhost:28334", GrafanaURL: "http://localhost:28332",
		Now: func() time.Time { return fixed },
	}
	traceID := "0123456789abcdef0123456789abcdef"
	links := runner.links("ORDER-'quoted", &traceID)
	for name, raw := range map[string]string{"grafana": links.Grafana, "loki": links.Loki, "tempo": *links.Tempo} {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("%s URL: %v", name, err)
		}
		if parsed.Query().Get("schemaVersion") != "1" || parsed.Query().Get("panes") == "" {
			t.Fatalf("%s is not a Grafana Explore panes URL: %s", name, raw)
		}
	}
	if !strings.Contains(links.Grafana, url.QueryEscape("ORDER-''quoted")) {
		t.Fatalf("ClickHouse SQL literal was not escaped: %s", links.Grafana)
	}
}

func TestSchemaViolationRequiresFailureAndNoTimelineEvent(t *testing.T) {
	actual := EmptyActual()
	actual.IngestionFailureCount = 1
	actual.IngestionFailureTypes = []string{"SCHEMA_VIOLATION"}
	if !checksPassed(Evaluate("S6", nil, actual)) {
		t.Fatal("valid schema violation result did not pass")
	}
	actual.EventCount = 1
	if checksPassed(Evaluate("S6", nil, actual)) {
		t.Fatal("schema violation passed with a timeline event")
	}
}
