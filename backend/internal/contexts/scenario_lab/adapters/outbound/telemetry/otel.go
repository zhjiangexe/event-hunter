package telemetry

import (
	"context"
	"crypto/rand"
	"event-hunter/backend/internal/contexts/scenario_lab/domain"
	"event-hunter/backend/internal/contexts/scenario_lab/ports"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"sync"
	"time"
)

type Adapter struct {
	once     sync.Once
	runs     metric.Int64Counter
	events   metric.Int64Counter
	retries  metric.Int64Counter
	dlq      metric.Int64Counter
	duration metric.Float64Histogram
}

func (adapter *Adapter) init() {
	adapter.once.Do(func() {
		meter := otel.Meter("event-hunter/scenario-lab")
		adapter.runs, _ = meter.Int64Counter("event_lab_scenario_runs_total")
		adapter.events, _ = meter.Int64Counter("event_lab_events_emitted_total")
		adapter.retries, _ = meter.Int64Counter("event_lab_processing_retries_total")
		adapter.dlq, _ = meter.Int64Counter("event_lab_dlq_total")
		adapter.duration, _ = meter.Float64Histogram("event_lab_scenario_duration_seconds", metric.WithUnit("s"))
	})
}
func (adapter *Adapter) Start(ctx context.Context, value string, record domain.RunRecord) (context.Context, func(), error) {
	traceID, err := trace.TraceIDFromHex(value)
	if err != nil {
		return nil, nil, fmt.Errorf("parse scenario trace ID: %w", err)
	}
	var parentSpanID trace.SpanID
	if _, err := rand.Read(parentSpanID[:]); err != nil {
		return nil, nil, fmt.Errorf("generate scenario parent span ID: %w", err)
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: parentSpanID, TraceFlags: trace.FlagsSampled, Remote: true})
	ctx = trace.ContextWithRemoteSpanContext(ctx, parent)
	ctx, span := otel.Tracer("event-hunter/scenario-lab").Start(ctx, "scenario.run", trace.WithAttributes(attribute.String("scenario.id", record.ScenarioID), attribute.String("scenario.run_id", record.RunID), attribute.String("correlation.id", record.CorrelationID), attribute.Bool("event_hunter.synthetic", record.Synthetic)))
	return ctx, func() { span.End() }, nil
}
func (adapter *Adapter) RunAccepted(ctx context.Context, record domain.RunRecord) {
	adapter.init()
	adapter.runs.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID), attribute.String("scenario.execution_mode", record.ExecutionMode)))
}
func (adapter *Adapter) EventPublished(ctx context.Context, record domain.RunRecord, message ports.Message, published ports.PublishedRecord) {
	adapter.init()
	slog.InfoContext(ctx, "scenario event published", "scenario.id", record.ScenarioID, "scenario.run_id", record.RunID, "event.id", message.EventID, "event.type", message.EventType, "correlation.id", record.CorrelationID, "kafka.topic", message.Topic, "kafka.partition", published.Partition, "kafka.offset", published.Offset)
	adapter.events.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID), attribute.String("event.type", message.EventType)))
}
func (adapter *Adapter) RetriesAndDLQ(ctx context.Context, record domain.RunRecord) {
	adapter.init()
	adapter.retries.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID)))
	adapter.dlq.Add(ctx, 1, metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID)))
}
func (adapter *Adapter) RunCompleted(ctx context.Context, record domain.RunRecord, status string, duration time.Duration) {
	adapter.init()
	if status == domain.RunPassed {
		slog.InfoContext(ctx, "scenario run passed", "scenario.id", record.ScenarioID, "scenario.run_id", record.RunID, "correlation.id", record.CorrelationID)
	}
	adapter.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attribute.String("scenario.id", record.ScenarioID), attribute.String("scenario.status", status)))
}
