-- +goose Up
-- ClickHouse-first ingestion POC：原始訊息先落在隔離 database，只有通過最低 Admission Contract
-- 的事件才會提升到 Event Hunter 可查詢的正式 POC read model。
--
-- raw_payload 可能含敏感資料，因此：
--   1. 不放在 grafana_reader 可讀的 event_hunter database；
--   2. 僅保留 7 天；
--   3. failure read model 只保存安全識別欄位與 SHA-256，不複製原始 payload。
CREATE DATABASE IF NOT EXISTS event_hunter_poc;

CREATE TABLE IF NOT EXISTS event_hunter_poc.poc_event_landing_raw
(
    raw_payload          String CODEC(ZSTD(3)),
    source_topic         LowCardinality(String),
    source_partition     Int32,
    source_offset        Int64,
    received_at          DateTime64(3, 'UTC') DEFAULT now64(3),

    payload_sha256       FixedString(64) MATERIALIZED lower(hex(SHA256(raw_payload))),
    valid_json           UInt8 MATERIALIZED isValidJSON(raw_payload),
    event_id             Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.eventId'), ''),
    event_type           Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.eventType'), ''),
    event_version        Nullable(UInt32) MATERIALIZED toUInt32OrNull(JSON_VALUE(raw_payload, '$.eventVersion')),
    occurred_at          Nullable(DateTime64(3, 'UTC')) MATERIALIZED parseDateTime64BestEffortOrNull(JSON_VALUE(raw_payload, '$.occurredAt'), 3, 'UTC'),
    producer             Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.producer'), ''),
    correlation_id       Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.correlationId'), ''),
    causation_id         Nullable(String) MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.causationId'), ''), 'null'),
    trace_id             Nullable(String) MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.traceId'), ''), 'null'),
    aggregate_type       Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.aggregateType'), ''),
    aggregate_id         Nullable(String) MATERIALIZED nullIf(JSON_VALUE(raw_payload, '$.aggregateId'), ''),
    sequence             Nullable(UInt64) MATERIALIZED toUInt64OrNull(JSON_VALUE(raw_payload, '$.sequence')),
    payload_kind         LowCardinality(String) MATERIALIZED JSONType(raw_payload, 'payload'),
    payload_keys         Array(String) MATERIALIZED if(
        payload_kind = 'Object', JSONExtractKeys(JSONExtractRaw(raw_payload, 'payload')), CAST([], 'Array(String)')
    ),
    payload_required_fields_valid UInt8 MATERIALIZED multiIf(
        event_type = 'OrderCancelled', hasAll(payload_keys, ['orderId', 'reason']),
        event_type = 'OrderCreated', hasAll(payload_keys, ['orderId', 'customerId', 'totalAmount', 'currency']),
        event_type = 'PaymentCompleted', hasAll(payload_keys, ['paymentId', 'orderId', 'amount', 'currency']),
        event_type = 'PaymentFailed', hasAll(payload_keys, ['paymentId', 'orderId', 'reasonCode', 'retryable', 'status']),
        event_type = 'PaymentRefunded', hasAll(payload_keys, ['paymentId', 'orderId', 'amount', 'reason']),
        event_type = 'PaymentVoided', hasAll(payload_keys, ['paymentId', 'orderId', 'reason']),
        event_type = 'ReturnReceived', hasAll(payload_keys, ['returnId', 'orderId', 'receivedAt', 'condition', 'status']),
        event_type = 'ReturnRequested', hasAll(payload_keys, ['returnId', 'orderId', 'shipmentId', 'reason', 'status']),
        event_type = 'ShipmentCreated', hasAll(payload_keys, ['shipmentId', 'orderId', 'provider', 'status']),
        event_type = 'ShipmentDelivered', hasAll(payload_keys, ['shipmentId', 'orderId', 'deliveredAt', 'recipient', 'status']),
        event_type = 'ShipmentDispatchFailed', hasAll(payload_keys, ['shipmentId', 'orderId', 'provider', 'reasonCode', 'retryable', 'status']),
        event_type = 'ShipmentDispatched', hasAll(payload_keys, ['shipmentId', 'orderId', 'provider', 'trackingNumber', 'status']),
        event_type = 'ShipmentInTransit', hasAll(payload_keys, ['shipmentId', 'orderId', 'location', 'status']),
        1
    ),

    admission_error_code LowCardinality(String) MATERIALIZED multiIf(
        length(raw_payload) > 1048576, 'PAYLOAD_TOO_LARGE',
        valid_json = 0, 'INVALID_JSON',
        event_id IS NULL OR event_type IS NULL OR event_version IS NULL OR occurred_at IS NULL OR
            producer IS NULL OR correlation_id IS NULL OR aggregate_type IS NULL OR aggregate_id IS NULL OR
            sequence IS NULL OR event_version = 0 OR sequence = 0 OR payload_kind = 'Null', 'MISSING_OR_INVALID_REQUIRED_FIELD',
        payload_kind != 'Object', 'PAYLOAD_NOT_OBJECT',
        payload_required_fields_valid = 0, 'SCHEMA_VIOLATION',
        'NONE'
    ),
    admission_warning_codes Array(String) MATERIALIZED arrayFilter(code -> code != '', [
        if(admission_error_code = 'NONE' AND coalesce(event_type NOT IN (
            'OrderCancelled', 'OrderCreated',
            'PaymentCompleted', 'PaymentFailed', 'PaymentRefunded', 'PaymentVoided',
            'ShipmentCreated', 'ShipmentDelivered', 'ShipmentDispatchFailed', 'ShipmentDispatched', 'ShipmentInTransit',
            'ReturnReceived', 'ReturnRequested'
        ), false), 'UNKNOWN_EVENT_TYPE', ''),
        if(admission_error_code = 'NONE' AND coalesce(event_version != 1, false), 'UNKNOWN_EVENT_VERSION', ''),
        if(admission_error_code = 'NONE' AND trace_id IS NOT NULL AND coalesce(
            (NOT match(trace_id, '^[0-9a-f]{32}$') OR trace_id = '00000000000000000000000000000000'),
            false), 'INVALID_TRACE_ID', ''),
        if(admission_error_code = 'NONE' AND coalesce((startsWith(event_type, 'Order') AND aggregate_type != 'Order') OR
            (startsWith(event_type, 'Payment') AND aggregate_type != 'Payment') OR
            (startsWith(event_type, 'Shipment') AND aggregate_type != 'Shipment') OR
            (startsWith(event_type, 'Return') AND aggregate_type != 'Return'), false),
            'AGGREGATE_TYPE_MISMATCH', '')
    ]),
    admission_status Enum8('SEARCHABLE' = 1, 'SEARCHABLE_WITH_WARNINGS' = 2, 'QUARANTINED' = 3) MATERIALIZED
        multiIf(
            admission_error_code != 'NONE', 'QUARANTINED',
            notEmpty(admission_warning_codes), 'SEARCHABLE_WITH_WARNINGS',
            'SEARCHABLE'
        )
)
ENGINE = ReplacingMergeTree(received_at)
PARTITION BY toYYYYMM(received_at)
ORDER BY (source_topic, source_partition, source_offset)
TTL received_at + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192;

-- 相容早期 POC DDL：ClickHouse JSON_VALUE 對 JSON null 會回傳字串 'null'；明確正規化為 SQL NULL。
-- MODIFY COLUMN 對相同 expression 可安全重複執行，後續 insert 一律使用修正版。
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    MODIFY COLUMN causation_id Nullable(String)
    MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.causationId'), ''), 'null');
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    MODIFY COLUMN trace_id Nullable(String)
    MATERIALIZED nullIf(nullIf(JSON_VALUE(raw_payload, '$.traceId'), ''), 'null');

-- 通過最低 searchability contract 的事件；SEARCHABLE 不等於完整業務 Schema 已驗證。
-- 這張表已是正式 ClickHouse-first promoted store。poc_ 名稱只為保留既有
-- connector offsets、migration history 與 rolling-upgrade 相容性。
CREATE TABLE IF NOT EXISTS event_hunter.poc_forensics_events
(
    event_id           String,
    event_type         LowCardinality(String),
    event_version      UInt32,
    occurred_at        DateTime64(3, 'UTC'),
    producer           LowCardinality(String),
    correlation_id     String,
    causation_id       Nullable(String),
    trace_id           Nullable(String),
    aggregate_type     LowCardinality(String),
    aggregate_id       String,
    sequence           UInt64,
    kafka_topic        LowCardinality(String),
    kafka_partition    UInt32,
    kafka_offset       UInt64,
    payload            String,
    payload_sha256     FixedString(64),
    admission_profile  LowCardinality(String) DEFAULT 'minimum-envelope-v1',
    admission_status   LowCardinality(String) DEFAULT 'SEARCHABLE',
    quality_flags      Array(String) DEFAULT [],
    ingested_at        DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (kafka_topic, kafka_partition, kafka_offset)
TTL occurred_at + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- Grafana 與 POC 驗收只需要安全的失敗摘要；raw_payload 仍留在隔離 landing database。
CREATE TABLE IF NOT EXISTS event_hunter.poc_event_admission_failures
(
    source_topic       LowCardinality(String),
    source_partition   UInt32,
    source_offset      UInt64,
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

-- 本機 bootstrap 會重複套用 migration。既有 POC 可能仍是 VALID/QUARANTINED 舊語意，
-- 因此必須先移除相依 MV、升級衍生欄位，再以新版欄位契約重建 MV。
DROP VIEW IF EXISTS event_hunter_poc.poc_admission_failures_mv;
DROP VIEW IF EXISTS event_hunter_poc.poc_valid_events_mv;

ALTER TABLE event_hunter_poc.poc_event_landing_raw
    ADD COLUMN IF NOT EXISTS payload_keys Array(String) MATERIALIZED if(
        payload_kind = 'Object', JSONExtractKeys(JSONExtractRaw(raw_payload, 'payload')), CAST([], 'Array(String)')
    ) AFTER payload_kind;
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    ADD COLUMN IF NOT EXISTS payload_required_fields_valid UInt8 MATERIALIZED multiIf(
        event_type = 'OrderCancelled', hasAll(payload_keys, ['orderId', 'reason']),
        event_type = 'OrderCreated', hasAll(payload_keys, ['orderId', 'customerId', 'totalAmount', 'currency']),
        event_type = 'PaymentCompleted', hasAll(payload_keys, ['paymentId', 'orderId', 'amount', 'currency']),
        event_type = 'PaymentFailed', hasAll(payload_keys, ['paymentId', 'orderId', 'reasonCode', 'retryable', 'status']),
        event_type = 'PaymentRefunded', hasAll(payload_keys, ['paymentId', 'orderId', 'amount', 'reason']),
        event_type = 'PaymentVoided', hasAll(payload_keys, ['paymentId', 'orderId', 'reason']),
        event_type = 'ReturnReceived', hasAll(payload_keys, ['returnId', 'orderId', 'receivedAt', 'condition', 'status']),
        event_type = 'ReturnRequested', hasAll(payload_keys, ['returnId', 'orderId', 'shipmentId', 'reason', 'status']),
        event_type = 'ShipmentCreated', hasAll(payload_keys, ['shipmentId', 'orderId', 'provider', 'status']),
        event_type = 'ShipmentDelivered', hasAll(payload_keys, ['shipmentId', 'orderId', 'deliveredAt', 'recipient', 'status']),
        event_type = 'ShipmentDispatchFailed', hasAll(payload_keys, ['shipmentId', 'orderId', 'provider', 'reasonCode', 'retryable', 'status']),
        event_type = 'ShipmentDispatched', hasAll(payload_keys, ['shipmentId', 'orderId', 'provider', 'trackingNumber', 'status']),
        event_type = 'ShipmentInTransit', hasAll(payload_keys, ['shipmentId', 'orderId', 'location', 'status']),
        1
    ) AFTER payload_keys;
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    MODIFY COLUMN admission_error_code LowCardinality(String) MATERIALIZED multiIf(
        length(raw_payload) > 1048576, 'PAYLOAD_TOO_LARGE',
        valid_json = 0, 'INVALID_JSON',
        event_id IS NULL OR event_type IS NULL OR event_version IS NULL OR occurred_at IS NULL OR
            producer IS NULL OR correlation_id IS NULL OR aggregate_type IS NULL OR aggregate_id IS NULL OR
            sequence IS NULL OR event_version = 0 OR sequence = 0 OR payload_kind = 'Null', 'MISSING_OR_INVALID_REQUIRED_FIELD',
        payload_kind != 'Object', 'PAYLOAD_NOT_OBJECT',
        payload_required_fields_valid = 0, 'SCHEMA_VIOLATION',
        'NONE'
    );
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    ADD COLUMN IF NOT EXISTS admission_warning_codes Array(String) MATERIALIZED arrayFilter(code -> code != '', [
        if(admission_error_code = 'NONE' AND coalesce(event_type NOT IN (
            'OrderCancelled', 'OrderCreated',
            'PaymentCompleted', 'PaymentFailed', 'PaymentRefunded', 'PaymentVoided',
            'ShipmentCreated', 'ShipmentDelivered', 'ShipmentDispatchFailed', 'ShipmentDispatched', 'ShipmentInTransit',
            'ReturnReceived', 'ReturnRequested'
        ), false), 'UNKNOWN_EVENT_TYPE', ''),
        if(admission_error_code = 'NONE' AND coalesce(event_version != 1, false), 'UNKNOWN_EVENT_VERSION', ''),
        if(admission_error_code = 'NONE' AND trace_id IS NOT NULL AND coalesce(
            (NOT match(trace_id, '^[0-9a-f]{32}$') OR trace_id = '00000000000000000000000000000000'),
            false), 'INVALID_TRACE_ID', ''),
        if(admission_error_code = 'NONE' AND coalesce((startsWith(event_type, 'Order') AND aggregate_type != 'Order') OR
            (startsWith(event_type, 'Payment') AND aggregate_type != 'Payment') OR
            (startsWith(event_type, 'Shipment') AND aggregate_type != 'Shipment') OR
            (startsWith(event_type, 'Return') AND aggregate_type != 'Return'), false),
            'AGGREGATE_TYPE_MISMATCH', '')
    ]) AFTER admission_error_code;
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    DROP COLUMN IF EXISTS admission_status;
ALTER TABLE event_hunter_poc.poc_event_landing_raw
    ADD COLUMN admission_status Enum8('SEARCHABLE' = 1, 'SEARCHABLE_WITH_WARNINGS' = 2, 'QUARANTINED' = 3)
    MATERIALIZED multiIf(
        admission_error_code != 'NONE', 'QUARANTINED',
        notEmpty(admission_warning_codes), 'SEARCHABLE_WITH_WARNINGS',
        'SEARCHABLE'
    ) AFTER admission_warning_codes;

ALTER TABLE event_hunter.poc_forensics_events
    ADD COLUMN IF NOT EXISTS admission_status LowCardinality(String) DEFAULT 'SEARCHABLE' AFTER admission_profile;
ALTER TABLE event_hunter.poc_forensics_events
    ADD COLUMN IF NOT EXISTS quality_flags Array(String) DEFAULT [] AFTER admission_status;

CREATE MATERIALIZED VIEW IF NOT EXISTS event_hunter_poc.poc_valid_events_mv
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
    toString(admission_status) AS admission_status,
    admission_warning_codes AS quality_flags,
    received_at AS ingested_at
FROM event_hunter_poc.poc_event_landing_raw
WHERE admission_status IN ('SEARCHABLE', 'SEARCHABLE_WITH_WARNINGS')
  AND source_partition >= 0 AND source_offset >= 0;

CREATE MATERIALIZED VIEW IF NOT EXISTS event_hunter_poc.poc_admission_failures_mv
TO event_hunter.poc_event_admission_failures
AS
SELECT
    source_topic,
    toUInt32(source_partition) AS source_partition,
    toUInt64(source_offset) AS source_offset,
    event_id,
    event_type,
    correlation_id,
    admission_error_code AS error_code,
    payload_sha256,
    'minimum-envelope-v1' AS admission_profile,
    received_at AS failed_at
FROM event_hunter_poc.poc_event_landing_raw
WHERE admission_status = 'QUARANTINED' AND source_partition >= 0 AND source_offset >= 0;

-- +goose Down
DROP VIEW IF EXISTS event_hunter_poc.poc_admission_failures_mv;
DROP VIEW IF EXISTS event_hunter_poc.poc_valid_events_mv;
DROP TABLE IF EXISTS event_hunter.poc_event_admission_failures;
DROP TABLE IF EXISTS event_hunter.poc_forensics_events;
DROP TABLE IF EXISTS event_hunter_poc.poc_event_landing_raw;
DROP DATABASE IF EXISTS event_hunter_poc;
