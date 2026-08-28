package scenariolab

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type OrderStarter interface {
	Start(context.Context, string, string) (string, error)
}

type HTTPOrderStarter struct {
	URL    string
	Client *http.Client
}

func (starter *HTTPOrderStarter) Start(ctx context.Context, runID, profile string) (string, error) {
	body, _ := json.Marshal(map[string]any{"customer_id": "SCENARIO-LAB", "total_amount": 990, "currency": "TWD", "simulation_profile": profile})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(starter.URL, "/")+"/api/v1/orders", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "scenario-lab-"+runID)
	client := starter.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("call live Order API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("live Order API status %s", response.Status)
	}
	var accepted struct {
		CorrelationID string `json:"correlation_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil {
		return "", err
	}
	if accepted.CorrelationID == "" {
		return "", fmt.Errorf("live Order API returned empty correlation_id")
	}
	return accepted.CorrelationID, nil
}

type Runner struct {
	Repository     Repository
	Publisher      Publisher
	Observer       Observer
	OrderStarter   OrderStarter
	Timeout        time.Duration
	PollInterval   time.Duration
	EventHunterURL string
	GrafanaURL     string
	Now            func() time.Time
}

func (runner *Runner) Start(ctx context.Context, scenarioID string) (Run, error) {
	definition, err := Scenario(scenarioID)
	if err != nil {
		return Run{}, err
	}
	runID := uuid.NewString()
	correlationID := "LAB-" + scenarioID + "-" + strings.ToUpper(strings.ReplaceAll(runID[:8], "-", ""))
	var traceID *string
	if definition.ExecutionMode == LiveServices {
		if runner.OrderStarter == nil {
			return Run{}, fmt.Errorf("live service engine unavailable")
		}
		correlationID, err = runner.OrderStarter.Start(ctx, runID, liveProfile(definition.ID))
		if err != nil {
			return Run{}, err
		}
	} else {
		value, traceErr := TraceID()
		if traceErr != nil {
			return Run{}, traceErr
		}
		traceID = &value
	}
	now := runner.now()
	record := RunRecord{
		RunID: runID, ScenarioID: definition.ID, ScenarioName: definition.Name,
		ExecutionMode: definition.ExecutionMode, Synthetic: definition.Synthetic,
		CorrelationID: correlationID, TraceID: traceID, Status: "ACCEPTED",
		ExpectedEventTypes: definition.ExpectedEventTypes, Actual: EmptyActual(), Checks: []Check{}, AcceptedAt: now,
	}
	if err := runner.Repository.Create(ctx, record); err != nil {
		return Run{}, err
	}
	scenarioRuntimeMetrics().runs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("scenario.id", definition.ID),
		attribute.String("scenario.execution_mode", definition.ExecutionMode),
	))
	go runner.execute(runID)
	return runner.runFromRecord(record), nil
}

func liveProfile(scenarioID string) string {
	switch scenarioID {
	case "S12":
		return "PAYMENT_FAILED"
	case "S13":
		return "SHIPMENT_DELIVERED"
	case "S14":
		return "RETURN_REFUND"
	default:
		return "NORMAL"
	}
}

func (runner *Runner) Get(ctx context.Context, runID string) (Run, error) {
	record, err := runner.Repository.Get(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	return runner.runFromRecord(record), nil
}

func (runner *Runner) execute(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), runner.timeout())
	defer cancel()
	record, err := runner.Repository.Get(ctx, runID)
	if err != nil {
		return
	}
	if record.ExecutionMode == LabInjection && record.TraceID != nil {
		ctx, err = scenarioTraceContext(ctx, *record.TraceID)
		if err != nil {
			runner.fail(runID, "FAILED", EmptyActual(), nil, err)
			return
		}
		var span trace.Span
		ctx, span = otel.Tracer("event-hunter/scenario-lab").Start(ctx, "scenario.run",
			trace.WithAttributes(
				attribute.String("scenario.id", record.ScenarioID),
				attribute.String("scenario.run_id", record.RunID),
				attribute.String("correlation.id", record.CorrelationID),
				attribute.Bool("event_hunter.synthetic", record.Synthetic),
			),
		)
		defer span.End()
	}
	started := runner.now()
	if err := runner.Repository.MarkRunning(ctx, runID, started); err != nil {
		return
	}

	if record.ExecutionMode == LabInjection {
		if err := runner.publishScenario(ctx, record); err != nil {
			runner.fail(runID, "FAILED", EmptyActual(), nil, err)
			return
		}
	}

	ticker := time.NewTicker(runner.pollInterval())
	defer ticker.Stop()
	var actual Actual
	var checks []Check
	for {
		actual, err = runner.Observer.Observe(ctx, record.CorrelationID)
		if err == nil {
			checks = Evaluate(record.ScenarioID, record.ExpectedEventTypes, actual)
			if checksPassed(checks) {
				slog.InfoContext(ctx, "scenario run passed",
					"scenario.id", record.ScenarioID, "scenario.run_id", record.RunID,
					"correlation.id", record.CorrelationID,
				)
				_ = runner.Repository.Complete(context.Background(), runID, "PASSED", actual, checks, nil, runner.now())
				scenarioRuntimeMetrics().duration.Record(ctx, runner.now().Sub(started).Seconds(), metric.WithAttributes(
					attribute.String("scenario.id", record.ScenarioID), attribute.String("scenario.status", "PASSED"),
				))
				return
			}
		}
		select {
		case <-ctx.Done():
			message := "timed out waiting for actual pipeline results"
			if err != nil {
				message = err.Error()
			}
			_ = runner.Repository.Complete(context.Background(), runID, "TIMED_OUT", actual, checks, &message, runner.now())
			scenarioRuntimeMetrics().duration.Record(context.Background(), runner.now().Sub(started).Seconds(), metric.WithAttributes(
				attribute.String("scenario.id", record.ScenarioID), attribute.String("scenario.status", "TIMED_OUT"),
			))
			return
		case <-ticker.C:
		}
	}
}

func (runner *Runner) publishScenario(ctx context.Context, record RunRecord) error {
	if runner.Publisher == nil {
		return fmt.Errorf("lab injection publisher unavailable")
	}
	emissions, err := BuildEmissions(record.ScenarioID, record.CorrelationID, *record.TraceID, runner.now())
	if err != nil {
		return err
	}
	for _, emission := range emissions {
		published, err := runner.Publisher.Publish(ctx, emission.Topic, emission.Key, emission.Value)
		if err != nil {
			return err
		}
		eventID, eventType := emissionIdentity(emission)
		slog.InfoContext(ctx, "scenario event published",
			"scenario.id", record.ScenarioID, "scenario.run_id", record.RunID,
			"event.id", eventID, "event.type", eventType, "correlation.id", record.CorrelationID,
			"kafka.topic", emission.Topic, "kafka.partition", published.Partition, "kafka.offset", published.Offset,
		)
		scenarioRuntimeMetrics().events.Add(ctx, 1, metric.WithAttributes(
			attribute.String("scenario.id", record.ScenarioID), attribute.String("event.type", eventType),
		))
		if record.ScenarioID == "S5" && emission.Envelope != nil && emission.Envelope.EventType == "PaymentCompleted" {
			attempts, err := AttemptEmissions(*emission.Envelope, published, runner.now())
			if err != nil {
				return err
			}
			for _, attempt := range attempts {
				if _, err := runner.Publisher.Publish(ctx, attempt.Topic, attempt.Key, attempt.Value); err != nil {
					return err
				}
			}
			scenarioRuntimeMetrics().retries.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID)))
			scenarioRuntimeMetrics().dlq.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID)))
		}
	}
	return nil
}

func Evaluate(scenarioID string, expectedEvents []string, actual Actual) []Check {
	checks := []Check{}
	if scenarioID != "S6" {
		checks = append(checks, Check{ID: "event-sequence", Label: "實際事件序列", Expected: expectedEvents, Actual: actual.EventTypes, Passed: slices.Equal(expectedEvents, actual.EventTypes)})
	}
	switch scenarioID {
	case "S2":
		checks = append(checks, Check{ID: "shipment-missing", Label: "ShipmentCreated 缺少", Expected: true, Actual: !slices.Contains(actual.EventTypes, "ShipmentCreated"), Passed: !slices.Contains(actual.EventTypes, "ShipmentCreated")})
	case "S3":
		checks = append(checks, Check{ID: "duplicate-event", Label: "偵測重複 event ID", Expected: true, Actual: len(actual.DuplicateEventIDs) > 0, Passed: len(actual.DuplicateEventIDs) > 0})
	case "S4":
		checks = append(checks, Check{ID: "out-of-order", Label: "偵測 Aggregate sequence 亂序", Expected: true, Actual: actual.OutOfOrder, Passed: actual.OutOfOrder})
	case "S5":
		expected := []string{"FAILED", "RETRY_SCHEDULED", "DLQ"}
		checks = append(checks, Check{ID: "processing-dlq", Label: "Processing attempts 到達 DLQ", Expected: expected, Actual: actual.ProcessingStatuses, Passed: slices.Equal(expected, actual.ProcessingStatuses)})
	case "S6":
		passed := actual.EventCount == 0 && slices.Contains(actual.IngestionFailureTypes, "SCHEMA_VIOLATION")
		checks = append(checks, Check{ID: "schema-violation-dlq", Label: "違規事件只進 ingestion failure", Expected: "SCHEMA_VIOLATION and 0 timeline events", Actual: map[string]any{"failure_types": actual.IngestionFailureTypes, "event_count": actual.EventCount}, Passed: passed})
	case "S7":
		checks = append(checks, Check{ID: "event-delay", Label: "最大事件延遲至少十分鐘", Expected: int64(600000), Actual: actual.MaxEventDelayMS, Passed: actual.MaxEventDelayMS >= 600000})
	}
	return checks
}

func checksPassed(checks []Check) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func (runner *Runner) runFromRecord(record RunRecord) Run {
	definition, _ := Scenario(record.ScenarioID)
	traceID := record.TraceID
	if traceID == nil {
		traceID = record.Actual.TraceID
	}
	return Run{
		RunID: record.RunID, Scenario: definition, CorrelationID: record.CorrelationID, TraceID: traceID,
		Status: record.Status, ExecutionMode: record.ExecutionMode, Synthetic: record.Synthetic,
		ExpectedEventTypes: record.ExpectedEventTypes, Actual: record.Actual, Checks: record.Checks,
		Links: runner.links(record.CorrelationID, traceID), Error: record.Error,
		AcceptedAt: record.AcceptedAt, StartedAt: record.StartedAt, CompletedAt: record.CompletedAt,
	}
}

func (runner *Runner) links(correlationID string, traceID *string) Links {
	ui := strings.TrimRight(runner.EventHunterURL, "/")
	grafana := strings.TrimRight(runner.GrafanaURL, "/")
	encoded := url.QueryEscape(correlationID)
	rangeValue := map[string]string{
		"from": strconv.FormatInt(runner.now().Add(-15*time.Minute).UnixMilli(), 10),
		"to":   strconv.FormatInt(runner.now().Add(5*time.Minute).UnixMilli(), 10),
	}
	links := Links{
		Timeline: ui + "/timeline?correlation_id=" + encoded,
		Grafana: grafanaExploreURL(grafana, "clickhouse", "grafana-clickhouse-datasource", map[string]any{
			"rawSql": "SELECT * FROM canonical_forensics_events WHERE correlation_id = " + sqlLiteral(correlationID) + " ORDER BY occurred_at",
			"format": 1,
		}, rangeValue),
		Loki: grafanaExploreURL(grafana, "loki", "loki", map[string]any{
			"expr": `{service_name=~".+"} | correlation_id=` + strconv.Quote(correlationID), "queryType": "range",
		}, rangeValue),
	}
	if traceID != nil {
		value := grafanaExploreURL(grafana, "tempo", "tempo", map[string]any{
			"query": *traceID, "queryType": "traceql",
		}, rangeValue)
		links.Tempo = &value
	}
	return links
}

func scenarioTraceContext(ctx context.Context, value string) (context.Context, error) {
	traceID, err := trace.TraceIDFromHex(value)
	if err != nil {
		return nil, fmt.Errorf("parse scenario trace ID: %w", err)
	}
	var parentSpanID trace.SpanID
	if _, err := rand.Read(parentSpanID[:]); err != nil {
		return nil, fmt.Errorf("generate scenario parent span ID: %w", err)
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: parentSpanID, TraceFlags: trace.FlagsSampled, Remote: true,
	})
	return trace.ContextWithRemoteSpanContext(ctx, parent), nil
}

func emissionIdentity(emission Emission) (string, string) {
	if emission.Envelope != nil {
		return emission.Envelope.EventID, emission.Envelope.EventType
	}
	var value struct {
		EventID   string `json:"eventId"`
		EventType string `json:"eventType"`
	}
	_ = json.Unmarshal(emission.Value, &value)
	return value.EventID, value.EventType
}

func grafanaExploreURL(baseURL, datasource, datasourceType string, query map[string]any, rangeValue map[string]string) string {
	query["refId"] = "A"
	query["datasource"] = map[string]string{"uid": datasource, "type": datasourceType}
	panes, _ := json.Marshal(map[string]any{
		"event-hunter": map[string]any{
			"datasource": datasource, "queries": []any{query}, "range": rangeValue,
		},
	})
	values := url.Values{"panes": {string(panes)}, "schemaVersion": {"1"}, "orgId": {"1"}}
	return baseURL + "/explore?" + values.Encode()
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

type runtimeMetrics struct {
	runs     metric.Int64Counter
	events   metric.Int64Counter
	retries  metric.Int64Counter
	dlq      metric.Int64Counter
	duration metric.Float64Histogram
}

var (
	metricsOnce sync.Once
	metricsSet  runtimeMetrics
)

func scenarioRuntimeMetrics() runtimeMetrics {
	metricsOnce.Do(func() {
		meter := otel.Meter("event-hunter/scenario-lab")
		metricsSet.runs, _ = meter.Int64Counter("event_lab_scenario_runs_total")
		metricsSet.events, _ = meter.Int64Counter("event_lab_events_emitted_total")
		metricsSet.retries, _ = meter.Int64Counter("event_lab_processing_retries_total")
		metricsSet.dlq, _ = meter.Int64Counter("event_lab_dlq_total")
		metricsSet.duration, _ = meter.Float64Histogram("event_lab_scenario_duration_seconds", metric.WithUnit("s"))
	})
	return metricsSet
}

func (runner *Runner) fail(runID, status string, actual Actual, checks []Check, err error) {
	message := err.Error()
	_ = runner.Repository.Complete(context.Background(), runID, status, actual, checks, &message, runner.now())
}

func (runner *Runner) now() time.Time {
	if runner.Now != nil {
		return runner.Now().UTC()
	}
	return time.Now().UTC()
}
func (runner *Runner) timeout() time.Duration {
	if runner.Timeout > 0 {
		return runner.Timeout
	}
	return 30 * time.Second
}
func (runner *Runner) pollInterval() time.Duration {
	if runner.PollInterval > 0 {
		return runner.PollInterval
	}
	return 500 * time.Millisecond
}
