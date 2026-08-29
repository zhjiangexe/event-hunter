package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	alertintake "event-hunter/backend/internal/contexts/investigation/application/alerts"
)

type GrafanaAlertRepository struct {
	db *sql.DB
}

func NewGrafanaAlertRepository(db *sql.DB) *GrafanaAlertRepository {
	return &GrafanaAlertRepository{db: db}
}

func (repository *GrafanaAlertRepository) Transact(ctx context.Context, operation func(alertintake.GrafanaAlertTransaction) error) error {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if err := operation(&grafanaAlertTransaction{transaction: transaction}); err != nil {
		return err
	}
	return transaction.Commit()
}

type grafanaAlertTransaction struct {
	transaction *sql.Tx
}

func (transaction *grafanaAlertTransaction) LockDedup(ctx context.Context, dedupKey string) error {
	_, err := transaction.transaction.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", dedupKey)
	return err
}

func (transaction *grafanaAlertTransaction) InvestigationByDedup(ctx context.Context, dedupKey string) (string, bool, error) {
	var investigationID string
	err := transaction.transaction.QueryRowContext(ctx, "SELECT COALESCE(investigation_case_id::text, '') FROM grafana_alert_receipts WHERE dedup_key = $1", dedupKey).Scan(&investigationID)
	return optionalInvestigation(investigationID, err)
}

func (transaction *grafanaAlertTransaction) LatestFiringInvestigation(ctx context.Context, fingerprint string) (string, bool, error) {
	var investigationID string
	err := transaction.transaction.QueryRowContext(ctx, "SELECT investigation_case_id::text FROM grafana_alert_receipts WHERE fingerprint = $1 AND alert_status = 'firing' AND investigation_case_id IS NOT NULL ORDER BY received_at DESC LIMIT 1", fingerprint).Scan(&investigationID)
	return optionalInvestigation(investigationID, err)
}

func (transaction *grafanaAlertTransaction) OpenInvestigationByCorrelation(ctx context.Context, correlationID string) (string, bool, error) {
	var investigationID string
	err := transaction.transaction.QueryRowContext(ctx, "SELECT id::text FROM investigation_cases WHERE correlation_id = $1 AND status <> 'CLOSED' ORDER BY created_at DESC LIMIT 1", correlationID).Scan(&investigationID)
	return optionalInvestigation(investigationID, err)
}

func optionalInvestigation(investigationID string, err error) (string, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return investigationID, err == nil, err
}

func (transaction *grafanaAlertTransaction) CreateInvestigation(ctx context.Context, investigation alertintake.GrafanaAlertInvestigation) (string, error) {
	var investigationID string
	err := transaction.transaction.QueryRowContext(ctx, `INSERT INTO investigation_cases
		(case_no, title, severity, status, correlation_id, priority, last_updated_by, incident_from, incident_to, incident_window_source)
		VALUES ($1, $2, $3, 'OPEN', $4, $5, 'grafana-alert', $6, $7, $8) RETURNING id::text`,
		investigation.CaseNo, investigation.Title, investigation.Severity, investigation.CorrelationID,
		priorityForSeverity(investigation.Severity), investigation.IncidentFrom, investigation.IncidentTo, investigation.WindowSource,
	).Scan(&investigationID)
	return investigationID, err
}

func priorityForSeverity(severity string) string {
	switch severity {
	case "CRITICAL":
		return "P0"
	case "HIGH":
		return "P1"
	case "MEDIUM":
		return "P2"
	default:
		return "P3"
	}
}

func (transaction *grafanaAlertTransaction) SaveReceipt(ctx context.Context, receipt alertintake.GrafanaAlertReceipt) (string, error) {
	labels, err := json.Marshal(receipt.Labels)
	if err != nil {
		return "", err
	}
	annotations, err := json.Marshal(receipt.Annotations)
	if err != nil {
		return "", err
	}
	var receiptID string
	err = transaction.transaction.QueryRowContext(ctx, `INSERT INTO grafana_alert_receipts
		(dedup_key, grafana_org_id, receiver, group_key, fingerprint, alert_status, correlation_id, severity, labels, annotations, generator_url, dashboard_url, panel_url, investigation_case_id, disposition)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9::jsonb,$10::jsonb,$11,$12,$13,NULLIF($14,'')::uuid,$15)
		RETURNING id::text`, receipt.DedupKey, receipt.OrgID, receipt.Receiver, receipt.GroupKey, receipt.Fingerprint, receipt.AlertStatus, receipt.CorrelationID, receipt.Severity, labels, annotations, receipt.GeneratorURL, receipt.DashboardURL, receipt.PanelURL, receipt.InvestigationID, receipt.Disposition).Scan(&receiptID)
	return receiptID, err
}

func (transaction *grafanaAlertTransaction) SaveEvidence(ctx context.Context, investigationID, receiptID, checksum string) error {
	_, err := transaction.transaction.ExecContext(ctx, `INSERT INTO case_evidence (investigation_case_id,evidence_type,reference,checksum,collected_at)
		VALUES ($1::uuid,'GRAFANA_ALERT',$2,$3,now())
		ON CONFLICT (investigation_case_id,evidence_type,reference) DO NOTHING`, investigationID, receiptID, checksum)
	return err
}
