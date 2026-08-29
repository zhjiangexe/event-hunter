package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	forensics "event-hunter/backend/internal/contexts/investigation/application/search"
)

const (
	DefaultTimelineLimit = 1000
	MaxTimelineLimit     = 10000
)

type ForensicsQuery struct {
	db *sql.DB
}

func NewForensicsQuery(db *sql.DB) *ForensicsQuery {
	return &ForensicsQuery{db: db}
}

func (query *ForensicsQuery) ByCorrelationID(ctx context.Context, request TimelineQuery) ([]forensics.ForensicsEvent, error) {
	if err := ValidateTimelineQuery(request); err != nil {
		return nil, err
	}
	const statement = `
SELECT event_id, event_type, event_version, occurred_at, producer, correlation_id,
       causation_id, trace_id, aggregate_type, aggregate_id, sequence, kafka_topic,
       kafka_partition, kafka_offset, service_version, payload, ingested_at
FROM (
    SELECT *, row_number() OVER (
        PARTITION BY kafka_topic, kafka_partition, kafka_offset
        ORDER BY ingested_at DESC
    ) AS delivery_rank
    FROM canonical_forensics_events
    WHERE correlation_id = ? AND occurred_at >= ? AND occurred_at < ?
)
WHERE delivery_rank = 1
ORDER BY occurred_at ASC, event_id ASC
LIMIT ?`

	rows, err := query.db.QueryContext(ctx, statement, request.CorrelationID, request.From, request.To, normalizedLimit(request.Limit))
	if err != nil {
		return nil, fmt.Errorf("query forensics timeline: %w", err)
	}
	defer rows.Close()

	result := make([]forensics.ForensicsEvent, 0)
	for rows.Next() {
		var event forensics.ForensicsEvent
		if err := rows.Scan(
			&event.EventID, &event.EventType, &event.EventVersion, &event.OccurredAt,
			&event.Producer, &event.CorrelationID, &event.CausationID, &event.TraceID,
			&event.AggregateType, &event.AggregateID, &event.Sequence, &event.KafkaTopic,
			&event.KafkaPartition, &event.KafkaOffset, &event.ServiceVersion, &event.Payload,
			&event.IngestedAt,
		); err != nil {
			return nil, fmt.Errorf("scan forensics event: %w", err)
		}
		event.OccurredAt, err = normalizeClickHouseTimestamp(event.OccurredAt)
		if err != nil {
			return nil, fmt.Errorf("event %s occurred_at: %w", event.EventID, err)
		}
		event.IngestedAt, err = normalizeClickHouseTimestamp(event.IngestedAt)
		if err != nil {
			return nil, fmt.Errorf("event %s ingested_at: %w", event.EventID, err)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forensics timeline: %w", err)
	}
	return result, nil
}

func (query *ForensicsQuery) SummaryByEventID(ctx context.Context, eventID string) (LegacyProcessingSummary, error) {
	const statement = `
SELECT count(), max(latest_attempt),
       argMax(latest_processing_status, tuple(latest_attempt, latest_observed_at)),
       countIf(latest_processing_status = 'FAILED')
FROM (
    SELECT attempt_id,
           argMax(attempt, observed_at) AS latest_attempt,
           argMax(processing_status, observed_at) AS latest_processing_status,
           max(observed_at) AS latest_observed_at
    FROM canonical_event_processing_attempts
    WHERE event_id = ?
    GROUP BY attempt_id
)`
	var summary LegacyProcessingSummary
	if err := query.db.QueryRowContext(ctx, statement, eventID).Scan(&summary.AttemptCount, &summary.LastAttempt, &summary.LastStatus, &summary.FailedCount); err != nil {
		return LegacyProcessingSummary{}, fmt.Errorf("query processing summary: %w", err)
	}
	return summary, nil
}

type TimelineQuery struct {
	CorrelationID string
	From          time.Time
	To            time.Time
	Limit         int
}

type LegacyProcessingSummary struct {
	AttemptCount int
	LastAttempt  int
	LastStatus   string
	FailedCount  int
}

func ValidateTimelineQuery(request TimelineQuery) error {
	if strings.TrimSpace(request.CorrelationID) == "" {
		return fmt.Errorf("correlation ID is required")
	}
	if request.From.IsZero() || request.To.IsZero() || !request.To.After(request.From) {
		return fmt.Errorf("timeline query requires a positive time window")
	}
	if request.To.Sub(request.From) > 7*24*time.Hour {
		return fmt.Errorf("timeline query window exceeds 7 days")
	}
	if request.Limit < 0 || request.Limit > MaxTimelineLimit {
		return fmt.Errorf("timeline limit must be between 0 and %d", MaxTimelineLimit)
	}
	return nil
}

func normalizedLimit(limit int) int {
	if limit == 0 {
		return DefaultTimelineLimit
	}
	return limit
}
