-- +goose Up
-- 保存無法通過事件 Schema 驗證的 ingestion 證據，不保存原始 payload，避免建立第二份敏感資料副本。
-- 原始失敗訊息只短期保存在受限的 Kafka DLQ；此表保存來源位置、錯誤與 payload SHA-256。
CREATE TABLE IF NOT EXISTS event_ingestion_failures
(
    failure_id          UUID DEFAULT generateUUIDv4(),
    source_topic       LowCardinality(String),
    source_partition   UInt32,
    source_offset      UInt64,
    event_id           Nullable(String),
    event_type         Nullable(String),
    correlation_id     Nullable(String),
    error_type         Enum8(
        'INVALID_JSON' = 1,
        'UNKNOWN_EVENT_TYPE' = 2,
        'UNKNOWN_EVENT_VERSION' = 3,
        'SCHEMA_VIOLATION' = 4,
        'MAPPING_FAILURE' = 5
    ),
    error_code          LowCardinality(String),
    error_summary       String,
    payload_sha256      FixedString(64),
    failed_at           DateTime64(3, 'UTC'),
    observed_at         DateTime64(3, 'UTC') DEFAULT now64(3),
    CONSTRAINT payload_sha256_lower_hex CHECK match(payload_sha256, '^[a-f0-9]{64}$')
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(failed_at)
ORDER BY (source_topic, source_partition, source_offset, failed_at)
TTL failed_at + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- +goose Down
-- 此表可由保留期內的 DLQ 重建；回滾前仍須確認稽核與調查保存需求。
DROP TABLE IF EXISTS event_ingestion_failures;
