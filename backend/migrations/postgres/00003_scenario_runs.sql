-- +goose Up
-- Scenario Lab 只保存執行控制面與實際回查摘要；Domain Events 仍只經 Kafka 進 ClickHouse。
CREATE TABLE scenario_runs (
    id                   UUID PRIMARY KEY,
    scenario_id          VARCHAR(3) NOT NULL,
    scenario_name        VARCHAR(100) NOT NULL,
    execution_mode       VARCHAR(30) NOT NULL,
    synthetic            BOOLEAN NOT NULL,
    correlation_id       VARCHAR(200) NOT NULL,
    trace_id             VARCHAR(32),
    status               VARCHAR(20) NOT NULL,
    expected_event_types JSONB NOT NULL,
    actual               JSONB NOT NULL DEFAULT '{}'::jsonb,
    checks               JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message        TEXT,
    accepted_at          TIMESTAMPTZ NOT NULL,
    started_at           TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    CONSTRAINT scenario_runs_scenario_id_chk CHECK (scenario_id ~ '^S([1-9]|1[01])$'),
    CONSTRAINT scenario_runs_execution_mode_chk CHECK (execution_mode IN ('LIVE_SERVICES', 'LAB_INJECTION')),
    CONSTRAINT scenario_runs_status_chk CHECK (status IN ('ACCEPTED', 'RUNNING', 'PASSED', 'FAILED', 'TIMED_OUT')),
    CONSTRAINT scenario_runs_trace_id_chk CHECK (trace_id IS NULL OR trace_id ~ '^[a-f0-9]{32}$'),
    CONSTRAINT scenario_runs_expected_array_chk CHECK (jsonb_typeof(expected_event_types) = 'array'),
    CONSTRAINT scenario_runs_actual_object_chk CHECK (jsonb_typeof(actual) = 'object'),
    CONSTRAINT scenario_runs_checks_array_chk CHECK (jsonb_typeof(checks) = 'array')
);

CREATE INDEX scenario_runs_scenario_accepted_idx ON scenario_runs (scenario_id, accepted_at DESC);
CREATE INDEX scenario_runs_correlation_idx ON scenario_runs (correlation_id);

-- +goose Down
DROP TABLE IF EXISTS scenario_runs;
