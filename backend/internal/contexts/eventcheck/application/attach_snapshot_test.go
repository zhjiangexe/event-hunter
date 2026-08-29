package application

import (
	"context"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type linkRepository struct {
	link     ports.InvestigationSnapshotLink
	attached bool
	audits   []ports.AuditRecord
}

func (repository *linkRepository) Attach(_ context.Context, investigationID, snapshotID string, expectedVersion int64, actor domain.SnapshotActor, linkedAt time.Time) (ports.InvestigationSnapshotLink, bool, error) {
	if repository.link.SnapshotID != "" {
		return repository.link, false, nil
	}
	repository.link = ports.InvestigationSnapshotLink{InvestigationID: investigationID, SnapshotID: snapshotID, LinkedBy: actor.Subject, LinkedByRole: actor.Role, LinkedAt: linkedAt, CaseLockVersion: expectedVersion + 1}
	repository.attached = true
	return repository.link, true, nil
}
func (repository *linkRepository) List(context.Context, string) ([]ports.InvestigationSnapshotLink, error) {
	if repository.link.SnapshotID == "" {
		return []ports.InvestigationSnapshotLink{}, nil
	}
	return []ports.InvestigationSnapshotLink{repository.link}, nil
}
func (repository *linkRepository) RecordEventCheckAudit(_ context.Context, record ports.AuditRecord) error {
	repository.audits = append(repository.audits, record)
	return nil
}

type attachmentUnitOfWork struct{}

func (attachmentUnitOfWork) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func TestAttachAuditsOnlyTheFirstLink(t *testing.T) {
	repository := &linkRepository{}
	service := NewSnapshotAttachmentHandler(repository, repository, attachmentUnitOfWork{})
	service.now = func() time.Time { return time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC) }
	actor := domain.SnapshotActor{Subject: "investigator-1", Role: "ADMIN"}
	first, attached, err := service.Attach(context.Background(), "case-1", "snapshot-1", 4, actor, "request-1")
	if err != nil || !attached || first.CaseLockVersion != 5 {
		t.Fatalf("first attach = %#v, %v, %v", first, attached, err)
	}
	_, attached, err = service.Attach(context.Background(), "case-1", "snapshot-1", 5, actor, "request-2")
	if err != nil || attached || len(repository.audits) != 1 {
		t.Fatalf("duplicate attach = %v, %v, audits=%d", attached, err, len(repository.audits))
	}
}
