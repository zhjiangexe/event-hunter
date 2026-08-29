package compatibility

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

type AuditWriter interface {
	RecordAudit(ctx context.Context, actor ports.Actor, action, resourceID, requestID string, metadata map[string]any) error
}

type Command struct {
	InvestigationID string
	FindingID       string
	ExpectedVersion int64
	Status          domain.PatternFeedbackStatus
	Actor           ports.Actor
	RequestID       string
}

type PatternFeedbackService struct {
	repository ports.PatternFeedbackRepository
	audit      AuditWriter
	unit       ports.UnitOfWork
	now        func() time.Time
}

func NewPatternFeedbackService(repository ports.PatternFeedbackRepository, audit AuditWriter, unit ports.UnitOfWork) *PatternFeedbackService {
	return &PatternFeedbackService{repository: repository, audit: audit, unit: unit, now: time.Now}
}

func (service *PatternFeedbackService) Reclassify(ctx context.Context, command Command) (domain.PatternFindingFeedback, error) {
	var updated domain.PatternFindingFeedback
	err := service.unit.WithinTransaction(ctx, func(transactionContext context.Context) error {
		current, err := service.repository.FindPatternFeedback(transactionContext, command.InvestigationID, command.FindingID)
		if err != nil {
			return err
		}
		updated, err = current.Reclassify(command.ExpectedVersion, command.Status, command.Actor.Subject, command.Actor.Role, service.now())
		if err != nil {
			return err
		}
		if err := service.repository.SavePatternFeedback(transactionContext, updated, command.ExpectedVersion); err != nil {
			return err
		}
		return service.audit.RecordAudit(transactionContext, command.Actor, "CLASSIFY_PATTERN_FINDING", command.InvestigationID, command.RequestID, map[string]any{
			"finding_id": updated.FindingID, "status": string(updated.Status), "feedback_lock_version": updated.LockVersion,
		})
	})
	return updated, err
}
