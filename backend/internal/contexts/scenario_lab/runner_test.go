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
	eventCheck, err := url.Parse(links.Timeline)
	if err != nil {
		t.Fatal(err)
	}
	if eventCheck.Path != "/event-check" || eventCheck.Query().Get("identifier_type") != "CORRELATION_ID" || eventCheck.Query().Get("identifier") != "ORDER-'quoted" || eventCheck.Query().Get("tab") != "timeline" {
		t.Fatalf("Event Check link = %s", links.Timeline)
	}
	if eventCheck.Query().Get("from") != "2026-08-21T08:45:00Z" || eventCheck.Query().Get("to") != "2026-08-21T09:05:00Z" {
		t.Fatalf("Event Check window = %s", links.Timeline)
	}
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

func TestListReturnsPersistedHistoryWithoutExecutingOrPolling(t *testing.T) {
	started := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	completed := started.Add(3 * time.Second)
	repository := &runRepositoryStub{records: []RunRecord{{
		RunID: "4bd00d41-8b30-476b-82e8-e0474419f7f7", ScenarioID: "S8", ScenarioName: "PAYMENT_FAILED_AND_CANCELLED",
		ExecutionMode: LabInjection, Synthetic: true, CorrelationID: "LAB-S8-TEST", Status: "PASSED",
		ExpectedEventTypes: []string{"OrderCreated", "PaymentFailed", "OrderCancelled"}, Actual: EmptyActual(), Checks: []Check{},
		AcceptedAt: started.Add(-time.Second), StartedAt: &started, CompletedAt: &completed,
	}}}
	runner := Runner{Repository: repository, EventHunterURL: "http://localhost:28334", GrafanaURL: "http://localhost:28332", Now: func() time.Time { return completed }}
	filter := RunFilter{ScenarioID: "S8", Status: "PASSED", PageSize: 20}

	page, err := runner.List(t.Context(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if repository.listCalls != 1 || repository.filter != filter {
		t.Fatalf("repository calls = %d, filter = %#v", repository.listCalls, repository.filter)
	}
	if len(page.Items) != 1 || page.Items[0].CurrentStep != "驗收通過" || page.Items[0].DurationMS == nil || *page.Items[0].DurationMS != 3000 {
		t.Fatalf("page = %#v", page)
	}
}

type runRepositoryStub struct {
	records   []RunRecord
	filter    RunFilter
	listCalls int
}

func (repository *runRepositoryStub) Create(context.Context, RunRecord) error { return nil }
func (repository *runRepositoryStub) Get(context.Context, string) (RunRecord, error) {
	return RunRecord{}, ErrRunNotFound
}
func (repository *runRepositoryStub) List(_ context.Context, filter RunFilter) ([]RunRecord, error) {
	repository.filter = filter
	repository.listCalls++
	return repository.records, nil
}
func (repository *runRepositoryStub) MarkRunning(context.Context, string, time.Time) error {
	return nil
}
func (repository *runRepositoryStub) Complete(context.Context, string, string, Actual, []Check, *string, time.Time) error {
	return nil
}
