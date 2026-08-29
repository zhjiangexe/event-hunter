package clickhouse

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"event-hunter/backend/internal/contexts/quality/domain"
)

type Config struct {
	URL      string
	Database string
	User     string
	Password string
}

type Aggregator struct {
	config Config
	client *http.Client
}

func NewAggregator(config Config, client *http.Client) *Aggregator {
	if client == nil {
		panic("ClickHouse HTTP client is required")
	}
	return &Aggregator{config: config, client: client}
}

func (aggregator *Aggregator) Aggregate(ctx context.Context, window domain.Window) error {
	endpoint, err := url.Parse(aggregator.config.URL)
	if err != nil {
		return fmt.Errorf("parse ClickHouse URL: %w", err)
	}
	parameters := endpoint.Query()
	parameters.Set("database", aggregator.config.Database)
	endpoint.RawQuery = parameters.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewBufferString(qualityQuery(window)))
	if err != nil {
		return err
	}
	request.SetBasicAuth(aggregator.config.User, aggregator.config.Password)
	request.Header.Set("Content-Type", "text/plain")
	response, err := aggregator.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("ClickHouse returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func qualityQuery(window domain.Window) string {
	return fmt.Sprintf(`
INSERT INTO event_quality_metrics
(window_start, window_end, topic_name, kafka_partition, consumer_group_id,
 event_count, duplicate_count, schema_violation_count, out_of_order_count,
 dlq_count, max_event_delay_ms, consumer_lag_messages, max_processing_latency_ms, source)
WITH
  toDateTime64('%s', 3, 'UTC') AS ws,
  toDateTime64('%s', 3, 'UTC') AS we,
  deliveries AS (
    SELECT kafka_topic, kafka_partition, kafka_offset, any(event_id) AS event_id,
           any(aggregate_type) AS aggregate_type, any(aggregate_id) AS aggregate_id,
           any(sequence) AS sequence, max(greatest(toInt64(dateDiff('millisecond', occurred_at, ingested_at)), 0)) AS delay_ms
    FROM canonical_forensics_events
    WHERE ingested_at >= ws AND ingested_at < we
    GROUP BY kafka_topic, kafka_partition, kafka_offset
  ),
  ordered AS (
    SELECT *, min(kafka_offset) OVER (PARTITION BY event_id) AS first_offset,
           max(sequence) OVER (PARTITION BY aggregate_type, aggregate_id ORDER BY kafka_topic, kafka_partition, kafka_offset ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING) AS prior_sequence
    FROM deliveries
  ),
  grouped AS (
    SELECT kafka_topic, kafka_partition,
           count() AS event_count,
           uniqExactIf(event_id, event_id IN (SELECT event_id FROM deliveries GROUP BY event_id HAVING count() > 1)) AS duplicate_count,
           countIf(kafka_offset = first_offset AND isNotNull(prior_sequence) AND sequence <= prior_sequence) AS out_of_order_count,
           max(delay_ms) AS max_event_delay_ms
    FROM ordered
    GROUP BY kafka_topic, kafka_partition
  ),
  dimensions AS (
    SELECT kafka_topic, kafka_partition, '' AS consumer_group_id FROM grouped
    UNION DISTINCT
    SELECT topic_name, kafka_partition, consumer_group_id
    FROM redpanda_consumer_group_metrics
    WHERE sampled_at >= ws AND sampled_at <= we
    UNION DISTINCT
    SELECT source_topic, source_partition, '' AS consumer_group_id
    FROM event_ingestion_failures
    WHERE failed_at >= ws AND failed_at < we
    UNION DISTINCT
    SELECT kafka_topic, kafka_partition, consumer_group_id
    FROM canonical_event_processing_attempts
    WHERE observed_at >= ws AND observed_at < we
  )
SELECT ws, we, dimensions.kafka_topic, dimensions.kafka_partition, dimensions.consumer_group_id, event_count, duplicate_count,
       (SELECT countDistinct(tuple(source_topic, source_partition, source_offset))
        FROM event_ingestion_failures
        WHERE error_type = 'SCHEMA_VIOLATION' AND failed_at >= ws AND failed_at < we),
       out_of_order_count,
       (SELECT countDistinct(tuple(source_topic, source_partition, source_offset))
        FROM event_ingestion_failures
        WHERE failed_at >= ws AND failed_at < we)
       + (SELECT countDistinct(tuple(event_id, consumer_group_id))
          FROM canonical_event_processing_attempts
          WHERE processing_status = 'DLQ' AND observed_at >= ws AND observed_at < we),
       max_event_delay_ms,
       (SELECT max(lag_messages) FROM redpanda_consumer_group_metrics
        WHERE topic_name = dimensions.kafka_topic AND kafka_partition = dimensions.kafka_partition
          AND consumer_group_id = dimensions.consumer_group_id AND sampled_at >= ws AND sampled_at <= we),
       (SELECT max(dateDiff('millisecond', started_at, completed_at))
        FROM canonical_event_processing_attempts
        WHERE completed_at IS NOT NULL AND started_at >= ws AND started_at < we),
       'quality-worker-v1'
FROM dimensions LEFT JOIN grouped USING (kafka_topic, kafka_partition)`, window.From.Format("2006-01-02 15:04:05.000"), window.To.Format("2006-01-02 15:04:05.000"))
}
