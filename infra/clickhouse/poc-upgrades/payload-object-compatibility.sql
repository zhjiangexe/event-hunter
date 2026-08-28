-- One-time compatibility repair for POC environments created before
-- 00006_clickhouse_mv_ingestion_poc.sql switched from JSON_QUERY to JSONExtractRaw.
DROP VIEW IF EXISTS event_hunter_poc.poc_valid_events_mv;

ALTER TABLE event_hunter.poc_forensics_events
    UPDATE payload = arrayElement(JSONExtractArrayRaw(payload), 1)
    WHERE JSONType(payload) = 'Array' AND length(JSONExtractArrayRaw(payload)) = 1
    SETTINGS mutations_sync = 1;

CREATE MATERIALIZED VIEW event_hunter_poc.poc_valid_events_mv
TO event_hunter.poc_forensics_events
AS
SELECT
    assumeNotNull(event_id) AS event_id,
    assumeNotNull(event_type) AS event_type,
    assumeNotNull(event_version) AS event_version,
    assumeNotNull(occurred_at) AS occurred_at,
    assumeNotNull(producer) AS producer,
    assumeNotNull(correlation_id) AS correlation_id,
    causation_id,
    trace_id,
    assumeNotNull(aggregate_type) AS aggregate_type,
    assumeNotNull(aggregate_id) AS aggregate_id,
    assumeNotNull(sequence) AS sequence,
    source_topic AS kafka_topic,
    toUInt32(source_partition) AS kafka_partition,
    toUInt64(source_offset) AS kafka_offset,
    JSONExtractRaw(raw_payload, 'payload') AS payload,
    payload_sha256,
    'minimum-envelope-v1' AS admission_profile,
    received_at AS ingested_at
FROM event_hunter_poc.poc_event_landing_raw
WHERE admission_status = 'VALID' AND source_partition >= 0 AND source_offset >= 0;
