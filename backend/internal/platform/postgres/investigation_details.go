package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

type InvestigationDetailsRepository struct {
	db *sql.DB
}

func NewInvestigationDetailsRepository(db *sql.DB) *InvestigationDetailsRepository {
	return &InvestigationDetailsRepository{db: db}
}

func (repository *InvestigationDetailsRepository) RecordAudit(ctx context.Context, actor ports.Actor, action, resourceID, requestID string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = executorFromContext(ctx, repository.db).ExecContext(ctx, `INSERT INTO audit_logs (actor_id,actor_role,action,resource_type,resource_id,request_id,metadata) VALUES ($1,$2,$3,'INVESTIGATION_CASE',$4,$5,$6::jsonb)`, actor.Subject, actor.Role, action, resourceID, requestID, encoded)
	return err
}

func (repository *InvestigationDetailsRepository) Notes(ctx context.Context, investigationID string) ([]domain.CaseNote, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT id::text,body,author_id,author_role,created_at FROM case_notes WHERE investigation_case_id=$1::uuid ORDER BY created_at DESC,id DESC`, investigationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.CaseNote, 0)
	for rows.Next() {
		var item domain.CaseNote
		if err := rows.Scan(&item.ID, &item.Body, &item.AuthorID, &item.AuthorRole, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *InvestigationDetailsRepository) Audit(ctx context.Context, investigationID string) ([]ports.AuditEntry, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT id::text,actor_id,actor_role,action,request_id,trace_id,metadata,created_at FROM audit_logs WHERE resource_type='INVESTIGATION_CASE' AND resource_id=$1 ORDER BY created_at DESC,id DESC`, investigationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ports.AuditEntry, 0)
	for rows.Next() {
		var item ports.AuditEntry
		var traceID sql.NullString
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.ActorID, &item.ActorRole, &item.Action, &item.RequestID, &traceID, &metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		if traceID.Valid {
			item.TraceID = &traceID.String
		}
		if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *InvestigationDetailsRepository) Findings(ctx context.Context, investigationID string) ([]ports.PatternFinding, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT finding.id::text,finding.pattern_id,finding.pattern_version,finding.severity,finding.matched_conditions,finding.evidence_references,finding.recommended_next_query,finding.query_template_id,finding.query_window_from,finding.query_window_to,finding.idempotency_key,
		COALESCE(feedback.status,'UNREVIEWED'),COALESCE(feedback.actor_id,''),COALESCE(feedback.actor_role,''),feedback.updated_at,COALESCE(feedback.lock_version,0)
		FROM pattern_findings finding
		LEFT JOIN pattern_finding_feedback feedback ON feedback.finding_id=finding.id
		WHERE finding.investigation_case_id=$1::uuid ORDER BY finding.created_at DESC`, investigationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ports.PatternFinding, 0)
	for rows.Next() {
		var item ports.PatternFinding
		var conditions, evidence []byte
		var feedbackUpdatedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.PatternID, &item.PatternVersion, &item.Severity, &conditions, &evidence, &item.RecommendedNextQuery, &item.QueryTemplateID, &item.WindowFrom, &item.WindowTo, &item.IdempotencyKey, &item.FeedbackStatus, &item.FeedbackActorID, &item.FeedbackActorRole, &feedbackUpdatedAt, &item.FeedbackLockVersion); err != nil {
			return nil, err
		}
		if feedbackUpdatedAt.Valid {
			item.FeedbackUpdatedAt = &feedbackUpdatedAt.Time
		}
		if err := json.Unmarshal(conditions, &item.MatchedConditions); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(evidence, &item.EvidenceReferences); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *InvestigationDetailsRepository) FindPatternFeedback(ctx context.Context, investigationID, findingID string) (domain.PatternFindingFeedback, error) {
	var feedback domain.PatternFindingFeedback
	var actorID, actorRole sql.NullString
	var updatedAt sql.NullTime
	err := executorFromContext(ctx, repository.db).QueryRowContext(ctx, `SELECT finding.id::text,finding.investigation_case_id::text,
		COALESCE(feedback.status,'UNREVIEWED'),feedback.actor_id,feedback.actor_role,feedback.updated_at,COALESCE(feedback.lock_version,0)
		FROM pattern_findings finding
		LEFT JOIN pattern_finding_feedback feedback ON feedback.finding_id=finding.id
		WHERE finding.investigation_case_id=$1::uuid AND finding.id=$2::uuid`, investigationID, findingID).Scan(
		&feedback.FindingID, &feedback.InvestigationID, &feedback.Status, &actorID, &actorRole, &updatedAt, &feedback.LockVersion,
	)
	if err == sql.ErrNoRows {
		return domain.PatternFindingFeedback{}, domain.ErrPatternFindingNotFound
	}
	if err != nil {
		return domain.PatternFindingFeedback{}, err
	}
	if actorID.Valid {
		feedback.ActorID = actorID.String
	}
	if actorRole.Valid {
		feedback.ActorRole = actorRole.String
	}
	if updatedAt.Valid {
		feedback.UpdatedAt = &updatedAt.Time
	}
	return feedback, nil
}

func (repository *InvestigationDetailsRepository) SavePatternFeedback(ctx context.Context, feedback domain.PatternFindingFeedback, expectedVersion int64) error {
	executor := executorFromContext(ctx, repository.db)
	if expectedVersion == 0 {
		result, err := executor.ExecContext(ctx, `INSERT INTO pattern_finding_feedback (finding_id,investigation_case_id,status,actor_id,actor_role,lock_version,updated_at)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7) ON CONFLICT (finding_id) DO NOTHING`, feedback.FindingID, feedback.InvestigationID, feedback.Status, feedback.ActorID, feedback.ActorRole, feedback.LockVersion, feedback.UpdatedAt)
		if err != nil {
			return err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected != 1 {
			return domain.ErrPatternFeedbackConflict
		}
		return nil
	}
	result, err := executor.ExecContext(ctx, `UPDATE pattern_finding_feedback SET status=$1,actor_id=$2,actor_role=$3,lock_version=$4,updated_at=$5
		WHERE finding_id=$6::uuid AND investigation_case_id=$7::uuid AND lock_version=$8`, feedback.Status, feedback.ActorID, feedback.ActorRole, feedback.LockVersion, feedback.UpdatedAt, feedback.FindingID, feedback.InvestigationID, expectedVersion)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return domain.ErrPatternFeedbackConflict
	}
	return nil
}

func (repository *InvestigationDetailsRepository) Evidence(ctx context.Context, investigationID string) ([]ports.Evidence, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT evidence.id::text,evidence.evidence_type,evidence.reference,evidence.checksum,evidence.collected_at,receipt.generator_url,receipt.grafana_org_id
		FROM case_evidence evidence
		LEFT JOIN grafana_alert_receipts receipt
			ON evidence.evidence_type='GRAFANA_ALERT' AND evidence.reference=receipt.id::text
		WHERE evidence.investigation_case_id=$1::uuid
		ORDER BY evidence.collected_at DESC,evidence.evidence_type,evidence.id`, investigationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ports.Evidence, 0)
	for rows.Next() {
		var item ports.Evidence
		var checksum, generatorURL sql.NullString
		var grafanaOrgID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.EvidenceType, &item.Reference, &checksum, &item.CollectedAt, &generatorURL, &grafanaOrgID); err != nil {
			return nil, err
		}
		if checksum.Valid {
			item.Checksum = &checksum.String
		}
		if generatorURL.Valid {
			item.GeneratorURL = &generatorURL.String
		}
		if grafanaOrgID.Valid {
			item.GrafanaOrgID = &grafanaOrgID.Int64
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *InvestigationDetailsRepository) SaveFinding(ctx context.Context, investigationID string, finding ports.PatternFinding) error {
	conditions, err := json.Marshal(finding.MatchedConditions)
	if err != nil {
		return err
	}
	evidence, err := json.Marshal(finding.EvidenceReferences)
	if err != nil {
		return err
	}
	_, err = executorFromContext(ctx, repository.db).ExecContext(ctx, `INSERT INTO pattern_findings (investigation_case_id,pattern_id,pattern_version,severity,matched_conditions,evidence_references,recommended_next_query,query_template_id,query_window_from,query_window_to,idempotency_key) VALUES ($1::uuid,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8,$9,$10,$11) ON CONFLICT (idempotency_key) DO NOTHING`, investigationID, finding.PatternID, finding.PatternVersion, finding.Severity, conditions, evidence, finding.RecommendedNextQuery, finding.QueryTemplateID, finding.WindowFrom, finding.WindowTo, finding.IdempotencyKey)
	return err
}

func (repository *InvestigationDetailsRepository) SaveEvidence(ctx context.Context, investigationID, evidenceType, reference, checksum string) error {
	_, err := executorFromContext(ctx, repository.db).ExecContext(ctx, `INSERT INTO case_evidence (investigation_case_id,evidence_type,reference,checksum,collected_at) VALUES ($1::uuid,$2,$3,$4,now()) ON CONFLICT (investigation_case_id,evidence_type,reference) DO NOTHING`, investigationID, evidenceType, reference, checksum)
	return err
}
