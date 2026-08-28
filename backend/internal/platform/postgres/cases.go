package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

type CaseRepository struct {
	db *sql.DB
}

func NewCaseRepository(db *sql.DB) *CaseRepository {
	return &CaseRepository{db: db}
}

func (repository *CaseRepository) Create(ctx context.Context, investigationCase domain.InvestigationCase) (domain.InvestigationCase, error) {
	const query = `
INSERT INTO investigation_cases
	(id, case_no, title, severity, status, correlation_id, assignee, priority, tags,
	 related_correlation_ids, last_updated_by, root_cause, resolution_summary, fixed_version,
	 notes, workflow_id, lock_version, created_at, updated_at, closed_at,
	 incident_from, incident_to, incident_window_source)
VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
        COALESCE(NULLIF($2, ''), 'MANUAL-' || upper(substr(replace(gen_random_uuid()::text,'-',''),1,20))),
		$3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
		$21, $22, $23)
RETURNING id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
          to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
          notes, workflow_id, lock_version, created_at, updated_at, closed_at`

	return scanCase(executorFromContext(ctx, repository.db).QueryRowContext(ctx, query,
		investigationCase.ID, investigationCase.CaseNo, investigationCase.Title,
		investigationCase.Severity, investigationCase.Status, investigationCase.CorrelationID,
		investigationCase.Assignee, investigationCase.Priority, investigationCase.Tags,
		investigationCase.RelatedCorrelationIDs, investigationCase.LastUpdatedBy,
		investigationCase.RootCause, investigationCase.ResolutionSummary, investigationCase.FixedVersion,
		investigationCase.Notes, investigationCase.WorkflowID, investigationCase.LockVersion,
		investigationCase.CreatedAt, investigationCase.UpdatedAt,
		investigationCase.ClosedAt, investigationCase.IncidentWindow.From,
		investigationCase.IncidentWindow.To, investigationCase.IncidentWindow.Source,
	))
}

func (repository *CaseRepository) Get(ctx context.Context, id string) (domain.InvestigationCase, error) {
	const query = `
SELECT id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
       to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
       notes, workflow_id, lock_version, created_at, updated_at, closed_at
FROM investigation_cases WHERE id = $1::uuid`
	result, err := scanCase(executorFromContext(ctx, repository.db).QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.InvestigationCase{}, domain.ErrCaseNotFound
	}
	return result, err
}

func (repository *CaseRepository) Update(ctx context.Context, investigationCase domain.InvestigationCase) (domain.InvestigationCase, error) {
	const query = `
UPDATE investigation_cases
SET title = $2, severity = $3, status = $4, correlation_id = $5, assignee = $6,
    priority = $7, tags = $8, related_correlation_ids = $9, last_updated_by = $10,
    root_cause = $11, resolution_summary = $12, fixed_version = $13, notes = $14,
    workflow_id = $15, lock_version = lock_version + 1, updated_at = $16, closed_at = $17
WHERE id = $1::uuid AND lock_version = $18
RETURNING id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
          to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
          notes, workflow_id, lock_version, created_at, updated_at, closed_at`

	updated, err := scanCase(executorFromContext(ctx, repository.db).QueryRowContext(ctx, query,
		investigationCase.ID, investigationCase.Title, investigationCase.Severity,
		investigationCase.Status, investigationCase.CorrelationID, investigationCase.Assignee,
		investigationCase.Priority, investigationCase.Tags, investigationCase.RelatedCorrelationIDs,
		investigationCase.LastUpdatedBy, investigationCase.RootCause, investigationCase.ResolutionSummary,
		investigationCase.FixedVersion, investigationCase.Notes, investigationCase.WorkflowID,
		investigationCase.UpdatedAt, investigationCase.ClosedAt, investigationCase.LockVersion,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.InvestigationCase{}, domain.ErrOptimisticConflict
	}
	return updated, err
}

func (repository *CaseRepository) AppendNote(ctx context.Context, id string, expectedVersion int64, note domain.CaseNote, lastUpdatedBy string, updatedAt time.Time) (domain.InvestigationCase, error) {
	var updated domain.InvestigationCase
	err := withinTransaction(ctx, repository.db, func(transactionContext context.Context) error {
		executor := executorFromContext(transactionContext, repository.db)
		const updateQuery = `
UPDATE investigation_cases
SET lock_version = lock_version + 1, last_updated_by = $3, updated_at = $4
WHERE id = $1::uuid AND lock_version = $2 AND status <> 'CLOSED'
RETURNING id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
          to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
          notes, workflow_id, lock_version, created_at, updated_at, closed_at`
		var err error
		updated, err = scanCase(executor.QueryRowContext(transactionContext, updateQuery, id, expectedVersion, lastUpdatedBy, updatedAt))
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrOptimisticConflict
		}
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(transactionContext, `INSERT INTO case_notes (id,investigation_case_id,body,author_id,author_role,created_at) VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6)`, note.ID, id, note.Body, note.AuthorID, note.AuthorRole, note.CreatedAt)
		if err != nil {
			return err
		}
		return nil
	})
	return updated, err
}

func (repository *CaseRepository) AppendEvidence(ctx context.Context, investigationCase domain.InvestigationCase, expectedVersion int64, evidence domain.CaseEvidence) (domain.InvestigationCase, domain.CaseEvidence, bool, error) {
	var updated domain.InvestigationCase
	var persistedEvidence domain.CaseEvidence
	var attached bool
	err := withinTransaction(ctx, repository.db, func(transactionContext context.Context) error {
		executor := executorFromContext(transactionContext, repository.db)
		const lockQuery = `
SELECT id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
       to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
       notes, workflow_id, lock_version, created_at, updated_at, closed_at
FROM investigation_cases WHERE id = $1::uuid FOR UPDATE`
		current, err := scanCase(executor.QueryRowContext(transactionContext, lockQuery, investigationCase.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrCaseNotFound
		}
		if err != nil {
			return err
		}
		if current.LockVersion != expectedVersion {
			return domain.ErrOptimisticConflict
		}
		if current.Status == domain.StatusClosed {
			return domain.ErrInvalidTransition
		}

		existing, found, err := findCaseEvidence(transactionContext, executor, investigationCase.ID, evidence.EvidenceType, evidence.Reference)
		if err != nil {
			return err
		}
		if found {
			updated, persistedEvidence, attached = current, existing, false
			return nil
		}

		const insertQuery = `INSERT INTO case_evidence (id,investigation_case_id,evidence_type,reference,checksum,collected_at)
VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6)
ON CONFLICT (investigation_case_id,evidence_type,reference) DO NOTHING
RETURNING id::text,evidence_type,reference,checksum,collected_at`
		inserted := domain.CaseEvidence{}
		err = executor.QueryRowContext(transactionContext, insertQuery, evidence.ID, investigationCase.ID, evidence.EvidenceType, evidence.Reference, evidence.Checksum, evidence.CollectedAt).Scan(
			&inserted.ID, &inserted.EvidenceType, &inserted.Reference, &inserted.Checksum, &inserted.CollectedAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			existing, found, err = findCaseEvidence(transactionContext, executor, investigationCase.ID, evidence.EvidenceType, evidence.Reference)
			if err != nil {
				return err
			}
			if !found {
				return errors.New("evidence conflict row could not be loaded")
			}
			updated, persistedEvidence, attached = current, existing, false
			return nil
		}
		if err != nil {
			return err
		}

		const updateQuery = `
UPDATE investigation_cases
SET related_correlation_ids = $2, last_updated_by = $3, updated_at = $4, lock_version = lock_version + 1
WHERE id = $1::uuid
RETURNING id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
          to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
          notes, workflow_id, lock_version, created_at, updated_at, closed_at`
		updated, err = scanCase(executor.QueryRowContext(transactionContext, updateQuery,
			investigationCase.ID, investigationCase.RelatedCorrelationIDs, investigationCase.LastUpdatedBy, investigationCase.UpdatedAt,
		))
		if err != nil {
			return err
		}
		persistedEvidence, attached = inserted, true
		return nil
	})
	return updated, persistedEvidence, attached, err
}

type sqlQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func findCaseEvidence(ctx context.Context, queryer sqlQueryRower, investigationID, evidenceType, reference string) (domain.CaseEvidence, bool, error) {
	var result domain.CaseEvidence
	err := queryer.QueryRowContext(ctx, `SELECT id::text,evidence_type,reference,checksum,collected_at FROM case_evidence WHERE investigation_case_id=$1::uuid AND evidence_type=$2 AND reference=$3`, investigationID, evidenceType, reference).Scan(
		&result.ID, &result.EvidenceType, &result.Reference, &result.Checksum, &result.CollectedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CaseEvidence{}, false, nil
	}
	return result, err == nil, err
}

func (repository *CaseRepository) List(ctx context.Context, filter domain.CaseFilter) (domain.CasePage, error) {
	sortColumn := "created_at"
	if filter.SortBy == "updated_at" {
		sortColumn = "updated_at"
	}
	sortDirection := "DESC"
	comparison := "<"
	if filter.SortOrder == "asc" {
		sortDirection = "ASC"
		comparison = ">"
	}
	query := `SELECT id::text, case_no, title, severity, status, correlation_id, incident_from, incident_to, incident_window_source, assignee, priority, to_json(tags),
       to_json(related_correlation_ids), last_updated_by, root_cause, resolution_summary, fixed_version,
       notes, workflow_id, lock_version, created_at, updated_at, closed_at
FROM investigation_cases WHERE 1=1`
	args := make([]any, 0, 6)
	if value := strings.TrimSpace(filter.Query); value != "" {
		args = append(args, value)
		query += fmt.Sprintf(" AND (case_no ILIKE '%%' || $%d || '%%' OR title ILIKE '%%' || $%d || '%%')", len(args), len(args))
	}
	for _, candidate := range []struct {
		column string
		value  string
	}{
		{column: "status", value: strings.TrimSpace(filter.Status)},
		{column: "severity", value: strings.TrimSpace(filter.Severity)},
		{column: "assignee", value: strings.TrimSpace(filter.Assignee)},
		{column: "priority", value: strings.TrimSpace(filter.Priority)},
		{column: "correlation_id", value: strings.TrimSpace(filter.CorrelationID)},
	} {
		if candidate.value != "" {
			args = append(args, candidate.value)
			query += fmt.Sprintf(" AND %s=$%d", candidate.column, len(args))
		}
	}
	if tag := strings.ToLower(strings.TrimSpace(filter.Tag)); tag != "" {
		args = append(args, tag)
		query += fmt.Sprintf(" AND tags @> ARRAY[$%d]::text[]", len(args))
	}
	if filter.BeforeTime != nil && filter.BeforeID != "" {
		args = append(args, *filter.BeforeTime, filter.BeforeID)
		query += fmt.Sprintf(" AND (%s,id) %s ($%d,$%d::uuid)", sortColumn, comparison, len(args)-1, len(args))
	}
	query += fmt.Sprintf(" ORDER BY %s %s,id %s LIMIT %d", sortColumn, sortDirection, sortDirection, filter.PageSize+1)
	rows, err := executorFromContext(ctx, repository.db).QueryContext(ctx, query, args...)
	if err != nil {
		return domain.CasePage{}, err
	}
	defer rows.Close()
	items := make([]domain.InvestigationCase, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanCase(rows)
		if err != nil {
			return domain.CasePage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.CasePage{}, err
	}
	hasMore := len(items) > filter.PageSize
	if hasMore {
		items = items[:filter.PageSize]
	}
	return domain.CasePage{Items: items, HasMore: hasMore}, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCase(row rowScanner) (domain.InvestigationCase, error) {
	var result domain.InvestigationCase
	var tagsJSON, relatedJSON []byte
	err := row.Scan(
		&result.ID, &result.CaseNo, &result.Title, &result.Severity, &result.Status,
		&result.CorrelationID, &result.IncidentWindow.From, &result.IncidentWindow.To, &result.IncidentWindow.Source,
		&result.Assignee, &result.Priority, &tagsJSON,
		&relatedJSON, &result.LastUpdatedBy, &result.RootCause, &result.ResolutionSummary,
		&result.FixedVersion, &result.Notes, &result.WorkflowID, &result.LockVersion,
		&result.CreatedAt, &result.UpdatedAt, &result.ClosedAt,
	)
	if err != nil {
		return domain.InvestigationCase{}, fmt.Errorf("scan investigation case: %w", err)
	}
	if err := json.Unmarshal(tagsJSON, &result.Tags); err != nil {
		return domain.InvestigationCase{}, fmt.Errorf("scan investigation case tags: %w", err)
	}
	if err := json.Unmarshal(relatedJSON, &result.RelatedCorrelationIDs); err != nil {
		return domain.InvestigationCase{}, fmt.Errorf("scan related correlation ids: %w", err)
	}
	return result, nil
}
