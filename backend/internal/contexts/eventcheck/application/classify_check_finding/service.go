package classify_check_finding

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type Service struct {
	repository ports.SnapshotRepository
	audit      ports.AuditWriter
	unitOfWork ports.UnitOfWork
	now        func() time.Time
}

func NewService(repository ports.SnapshotRepository, audit ports.AuditWriter, unitOfWork ports.UnitOfWork) *Service {
	return &Service{repository: repository, audit: audit, unitOfWork: unitOfWork, now: time.Now}
}

func (service *Service) Classify(ctx context.Context, findingID string, status domain.FindingFeedbackStatus, expectedVersion int64, actor domain.SnapshotActor, requestID string) (domain.FindingFeedback, error) {
	now := service.now().UTC()
	var result domain.FindingFeedback
	err := service.unitOfWork.WithinTransaction(ctx, func(transactionContext context.Context) error {
		current, found, err := service.repository.FindFeedback(transactionContext, findingID)
		if err != nil {
			return err
		}
		if !found {
			if expectedVersion != 0 {
				return domain.ErrFeedbackConflict
			}
			result, err = domain.NewFindingFeedback(findingID, status, actor, now)
		} else {
			result = current
			err = result.Reclassify(status, actor, expectedVersion, now)
		}
		if err != nil {
			return err
		}
		if err := service.repository.SaveFeedback(transactionContext, result, expectedVersion); err != nil {
			return err
		}
		return service.audit.RecordEventCheckAudit(transactionContext, ports.AuditRecord{
			Actor: actor, Action: "CLASSIFY_CHECK_FINDING", ResourceType: "CHECK_FINDING", ResourceID: findingID,
			RequestID: requestID, Metadata: map[string]any{"status": status, "lock_version": result.LockVersion}, CreatedAt: now,
		})
	})
	return result, err
}
