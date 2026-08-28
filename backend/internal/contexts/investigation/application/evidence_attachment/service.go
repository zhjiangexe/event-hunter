package evidenceattachment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
	"github.com/google/uuid"
)

var (
	ErrInvalidAttachment = errors.New("invalid event evidence attachment")
	ErrEventNotFound     = errors.New("event evidence source not found")
)

type EventLookup interface {
	Search(ctx context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error)
}

type AuditWriter interface {
	RecordAudit(ctx context.Context, actor ports.Actor, action, resourceID, requestID string, metadata map[string]any) error
}

type AttachEventCommand struct {
	InvestigationID string
	ExpectedVersion int64
	EventID         string
	From            time.Time
	To              time.Time
	Actor           ports.Actor
	RequestID       string
}

type AttachEventResult struct {
	Investigation domain.InvestigationCase
	Evidence      domain.CaseEvidence
	Attached      bool
}

type VersionConflictError struct {
	CurrentVersion int64
}

func (err VersionConflictError) Error() string { return domain.ErrOptimisticConflict.Error() }
func (err VersionConflictError) Unwrap() error { return domain.ErrOptimisticConflict }

type SourceError struct{ Err error }

func (err SourceError) Error() string { return fmt.Sprintf("event evidence source: %v", err.Err) }
func (err SourceError) Unwrap() error { return err.Err }

type Service struct {
	cases      domain.CaseEvidenceRepository
	events     EventLookup
	audit      AuditWriter
	unitOfWork ports.UnitOfWork
	now        func() time.Time
}

func NewService(cases domain.CaseEvidenceRepository, events EventLookup, audit AuditWriter, unitsOfWork ...ports.UnitOfWork) *Service {
	var unitOfWork ports.UnitOfWork
	if len(unitsOfWork) > 0 {
		unitOfWork = unitsOfWork[0]
	}
	return &Service{cases: cases, events: events, audit: audit, unitOfWork: unitOfWork, now: time.Now}
}

func (service *Service) AttachEvent(ctx context.Context, command AttachEventCommand) (AttachEventResult, error) {
	command.InvestigationID = strings.TrimSpace(command.InvestigationID)
	command.EventID = strings.TrimSpace(command.EventID)
	if command.InvestigationID == "" || command.EventID == "" || len(command.EventID) > 200 || !command.To.After(command.From) || command.To.Sub(command.From) > 7*24*time.Hour {
		return AttachEventResult{}, ErrInvalidAttachment
	}
	current, err := service.cases.Get(ctx, command.InvestigationID)
	if err != nil {
		return AttachEventResult{}, err
	}
	if current.LockVersion != command.ExpectedVersion {
		return AttachEventResult{}, VersionConflictError{CurrentVersion: current.LockVersion}
	}
	events, err := service.events.Search(ctx, forensics.EventSearchFilter{
		From: command.From, To: command.To, Limit: 2, EventID: command.EventID,
	})
	if err != nil {
		return AttachEventResult{}, SourceError{Err: err}
	}
	var source *forensics.ForensicsEvent
	for index := range events {
		if events[index].EventID != command.EventID {
			continue
		}
		if source != nil {
			return AttachEventResult{}, ErrInvalidAttachment
		}
		source = &events[index]
	}
	if source == nil {
		return AttachEventResult{}, ErrEventNotFound
	}
	now := service.now().UTC()
	checksum := sha256.Sum256([]byte("EVENT:" + source.EventID))
	evidence, err := current.AttachEvent(uuid.NewString(), source.EventID, source.CorrelationID, fmt.Sprintf("%x", checksum[:]), now)
	if err != nil {
		return AttachEventResult{}, err
	}
	current.LastUpdatedBy = command.Actor.Subject
	current.UpdatedAt = now
	var updated domain.InvestigationCase
	var persistedEvidence domain.CaseEvidence
	var attached bool
	err = service.withinTransaction(ctx, func(transactionContext context.Context) error {
		var persistenceErr error
		updated, persistedEvidence, attached, persistenceErr = service.cases.AppendEvidence(transactionContext, current, command.ExpectedVersion, evidence)
		if errors.Is(persistenceErr, domain.ErrOptimisticConflict) {
			latest, latestErr := service.cases.Get(transactionContext, command.InvestigationID)
			if latestErr != nil {
				return persistenceErr
			}
			return VersionConflictError{CurrentVersion: latest.LockVersion}
		}
		if persistenceErr != nil {
			return persistenceErr
		}
		if attached {
			if err := service.audit.RecordAudit(transactionContext, command.Actor, "ATTACH_INVESTIGATION_EVENT", updated.ID, command.RequestID, map[string]any{
				"event_id": source.EventID, "event_correlation_id": source.CorrelationID,
				"evidence_id": persistedEvidence.ID, "lock_version": updated.LockVersion,
			}); err != nil {
				return fmt.Errorf("record attach investigation event audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return AttachEventResult{}, err
	}
	return AttachEventResult{Investigation: updated, Evidence: persistedEvidence, Attached: attached}, nil
}

func (service *Service) withinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if service.unitOfWork == nil {
		return operation(ctx)
	}
	return service.unitOfWork.WithinTransaction(ctx, operation)
}
