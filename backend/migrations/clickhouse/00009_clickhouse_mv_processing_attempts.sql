-- +goose Up
-- ClickHouse-first processing-attempt ingestion. Kafka Connect only transports the
-- original record and Kafka coordinates into the isolated raw database; ClickHouse
-- performs deterministic admission and promotes only valid telemetry.
CREATE DATABASE IF NOT EXISTS event_hunter_poc;

CREATE TABLE IF NOT EXISTS event_hunter_poc.poc_processing_attempt_landing_raw
(
    raw_payload          String CODEC(ZSTD(3)),
    source_topic         LowCardinality(String),
    source_partition     Int32,
    source_offset        Int64,
    received_at          DateTime64(3, 'UTC') DEFAULT now64(3),

    payload_sha256       FixedString(64) MATERIALIZED lower(hex(SHA256(raw_payload))),
    valid_json           UInt8 MATERIALIZED isValidJSON(raw_payload),
    root_keys            Array(String) MATERIALIZED if(valid_json = 1, JSONExtractKeys(raw_payload), CAST([], 'Array(String)')),
    required_fields_present UInt8 MATERIALIZED hasAll(root_keys, [
        'attemptId', 'eventId', 'eventType', 'correlationId', 'traceId',
        'consumerGroupId', 'consumerService', 'attempt', 'processingStatus',
        'retryReason', 'retryTopic', 'kafkaTopic', 'kafkaPartition', 'kafkaOffset',
        'startedAt', 'completedAt', 'observedAt'
    ]),
    attempt_id           Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.attemptId'), ''),
    event_id             Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.eventId'), ''),
    event_type           Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.eventType'), ''),
    correlation_id       Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.correlationId'), ''),
    trace_id             Nullable(String) MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.traceId'), ''), 'null'),
    consumer_group_id    Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.consumerGroupId'), ''),
    consumer_service     Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.consumerService'), ''),
    attempt              Nullable(UInt32) MATERIALIZED toUInt32OrNull(JSON_VALUE(raw_payload, '$.attempt')),
    processing_status    Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.processingStatus'), ''),
    retry_reason         Nullable(String) MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.retryReason'), ''), 'null'),
    retry_topic          Nullable(String) MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.retryTopic'), ''), 'null'),
    kafka_topic          Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.kafkaTopic'), ''),
    kafka_partition      Nullable(Int32) MATERIALIZED toInt32OrNull(JSON_VALUE(raw_payload, '$.kafkaPartition')),
    kafka_offset         Nullable(Int64) MATERIALIZED toInt64OrNull(JSON_VALUE(raw_payload, '$.kafkaOffset')),
    started_at           Nullable(DateTime64(3, 'UTC')) MATERIALIZED parseDateTime64BestEffortOrNull(JSON_VALUE(raw_payload, '$.startedAt'), 3, 'UTC'),
    completed_at         Nullable(DateTime64(3, 'UTC')) MATERIALIZED parseDateTime64BestEffortOrNull(nullIf(nullIf(JSON_VALUE(raw_payload, '$.completedAt'), ''), 'null'), 3, 'UTC'),
    observed_at          Nullable(DateTime64(3, 'UTC')) MATERIALIZED parseDateTime64BestEffortOrNull(JSON_VALUE(raw_payload, '$.observedAt'), 3, 'UTC'),

    admission_error_code LowCardinality(String) MATERIALIZED multiIf(
        length(raw_payload) > 1048576, 'PAYLOAD_TOO_LARGE',
        valid_json = 0, 'INVALID_JSON',
        required_fields_present = 0, 'MISSING_OR_INVALID_REQUIRED_FIELD',
        attempt_id IS NULL OR event_id IS NULL OR event_type IS NULL OR correlation_id IS NULL OR
            consumer_group_id IS NULL OR consumer_service IS NULL OR attempt IS NULL OR attempt = 0 OR
            processing_status IS NULL OR kafka_topic IS NULL OR kafka_partition IS NULL OR kafka_partition < 0 OR
            kafka_offset IS NULL OR kafka_offset < 0 OR started_at IS NULL OR observed_at IS NULL,
            'MISSING_OR_INVALID_REQUIRED_FIELD',
        processing_status NOT IN ('STARTED', 'FAILED', 'RETRY_SCHEDULED', 'SUCCEEDED', 'DLQ'), 'SCHEMA_VIOLATION',
        trace_id IS NOT NULL AND (NOT match(trace_id, '^[0-9a-f]{32}$') OR trace_id = '00000000000000000000000000000000'),
            'SCHEMA_VIOLATION',
        processing_status IN ('FAILED', 'RETRY_SCHEDULED', 'DLQ') AND (retry_reason IS NULL OR completed_at IS NULL),
            'SCHEMA_VIOLATION',
        processing_status = 'SUCCEEDED' AND completed_at IS NULL, 'SCHEMA_VIOLATION',
        completed_at IS NOT NULL AND completed_at < started_at, 'SCHEMA_VIOLATION',
        'NONE'
    ),
    admission_status Enum8('VALID' = 1, 'QUARANTINED' = 2) MATERIALIZED
        if(admission_error_code = 'NONE', 'VALID', 'QUARANTINED')
)
ENGINE = ReplacingMergeTree(received_at)
PARTITION BY toYYYYMM(received_at)
ORDER BY (source_topic, source_partition, source_offset)
TTL received_at + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS event_hunter.poc_event_processing_attempts
(
    attempt_id           String,
    event_id             String,
    event_type           LowCardinality(String),
    correlation_id       String,
    trace_id             Nullable(String),
    consumer_group_id    LowCardinality(String),
    consumer_service     LowCardinality(String),
    attempt              UInt32,
    processing_status    Enum8(
        'STARTED' = 1,
        'FAILED' = 2,
        'RETRY_SCHEDULED' = 3,
        'SUCCEEDED' = 4,
        'DLQ' = 5
    ),
    retry_reason         Nullable(String),
    retry_topic          Nullable(String),
    kafka_topic          LowCardinality(String),
    kafka_partition      UInt32,
    kafka_offset         UInt64,
    started_at           DateTime64(3, 'UTC'),
    completed_at         Nullable(DateTime64(3, 'UTC')),
    observed_at          DateTime64(3, 'UTC') DEFAULT now64(3),
    CONSTRAINT attempt_positive CHECK attempt >= 1,
    INDEX attempt_id_bf attempt_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX attempt_event_id_bf event_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = ReplacingMergeTree(observed_at)
PARTITION BY toYYYYMM(started_at)
ORDER BY attempt_id
TTL started_at + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- Keep direct fixture/manual inserts compatible with the legacy table contract.
-- Without this default, omitted observed_at values become the Unix epoch and
-- recent-window queries (including Grafana alert rules) cannot see the row.
ALTER TABLE event_hunter.poc_event_processing_attempts
    MODIFY COLUMN observed_at DateTime64(3, 'UTC') DEFAULT now64(3);

CREATE TABLE IF NOT EXISTS event_hunter.poc_processing_attempt_admission_failures
(
    source_topic       LowCardinality(String),
    source_partition   UInt32,
    source_offset      UInt64,
    attempt_id         Nullable(String),
    event_id           Nullable(String),
    event_type         Nullable(String),
    correlation_id     Nullable(String),
    error_code         LowCardinality(String),
    payload_sha256     FixedString(64),
    admission_profile  LowCardinality(String),
    failed_at          DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(failed_at)
PARTITION BY toYYYYMM(failed_at)
ORDER BY (source_topic, source_partition, source_offset)
TTL failed_at + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192;

ALTER TABLE event_hunter.poc_processing_attempt_admission_failures
    ADD COLUMN IF NOT EXISTS event_type Nullable(String) AFTER event_id;

DROP VIEW IF EXISTS event_hunter_poc.poc_valid_processing_attempts_mv;
DROP VIEW IF EXISTS event_hunter_poc.poc_processing_attempt_failures_mv;

CREATE MATERIALIZED VIEW event_hunter_poc.poc_valid_processing_attempts_mv
TO event_hunter.poc_event_processing_attempts
AS
SELECT
    assumeNotNull(attempt_id) AS attempt_id,
    assumeNotNull(event_id) AS event_id,
    assumeNotNull(event_type) AS event_type,
    assumeNotNull(correlation_id) AS correlation_id,
    trace_id,
    assumeNotNull(consumer_group_id) AS consumer_group_id,
    assumeNotNull(consumer_service) AS consumer_service,
    assumeNotNull(attempt) AS attempt,
    assumeNotNull(processing_status) AS processing_status,
    retry_reason,
    retry_topic,
    assumeNotNull(kafka_topic) AS kafka_topic,
    toUInt32(assumeNotNull(kafka_partition)) AS kafka_partition,
    toUInt64(assumeNotNull(kafka_offset)) AS kafka_offset,
    assumeNotNull(started_at) AS started_at,
    completed_at,
    assumeNotNull(observed_at) AS observed_at
FROM event_hunter_poc.poc_processing_attempt_landing_raw
WHERE admission_status = 'VALID' AND source_partition >= 0 AND source_offset >= 0;

CREATE MATERIALIZED VIEW event_hunter_poc.poc_processing_attempt_failures_mv
TO event_hunter.poc_processing_attempt_admission_failures
AS
SELECT
    source_topic,
    toUInt32(source_partition) AS source_partition,
    toUInt64(source_offset) AS source_offset,
    attempt_id,
    event_id,
    event_type,
    correlation_id,
    admission_error_code AS error_code,
    payload_sha256,
    'processing-attempt-contract-v1' AS admission_profile,
    received_at AS failed_at
FROM event_hunter_poc.poc_processing_attempt_landing_raw
WHERE admission_status = 'QUARANTINED' AND source_partition >= 0 AND source_offset >= 0;

-- ClickHouse-first is the default source on a fresh database. The historical
-- table remains available through the standby view only for migration evidence.
CREATE VIEW IF NOT EXISTS event_hunter.canonical_event_processing_attempts AS
SELECT * FROM event_hunter.poc_event_processing_attempts;

CREATE VIEW IF NOT EXISTS event_hunter.canonical_event_processing_attempts_candidate AS
SELECT * FROM event_hunter.event_processing_attempts;

-- +goose Down
DROP VIEW IF EXISTS event_hunter.canonical_event_processing_attempts_candidate;
DROP VIEW IF EXISTS event_hunter.canonical_event_processing_attempts;
DROP VIEW IF EXISTS event_hunter_poc.poc_processing_attempt_failures_mv;
DROP VIEW IF EXISTS event_hunter_poc.poc_valid_processing_attempts_mv;
DROP TABLE IF EXISTS event_hunter.poc_processing_attempt_admission_failures;
DROP TABLE IF EXISTS event_hunter.poc_event_processing_attempts;
DROP TABLE IF EXISTS event_hunter_poc.poc_processing_attempt_landing_raw;
