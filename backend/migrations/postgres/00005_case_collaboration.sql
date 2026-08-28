-- +goose Up
-- Phase 1.1 case collaboration: mutable collaboration metadata remains on the
-- aggregate row, while human notes are append-only records.
ALTER TABLE investigation_cases
    ADD COLUMN priority VARCHAR(2) NOT NULL DEFAULT 'P2',
    ADD COLUMN tags TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    ADD COLUMN related_correlation_ids TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    ADD COLUMN last_updated_by VARCHAR(200) NOT NULL DEFAULT 'system';

UPDATE investigation_cases
SET priority = CASE severity
    WHEN 'CRITICAL' THEN 'P0'
    WHEN 'HIGH' THEN 'P1'
    WHEN 'MEDIUM' THEN 'P2'
    ELSE 'P3'
END;

ALTER TABLE investigation_cases
    ADD CONSTRAINT investigation_cases_priority_chk CHECK (priority IN ('P0', 'P1', 'P2', 'P3')),
    ADD CONSTRAINT investigation_cases_tags_count_chk CHECK (cardinality(tags) <= 10),
    ADD CONSTRAINT investigation_cases_related_count_chk CHECK (cardinality(related_correlation_ids) <= 20),
    ADD CONSTRAINT investigation_cases_related_self_chk CHECK (NOT (correlation_id = ANY(related_correlation_ids)));

CREATE INDEX investigation_cases_priority_created_idx
    ON investigation_cases (priority, created_at DESC, id DESC);
CREATE INDEX investigation_cases_tags_gin_idx
    ON investigation_cases USING GIN (tags);
CREATE INDEX investigation_cases_related_correlations_gin_idx
    ON investigation_cases USING GIN (related_correlation_ids);

CREATE TABLE case_notes (
    id                    UUID PRIMARY KEY,
    investigation_case_id UUID NOT NULL REFERENCES investigation_cases(id) ON DELETE RESTRICT,
    body                  TEXT NOT NULL,
    author_id             VARCHAR(200) NOT NULL,
    author_role           VARCHAR(50) NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    CONSTRAINT case_notes_body_chk CHECK (length(btrim(body)) BETWEEN 1 AND 2000)
);

CREATE INDEX case_notes_case_created_idx
    ON case_notes (investigation_case_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS case_notes;
DROP INDEX IF EXISTS investigation_cases_related_correlations_gin_idx;
DROP INDEX IF EXISTS investigation_cases_tags_gin_idx;
DROP INDEX IF EXISTS investigation_cases_priority_created_idx;
ALTER TABLE investigation_cases
    DROP COLUMN IF EXISTS last_updated_by,
    DROP COLUMN IF EXISTS related_correlation_ids,
    DROP COLUMN IF EXISTS tags,
    DROP COLUMN IF EXISTS priority;
