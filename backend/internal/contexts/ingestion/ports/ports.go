package ports

import (
	"context"

	"event-hunter/backend/internal/contexts/ingestion/domain"
)

type SourceRecord interface {
	FailureRecord() domain.DLQRecord
}

type Source interface {
	Poll(context.Context) ([]SourceRecord, error)
	Commit(context.Context, SourceRecord) error
	Ping(context.Context) error
}

type FailureRepository interface {
	Insert(context.Context, domain.TechnicalFailure) error
	Ping(context.Context) error
}

type Reporter interface {
	PollFailed(context.Context, error)
	ProjectionFailed(context.Context, domain.DLQRecord, error)
	CommitFailed(context.Context, domain.DLQRecord, error)
}
