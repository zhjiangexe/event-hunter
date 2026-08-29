package ports

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/scenario_lab/domain"
)

type Repository interface {
	Create(context.Context, domain.RunRecord) error
	Get(context.Context, string) (domain.RunRecord, error)
	List(context.Context, domain.RunFilter) ([]domain.RunRecord, error)
	MarkRunning(context.Context, string, time.Time) error
	Complete(context.Context, string, string, domain.Actual, []domain.Check, *string, time.Time) error
}

type PublishedRecord struct {
	Partition int32
	Offset    int64
}
type AttemptSource struct {
	EventID       string
	EventType     string
	CorrelationID string
	TraceID       *string
}
type Message struct {
	Topic         string
	Key           string
	Value         []byte
	EventID       string
	EventType     string
	AttemptSource *AttemptSource
}

type Publisher interface {
	Publish(context.Context, string, string, []byte) (PublishedRecord, error)
}
type Observer interface {
	Observe(context.Context, string) (domain.Actual, error)
}
type OrderStarter interface {
	Start(context.Context, string, string) (string, error)
}
type EmissionBuilder interface {
	BuildScenario(scenarioID, correlationID, traceID string, now time.Time) ([]Message, error)
	BuildAttempts(source AttemptSource, record PublishedRecord, now time.Time) ([]Message, error)
}
type LinkBuilder interface {
	Build(correlationID string, traceID *string, now time.Time) domain.Links
}
type TraceStarter interface {
	Start(context.Context, string, domain.RunRecord) (context.Context, func(), error)
}
type Telemetry interface {
	RunAccepted(context.Context, domain.RunRecord)
	EventPublished(context.Context, domain.RunRecord, Message, PublishedRecord)
	RetriesAndDLQ(context.Context, domain.RunRecord)
	RunCompleted(context.Context, domain.RunRecord, string, time.Duration)
}
type Clock interface{ Now() time.Time }
