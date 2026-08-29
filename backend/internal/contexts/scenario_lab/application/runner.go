package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"event-hunter/backend/internal/contexts/scenario_lab/domain"
	"event-hunter/backend/internal/contexts/scenario_lab/ports"
)

type Runner struct {
	Repository   ports.Repository
	Publisher    ports.Publisher
	Observer     ports.Observer
	OrderStarter ports.OrderStarter
	Emissions    ports.EmissionBuilder
	Links        ports.LinkBuilder
	Traces       ports.TraceStarter
	Telemetry    ports.Telemetry
	Clock        ports.Clock
	Timeout      time.Duration
	PollInterval time.Duration
}

func (runner *Runner) Start(ctx context.Context, scenarioID string) (domain.Run, error) {
	definition, err := domain.Scenario(scenarioID)
	if err != nil {
		return domain.Run{}, err
	}
	runID := uuid.NewString()
	correlationID := "LAB-" + scenarioID + "-" + strings.ToUpper(runID[:8])
	var traceID *string
	if definition.ExecutionMode == domain.LiveServices {
		if runner.OrderStarter == nil {
			return domain.Run{}, fmt.Errorf("live service engine unavailable")
		}
		correlationID, err = runner.OrderStarter.Start(ctx, runID, domain.LiveProfile(definition.ID))
		if err != nil {
			return domain.Run{}, err
		}
	} else {
		value, traceErr := newTraceID()
		if traceErr != nil {
			return domain.Run{}, traceErr
		}
		traceID = &value
	}
	record := domain.RunRecord{RunID: runID, ScenarioID: definition.ID, ScenarioName: definition.Name, ExecutionMode: definition.ExecutionMode, Synthetic: definition.Synthetic, CorrelationID: correlationID, TraceID: traceID, Status: domain.RunAccepted, ExpectedEventTypes: definition.ExpectedEventTypes, Actual: domain.EmptyActual(), Checks: []domain.Check{}, AcceptedAt: runner.now()}
	if err := runner.Repository.Create(ctx, record); err != nil {
		return domain.Run{}, err
	}
	if runner.Telemetry != nil {
		runner.Telemetry.RunAccepted(ctx, record)
	}
	go runner.execute(runID)
	return runner.runFromRecord(record), nil
}

func (runner *Runner) Get(ctx context.Context, runID string) (domain.Run, error) {
	record, err := runner.Repository.Get(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	return runner.runFromRecord(record), nil
}
func (runner *Runner) List(ctx context.Context, filter domain.RunFilter) (domain.RunPage, error) {
	records, err := runner.Repository.List(ctx, filter)
	if err != nil {
		return domain.RunPage{}, err
	}
	items := make([]domain.Run, 0, len(records))
	for _, record := range records {
		items = append(items, runner.runFromRecord(record))
	}
	return domain.RunPage{Items: items}, nil
}

func (runner *Runner) execute(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), runner.timeout())
	defer cancel()
	record, err := runner.Repository.Get(ctx, runID)
	if err != nil {
		return
	}
	if record.ExecutionMode == domain.LabInjection && record.TraceID != nil {
		if runner.Traces == nil {
			runner.fail(runID, domain.RunFailed, domain.EmptyActual(), nil, fmt.Errorf("scenario trace starter unavailable"))
			return
		}
		var end func()
		ctx, end, err = runner.Traces.Start(ctx, *record.TraceID, record)
		if err != nil {
			runner.fail(runID, domain.RunFailed, domain.EmptyActual(), nil, err)
			return
		}
		defer end()
	}
	started := runner.now()
	if err := runner.Repository.MarkRunning(ctx, runID, started); err != nil {
		return
	}
	if record.ExecutionMode == domain.LabInjection {
		if err := runner.publishScenario(ctx, record); err != nil {
			runner.fail(runID, domain.RunFailed, domain.EmptyActual(), nil, err)
			return
		}
	}
	ticker := time.NewTicker(runner.pollInterval())
	defer ticker.Stop()
	var actual domain.Actual
	var checks []domain.Check
	for {
		actual, err = runner.Observer.Observe(ctx, record.CorrelationID)
		if err == nil {
			checks = domain.Evaluate(record.ScenarioID, record.ExpectedEventTypes, actual)
			if domain.ChecksPassed(checks) {
				completed := runner.now()
				_ = runner.Repository.Complete(context.Background(), runID, domain.RunPassed, actual, checks, nil, completed)
				if runner.Telemetry != nil {
					runner.Telemetry.RunCompleted(ctx, record, domain.RunPassed, completed.Sub(started))
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			message := "timed out waiting for actual pipeline results"
			if err != nil {
				message = err.Error()
			}
			completed := runner.now()
			_ = runner.Repository.Complete(context.Background(), runID, domain.RunTimedOut, actual, checks, &message, completed)
			if runner.Telemetry != nil {
				runner.Telemetry.RunCompleted(context.Background(), record, domain.RunTimedOut, completed.Sub(started))
			}
			return
		case <-ticker.C:
		}
	}
}

func (runner *Runner) publishScenario(ctx context.Context, record domain.RunRecord) error {
	if runner.Publisher == nil || runner.Emissions == nil {
		return fmt.Errorf("lab injection publisher unavailable")
	}
	messages, err := runner.Emissions.BuildScenario(record.ScenarioID, record.CorrelationID, *record.TraceID, runner.now())
	if err != nil {
		return err
	}
	for _, message := range messages {
		published, err := runner.Publisher.Publish(ctx, message.Topic, message.Key, message.Value)
		if err != nil {
			return err
		}
		if runner.Telemetry != nil {
			runner.Telemetry.EventPublished(ctx, record, message, published)
		}
		if record.ScenarioID == "S5" && message.EventType == "PaymentCompleted" && message.AttemptSource != nil {
			attempts, err := runner.Emissions.BuildAttempts(*message.AttemptSource, published, runner.now())
			if err != nil {
				return err
			}
			for _, attempt := range attempts {
				if _, err := runner.Publisher.Publish(ctx, attempt.Topic, attempt.Key, attempt.Value); err != nil {
					return err
				}
			}
			if runner.Telemetry != nil {
				runner.Telemetry.RetriesAndDLQ(ctx, record)
			}
		}
	}
	return nil
}

func (runner *Runner) runFromRecord(record domain.RunRecord) domain.Run {
	definition, _ := domain.Scenario(record.ScenarioID)
	traceID := record.TraceID
	if traceID == nil {
		traceID = record.Actual.TraceID
	}
	links := domain.Links{}
	if runner.Links != nil {
		links = runner.Links.Build(record.CorrelationID, traceID, runner.now())
	}
	run := domain.Run{RunID: record.RunID, Scenario: definition, CorrelationID: record.CorrelationID, TraceID: traceID, Status: record.Status, ExecutionMode: record.ExecutionMode, Synthetic: record.Synthetic, ExpectedEventTypes: record.ExpectedEventTypes, Actual: record.Actual, Checks: record.Checks, Links: links, Error: record.Error, AcceptedAt: record.AcceptedAt, StartedAt: record.StartedAt, CompletedAt: record.CompletedAt, CurrentStep: domain.CurrentStep(record.Status)}
	if record.StartedAt != nil {
		end := runner.now()
		if record.CompletedAt != nil {
			end = *record.CompletedAt
		}
		duration := end.Sub(*record.StartedAt).Milliseconds()
		if duration < 0 {
			duration = 0
		}
		run.DurationMS = &duration
	}
	return run
}

func (runner *Runner) fail(runID, status string, actual domain.Actual, checks []domain.Check, err error) {
	message := err.Error()
	_ = runner.Repository.Complete(context.Background(), runID, status, actual, checks, &message, runner.now())
}
func (runner *Runner) now() time.Time {
	if runner.Clock != nil {
		return runner.Clock.Now().UTC()
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
func newTraceID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
