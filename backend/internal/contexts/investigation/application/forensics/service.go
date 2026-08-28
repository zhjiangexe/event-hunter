package forensics

import (
	"context"
	"time"
)

type EventSearchFilter struct {
	From           time.Time
	To             time.Time
	Limit          int
	IncludePayload bool
	CorrelationID  string
	EventType      string
	AggregateID    string
	TraceID        string
	EventID        string
	Producer       string
	CausationID    string
	KafkaTopic     string
	EventVersion   *uint32
	KafkaPartition *uint32
	KafkaOffset    *uint64
	EventTypes     []string
	CorrelationIDs []string
}

type ForensicsEvent struct {
	EventID          string   `json:"event_id"`
	EventType        string   `json:"event_type"`
	EventVersion     uint32   `json:"event_version"`
	OccurredAt       string   `json:"occurred_at"`
	Producer         string   `json:"producer"`
	CorrelationID    string   `json:"correlation_id"`
	CausationID      *string  `json:"causation_id"`
	TraceID          *string  `json:"trace_id"`
	AggregateType    string   `json:"aggregate_type"`
	AggregateID      string   `json:"aggregate_id"`
	Sequence         uint64   `json:"sequence"`
	KafkaTopic       string   `json:"kafka_topic"`
	KafkaPartition   uint32   `json:"kafka_partition"`
	KafkaOffset      uint64   `json:"kafka_offset"`
	ServiceVersion   *string  `json:"service_version"`
	AdmissionStatus  string   `json:"admission_status"`
	QualityFlags     []string `json:"quality_flags"`
	AdmissionProfile string   `json:"admission_profile"`
	Payload          string   `json:"payload"`
	IngestedAt       string   `json:"ingested_at"`
}

type ProcessingSummary struct {
	AttemptCount   int
	LastAttempt    int
	FinalStatus    string
	ConsumerGroups []string
	LastAttemptAt  string
}

type ReadModel interface {
	Search(ctx context.Context, filter EventSearchFilter) ([]ForensicsEvent, error)
	ProcessingSummaries(ctx context.Context, eventIDs []string) (map[string]ProcessingSummary, error)
}

type ForensicsService struct {
	readModel ReadModel
}

func NewForensicsService(readModel ReadModel) *ForensicsService {
	return &ForensicsService{readModel: readModel}
}

func (service *ForensicsService) Search(ctx context.Context, filter EventSearchFilter) ([]ForensicsEvent, error) {
	return service.readModel.Search(ctx, filter)
}

func (service *ForensicsService) ProcessingSummaries(ctx context.Context, eventIDs []string) (map[string]ProcessingSummary, error) {
	return service.readModel.ProcessingSummaries(ctx, eventIDs)
}
