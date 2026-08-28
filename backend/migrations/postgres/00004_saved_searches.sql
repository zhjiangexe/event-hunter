-- +goose Up
-- Saved Search 是使用者個人控制面資料；只保存 allowlisted bounded query state，不保存 payload 或任意 SQL。
CREATE TABLE saved_searches (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_subject  VARCHAR(200) NOT NULL,
    name           VARCHAR(80) NOT NULL,
    target         VARCHAR(20) NOT NULL,
    query_state    JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT saved_searches_target_chk CHECK (target IN ('TIMELINE', 'JOURNEY')),
    CONSTRAINT saved_searches_query_object_chk CHECK (jsonb_typeof(query_state) = 'object')
);

CREATE UNIQUE INDEX saved_searches_owner_name_uq
    ON saved_searches (owner_subject, lower(name));
CREATE INDEX saved_searches_owner_updated_idx
    ON saved_searches (owner_subject, updated_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS saved_searches;
