package caselifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
	"github.com/google/uuid"
)

var (
	ErrInvalidTransition = domain.ErrInvalidTransition
	ErrCloseRequired     = domain.ErrCloseRequired
	ErrResolutionFields  = domain.ErrResolutionFields
)

type Actor = ports.Actor
type AuditEntry = ports.AuditEntry
type CaseRepository = domain.CaseRepository
type CaseFilter = domain.CaseFilter
type CasePage = domain.CasePage
type InvestigationDetailsRepository = ports.InvestigationDetailsRepository
type PatternFinding = ports.PatternFinding
type Evidence = ports.Evidence

type CasePatch struct {
	Title                 *string
	Status                *domain.CaseStatus
	Severity              *domain.Severity
	Assignee              *string
	Priority              *domain.CasePriority
	Tags                  *[]string
	RelatedCorrelationIDs *[]string
	RootCause             *string
	ResolutionSummary     *string
	FixedVersion          *string
}

type VersionConflictError struct {
	CurrentVersion int64
}

func (err VersionConflictError) Error() string { return domain.ErrOptimisticConflict.Error() }
func (err VersionConflictError) Unwrap() error { return domain.ErrOptimisticConflict }

type Service struct {
	cases      CaseRepository
	details    InvestigationDetailsRepository
	unitOfWork ports.UnitOfWork
	now        func() time.Time
}

func NewService(cases CaseRepository, details InvestigationDetailsRepository, unitsOfWork ...ports.UnitOfWork) *Service {
	var unitOfWork ports.UnitOfWork
	if len(unitsOfWork) > 0 {
		unitOfWork = unitsOfWork[0]
	}
	return &Service{cases: cases, details: details, unitOfWork: unitOfWork, now: time.Now}
}

func (service *Service) List(ctx context.Context, filter CaseFilter) (CasePage, error) {
	return service.cases.List(ctx, filter)
}

func (service *Service) Create(ctx context.Context, title string, severity domain.Severity, correlationID string, incidentWindow domain.IncidentWindow, actor Actor, requestID string) (domain.InvestigationCase, error) {
	now := service.now().UTC()
	newCase, err := domain.NewInvestigationCase(title, severity, correlationID, incidentWindow, actor.Subject, now)
	if err != nil {
		return domain.InvestigationCase{}, err
	}
	var result domain.InvestigationCase
	err = service.withinTransaction(ctx, func(transactionContext context.Context) error {
		var err error
		result, err = service.cases.Create(transactionContext, newCase)
		if err != nil {
			return err
		}
		if err := service.details.RecordAudit(transactionContext, actor, "CREATE_INVESTIGATION", result.ID, requestID, map[string]any{
			"severity": string(result.Severity), "correlation_id": result.CorrelationID,
			"incident_from": result.IncidentWindow.From, "incident_to": result.IncidentWindow.To,
			"incident_window_source": string(result.IncidentWindow.Source),
		}); err != nil {
			return fmt.Errorf("record create investigation audit: %w", err)
		}
		return nil
	})
	return result, err
}

func (service *Service) Get(ctx context.Context, id string) (domain.InvestigationCase, error) {
	return service.cases.Get(ctx, id)
}

type CaseDetails struct {
	Case     domain.InvestigationCase
	Findings []PatternFinding
	Evidence []Evidence
	Notes    []domain.CaseNote
}

type CaseSummaryDetails struct {
	CaseDetails
	Audit []AuditEntry
}

func (service *Service) GetDetails(ctx context.Context, id string) (CaseDetails, error) {
	loadedCase, err := service.cases.Get(ctx, id)
	if err != nil {
		return CaseDetails{}, err
	}
	findings, err := service.details.Findings(ctx, id)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load investigation findings: %w", err)
	}
	evidence, err := service.details.Evidence(ctx, id)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load investigation evidence: %w", err)
	}
	notes, err := service.details.Notes(ctx, id)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load investigation notes: %w", err)
	}
	return CaseDetails{Case: loadedCase, Findings: findings, Evidence: evidence, Notes: notes}, nil
}

func (service *Service) GetSummaryDetails(ctx context.Context, id string) (CaseSummaryDetails, error) {
	details, err := service.GetDetails(ctx, id)
	if err != nil {
		return CaseSummaryDetails{}, err
	}
	audit, err := service.details.Audit(ctx, id)
	if err != nil {
		return CaseSummaryDetails{}, fmt.Errorf("load investigation audit: %w", err)
	}
	return CaseSummaryDetails{CaseDetails: details, Audit: audit}, nil
}

func (service *Service) Update(ctx context.Context, id string, expectedVersion int64, patch CasePatch, actor Actor, requestID string) (domain.InvestigationCase, error) {
	var updated domain.InvestigationCase
	err := service.withinTransaction(ctx, func(transactionContext context.Context) error {
		current, err := service.cases.Get(transactionContext, id)
		if err != nil {
			return err
		}
		fromStatus := current.Status
		if patch.Status != nil && *patch.Status == domain.StatusClosed {
			return ErrCloseRequired
		}
		if patch.Status != nil {
			if err := current.TransitionTo(*patch.Status, patch.RootCause, patch.ResolutionSummary); err != nil {
				return err
			}
		}
		if err := applyPatch(&current, patch); err != nil {
			return err
		}
		current.LockVersion = expectedVersion
		current.LastUpdatedBy = actor.Subject
		current.UpdatedAt = service.now().UTC()
		updated, err = service.cases.Update(transactionContext, current)
		if errors.Is(err, domain.ErrOptimisticConflict) {
			latest, latestErr := service.cases.Get(transactionContext, id)
			if latestErr != nil {
				return err
			}
			return VersionConflictError{CurrentVersion: latest.LockVersion}
		}
		if err != nil {
			return err
		}
		metadata := map[string]any{"from_status": string(fromStatus), "lock_version": updated.LockVersion}
		if patch.Status != nil {
			metadata["to_status"] = string(*patch.Status)
		}
		if err := service.details.RecordAudit(transactionContext, actor, "UPDATE_INVESTIGATION", updated.ID, requestID, metadata); err != nil {
			return fmt.Errorf("record update investigation audit: %w", err)
		}
		return nil
	})
	return updated, err
}

func (service *Service) Close(ctx context.Context, id string, expectedVersion int64, rootCause, resolutionSummary string, fixedVersion *string, actor Actor, requestID string) (domain.InvestigationCase, error) {
	var updated domain.InvestigationCase
	err := service.withinTransaction(ctx, func(transactionContext context.Context) error {
		current, err := service.cases.Get(transactionContext, id)
		if err != nil {
			return err
		}
		if current.Status == domain.StatusClosed {
			return VersionConflictError{CurrentVersion: current.LockVersion}
		}
		now := service.now().UTC()
		if err := current.Close(now, rootCause, resolutionSummary, fixedVersion); err != nil {
			return err
		}
		current.LockVersion = expectedVersion
		current.LastUpdatedBy = actor.Subject
		updated, err = service.cases.Update(transactionContext, current)
		if errors.Is(err, domain.ErrOptimisticConflict) {
			latest, latestErr := service.cases.Get(transactionContext, id)
			if errors.Is(latestErr, domain.ErrCaseNotFound) {
				return domain.ErrCaseNotFound
			}
			if latestErr != nil {
				return err
			}
			return VersionConflictError{CurrentVersion: latest.LockVersion}
		}
		if err != nil {
			return err
		}
		if err := service.details.RecordAudit(transactionContext, actor, "CLOSE_INVESTIGATION", updated.ID, requestID, map[string]any{"lock_version": updated.LockVersion}); err != nil {
			return fmt.Errorf("record close investigation audit: %w", err)
		}
		return nil
	})
	return updated, err
}

func (service *Service) Audit(ctx context.Context, id string) ([]AuditEntry, error) {
	return service.details.Audit(ctx, id)
}

func (service *Service) Findings(ctx context.Context, id string) ([]PatternFinding, error) {
	return service.details.Findings(ctx, id)
}

func (service *Service) Evidence(ctx context.Context, id string) ([]Evidence, error) {
	return service.details.Evidence(ctx, id)
}

func (service *Service) Notes(ctx context.Context, id string) ([]domain.CaseNote, error) {
	return service.details.Notes(ctx, id)
}

func (service *Service) AddNote(ctx context.Context, id string, expectedVersion int64, body string, actor Actor, requestID string) (domain.InvestigationCase, domain.CaseNote, error) {
	var updated domain.InvestigationCase
	var note domain.CaseNote
	err := service.withinTransaction(ctx, func(transactionContext context.Context) error {
		current, err := service.cases.Get(transactionContext, id)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		note, err = current.WriteNote(uuid.NewString(), body, actor.Subject, actor.Role, now)
		if err != nil {
			return err
		}
		updated, err = service.cases.AppendNote(transactionContext, id, expectedVersion, note, actor.Subject, now)
		if errors.Is(err, domain.ErrOptimisticConflict) {
			latest, latestErr := service.cases.Get(transactionContext, id)
			if latestErr != nil {
				return err
			}
			return VersionConflictError{CurrentVersion: latest.LockVersion}
		}
		if err != nil {
			return err
		}
		if err := service.details.RecordAudit(transactionContext, actor, "ADD_INVESTIGATION_NOTE", updated.ID, requestID, map[string]any{"note_id": note.ID, "lock_version": updated.LockVersion}); err != nil {
			return fmt.Errorf("record add investigation note audit: %w", err)
		}
		return nil
	})
	return updated, note, err
}

func (service *Service) SaveFinding(ctx context.Context, id string, finding PatternFinding) error {
	return service.details.SaveFinding(ctx, id, finding)
}

func (service *Service) SaveEvidence(ctx context.Context, id, evidenceType, reference, checksum string) error {
	return service.details.SaveEvidence(ctx, id, evidenceType, reference, checksum)
}

func (service *Service) RecordAudit(ctx context.Context, actor Actor, action, resourceID, requestID string, metadata map[string]any) error {
	return service.details.RecordAudit(ctx, actor, action, resourceID, requestID, metadata)
}

func (service *Service) withinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if service.unitOfWork == nil {
		return operation(ctx)
	}
	return service.unitOfWork.WithinTransaction(ctx, operation)
}

func applyPatch(current *domain.InvestigationCase, patch CasePatch) error {
	if patch.Title != nil {
		if err := current.ChangeTitle(*patch.Title); err != nil {
			return err
		}
	}
	if patch.Severity != nil {
		if err := current.ChangeSeverity(*patch.Severity); err != nil {
			return err
		}
	}
	if patch.Assignee != nil {
		if err := current.Assign(patch.Assignee); err != nil {
			return err
		}
	}
	if patch.Priority != nil {
		if err := current.ChangePriority(*patch.Priority); err != nil {
			return err
		}
	}
	if patch.Tags != nil {
		if err := current.ReplaceTags(*patch.Tags); err != nil {
			return err
		}
	}
	if patch.RelatedCorrelationIDs != nil {
		if err := current.ReplaceRelatedCorrelationIDs(*patch.RelatedCorrelationIDs); err != nil {
			return err
		}
	}
	if patch.RootCause != nil {
		if err := current.SetRootCause(patch.RootCause); err != nil {
			return err
		}
	}
	if patch.ResolutionSummary != nil {
		if err := current.SetResolutionSummary(patch.ResolutionSummary); err != nil {
			return err
		}
	}
	if patch.FixedVersion != nil {
		if err := current.SetFixedVersion(patch.FixedVersion); err != nil {
			return err
		}
	}
	return nil
}
