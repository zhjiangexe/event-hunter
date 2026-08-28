package domain

import (
	"errors"
	"strings"
	"time"
)

type PatternFeedbackStatus string

const (
	PatternFeedbackUnreviewed    PatternFeedbackStatus = "UNREVIEWED"
	PatternFeedbackConfirmed     PatternFeedbackStatus = "CONFIRMED"
	PatternFeedbackFalsePositive PatternFeedbackStatus = "FALSE_POSITIVE"
	PatternFeedbackNeedsReview   PatternFeedbackStatus = "NEEDS_REVIEW"
)

var (
	ErrPatternFindingNotFound  = errors.New("pattern finding not found")
	ErrPatternFeedbackConflict = errors.New("pattern feedback version conflict")
	ErrInvalidPatternFeedback  = errors.New("invalid pattern feedback")
)

// PatternFindingFeedback is the human classification of an immutable finding.
// UNREVIEWED is the virtual initial state and is not persisted until classified.
type PatternFindingFeedback struct {
	FindingID       string
	InvestigationID string
	Status          PatternFeedbackStatus
	ActorID         string
	ActorRole       string
	UpdatedAt       *time.Time
	LockVersion     int64
}

func (feedback PatternFindingFeedback) Reclassify(expectedVersion int64, status PatternFeedbackStatus, actorID, actorRole string, now time.Time) (PatternFindingFeedback, error) {
	if feedback.LockVersion != expectedVersion {
		return PatternFindingFeedback{}, ErrPatternFeedbackConflict
	}
	if status != PatternFeedbackConfirmed && status != PatternFeedbackFalsePositive && status != PatternFeedbackNeedsReview {
		return PatternFindingFeedback{}, ErrInvalidPatternFeedback
	}
	actorID = strings.TrimSpace(actorID)
	actorRole = strings.TrimSpace(actorRole)
	if actorID == "" || actorRole == "" {
		return PatternFindingFeedback{}, ErrInvalidPatternFeedback
	}
	feedback.Status = status
	feedback.ActorID = actorID
	feedback.ActorRole = actorRole
	updatedAt := now.UTC()
	feedback.UpdatedAt = &updatedAt
	feedback.LockVersion++
	return feedback, nil
}
