package classify_check_finding

import (
	"context"
	"errors"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type feedbackRepository struct {
	feedback *domain.FindingFeedback
	audits   []ports.AuditRecord
}

func (*feedbackRepository) Create(context.Context, domain.CheckSnapshot) (domain.CheckSnapshot, bool, error) {
	panic("not used")
}
func (*feedbackRepository) Get(context.Context, string) (domain.CheckSnapshot, error) {
	panic("not used")
}
func (*feedbackRepository) ListFeedback(context.Context, string) ([]domain.FindingFeedback, error) {
	panic("not used")
}
func (repository *feedbackRepository) FindFeedback(_ context.Context, findingID string) (domain.FindingFeedback, bool, error) {
	if repository.feedback == nil {
		return domain.FindingFeedback{FindingID: findingID}, false, nil
	}
	return *repository.feedback, true, nil
}
func (repository *feedbackRepository) SaveFeedback(_ context.Context, feedback domain.FindingFeedback, expectedVersion int64) error {
	if repository.feedback == nil && expectedVersion != 0 {
		return domain.ErrFeedbackConflict
	}
	if repository.feedback != nil && repository.feedback.LockVersion != expectedVersion {
		return domain.ErrFeedbackConflict
	}
	repository.feedback = &feedback
	return nil
}
func (repository *feedbackRepository) RecordEventCheckAudit(_ context.Context, record ports.AuditRecord) error {
	repository.audits = append(repository.audits, record)
	return nil
}

type directUnitOfWork struct{}

func (directUnitOfWork) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func TestClassifyCreatesAndOptimisticallyUpdatesFeedback(t *testing.T) {
	repository := &feedbackRepository{}
	service := NewService(repository, repository, directUnitOfWork{})
	service.now = func() time.Time { return time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC) }
	actor := domain.SnapshotActor{Subject: "investigator-1", Role: "INVESTIGATOR"}
	created, err := service.Classify(context.Background(), "finding-1", domain.FeedbackNeedsReview, 0, actor, "request-1")
	if err != nil || created.LockVersion != 1 {
		t.Fatalf("create feedback = %#v, %v", created, err)
	}
	updated, err := service.Classify(context.Background(), "finding-1", domain.FeedbackConfirmed, 1, actor, "request-2")
	if err != nil || updated.LockVersion != 2 || updated.Status != domain.FeedbackConfirmed {
		t.Fatalf("update feedback = %#v, %v", updated, err)
	}
	_, err = service.Classify(context.Background(), "finding-1", domain.FeedbackFalsePositive, 1, actor, "request-3")
	if !errors.Is(err, domain.ErrFeedbackConflict) {
		t.Fatalf("stale feedback error = %v", err)
	}
	if len(repository.audits) != 2 {
		t.Fatalf("audit count = %d", len(repository.audits))
	}
}
