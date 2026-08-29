package clickhouse

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ingestionissues "event-hunter/backend/internal/contexts/investigation/application/search"
)

func (model *HTTPReadModel) SearchIngestionIssues(ctx context.Context, filter ingestionissues.Filter) ([]ingestionissues.Issue, error) {
	if filter.From.IsZero() || filter.To.IsZero() || !filter.To.After(filter.From) || filter.To.Sub(filter.From) > ingestionissues.MaxWindow {
		return nil, fmt.Errorf("ingestion issue query requires a positive window no larger than seven days")
	}
	if filter.PageSize < 1 || filter.PageSize > ingestionissues.MaxPageSize {
		return nil, fmt.Errorf("ingestion issue page size must be between 1 and %d", ingestionissues.MaxPageSize)
	}
	conditions := []string{
		fmt.Sprintf("occurred_at >= toDateTime64('%s',3,'UTC')", formatClickHouseTime(filter.From)),
		fmt.Sprintf("occurred_at < toDateTime64('%s',3,'UTC')", formatClickHouseTime(filter.To)),
	}
	for _, condition := range []struct {
		column string
		value  string
	}{
		{"kind", string(filter.Kind)},
		{"error_code", filter.ErrorCode},
		{"source_topic", filter.SourceTopic},
		{"correlation_id", filter.CorrelationID},
	} {
		if value := strings.TrimSpace(condition.value); value != "" {
			conditions = append(conditions, condition.column+"="+quote(value))
		}
	}
	if filter.Cursor != nil {
		cursorTime := formatClickHouseTime(filter.Cursor.OccurredAt)
		conditions = append(conditions, fmt.Sprintf(
			"(occurred_at < toDateTime64('%s',3,'UTC') OR (occurred_at = toDateTime64('%s',3,'UTC') AND id < %s))",
			cursorTime, cursorTime, quote(filter.Cursor.IssueID),
		))
	}

	statement := fmt.Sprintf(`SELECT
    id,kind,occurred_at,pipeline,error_code,event_id,event_type,correlation_id,
    source_topic,source_partition,source_offset,dlq_topic,dlq_partition,dlq_offset,
    payload_sha256,admission_profile,connector_name,connector_task,failure_stage,exception_class
FROM (
    SELECT
        toString(failure_id) AS id,
        'CONTRACT_VALIDATION' AS kind,
        failed_at AS occurred_at,
        'redpanda-connect/domain-events' AS pipeline,
        error_code,
        event_id,event_type,correlation_id,
        toNullable(toString(source_topic)) AS source_topic,
        toNullable(source_partition) AS source_partition,
        toNullable(source_offset) AS source_offset,
        CAST(NULL,'Nullable(String)') AS dlq_topic,
        CAST(NULL,'Nullable(UInt32)') AS dlq_partition,
        CAST(NULL,'Nullable(UInt64)') AS dlq_offset,
        toString(payload_sha256) AS payload_sha256,
        toNullable('domain-event-json-schema-v1') AS admission_profile,
        CAST(NULL,'Nullable(String)') AS connector_name,
        CAST(NULL,'Nullable(UInt32)') AS connector_task,
        CAST(NULL,'Nullable(String)') AS failure_stage,
        CAST(NULL,'Nullable(String)') AS exception_class
    FROM event_ingestion_failures
    UNION ALL
    SELECT
        lower(hex(SHA256(concat(source_topic,':',toString(source_partition),':',toString(source_offset),':',toString(payload_sha256))))) AS id,
        'ADMISSION_QUARANTINE' AS kind,
        failed_at AS occurred_at,
        'clickhouse/minimum-envelope-v1' AS pipeline,
        error_code,
        event_id,event_type,correlation_id,
        toNullable(toString(source_topic)) AS source_topic,
        toNullable(source_partition) AS source_partition,
        toNullable(source_offset) AS source_offset,
        CAST(NULL,'Nullable(String)') AS dlq_topic,
        CAST(NULL,'Nullable(UInt32)') AS dlq_partition,
        CAST(NULL,'Nullable(UInt64)') AS dlq_offset,
        toString(payload_sha256) AS payload_sha256,
        toNullable(toString(admission_profile)) AS admission_profile,
        CAST(NULL,'Nullable(String)') AS connector_name,
        CAST(NULL,'Nullable(UInt32)') AS connector_task,
        CAST(NULL,'Nullable(String)') AS failure_stage,
        CAST(NULL,'Nullable(String)') AS exception_class
    FROM poc_event_admission_failures
    UNION ALL
    SELECT
        lower(hex(SHA256(concat(source_topic,':',toString(source_partition),':',toString(source_offset),':',toString(payload_sha256))))) AS id,
        'ADMISSION_QUARANTINE' AS kind,
        failed_at AS occurred_at,
        'clickhouse/processing-attempt-contract-v1' AS pipeline,
        error_code,
        event_id,event_type,correlation_id,
        toNullable(toString(source_topic)) AS source_topic,
        toNullable(source_partition) AS source_partition,
        toNullable(source_offset) AS source_offset,
        CAST(NULL,'Nullable(String)') AS dlq_topic,
        CAST(NULL,'Nullable(UInt32)') AS dlq_partition,
        CAST(NULL,'Nullable(UInt64)') AS dlq_offset,
        toString(payload_sha256) AS payload_sha256,
        toNullable(toString(admission_profile)) AS admission_profile,
        CAST(NULL,'Nullable(String)') AS connector_name,
        CAST(NULL,'Nullable(UInt32)') AS connector_task,
        CAST(NULL,'Nullable(String)') AS failure_stage,
        CAST(NULL,'Nullable(String)') AS exception_class
    FROM poc_processing_attempt_admission_failures
    UNION ALL
    SELECT
        toString(failure_id) AS id,
        'TECHNICAL_DLQ' AS kind,
        observed_at AS occurred_at,
        'kafka-connect/clickhouse-sink' AS pipeline,
        'CONNECTOR_TASK_FAILURE' AS error_code,
        CAST(NULL,'Nullable(String)') AS event_id,
        CAST(NULL,'Nullable(String)') AS event_type,
        CAST(NULL,'Nullable(String)') AS correlation_id,
        source_topic,source_partition,source_offset,
        toNullable(toString(dlq_topic)) AS dlq_topic,
        toNullable(dlq_partition) AS dlq_partition,
        toNullable(dlq_offset) AS dlq_offset,
        toString(payload_sha256) AS payload_sha256,
        CAST(NULL,'Nullable(String)') AS admission_profile,
        connector_name,connector_task,failure_stage,exception_class
    FROM ingestion_technical_failures
)
WHERE %s
ORDER BY occurred_at DESC,id DESC
LIMIT %d
FORMAT JSONEachRow`, strings.Join(conditions, " AND "), filter.PageSize+1)
	data, err := model.execute(ctx, statement)
	if err != nil {
		return nil, err
	}
	return decodeIngestionIssues(data)
}

func decodeIngestionIssues(data []byte) ([]ingestionissues.Issue, error) {
	issues := make([]ingestionissues.Issue, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var issue ingestionissues.Issue
		if err := json.Unmarshal(scanner.Bytes(), &issue); err != nil {
			return nil, fmt.Errorf("decode ingestion issue: %w", err)
		}
		normalized, err := normalizeClickHouseTimestamp(issue.OccurredAt)
		if err != nil {
			return nil, fmt.Errorf("ingestion issue %s occurred_at: %w", issue.ID, err)
		}
		issue.OccurredAt = normalized
		issues = append(issues, issue)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return issues, nil
}
