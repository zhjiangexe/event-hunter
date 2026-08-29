package application

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/ingestion/domain"
	"event-hunter/backend/internal/contexts/ingestion/ports"
)

type Projector struct {
	source     ports.Source
	repository ports.FailureRepository
	reporter   ports.Reporter
	retryDelay time.Duration
	now        func() time.Time
}

func NewProjector(source ports.Source, repository ports.FailureRepository, reporter ports.Reporter, retryDelay time.Duration) *Projector {
	if source == nil || repository == nil || reporter == nil {
		panic("technical DLQ source, repository, and reporter are required")
	}
	return &Projector{source: source, repository: repository, reporter: reporter, retryDelay: retryDelay, now: time.Now}
}

func (projector *Projector) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		records, err := projector.source.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			projector.reporter.PollFailed(ctx, err)
			continue
		}
		for _, record := range records {
			if ctx.Err() != nil {
				break
			}
			input := record.FailureRecord()
			failure := domain.Summarize(input, projector.now())
			if !projector.retry(ctx, input, func() error { return projector.repository.Insert(ctx, failure) }, projector.reporter.ProjectionFailed) {
				break
			}
			if !projector.retry(ctx, input, func() error { return projector.source.Commit(ctx, record) }, projector.reporter.CommitFailed) {
				break
			}
		}
	}
	return ctx.Err()
}

func (projector *Projector) retry(ctx context.Context, record domain.DLQRecord, operation func() error, report func(context.Context, domain.DLQRecord, error)) bool {
	for ctx.Err() == nil {
		if err := operation(); err != nil {
			report(ctx, record, err)
			if !waitForRetry(ctx, projector.retryDelay) {
				return false
			}
			continue
		}
		return true
	}
	return false
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
