-- +goose Up
-- 事件鑑識主表：由 Kafka ingestion pipeline 寫入，不由 Event Hunter API 與 PostgreSQL 雙寫。
-- 資料採 append-only；重複事件保留來源 offset，交由查詢與 Pattern 判斷，不直接 UPDATE 歷史事件。
CREATE TABLE IF NOT EXISTS forensics_events
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
    service_version    Nullable(String),
    payload            String,
    ingested_at        DateTime64(3, 'UTC') DEFAULT now64(3),
    INDEX event_id_bf event_id TYPE bloom_filter(0.01) GRANULARITY 4,
    INDEX trace_id_bf trace_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
-- 月分區方便依保存期限清理；不是主要查詢索引。
PARTITION BY toYYYYMM(occurred_at)
-- correlation_id 放在排序鍵最前方，優先支援 Business Timeline 的有界查詢。
ORDER BY (correlation_id, aggregate_id, occurred_at, event_id)
-- MVP 明定保留 90 天，過期資料由 ClickHouse 非同步清理。
TTL occurred_at + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- +goose Down
-- 回滾會刪除整張事件分析表，只能在確認環境與資料可重建時執行。
DROP TABLE IF EXISTS forensics_events;
