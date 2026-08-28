-- Quality worker 可選的 consumer lag snapshot；沒有 broker sample 時保留 NULL。
CREATE TABLE IF NOT EXISTS redpanda_consumer_group_metrics
(
    sampled_at          DateTime64(3, 'UTC'),
    topic_name          LowCardinality(String),
    kafka_partition      UInt32,
    consumer_group_id   LowCardinality(String),
    lag_messages        UInt64
)
ENGINE = MergeTree
ORDER BY (topic_name, consumer_group_id, kafka_partition, sampled_at)
TTL sampled_at + INTERVAL 90 DAY DELETE;
