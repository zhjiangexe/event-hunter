-- +goose Up
-- Incident Window 是案件的不可變調查基準。舊案件以 created_at 結束、往前 24 小時回填，
-- 避免每次開啟時使用 rolling now 造成歷史證據看似消失。
ALTER TABLE investigation_cases
    ADD COLUMN IF NOT EXISTS incident_from TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS incident_to TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS incident_window_source VARCHAR(30);

UPDATE investigation_cases
SET incident_from = COALESCE(incident_from, created_at - INTERVAL '24 hours'),
    incident_to = COALESCE(incident_to, created_at),
    incident_window_source = COALESCE(incident_window_source, 'LEGACY_CREATED_AT')
WHERE incident_from IS NULL OR incident_to IS NULL OR incident_window_source IS NULL;

ALTER TABLE investigation_cases
    ALTER COLUMN incident_from SET NOT NULL,
    ALTER COLUMN incident_to SET NOT NULL,
    ALTER COLUMN incident_window_source SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'investigation_cases_incident_window_chk'
          AND conrelid = 'investigation_cases'::regclass
    ) THEN
        ALTER TABLE investigation_cases
            ADD CONSTRAINT investigation_cases_incident_window_chk
            CHECK (incident_to > incident_from AND incident_to - incident_from <= INTERVAL '7 days');
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'investigation_cases_incident_source_chk'
          AND conrelid = 'investigation_cases'::regclass
    ) THEN
        ALTER TABLE investigation_cases
            ADD CONSTRAINT investigation_cases_incident_source_chk
            CHECK (incident_window_source IN ('TIMELINE_SEARCH', 'MANUAL_DEFAULT', 'GRAFANA_ALERT', 'LEGACY_CREATED_AT'));
    END IF;
END $$;

-- +goose Down
ALTER TABLE investigation_cases
    DROP CONSTRAINT IF EXISTS investigation_cases_incident_source_chk,
    DROP CONSTRAINT IF EXISTS investigation_cases_incident_window_chk,
    DROP COLUMN IF EXISTS incident_window_source,
    DROP COLUMN IF EXISTS incident_to,
    DROP COLUMN IF EXISTS incident_from;
