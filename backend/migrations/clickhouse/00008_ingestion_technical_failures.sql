-- +goose Up
-- Kafka Connect technical DLQ 的安全投影。原始 Kafka value、exception message 與 stack trace
-- 可能包含敏感資料，因此只保存 transport identity、錯誤類別與 SHA-256。
CREATE TABLE IF NOT EXISTS ingestion_technical_failures
(
    failure_id          FixedString(64),
    dlq_topic           LowCardinality(String),
    dlq_partition       UInt32,
    dlq_offset          UInt64,
    source_topic        Nullable(String),
    source_partition    Nullable(UInt32),
    source_offset       Nullable(UInt64),
    connector_name      Nullable(String),
    connector_task      Nullable(UInt32),
    failure_stage       Nullable(String),
    exception_class     Nullable(String),
    payload_sha256      FixedString(64),
    observed_at         DateTime64(3, 'UTC'),
    ingested_at         DateTime64(3, 'UTC') DEFAULT now64(3),
    CONSTRAINT failure_id_lower_hex CHECK match(failure_id, '^[a-f0-9]{64}$'),
    CONSTRAINT payload_sha256_lower_hex CHECK match(payload_sha256, '^[a-f0-9]{64}$')
)
ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(observed_at)
ORDER BY (dlq_topic, dlq_partition, dlq_offset)
TTL observed_at + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192;

-- +goose Down
DROP TABLE IF EXISTS ingestion_technical_failures;
