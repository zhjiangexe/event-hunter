-- +goose Up
-- Consumer 處理嘗試的可選 Read Model，用於呈現 retry／DLQ；它不是 Kafka offset 的權威來源。
-- attempt 必須在 event_id + consumer_group_id 範圍解讀，不同 Consumer Group 不共用重試次數。
CREATE TABLE IF NOT EXISTS event_processing_attempts
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
ENGINE = MergeTree
-- 依開始處理時間分區，讓 90 天 TTL 能以時間範圍清理。
PARTITION BY toYYYYMM(started_at)
-- attempt_id 是來源產生的冪等識別碼；查詢仍須先去除相同 attempt_id 的 sink redelivery。
-- 同一事件與 Consumer Group 的所有 attempt 會相鄰，方便彙整最終狀態。
ORDER BY (correlation_id, event_id, consumer_group_id, attempt, started_at, attempt_id)
TTL started_at + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- +goose Down
-- 此表可由 OTel／Retry Topic 遙測重新產生，但回滾前仍須確認資料保存需求。
DROP TABLE IF EXISTS event_processing_attempts;
