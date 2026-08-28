-- +goose Up
-- Check Snapshots are immutable evidence aggregates. They preserve the pinned
-- model, as-of time, deterministic result and references, but never copy raw
-- canonical event payloads from ClickHouse.
CREATE TABLE check_snapshots (
    id                       UUID PRIMARY KEY,
    provenance               VARCHAR(40) NOT NULL,
    created_by               VARCHAR(200) NOT NULL,
    created_by_role          VARCHAR(30) NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL,
    evaluation_request       JSONB NOT NULL,
    as_of                    TIMESTAMPTZ NOT NULL,
    source_health            JSONB NOT NULL,
    model_id                 VARCHAR(64) NOT NULL,
    model_version            INTEGER NOT NULL,
    model_kind               VARCHAR(30) NOT NULL,
    model_source_path        TEXT NOT NULL,
    model_checksum           CHAR(64) NOT NULL,
    result                   JSONB NOT NULL,
    event_set_hash           CHAR(64) NOT NULL,
    evaluation_hash          CHAR(64) NOT NULL,
    result_schema_version    INTEGER NOT NULL,
    retention_profile        JSONB,
    idempotency_actor        VARCHAR(200) NOT NULL,
    idempotency_key          VARCHAR(200) NOT NULL,
    idempotency_request_hash CHAR(64) NOT NULL,
    CONSTRAINT check_snapshots_provenance_chk CHECK (provenance IN ('LIVE_EVALUATION', 'LEGACY_PATTERN_MIGRATION')),
    CONSTRAINT check_snapshots_actor_role_chk CHECK (created_by_role IN ('INVESTIGATOR', 'ADMIN')),
    CONSTRAINT check_snapshots_model_version_chk CHECK (model_version >= 1),
    CONSTRAINT check_snapshots_model_kind_chk CHECK (model_kind IN ('FLOW', 'GLOBAL_CHECK')),
    CONSTRAINT check_snapshots_result_version_chk CHECK (result_schema_version = 1),
    CONSTRAINT check_snapshots_model_checksum_chk CHECK (model_checksum ~ '^[a-f0-9]{64}$'),
    CONSTRAINT check_snapshots_event_hash_chk CHECK (event_set_hash ~ '^[a-f0-9]{64}$'),
    CONSTRAINT check_snapshots_evaluation_hash_chk CHECK (evaluation_hash ~ '^[a-f0-9]{64}$'),
    CONSTRAINT check_snapshots_request_hash_chk CHECK (idempotency_request_hash ~ '^[a-f0-9]{64}$'),
    CONSTRAINT check_snapshots_request_object_chk CHECK (jsonb_typeof(evaluation_request) = 'object'),
    CONSTRAINT check_snapshots_health_object_chk CHECK (jsonb_typeof(source_health) = 'object'),
    CONSTRAINT check_snapshots_result_object_chk CHECK (jsonb_typeof(result) = 'object'),
    CONSTRAINT check_snapshots_idempotency_uq UNIQUE (idempotency_actor, idempotency_key)
);

CREATE INDEX check_snapshots_model_idx ON check_snapshots (model_id, model_version, created_at DESC);
CREATE INDEX check_snapshots_created_idx ON check_snapshots (created_at DESC);

CREATE TABLE check_snapshot_event_refs (
    snapshot_id        UUID NOT NULL REFERENCES check_snapshots(id) ON DELETE RESTRICT,
    event_id           VARCHAR(200) NOT NULL,
    event_type         VARCHAR(128) NOT NULL,
    event_version      INTEGER NOT NULL,
    occurred_at        TIMESTAMPTZ NOT NULL,
    producer           VARCHAR(200) NOT NULL,
    aggregate_type     VARCHAR(100) NOT NULL,
    aggregate_id       VARCHAR(200) NOT NULL,
    sequence           NUMERIC(20,0) NOT NULL,
    correlation_id     VARCHAR(200) NOT NULL,
    trace_id           VARCHAR(32),
    payload_sha256     CHAR(64) NOT NULL,
    ordinal            INTEGER NOT NULL,
    disposition        VARCHAR(20) NOT NULL,
    adjustment_reason  VARCHAR(500),
    source_available   BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (snapshot_id, disposition, ordinal),
    CONSTRAINT check_snapshot_event_version_chk CHECK (event_version >= 1),
    CONSTRAINT check_snapshot_event_ordinal_chk CHECK (ordinal >= 0),
    CONSTRAINT check_snapshot_event_disposition_chk CHECK (disposition IN ('INCLUDED', 'EXCLUDED')),
    CONSTRAINT check_snapshot_event_payload_hash_chk CHECK (payload_sha256 ~ '^[a-f0-9]{64}$'),
    CONSTRAINT check_snapshot_event_trace_chk CHECK (trace_id IS NULL OR trace_id ~ '^[a-f0-9]{32}$')
);

CREATE INDEX check_snapshot_event_refs_event_idx ON check_snapshot_event_refs (event_id, snapshot_id);
CREATE INDEX check_snapshot_event_refs_correlation_idx ON check_snapshot_event_refs (correlation_id, snapshot_id);

CREATE TABLE check_snapshot_relations (
    snapshot_id       UUID NOT NULL REFERENCES check_snapshots(id) ON DELETE RESTRICT,
    ordinal           INTEGER NOT NULL,
    from_event_id     VARCHAR(200),
    to_event_id       VARCHAR(200) NOT NULL,
    relation_type     VARCHAR(40) NOT NULL,
    source_field      VARCHAR(200),
    source_model_id   VARCHAR(64),
    source_rule_id    VARCHAR(64),
    PRIMARY KEY (snapshot_id, ordinal),
    CONSTRAINT check_snapshot_relation_ordinal_chk CHECK (ordinal >= 0),
    CONSTRAINT check_snapshot_relation_type_chk CHECK (relation_type IN ('SEED', 'SAME_CORRELATION', 'SAME_AGGREGATE', 'CAUSATION', 'BUSINESS_KEY', 'PARENT_CHILD', 'CUSTOM_INCLUDE'))
);

CREATE TABLE check_findings (
    id                            UUID PRIMARY KEY,
    snapshot_id                   UUID NOT NULL REFERENCES check_snapshots(id) ON DELETE RESTRICT,
    ordinal                       INTEGER NOT NULL,
    rule_kind                     VARCHAR(40) NOT NULL,
    rule_id                       VARCHAR(64) NOT NULL,
    rule_version                  INTEGER NOT NULL,
    rule_checksum                 CHAR(64) NOT NULL,
    severity                      VARCHAR(20) NOT NULL,
    code                          VARCHAR(64) NOT NULL,
    expectation_state             VARCHAR(30),
    evidence_references           JSONB NOT NULL,
    recommended_query_template_id VARCHAR(128),
    created_at                    TIMESTAMPTZ NOT NULL,
    CONSTRAINT check_findings_snapshot_ordinal_uq UNIQUE (snapshot_id, ordinal),
    CONSTRAINT check_findings_rule_version_chk CHECK (rule_version >= 1),
    CONSTRAINT check_findings_rule_checksum_chk CHECK (rule_checksum ~ '^[a-f0-9]{64}$'),
    CONSTRAINT check_findings_severity_chk CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT check_findings_evidence_array_chk CHECK (jsonb_typeof(evidence_references) = 'array')
);

CREATE INDEX check_findings_snapshot_idx ON check_findings (snapshot_id, ordinal);
CREATE INDEX check_findings_code_idx ON check_findings (code, created_at DESC);

CREATE TABLE check_finding_feedback (
    finding_id    UUID PRIMARY KEY REFERENCES check_findings(id) ON DELETE RESTRICT,
    status        VARCHAR(32) NOT NULL,
    actor_id      VARCHAR(200) NOT NULL,
    actor_role    VARCHAR(30) NOT NULL,
    lock_version  BIGINT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT check_finding_feedback_status_chk CHECK (status IN ('CONFIRMED', 'FALSE_POSITIVE', 'NEEDS_REVIEW')),
    CONSTRAINT check_finding_feedback_role_chk CHECK (actor_role IN ('INVESTIGATOR', 'ADMIN')),
    CONSTRAINT check_finding_feedback_lock_chk CHECK (lock_version >= 1)
);

CREATE TABLE investigation_check_snapshots (
    investigation_case_id UUID NOT NULL REFERENCES investigation_cases(id) ON DELETE RESTRICT,
    snapshot_id            UUID NOT NULL REFERENCES check_snapshots(id) ON DELETE RESTRICT,
    linked_by              VARCHAR(200) NOT NULL,
    linked_by_role         VARCHAR(30) NOT NULL,
    linked_at              TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (investigation_case_id, snapshot_id),
    CONSTRAINT investigation_snapshot_role_chk CHECK (linked_by_role IN ('INVESTIGATOR', 'ADMIN'))
);

CREATE INDEX investigation_check_snapshots_linked_idx ON investigation_check_snapshots (investigation_case_id, linked_at DESC, snapshot_id);

-- +goose Down
DROP TABLE IF EXISTS investigation_check_snapshots;
DROP TABLE IF EXISTS check_finding_feedback;
DROP TABLE IF EXISTS check_findings;
DROP TABLE IF EXISTS check_snapshot_relations;
DROP TABLE IF EXISTS check_snapshot_event_refs;
DROP TABLE IF EXISTS check_snapshots;
