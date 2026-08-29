package clickhouse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	forensics "event-hunter/backend/internal/contexts/investigation/application/search"
)

const forensicsEventColumns = "event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,service_version,admission_status,quality_flags,admission_profile,ingested_at"

type HTTPReadModelConfig struct {
	URL            string
	Database       string
	User           string
	Password       string
	QueryTimeout   time.Duration
	MaxResultRows  int
	MaxResultBytes int64
	MaxRowsToRead  int64
	MaxBytesToRead int64
	MaxThreads     int
	Client         *http.Client
}

type HTTPReadModel struct {
	config HTTPReadModelConfig
}

func NewHTTPReadModel(config HTTPReadModelConfig) *HTTPReadModel {
	if config.Client == nil {
		config.Client = http.DefaultClient
	}
	return &HTTPReadModel{config: config}
}

func (model *HTTPReadModel) Search(ctx context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error) {
	if err := validateSearchFilter(filter); err != nil {
		return nil, err
	}
	conditions := []string{
		fmt.Sprintf("occurred_at >= toDateTime64('%s',3,'UTC')", formatClickHouseTime(filter.From)),
		fmt.Sprintf("occurred_at < toDateTime64('%s',3,'UTC')", formatClickHouseTime(filter.To)),
	}
	for _, condition := range []struct {
		column string
		value  string
	}{
		{"correlation_id", filter.CorrelationID},
		{"event_type", filter.EventType},
		{"aggregate_id", filter.AggregateID},
		{"trace_id", filter.TraceID},
		{"event_id", filter.EventID},
		{"producer", filter.Producer},
		{"causation_id", filter.CausationID},
		{"kafka_topic", filter.KafkaTopic},
	} {
		if value := strings.TrimSpace(condition.value); value != "" {
			conditions = append(conditions, condition.column+"="+quote(value))
		}
	}
	if values := quotedUnique(filter.EventTypes); len(values) > 0 {
		conditions = append(conditions, "event_type IN ("+strings.Join(values, ",")+")")
	}
	if values := quotedUnique(filter.CorrelationIDs); len(values) > 0 {
		conditions = append(conditions, "correlation_id IN ("+strings.Join(values, ",")+")")
	}
	if filter.EventVersion != nil {
		conditions = append(conditions, fmt.Sprintf("event_version=%d", *filter.EventVersion))
	}
	if filter.KafkaPartition != nil {
		conditions = append(conditions, fmt.Sprintf("kafka_partition=%d", *filter.KafkaPartition))
	}
	if filter.KafkaOffset != nil {
		conditions = append(conditions, fmt.Sprintf("kafka_offset=%d", *filter.KafkaOffset))
	}
	columns := forensicsEventColumns
	if filter.IncludePayload {
		columns += ",payload"
	}
	statement := deduplicatedQuery(columns, strings.Join(conditions, " AND "), filter.Limit)
	data, err := model.execute(ctx, statement)
	if err != nil {
		return nil, err
	}
	return decodeEvents(data)
}

// CorrelationEventWindow discovers the stable event-time anchor used by
// Pattern analysis. This is intentionally a narrow aggregate query instead of
// relaxing the seven-day limit on the general event-search surface.
func (model *HTTPReadModel) CorrelationEventWindow(ctx context.Context, correlationID string) (time.Time, time.Time, int, error) {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("correlation event window requires a correlation ID")
	}
	statement := fmt.Sprintf(`SELECT min(occurred_at) AS first_occurred_at,max(occurred_at) AS last_occurred_at,count() AS event_count
FROM (
  SELECT occurred_at
  FROM (
    SELECT occurred_at,row_number() OVER (PARTITION BY kafka_topic,kafka_partition,kafka_offset ORDER BY ingested_at DESC) AS _delivery_rank
    FROM canonical_forensics_events
    WHERE correlation_id=%s
  )
  WHERE _delivery_rank=1
)
FORMAT JSONEachRow`, quote(correlationID))
	data, err := model.execute(ctx, statement)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	return decodeCorrelationEventWindow(data)
}

func decodeCorrelationEventWindow(data []byte) (time.Time, time.Time, int, error) {
	var row struct {
		FirstOccurredAt string `json:"first_occurred_at"`
		LastOccurredAt  string `json:"last_occurred_at"`
		EventCount      int    `json:"event_count"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &row); err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("decode correlation event window: %w", err)
	}
	if row.EventCount == 0 {
		return time.Time{}, time.Time{}, 0, nil
	}
	firstValue, err := normalizeClickHouseTimestamp(row.FirstOccurredAt)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("first occurred_at: %w", err)
	}
	lastValue, err := normalizeClickHouseTimestamp(row.LastOccurredAt)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("last occurred_at: %w", err)
	}
	first, err := time.Parse(time.RFC3339Nano, firstValue)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	last, err := time.Parse(time.RFC3339Nano, lastValue)
	if err != nil {
		return time.Time{}, time.Time{}, 0, err
	}
	return first.UTC(), last.UTC(), row.EventCount, nil
}

func (model *HTTPReadModel) ProcessingSummaries(ctx context.Context, eventIDs []string) (map[string]forensics.ProcessingSummary, error) {
	if len(eventIDs) == 0 {
		return map[string]forensics.ProcessingSummary{}, nil
	}
	if len(eventIDs) > MaxTimelineLimit {
		return nil, fmt.Errorf("processing summary event limit exceeds %d", MaxTimelineLimit)
	}
	quotedIDs := make([]string, 0, len(eventIDs))
	seen := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID = strings.TrimSpace(eventID); eventID == "" {
			continue
		}
		if _, exists := seen[eventID]; exists {
			continue
		}
		seen[eventID] = struct{}{}
		quotedIDs = append(quotedIDs, quote(eventID))
	}
	if len(quotedIDs) == 0 {
		return map[string]forensics.ProcessingSummary{}, nil
	}
	statement := fmt.Sprintf("SELECT latest_event_id AS event_id,count() AS attempt_count,max(latest_attempt) AS last_attempt,argMax(latest_processing_status,tuple(latest_attempt,latest_observed_at)) AS final_status,groupUniqArray(latest_consumer_group_id) AS consumer_groups,max(latest_observed_at) AS last_attempt_at FROM (SELECT attempt_id,argMax(event_id,observed_at) AS latest_event_id,argMax(attempt,observed_at) AS latest_attempt,argMax(processing_status,observed_at) AS latest_processing_status,argMax(consumer_group_id,observed_at) AS latest_consumer_group_id,max(observed_at) AS latest_observed_at FROM canonical_event_processing_attempts WHERE event_id IN (%s) GROUP BY attempt_id) GROUP BY latest_event_id FORMAT JSONEachRow", strings.Join(quotedIDs, ","))
	data, err := model.execute(ctx, statement)
	if err != nil {
		return nil, err
	}
	result := make(map[string]forensics.ProcessingSummary)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var row struct {
			EventID        string   `json:"event_id"`
			AttemptCount   int      `json:"attempt_count"`
			LastAttempt    int      `json:"last_attempt"`
			FinalStatus    string   `json:"final_status"`
			ConsumerGroups []string `json:"consumer_groups"`
			LastAttemptAt  string   `json:"last_attempt_at"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, err
		}
		result[row.EventID] = forensics.ProcessingSummary{
			AttemptCount: row.AttemptCount, LastAttempt: row.LastAttempt, FinalStatus: row.FinalStatus,
			ConsumerGroups: row.ConsumerGroups,
		}
		if row.LastAttemptAt != "" {
			normalized, err := normalizeClickHouseTimestamp(row.LastAttemptAt)
			if err != nil {
				return nil, fmt.Errorf("processing summary %s last_attempt_at: %w", row.EventID, err)
			}
			summary := result[row.EventID]
			summary.LastAttemptAt = normalized
			result[row.EventID] = summary
		}
	}
	return result, scanner.Err()
}

func (model *HTTPReadModel) execute(ctx context.Context, statement string) ([]byte, error) {
	queryContext, cancel := context.WithTimeout(ctx, model.config.QueryTimeout)
	defer cancel()
	endpoint, err := url.Parse(model.config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse ClickHouse URL: %w", err)
	}
	settings := endpoint.Query()
	settings.Set("database", model.config.Database)
	settings.Set("readonly", "2")
	settings.Set("max_execution_time", strconv.FormatFloat(model.config.QueryTimeout.Seconds(), 'f', -1, 64))
	settings.Set("max_result_rows", strconv.Itoa(model.config.MaxResultRows))
	settings.Set("max_result_bytes", strconv.FormatInt(model.config.MaxResultBytes, 10))
	settings.Set("max_rows_to_read", strconv.FormatInt(model.config.MaxRowsToRead, 10))
	settings.Set("max_bytes_to_read", strconv.FormatInt(model.config.MaxBytesToRead, 10))
	settings.Set("result_overflow_mode", "throw")
	settings.Set("max_threads", strconv.Itoa(model.config.MaxThreads))
	endpoint.RawQuery = settings.Encode()

	request, err := http.NewRequestWithContext(queryContext, http.MethodPost, endpoint.String(), strings.NewReader(statement))
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(model.config.User, model.config.Password)
	response, err := model.config.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("ClickHouse status %s", response.Status)
	}
	return io.ReadAll(response.Body)
}

func validateSearchFilter(filter forensics.EventSearchFilter) error {
	if filter.From.IsZero() || filter.To.IsZero() || !filter.To.After(filter.From) {
		return fmt.Errorf("event search requires a positive time window")
	}
	if filter.To.Sub(filter.From) > 7*24*time.Hour {
		return fmt.Errorf("event search window exceeds 7 days")
	}
	if filter.Limit < 1 || filter.Limit > MaxTimelineLimit {
		return fmt.Errorf("event search limit must be between 1 and %d", MaxTimelineLimit)
	}
	if len(filter.EventTypes) > 1000 || len(filter.CorrelationIDs) > 1000 {
		return fmt.Errorf("event search qualifier count exceeds %d", 1000)
	}
	return nil
}

func deduplicatedQuery(columns, conditions string, limit int) string {
	return fmt.Sprintf("SELECT %s FROM (SELECT *,row_number() OVER (PARTITION BY kafka_topic,kafka_partition,kafka_offset ORDER BY ingested_at DESC) AS _delivery_rank FROM canonical_forensics_events WHERE %s) WHERE _delivery_rank=1 ORDER BY occurred_at,event_id LIMIT %d FORMAT JSONEachRow", columns, conditions, limit)
}

func decodeEvents(data []byte) ([]forensics.ForensicsEvent, error) {
	result := make([]forensics.ForensicsEvent, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var event forensics.ForensicsEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		var err error
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
	return result, scanner.Err()
}

func normalizeClickHouseTimestamp(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("timestamp is empty")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", value, time.UTC)
	if err != nil {
		return "", fmt.Errorf("invalid ClickHouse timestamp %q", value)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func formatClickHouseTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05.000")
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quotedUnique(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, quote(value))
	}
	return result
}
