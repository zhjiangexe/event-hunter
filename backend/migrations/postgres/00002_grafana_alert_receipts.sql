-- +goose Up
-- Grafana webhook 可能重送同一通知；receipt 以確定性 dedup_key 防止重複建立案件或 Evidence。
-- 僅保存正規化 labels／annotations 與穩定 URL，不保存 HMAC secret、HTTP header 或未處理的 raw body。
CREATE TABLE grafana_alert_receipts (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dedup_key               VARCHAR(64) NOT NULL UNIQUE,
    grafana_org_id          BIGINT NOT NULL,
    receiver                VARCHAR(300) NOT NULL,
    group_key               TEXT NOT NULL,
    fingerprint             VARCHAR(300) NOT NULL,
    alert_status            VARCHAR(20) NOT NULL,
    correlation_id          VARCHAR(200),
    severity                VARCHAR(20),
    labels                  JSONB NOT NULL,
    annotations             JSONB NOT NULL,
    generator_url           TEXT,
    dashboard_url           TEXT,
    panel_url               TEXT,
    investigation_case_id   UUID REFERENCES investigation_cases(id) ON DELETE RESTRICT,
    disposition             VARCHAR(30) NOT NULL,
    received_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT grafana_alert_receipts_status_chk
        CHECK (alert_status IN ('firing', 'resolved')),
    CONSTRAINT grafana_alert_receipts_severity_chk
        CHECK (severity IS NULL OR severity IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    CONSTRAINT grafana_alert_receipts_labels_object_chk
        CHECK (jsonb_typeof(labels) = 'object'),
    CONSTRAINT grafana_alert_receipts_annotations_object_chk
        CHECK (jsonb_typeof(annotations) = 'object'),
    CONSTRAINT grafana_alert_receipts_disposition_chk
        CHECK (disposition IN ('CREATED_CASE', 'LINKED_CASE', 'IGNORED', 'RECORDED_RESOLUTION')),
    CONSTRAINT grafana_alert_receipts_dedup_key_chk
        CHECK (dedup_key ~ '^[a-f0-9]{64}$')
);

CREATE INDEX grafana_alert_receipts_correlation_idx
    ON grafana_alert_receipts (correlation_id, received_at DESC)
    WHERE correlation_id IS NOT NULL;
CREATE INDEX grafana_alert_receipts_case_idx
    ON grafana_alert_receipts (investigation_case_id, received_at DESC)
    WHERE investigation_case_id IS NOT NULL;
CREATE INDEX grafana_alert_receipts_fingerprint_idx
    ON grafana_alert_receipts (fingerprint, received_at DESC);

-- +goose Down
-- Receipt 是案件稽核證據；正式回滾前必須先確認保存與匯出要求。
DROP TABLE IF EXISTS grafana_alert_receipts;
