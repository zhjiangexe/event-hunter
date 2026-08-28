package domain

import (
	"errors"
	"testing"
	"time"
)

func TestPatternFindingFeedbackReclassifyAdvancesVersion(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.FixedZone("TST", 8*60*60))
	feedback := PatternFindingFeedback{FindingID: "finding-1", InvestigationID: "case-1", Status: PatternFeedbackUnreviewed}
	updated, err := feedback.Reclassify(0, PatternFeedbackConfirmed, "investigator-1", "INVESTIGATOR", now)
	if err != nil {
		t.Fatalf("Reclassify() error = %v", err)
	}
	if updated.Status != PatternFeedbackConfirmed || updated.LockVersion != 1 || updated.UpdatedAt == nil || updated.UpdatedAt.Location() != time.UTC {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestPatternFindingFeedbackRejectsStaleOrInvalidChange(t *testing.T) {
	feedback := PatternFindingFeedback{LockVersion: 2}
	if _, err := feedback.Reclassify(1, PatternFeedbackConfirmed, "actor", "INVESTIGATOR", time.Now()); !errors.Is(err, ErrPatternFeedbackConflict) {
		t.Fatalf("stale error = %v", err)
	}
	if _, err := feedback.Reclassify(2, PatternFeedbackUnreviewed, "actor", "INVESTIGATOR", time.Now()); !errors.Is(err, ErrInvalidPatternFeedback) {
		t.Fatalf("invalid status error = %v", err)
	}
}
