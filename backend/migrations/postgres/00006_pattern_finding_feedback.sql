-- +goose Up
-- Keep deterministic findings append-only. Human classification is mutable
-- current state protected by optimistic locking and audited by the application.
CREATE TABLE pattern_finding_feedback (
    finding_id             UUID PRIMARY KEY REFERENCES pattern_findings(id) ON DELETE RESTRICT,
    investigation_case_id  UUID NOT NULL REFERENCES investigation_cases(id) ON DELETE RESTRICT,
    status                 VARCHAR(32) NOT NULL,
    actor_id               VARCHAR(200) NOT NULL,
    actor_role             VARCHAR(50) NOT NULL,
    lock_version           BIGINT NOT NULL DEFAULT 1,
    updated_at             TIMESTAMPTZ NOT NULL,
    CONSTRAINT pattern_finding_feedback_status_chk CHECK (status IN ('CONFIRMED', 'FALSE_POSITIVE', 'NEEDS_REVIEW')),
    CONSTRAINT pattern_finding_feedback_lock_chk CHECK (lock_version >= 1)
);

CREATE INDEX pattern_finding_feedback_case_idx
    ON pattern_finding_feedback (investigation_case_id, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS pattern_finding_feedback;
