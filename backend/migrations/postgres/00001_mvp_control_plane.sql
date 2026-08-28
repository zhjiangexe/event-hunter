-- +goose Up
-- PostgreSQL 是 Event Hunter 的控制面，只保存可交易的案件、Finding、Evidence 索引與稽核資料。
-- UUID 由資料庫產生，避免 API 與背景工作各自實作不同的識別碼策略。
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 調查案件是唯一會反覆更新的 MVP Aggregate Root。
-- lock_version 必須搭配 UPDATE ... WHERE lock_version = ?，用來防止多人同時修改時互相覆蓋。
CREATE TABLE investigation_cases (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_no             VARCHAR(50) NOT NULL UNIQUE,
    title               VARCHAR(300) NOT NULL,
    severity            VARCHAR(20) NOT NULL,
    status              VARCHAR(30) NOT NULL DEFAULT 'OPEN',
    correlation_id      VARCHAR(200) NOT NULL,
    assignee            VARCHAR(200),
    root_cause          TEXT,
    resolution_summary  TEXT,
    fixed_version       VARCHAR(200),
    notes               TEXT,
    workflow_id         VARCHAR(200),
    lock_version        BIGINT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at           TIMESTAMPTZ,
    CONSTRAINT investigation_cases_severity_chk
        CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT investigation_cases_status_chk
        CHECK (status IN ('OPEN', 'INVESTIGATING', 'WAITING_APPROVAL', 'RESOLVED', 'CLOSED')),
    CONSTRAINT investigation_cases_lock_version_chk
        CHECK (lock_version >= 0),
    -- CLOSED 與 closed_at 必須同時成立；非 CLOSED 案件不可預先填入結案時間。
    CONSTRAINT investigation_cases_closed_at_chk
        CHECK ((status = 'CLOSED') = (closed_at IS NOT NULL))
);

-- correlation_id 支援從業務識別碼尋找案件；status／assignee 支援案件工作佇列。
CREATE INDEX investigation_cases_correlation_idx
    ON investigation_cases (correlation_id, created_at DESC);
CREATE INDEX investigation_cases_status_idx
    ON investigation_cases (status, created_at DESC);
CREATE INDEX investigation_cases_assignee_idx
    ON investigation_cases (assignee, created_at DESC)
    WHERE assignee IS NOT NULL;

-- Pattern Finding 是確定性規則的歷史輸出，採 append-only，不使用 lock_version。
-- 相同案件、Pattern 版本與查詢視窗的重試，以 idempotency_key 防止重複寫入。
CREATE TABLE pattern_findings (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_case_id   UUID NOT NULL REFERENCES investigation_cases(id) ON DELETE RESTRICT,
    pattern_id              VARCHAR(200) NOT NULL,
    pattern_version         INTEGER NOT NULL,
    severity                VARCHAR(20) NOT NULL,
    matched_conditions      JSONB NOT NULL,
    evidence_references     JSONB NOT NULL,
    recommended_next_query  TEXT NOT NULL,
    query_template_id       VARCHAR(200) NOT NULL,
    query_window_from       TIMESTAMPTZ NOT NULL,
    query_window_to         TIMESTAMPTZ NOT NULL,
    idempotency_key         VARCHAR(200) NOT NULL UNIQUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT pattern_findings_pattern_version_chk CHECK (pattern_version >= 1),
    CONSTRAINT pattern_findings_severity_chk
        CHECK (severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT pattern_findings_conditions_array_chk
        CHECK (jsonb_typeof(matched_conditions) = 'array'),
    CONSTRAINT pattern_findings_evidence_array_chk
        CHECK (jsonb_typeof(evidence_references) = 'array'),
    CONSTRAINT pattern_findings_query_window_chk
        CHECK (query_window_to > query_window_from)
);

CREATE INDEX pattern_findings_case_idx
    ON pattern_findings (investigation_case_id, created_at DESC);
CREATE INDEX pattern_findings_pattern_idx
    ON pattern_findings (pattern_id, created_at DESC);

-- Evidence 只保存穩定參照與完整性雜湊，不複製完整 Event payload、Log 或 Trace。
CREATE TABLE case_evidence (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_case_id   UUID NOT NULL REFERENCES investigation_cases(id) ON DELETE RESTRICT,
    evidence_type           VARCHAR(30) NOT NULL,
    reference               TEXT NOT NULL,
    checksum                VARCHAR(128),
    collected_at            TIMESTAMPTZ NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT case_evidence_type_chk
        CHECK (evidence_type IN (
            'EVENT', 'TRACE', 'LOG', 'METRIC', 'GRAFANA_ALERT',
            'QUALITY_VIOLATION', 'PATTERN_FINDING', 'REPORT'
        )),
    -- checksum 使用 SHA-256 小寫十六進位；尚無內容雜湊的外部參照可暫時為 NULL。
    CONSTRAINT case_evidence_checksum_chk
        CHECK (checksum IS NULL OR checksum ~ '^[a-f0-9]{64}$'),
    -- 同一案件的同類型 reference 只能保存一次，讓收集流程可以安全重試。
    CONSTRAINT case_evidence_reference_uq
        UNIQUE (investigation_case_id, evidence_type, reference)
);

CREATE INDEX case_evidence_case_idx
    ON case_evidence (investigation_case_id, collected_at DESC);

-- 稽核紀錄採 append-only，保存誰在何時對哪個資源做了什麼；metadata 不得放入完整敏感 payload。
CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id        VARCHAR(200) NOT NULL,
    actor_role      VARCHAR(30) NOT NULL,
    action          VARCHAR(100) NOT NULL,
    resource_type   VARCHAR(100) NOT NULL,
    resource_id     VARCHAR(200) NOT NULL,
    request_id      VARCHAR(200) NOT NULL,
    trace_id        VARCHAR(100),
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audit_logs_actor_role_chk
        CHECK (actor_role IN ('VIEWER', 'INVESTIGATOR', 'ADMIN')),
    CONSTRAINT audit_logs_metadata_object_chk
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX audit_logs_resource_idx
    ON audit_logs (resource_type, resource_id, created_at DESC);
CREATE INDEX audit_logs_actor_idx
    ON audit_logs (actor_id, created_at DESC);
CREATE INDEX audit_logs_request_idx
    ON audit_logs (request_id);

-- +goose Down
-- 依照外鍵相依關係反向刪除；此區只供 migration rollback，不由應用程式執行。
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS case_evidence;
DROP TABLE IF EXISTS pattern_findings;
DROP TABLE IF EXISTS investigation_cases;
