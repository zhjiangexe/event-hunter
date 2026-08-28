package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	evaluateports "event-hunter/backend/internal/contexts/eventcheck/ports"
	"event-hunter/backend/internal/contexts/investigation/application/forensics"
)

// FindCanonicalEvents implements the Event Check read port using the existing
// canonical ClickHouse read model. Payload is used only for deterministic
// business-key resolution and checksums; it is not returned by the API.
func (model *HTTPReadModel) FindCanonicalEvents(ctx context.Context, query evaluateports.CanonicalEventQuery) (evaluateports.CanonicalEventResult, error) {
	values, err := model.Search(ctx, forensics.EventSearchFilter{
		From: query.From, To: query.To, Limit: query.Limit, IncludePayload: true,
	})
	if err != nil {
		return evaluateports.CanonicalEventResult{}, err
	}
	result := evaluateports.CanonicalEventResult{
		Events: make([]domain.Event, 0, len(values)), Truncated: len(values) >= query.Limit,
	}
	for _, value := range values {
		occurredAt, err := time.Parse(time.RFC3339Nano, value.OccurredAt)
		if err != nil {
			return evaluateports.CanonicalEventResult{}, fmt.Errorf("event %s occurred_at: %w", value.EventID, err)
		}
		ingestedAt, err := time.Parse(time.RFC3339Nano, value.IngestedAt)
		if err != nil {
			return evaluateports.CanonicalEventResult{}, fmt.Errorf("event %s ingested_at: %w", value.EventID, err)
		}
		if result.Watermark == nil || ingestedAt.After(*result.Watermark) {
			watermark := ingestedAt.UTC()
			result.Watermark = &watermark
		}
		payload := map[string]any{}
		if value.Payload != "" {
			if err := json.Unmarshal([]byte(value.Payload), &payload); err != nil {
				return evaluateports.CanonicalEventResult{}, fmt.Errorf("event %s payload: %w", value.EventID, err)
			}
		}
		result.Events = append(result.Events, domain.Event{
			ID: value.EventID, Type: value.EventType, Version: int(value.EventVersion), OccurredAt: occurredAt.UTC(),
			Producer: value.Producer, CorrelationID: value.CorrelationID, CausationID: value.CausationID,
			TraceID: value.TraceID, AggregateType: value.AggregateType, AggregateID: value.AggregateID,
			Sequence: value.Sequence, KafkaTopic: value.KafkaTopic, KafkaPartition: value.KafkaPartition,
			KafkaOffset: value.KafkaOffset, Payload: payload,
		})
	}
	return result, nil
}
