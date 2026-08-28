-- +goose Up
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS trace_parent VARCHAR(55);
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS trace_state TEXT;

-- +goose Down
ALTER TABLE outbox_events DROP COLUMN IF EXISTS trace_state;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS trace_parent;
