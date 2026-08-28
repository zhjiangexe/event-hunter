-- +goose Up
-- Event Hunter runtime readers query only canonical_forensics_events.
-- ClickHouse-first is the formally adopted default. The standby view preserves
-- the historical table for bounded migration comparison; it has no active writer.
CREATE VIEW IF NOT EXISTS canonical_forensics_events AS
SELECT
    event_id, event_type, event_version, occurred_at, producer, correlation_id,
    causation_id, trace_id, aggregate_type, aggregate_id, sequence,
    kafka_topic, kafka_partition, kafka_offset,
    CAST(NULL, 'Nullable(String)') AS service_version,
    admission_status, quality_flags, admission_profile,
    payload, ingested_at
FROM poc_forensics_events;

-- Historical standby source. It is retained only as migration evidence.
CREATE VIEW IF NOT EXISTS canonical_forensics_events_candidate AS
SELECT
    event_id, event_type, event_version, occurred_at, producer, correlation_id,
    causation_id, trace_id, aggregate_type, aggregate_id, sequence,
    kafka_topic, kafka_partition, kafka_offset, service_version,
    'SEARCHABLE' AS admission_status,
    CAST([], 'Array(String)') AS quality_flags,
    'historical-domain-event-json-schema-v1' AS admission_profile,
    payload, ingested_at
FROM forensics_events;

-- +goose Down
DROP VIEW IF EXISTS canonical_forensics_events_candidate;
DROP VIEW IF EXISTS canonical_forensics_events;
