package compatibility

import (
	"context"
	"errors"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

func TestReclassifyPersistsAndAuditsInOneUnit(t *testing.T) {
	repository := &feedbackRepositoryFake{feedback: domain.PatternFindingFeedback{FindingID: "finding-1", InvestigationID: "case-1", Status: domain.PatternFeedbackUnreviewed}}
	audit := &feedbackAuditFake{}
	unit := &feedbackUnitFake{}
	service := NewPatternFeedbackService(repository, audit, unit)
	service.now = func() time.Time { return time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC) }

	result, err := service.Reclassify(t.Context(), Command{InvestigationID: "case-1", FindingID: "finding-1", ExpectedVersion: 0, Status: domain.PatternFeedbackConfirmed, Actor: ports.Actor{Subject: "investigator-1", Role: "INVESTIGATOR"}, RequestID: "request-1"})
	if err != nil {
		t.Fatalf("Reclassify() error = %v", err)
	}
	if result.LockVersion != 1 || repository.expectedVersion != 0 || audit.action != "CLASSIFY_PATTERN_FINDING" || !unit.called {
		t.Fatalf("result=%#v repository=%#v audit=%#v unit=%#v", result, repository, audit, unit)
	}
}

func TestReclassifyRejectsStaleVersionBeforeWrite(t *testing.T) {
	repository := &feedbackRepositoryFake{feedback: domain.PatternFindingFeedback{FindingID: "finding-1", InvestigationID: "case-1", Status: domain.PatternFeedbackNeedsReview, LockVersion: 2}}
	service := NewPatternFeedbackService(repository, &feedbackAuditFake{}, &feedbackUnitFake{})
	_, err := service.Reclassify(t.Context(), Command{InvestigationID: "case-1", FindingID: "finding-1", ExpectedVersion: 1, Status: domain.PatternFeedbackConfirmed, Actor: ports.Actor{Subject: "actor", Role: "INVESTIGATOR"}})
	if !errors.Is(err, domain.ErrPatternFeedbackConflict) || repository.saved {
		t.Fatalf("error=%v saved=%v", err, repository.saved)
	}
}

type feedbackRepositoryFake struct {
	feedback        domain.PatternFindingFeedback
	expectedVersion int64
	saved           bool
}

func (repository *feedbackRepositoryFake) FindPatternFeedback(context.Context, string, string) (domain.PatternFindingFeedback, error) {
	return repository.feedback, nil
}

func (repository *feedbackRepositoryFake) SavePatternFeedback(_ context.Context, feedback domain.PatternFindingFeedback, expectedVersion int64) error {
	repository.feedback = feedback
	repository.expectedVersion = expectedVersion
	repository.saved = true
	return nil
}

type feedbackAuditFake struct{ action string }

func (audit *feedbackAuditFake) RecordAudit(_ context.Context, _ ports.Actor, action, _ string, _ string, _ map[string]any) error {
	audit.action = action
	return nil
}

type feedbackUnitFake struct{ called bool }

func (unit *feedbackUnitFake) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	unit.called = true
	return operation(ctx)
}
