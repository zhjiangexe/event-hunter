-- +goose Up
-- Grafana 使用的事件品質聚合 Read Model；Event Hunter MVP 不另外開發 Runtime Quality Console。
-- 每一列是「統計時間窗 × Topic partition × Consumer Group」，不是可更新的交易狀態。
-- 此表不會由 ClickHouse 自動填值，必須由品質聚合工作以 append-only 方式寫入。
CREATE TABLE IF NOT EXISTS event_quality_metrics
(
    metric_id                 UUID DEFAULT generateUUIDv4(),
    window_start              DateTime64(3, 'UTC'),
    window_end                DateTime64(3, 'UTC'),
    calculated_at            DateTime64(3, 'UTC') DEFAULT now64(3),
    topic_name               LowCardinality(String),
    kafka_partition          UInt32,
    consumer_group_id        LowCardinality(String),
    -- 以下 count 都只代表 [window_start, window_end) 內的觀測值。
    event_count              UInt64,
    duplicate_count          UInt64,
    schema_violation_count   UInt64,
    out_of_order_count       UInt64,
    dlq_count                UInt64,
    -- Event delay 是 ingested_at - occurred_at；不是 Kafka Consumer backlog。
    max_event_delay_ms       UInt64,
    -- Consumer lag 是 broker 最新 offset 與 group committed offset 的訊息數差；可能沒有來源遙測。
    consumer_lag_messages    Nullable(UInt64),
    -- Consumer 單次處理耗時；沒有 event_processing_attempts 時可以為 NULL。
    max_processing_latency_ms Nullable(UInt64),
    source                   LowCardinality(String),
    CONSTRAINT valid_quality_window CHECK window_end > window_start
)
ENGINE = MergeTree
-- 依統計窗口月份分區，排序鍵配合 Topic／Consumer Group Dashboard 查詢。
PARTITION BY toYYYYMM(window_start)
ORDER BY (topic_name, consumer_group_id, kafka_partition, window_start, calculated_at)
-- 與原始鑑識事件採相同 90 天 MVP 保存期。
TTL window_end + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- +goose Down
-- 品質指標可由事件資料重算；正式回滾仍應先確認 Dashboard 與告警影響。
DROP TABLE IF EXISTS event_quality_metrics;
