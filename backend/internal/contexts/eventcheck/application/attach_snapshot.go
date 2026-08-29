package application

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type SnapshotAttachmentHandler struct {
	repository ports.InvestigationSnapshotRepository
	audit      ports.AuditWriter
	unitOfWork ports.UnitOfWork
	now        func() time.Time
}

func NewSnapshotAttachmentHandler(repository ports.InvestigationSnapshotRepository, audit ports.AuditWriter, unitOfWork ports.UnitOfWork) *SnapshotAttachmentHandler {
	return &SnapshotAttachmentHandler{repository: repository, audit: audit, unitOfWork: unitOfWork, now: time.Now}
}

func (service *SnapshotAttachmentHandler) Attach(ctx context.Context, investigationID, snapshotID string, expectedVersion int64, actor domain.SnapshotActor, requestID string) (ports.InvestigationSnapshotLink, bool, error) {
	now := service.now().UTC()
	var link ports.InvestigationSnapshotLink
	var attached bool
	err := service.unitOfWork.WithinTransaction(ctx, func(transactionContext context.Context) error {
		var err error
		link, attached, err = service.repository.Attach(transactionContext, investigationID, snapshotID, expectedVersion, actor, now)
		if err != nil || !attached {
			return err
		}
		return service.audit.RecordEventCheckAudit(transactionContext, ports.AuditRecord{
			Actor: actor, Action: "ATTACH_CHECK_SNAPSHOT", ResourceType: "INVESTIGATION_CASE", ResourceID: investigationID,
			RequestID: requestID, Metadata: map[string]any{"snapshot_id": snapshotID, "lock_version": link.CaseLockVersion}, CreatedAt: now,
		})
	})
	return link, attached, err
}

func (service *SnapshotAttachmentHandler) List(ctx context.Context, investigationID string) ([]ports.InvestigationSnapshotLink, error) {
	return service.repository.List(ctx, investigationID)
}
