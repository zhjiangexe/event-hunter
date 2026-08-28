package clickhouse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/overview"
)

func (model *HTTPReadModel) Overview(ctx context.Context, from, to time.Time) (overview.EventSnapshot, error) {
	window := fmt.Sprintf(
		"ingested_at >= toDateTime64('%s',3,'UTC') AND ingested_at < toDateTime64('%s',3,'UTC')",
		formatClickHouseTime(from), formatClickHouseTime(to),
	)
	aggregateQuery := fmt.Sprintf(`SELECT count() AS event_count,maxOrNull(ingested_at) AS latest_event_at,(SELECT maxOrNull(observed_at) FROM canonical_event_processing_attempts WHERE observed_at >= toDateTime64('%s',3,'UTC') AND observed_at < toDateTime64('%s',3,'UTC')) AS latest_processing_attempt_at FROM (SELECT *,row_number() OVER (PARTITION BY kafka_topic,kafka_partition,kafka_offset ORDER BY ingested_at DESC) AS _delivery_rank FROM canonical_forensics_events WHERE %s) WHERE _delivery_rank=1 FORMAT JSONEachRow`, formatClickHouseTime(from), formatClickHouseTime(to), window)

	data, err := model.execute(ctx, aggregateQuery)
	if err != nil {
		return overview.EventSnapshot{}, err
	}
	var row struct {
		EventCount                int64   `json:"event_count"`
		LatestEventAt             *string `json:"latest_event_at"`
		LatestProcessingAttemptAt *string `json:"latest_processing_attempt_at"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &row); err != nil {
		return overview.EventSnapshot{}, fmt.Errorf("decode ClickHouse overview aggregate: %w", err)
	}
	snapshot := overview.EventSnapshot{EventCount: row.EventCount}
	if snapshot.LatestEventAt, err = parseOptionalClickHouseTime(row.LatestEventAt); err != nil {
		return overview.EventSnapshot{}, fmt.Errorf("latest event time: %w", err)
	}
	if snapshot.LatestProcessingAttemptAt, err = parseOptionalClickHouseTime(row.LatestProcessingAttemptAt); err != nil {
		return overview.EventSnapshot{}, fmt.Errorf("latest processing attempt time: %w", err)
	}

	if snapshot.TopProducers, err = model.overviewBreakdown(ctx, "producer", window); err != nil {
		return overview.EventSnapshot{}, err
	}
	if snapshot.TopEventTypes, err = model.overviewBreakdown(ctx, "event_type", window); err != nil {
		return overview.EventSnapshot{}, err
	}
	return snapshot, nil
}

func (model *HTTPReadModel) overviewBreakdown(ctx context.Context, column, window string) ([]overview.CountByKey, error) {
	if column != "producer" && column != "event_type" {
		return nil, fmt.Errorf("unsupported overview breakdown column %q", column)
	}
	statement := fmt.Sprintf(`SELECT %s AS key,count() AS count FROM (SELECT *,row_number() OVER (PARTITION BY kafka_topic,kafka_partition,kafka_offset ORDER BY ingested_at DESC) AS _delivery_rank FROM canonical_forensics_events WHERE %s) WHERE _delivery_rank=1 GROUP BY %s ORDER BY count DESC,key LIMIT 5 FORMAT JSONEachRow`, column, window, column)
	data, err := model.execute(ctx, statement)
	if err != nil {
		return nil, err
	}
	result := make([]overview.CountByKey, 0, 5)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var item overview.CountByKey
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, fmt.Errorf("decode ClickHouse overview breakdown: %w", err)
		}
		result = append(result, item)
	}
	return result, scanner.Err()
}

func parseOptionalClickHouseTime(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	normalized, err := normalizeClickHouseTimestamp(*value)
	if err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, normalized)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

var _ overview.EventReader = (*HTTPReadModel)(nil)
