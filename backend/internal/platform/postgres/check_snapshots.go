package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	listchecksnapshots "event-hunter/backend/internal/contexts/eventcheck/application/list_check_snapshots"
	eventcheckdomain "event-hunter/backend/internal/contexts/eventcheck/domain"
	eventcheckports "event-hunter/backend/internal/contexts/eventcheck/ports"
	investigationdomain "event-hunter/backend/internal/contexts/investigation/domain"
)

type CheckSnapshotRepository struct {
	db *sql.DB
}

func NewCheckSnapshotRepository(db *sql.DB) *CheckSnapshotRepository {
	return &CheckSnapshotRepository{db: db}
}

func (repository *CheckSnapshotRepository) Create(ctx context.Context, snapshot eventcheckdomain.CheckSnapshot) (eventcheckdomain.CheckSnapshot, bool, error) {
	executor := executorFromContext(ctx, repository.db)
	var insertedID string
	err := executor.QueryRowContext(ctx, `INSERT INTO check_snapshots
        (id,provenance,created_by,created_by_role,created_at,evaluation_request,as_of,source_health,
         model_id,model_version,model_kind,model_source_path,model_checksum,result,event_set_hash,evaluation_hash,
         result_schema_version,retention_profile,idempotency_actor,idempotency_key,idempotency_request_hash)
        VALUES ($1::uuid,$2,$3,$4,$5,$6::jsonb,$7,$8::jsonb,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,$17,$18::jsonb,$19,$20,$21)
        ON CONFLICT (idempotency_actor,idempotency_key) DO NOTHING RETURNING id::text`,
		snapshot.ID, snapshot.Provenance, snapshot.CreatedBy, snapshot.CreatedByRole, snapshot.CreatedAt,
		[]byte(snapshot.EvaluationRequest), snapshot.AsOf, []byte(snapshot.SourceHealth), snapshot.Model.ID,
		snapshot.Model.Version, snapshot.Model.Kind, snapshot.Model.SourcePath, snapshot.Model.Checksum,
		[]byte(snapshot.Result), snapshot.EventSetHash, snapshot.EvaluationHash, snapshot.ResultSchemaVersion,
		nullJSON(snapshot.RetentionProfile), snapshot.IdempotencyActor, snapshot.IdempotencyKey,
		snapshot.IdempotencyRequestHash,
	).Scan(&insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		existingID, lookupErr := repository.snapshotIDByIdempotency(ctx, snapshot.IdempotencyActor, snapshot.IdempotencyKey)
		if lookupErr != nil {
			return eventcheckdomain.CheckSnapshot{}, false, lookupErr
		}
		existing, lookupErr := repository.Get(ctx, existingID)
		return existing, false, lookupErr
	}
	if err != nil {
		return eventcheckdomain.CheckSnapshot{}, false, err
	}
	for _, reference := range snapshot.EventReferences {
		_, err = executor.ExecContext(ctx, `INSERT INTO check_snapshot_event_refs
            (snapshot_id,event_id,event_type,event_version,occurred_at,producer,aggregate_type,aggregate_id,sequence,
             correlation_id,trace_id,payload_sha256,ordinal,disposition,adjustment_reason,source_available)
            VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::numeric,$10,$11,$12,$13,$14,$15,$16)`,
			snapshot.ID, reference.EventID, reference.EventType, reference.EventVersion, reference.OccurredAt,
			reference.Producer, reference.AggregateType, reference.AggregateID, strconv.FormatUint(reference.Sequence, 10),
			reference.CorrelationID, reference.TraceID, reference.PayloadSHA256, reference.Ordinal,
			reference.Disposition, reference.AdjustmentReason, reference.SourceAvailable)
		if err != nil {
			return eventcheckdomain.CheckSnapshot{}, false, err
		}
	}
	for _, relation := range snapshot.Relationships {
		_, err = executor.ExecContext(ctx, `INSERT INTO check_snapshot_relations
            (snapshot_id,ordinal,from_event_id,to_event_id,relation_type,source_field,source_model_id,source_rule_id)
            VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8)`, snapshot.ID, relation.Ordinal, relation.FromEventID,
			relation.ToEventID, relation.RelationType, relation.SourceField, relation.SourceModelID, relation.SourceRuleID)
		if err != nil {
			return eventcheckdomain.CheckSnapshot{}, false, err
		}
	}
	for ordinal, finding := range snapshot.Findings {
		_, err = executor.ExecContext(ctx, `INSERT INTO check_findings
            (id,snapshot_id,ordinal,rule_kind,rule_id,rule_version,rule_checksum,severity,code,expectation_state,
             evidence_references,recommended_query_template_id,created_at)
            VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13)`, finding.ID,
			snapshot.ID, ordinal, finding.RuleKind, finding.RuleID, finding.RuleVersion, finding.RuleChecksum,
			finding.Severity, finding.Code, finding.ExpectationState, []byte(finding.EvidenceReferences),
			finding.RecommendedQueryTemplateID, snapshot.CreatedAt)
		if err != nil {
			return eventcheckdomain.CheckSnapshot{}, false, err
		}
	}
	return snapshot, true, nil
}

func (repository *CheckSnapshotRepository) Get(ctx context.Context, id string) (eventcheckdomain.CheckSnapshot, error) {
	executor := executorFromContext(ctx, repository.db)
	var snapshot eventcheckdomain.CheckSnapshot
	var requestJSON, healthJSON, resultJSON, retentionJSON []byte
	err := executor.QueryRowContext(ctx, `SELECT id::text,provenance,created_by,created_by_role,created_at,evaluation_request,
        as_of,source_health,model_id,model_version,model_kind,model_source_path,model_checksum,result,event_set_hash,
        evaluation_hash,result_schema_version,retention_profile,idempotency_actor,idempotency_key,idempotency_request_hash
        FROM check_snapshots WHERE id=$1::uuid`, id).Scan(
		&snapshot.ID, &snapshot.Provenance, &snapshot.CreatedBy, &snapshot.CreatedByRole, &snapshot.CreatedAt,
		&requestJSON, &snapshot.AsOf, &healthJSON, &snapshot.Model.ID, &snapshot.Model.Version, &snapshot.Model.Kind,
		&snapshot.Model.SourcePath, &snapshot.Model.Checksum, &resultJSON, &snapshot.EventSetHash,
		&snapshot.EvaluationHash, &snapshot.ResultSchemaVersion, &retentionJSON, &snapshot.IdempotencyActor,
		&snapshot.IdempotencyKey, &snapshot.IdempotencyRequestHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return eventcheckdomain.CheckSnapshot{}, eventcheckdomain.ErrSnapshotNotFound
	}
	if err != nil {
		return eventcheckdomain.CheckSnapshot{}, err
	}
	snapshot.EvaluationRequest = append([]byte(nil), requestJSON...)
	snapshot.SourceHealth = append([]byte(nil), healthJSON...)
	snapshot.Result = append([]byte(nil), resultJSON...)
	snapshot.RetentionProfile = append([]byte(nil), retentionJSON...)
	if snapshot.EventReferences, err = repository.eventReferences(ctx, snapshot.ID); err != nil {
		return eventcheckdomain.CheckSnapshot{}, err
	}
	if snapshot.Relationships, err = repository.relationships(ctx, snapshot.ID); err != nil {
		return eventcheckdomain.CheckSnapshot{}, err
	}
	if snapshot.Findings, err = repository.findings(ctx, snapshot.ID); err != nil {
		return eventcheckdomain.CheckSnapshot{}, err
	}
	return snapshot, nil
}

func (repository *CheckSnapshotRepository) ListCheckSnapshotSummaries(ctx context.Context, filter listchecksnapshots.Filter) ([]listchecksnapshots.Summary, error) {
	query := `SELECT snapshot.id::text,snapshot.created_by,snapshot.created_by_role,snapshot.created_at,
		snapshot.evaluation_request,snapshot.as_of,snapshot.source_health->>'status',
		snapshot.model_id,snapshot.model_version,snapshot.model_kind,snapshot.result->>'check_status',
		(SELECT count(*) FROM check_snapshot_event_refs event_ref WHERE event_ref.snapshot_id=snapshot.id AND event_ref.disposition='INCLUDED'),
		(SELECT count(*) FROM check_findings finding WHERE finding.snapshot_id=snapshot.id),
		(SELECT count(*) FROM investigation_check_snapshots link WHERE link.snapshot_id=snapshot.id)
		FROM check_snapshots snapshot WHERE 1=1`
	args := make([]any, 0, 5)
	if filter.Identifier != "" {
		args = append(args, filter.Identifier)
		query += fmt.Sprintf(" AND snapshot.evaluation_request->'identifier'->>'value' ILIKE '%%' || $%d || '%%'", len(args))
	}
	if filter.CheckStatus != "" {
		args = append(args, filter.CheckStatus)
		query += fmt.Sprintf(" AND snapshot.result->>'check_status'=$%d", len(args))
	}
	if filter.Cursor != nil {
		args = append(args, filter.Cursor.CreatedAt, filter.Cursor.ID)
		query += fmt.Sprintf(" AND (snapshot.created_at,snapshot.id) < ($%d,$%d::uuid)", len(args)-1, len(args))
	}
	args = append(args, filter.PageSize)
	query += fmt.Sprintf(" ORDER BY snapshot.created_at DESC,snapshot.id DESC LIMIT $%d", len(args))

	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]listchecksnapshots.Summary, 0)
	for rows.Next() {
		var item listchecksnapshots.Summary
		var requestJSON []byte
		if err := rows.Scan(&item.ID, &item.CreatedBy, &item.CreatedByRole, &item.CreatedAt, &requestJSON, &item.AsOf,
			&item.SourceHealthStatus, &item.Model.ID, &item.Model.Version, &item.Model.Kind, &item.CheckStatus,
			&item.EventCount, &item.FindingCount, &item.LinkedCaseCount); err != nil {
			return nil, err
		}
		item.EvaluationRequest = append([]byte(nil), requestJSON...)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (repository *CheckSnapshotRepository) FindFeedback(ctx context.Context, findingID string) (eventcheckdomain.FindingFeedback, bool, error) {
	var feedback eventcheckdomain.FindingFeedback
	err := executorFromContext(ctx, repository.db).QueryRowContext(ctx, `SELECT finding_id::text,status,
		actor_id,actor_role,updated_at,lock_version FROM check_finding_feedback
		WHERE finding_id=$1::uuid`, findingID).Scan(&feedback.FindingID, &feedback.Status, &feedback.ActorID,
		&feedback.ActorRole, &feedback.UpdatedAt, &feedback.LockVersion)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		existsErr := executorFromContext(ctx, repository.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM check_findings WHERE id=$1::uuid)`, findingID).Scan(&exists)
		if existsErr != nil {
			return eventcheckdomain.FindingFeedback{}, false, existsErr
		}
		if exists {
			return eventcheckdomain.FindingFeedback{FindingID: findingID}, false, nil
		}
		return eventcheckdomain.FindingFeedback{}, false, eventcheckdomain.ErrFindingNotFound
	}
	if err != nil {
		return eventcheckdomain.FindingFeedback{}, false, err
	}
	return feedback, true, nil
}

func (repository *CheckSnapshotRepository) ListFeedback(ctx context.Context, snapshotID string) ([]eventcheckdomain.FindingFeedback, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT feedback.finding_id::text,feedback.status,
		feedback.actor_id,feedback.actor_role,feedback.updated_at,feedback.lock_version
		FROM check_finding_feedback feedback
		JOIN check_findings finding ON finding.id=feedback.finding_id
		WHERE finding.snapshot_id=$1::uuid ORDER BY finding.ordinal`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []eventcheckdomain.FindingFeedback{}
	for rows.Next() {
		var feedback eventcheckdomain.FindingFeedback
		if err := rows.Scan(&feedback.FindingID, &feedback.Status, &feedback.ActorID, &feedback.ActorRole, &feedback.UpdatedAt, &feedback.LockVersion); err != nil {
			return nil, err
		}
		result = append(result, feedback)
	}
	return result, rows.Err()
}

func (repository *CheckSnapshotRepository) SaveFeedback(ctx context.Context, feedback eventcheckdomain.FindingFeedback, expectedVersion int64) error {
	executor := executorFromContext(ctx, repository.db)
	if expectedVersion == 0 {
		result, err := executor.ExecContext(ctx, `INSERT INTO check_finding_feedback
            (finding_id,status,actor_id,actor_role,lock_version,updated_at) VALUES ($1::uuid,$2,$3,$4,$5,$6)
            ON CONFLICT (finding_id) DO NOTHING`, feedback.FindingID, feedback.Status, feedback.ActorID,
			feedback.ActorRole, feedback.LockVersion, feedback.UpdatedAt)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return eventcheckdomain.ErrFeedbackConflict
		}
		return nil
	}
	result, err := executor.ExecContext(ctx, `UPDATE check_finding_feedback SET status=$2,actor_id=$3,actor_role=$4,
        lock_version=$5,updated_at=$6 WHERE finding_id=$1::uuid AND lock_version=$7`, feedback.FindingID,
		feedback.Status, feedback.ActorID, feedback.ActorRole, feedback.LockVersion, feedback.UpdatedAt, expectedVersion)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return eventcheckdomain.ErrFeedbackConflict
	}
	return nil
}

func (repository *CheckSnapshotRepository) Attach(ctx context.Context, investigationID, snapshotID string, expectedVersion int64, actor eventcheckdomain.SnapshotActor, linkedAt time.Time) (eventcheckports.InvestigationSnapshotLink, bool, error) {
	executor := executorFromContext(ctx, repository.db)
	var currentVersion int64
	var status string
	err := executor.QueryRowContext(ctx, `SELECT lock_version,status FROM investigation_cases WHERE id=$1::uuid FOR UPDATE`, investigationID).Scan(&currentVersion, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return eventcheckports.InvestigationSnapshotLink{}, false, investigationdomain.ErrCaseNotFound
	}
	if err != nil {
		return eventcheckports.InvestigationSnapshotLink{}, false, err
	}
	if currentVersion != expectedVersion {
		return eventcheckports.InvestigationSnapshotLink{}, false, investigationdomain.ErrOptimisticConflict
	}
	if status == string(investigationdomain.StatusClosed) {
		return eventcheckports.InvestigationSnapshotLink{}, false, investigationdomain.ErrInvalidTransition
	}
	var snapshotExists bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM check_snapshots WHERE id=$1::uuid)`, snapshotID).Scan(&snapshotExists); err != nil {
		return eventcheckports.InvestigationSnapshotLink{}, false, err
	}
	if !snapshotExists {
		return eventcheckports.InvestigationSnapshotLink{}, false, eventcheckdomain.ErrSnapshotNotFound
	}
	result, err := executor.ExecContext(ctx, `INSERT INTO investigation_check_snapshots
        (investigation_case_id,snapshot_id,linked_by,linked_by_role,linked_at) VALUES ($1::uuid,$2::uuid,$3,$4,$5)
        ON CONFLICT (investigation_case_id,snapshot_id) DO NOTHING`, investigationID, snapshotID, actor.Subject, actor.Role, linkedAt)
	if err != nil {
		return eventcheckports.InvestigationSnapshotLink{}, false, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		link, loadErr := repository.link(ctx, investigationID, snapshotID)
		link.CaseLockVersion = currentVersion
		return link, false, loadErr
	}
	update, err := executor.ExecContext(ctx, `UPDATE investigation_cases SET lock_version=lock_version+1,
        last_updated_by=$2,updated_at=$3 WHERE id=$1::uuid AND lock_version=$4`, investigationID, actor.Subject, linkedAt, expectedVersion)
	if err != nil {
		return eventcheckports.InvestigationSnapshotLink{}, false, err
	}
	updatedRows, _ := update.RowsAffected()
	if updatedRows != 1 {
		return eventcheckports.InvestigationSnapshotLink{}, false, investigationdomain.ErrOptimisticConflict
	}
	return eventcheckports.InvestigationSnapshotLink{
		InvestigationID: investigationID, SnapshotID: snapshotID, LinkedBy: actor.Subject,
		LinkedByRole: actor.Role, LinkedAt: linkedAt, CaseLockVersion: currentVersion + 1,
	}, true, nil
}

func (repository *CheckSnapshotRepository) List(ctx context.Context, investigationID string) ([]eventcheckports.InvestigationSnapshotLink, error) {
	executor := executorFromContext(ctx, repository.db)
	var exists bool
	if err := executor.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM investigation_cases WHERE id=$1::uuid)`, investigationID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, investigationdomain.ErrCaseNotFound
	}
	rows, err := executor.QueryContext(ctx, `SELECT investigation_case_id::text,snapshot_id::text,linked_by,linked_by_role,linked_at
        FROM investigation_check_snapshots WHERE investigation_case_id=$1::uuid ORDER BY linked_at,snapshot_id`, investigationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]eventcheckports.InvestigationSnapshotLink, 0)
	for rows.Next() {
		var link eventcheckports.InvestigationSnapshotLink
		if err := rows.Scan(&link.InvestigationID, &link.SnapshotID, &link.LinkedBy, &link.LinkedByRole, &link.LinkedAt); err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, rows.Err()
}

func (repository *CheckSnapshotRepository) RecordEventCheckAudit(ctx context.Context, record eventcheckports.AuditRecord) error {
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return err
	}
	_, err = executorFromContext(ctx, repository.db).ExecContext(ctx, `INSERT INTO audit_logs
        (actor_id,actor_role,action,resource_type,resource_id,request_id,metadata,created_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`, record.Actor.Subject, record.Actor.Role, record.Action,
		record.ResourceType, record.ResourceID, record.RequestID, metadata, record.CreatedAt)
	return err
}

func (repository *CheckSnapshotRepository) snapshotIDByIdempotency(ctx context.Context, actor, key string) (string, error) {
	var id string
	err := executorFromContext(ctx, repository.db).QueryRowContext(ctx, `SELECT id::text FROM check_snapshots WHERE idempotency_actor=$1 AND idempotency_key=$2`, actor, key).Scan(&id)
	return id, err
}

func (repository *CheckSnapshotRepository) eventReferences(ctx context.Context, snapshotID string) ([]eventcheckdomain.SnapshotEventReference, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT event_id,event_type,event_version,occurred_at,
        producer,aggregate_type,aggregate_id,sequence::text,correlation_id,trace_id,payload_sha256,ordinal,disposition,
		adjustment_reason,source_available FROM check_snapshot_event_refs WHERE snapshot_id=$1::uuid
		ORDER BY CASE disposition WHEN 'INCLUDED' THEN 0 ELSE 1 END,ordinal`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]eventcheckdomain.SnapshotEventReference, 0)
	for rows.Next() {
		var item eventcheckdomain.SnapshotEventReference
		var sequence string
		if err := rows.Scan(&item.EventID, &item.EventType, &item.EventVersion, &item.OccurredAt, &item.Producer,
			&item.AggregateType, &item.AggregateID, &sequence, &item.CorrelationID, &item.TraceID, &item.PayloadSHA256,
			&item.Ordinal, &item.Disposition, &item.AdjustmentReason, &item.SourceAvailable); err != nil {
			return nil, err
		}
		item.Sequence, err = strconv.ParseUint(sequence, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("snapshot event sequence: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *CheckSnapshotRepository) relationships(ctx context.Context, snapshotID string) ([]eventcheckdomain.SnapshotRelationship, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT ordinal,from_event_id,to_event_id,relation_type,
        source_field,source_model_id,source_rule_id FROM check_snapshot_relations WHERE snapshot_id=$1::uuid ORDER BY ordinal`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]eventcheckdomain.SnapshotRelationship, 0)
	for rows.Next() {
		var item eventcheckdomain.SnapshotRelationship
		if err := rows.Scan(&item.Ordinal, &item.FromEventID, &item.ToEventID, &item.RelationType, &item.SourceField, &item.SourceModelID, &item.SourceRuleID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *CheckSnapshotRepository) findings(ctx context.Context, snapshotID string) ([]eventcheckdomain.SnapshotFinding, error) {
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, `SELECT id::text,rule_kind,rule_id,rule_version,
        rule_checksum,severity,code,expectation_state,evidence_references,recommended_query_template_id
        FROM check_findings WHERE snapshot_id=$1::uuid ORDER BY ordinal`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]eventcheckdomain.SnapshotFinding, 0)
	for rows.Next() {
		var item eventcheckdomain.SnapshotFinding
		var evidence []byte
		if err := rows.Scan(&item.ID, &item.RuleKind, &item.RuleID, &item.RuleVersion, &item.RuleChecksum, &item.Severity,
			&item.Code, &item.ExpectationState, &evidence, &item.RecommendedQueryTemplateID); err != nil {
			return nil, err
		}
		item.EvidenceReferences = append([]byte(nil), evidence...)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *CheckSnapshotRepository) link(ctx context.Context, investigationID, snapshotID string) (eventcheckports.InvestigationSnapshotLink, error) {
	var result eventcheckports.InvestigationSnapshotLink
	err := executorFromContext(ctx, repository.db).QueryRowContext(ctx, `SELECT investigation_case_id::text,snapshot_id::text,
        linked_by,linked_by_role,linked_at FROM investigation_check_snapshots WHERE investigation_case_id=$1::uuid AND snapshot_id=$2::uuid`,
		investigationID, snapshotID).Scan(&result.InvestigationID, &result.SnapshotID, &result.LinkedBy, &result.LinkedByRole, &result.LinkedAt)
	return result, err
}

func nullJSON(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return []byte(value)
}
