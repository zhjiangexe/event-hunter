-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE shipments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id VARCHAR(200) NOT NULL UNIQUE,
    correlation_id VARCHAR(200) NOT NULL UNIQUE,
    provider VARCHAR(200) NOT NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('CREATED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id VARCHAR(200) NOT NULL,
    event_type VARCHAR(200) NOT NULL,
    topic_name VARCHAR(249) NOT NULL,
    correlation_id VARCHAR(200) NOT NULL,
    trace_id VARCHAR(32),
    trace_parent VARCHAR(55),
    trace_state TEXT,
    service_version VARCHAR(200) NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX shipping_outbox_topic_idx ON outbox_events (topic_name, created_at, id);

-- +goose Down
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS shipments;
