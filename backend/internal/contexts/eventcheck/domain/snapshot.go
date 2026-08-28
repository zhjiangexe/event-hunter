package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidSnapshot        = errors.New("invalid Check Snapshot")
	ErrSnapshotNotFound       = errors.New("Check Snapshot not found")
	ErrFindingNotFound        = errors.New("Check Finding not found")
	ErrInvalidFindingFeedback = errors.New("invalid Check Finding feedback")
	ErrFeedbackConflict       = errors.New("Check Finding feedback optimistic lock conflict")
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type SnapshotActor struct {
	Subject string
	Role    string
}

type SnapshotModel struct {
	ID         string
	Version    int
	Kind       string
	SourcePath string
	Checksum   string
}

type SnapshotEventReference struct {
	EventID          string
	EventType        string
	EventVersion     int
	OccurredAt       time.Time
	Producer         string
	AggregateType    string
	AggregateID      string
	Sequence         uint64
	CorrelationID    string
	TraceID          *string
	PayloadSHA256    string
	Ordinal          int
	Disposition      string
	AdjustmentReason *string
	SourceAvailable  bool
}

type SnapshotRelationship struct {
	Ordinal       int
	FromEventID   *string
	ToEventID     string
	RelationType  string
	SourceField   *string
	SourceModelID *string
	SourceRuleID  *string
}

type SnapshotFinding struct {
	ID                         string
	RuleKind                   string
	RuleID                     string
	RuleVersion                int
	RuleChecksum               string
	Severity                   string
	Code                       string
	ExpectationState           *string
	EvidenceReferences         json.RawMessage
	RecommendedQueryTemplateID *string
}

// CheckSnapshot is the immutable aggregate saved only after the application
// re-evaluates canonical events. It owns references and deterministic output,
// never raw event payloads.
type CheckSnapshot struct {
	ID                     string
	Provenance             string
	CreatedBy              string
	CreatedByRole          string
	CreatedAt              time.Time
	EvaluationRequest      json.RawMessage
	AsOf                   time.Time
	SourceHealth           json.RawMessage
	Model                  SnapshotModel
	Result                 json.RawMessage
	EventReferences        []SnapshotEventReference
	Relationships          []SnapshotRelationship
	Findings               []SnapshotFinding
	EventSetHash           string
	EvaluationHash         string
	ResultSchemaVersion    int
	RetentionProfile       json.RawMessage
	IdempotencyActor       string
	IdempotencyKey         string
	IdempotencyRequestHash string
}

func NewCheckSnapshot(snapshot CheckSnapshot) (CheckSnapshot, error) {
	if strings.TrimSpace(snapshot.ID) == "" || snapshot.Provenance != "LIVE_EVALUATION" ||
		strings.TrimSpace(snapshot.CreatedBy) == "" || !snapshotActorRole(snapshot.CreatedByRole) || snapshot.CreatedAt.IsZero() ||
		len(snapshot.EvaluationRequest) == 0 || snapshot.AsOf.IsZero() || len(snapshot.SourceHealth) == 0 || len(snapshot.Result) == 0 ||
		strings.TrimSpace(snapshot.Model.ID) == "" || snapshot.Model.Version < 1 || !sha256Pattern.MatchString(snapshot.Model.Checksum) ||
		!sha256Pattern.MatchString(snapshot.EventSetHash) || !sha256Pattern.MatchString(snapshot.EvaluationHash) ||
		snapshot.ResultSchemaVersion != 1 || strings.TrimSpace(snapshot.IdempotencyActor) == "" ||
		strings.TrimSpace(snapshot.IdempotencyKey) == "" || len(snapshot.IdempotencyKey) > 200 ||
		!sha256Pattern.MatchString(snapshot.IdempotencyRequestHash) {
		return CheckSnapshot{}, ErrInvalidSnapshot
	}
	for _, reference := range snapshot.EventReferences {
		if strings.TrimSpace(reference.EventID) == "" || !sha256Pattern.MatchString(reference.PayloadSHA256) ||
			(reference.Disposition != "INCLUDED" && reference.Disposition != "EXCLUDED") {
			return CheckSnapshot{}, ErrInvalidSnapshot
		}
	}
	for _, finding := range snapshot.Findings {
		if strings.TrimSpace(finding.ID) == "" || strings.TrimSpace(finding.RuleID) == "" ||
			finding.RuleVersion < 1 || !sha256Pattern.MatchString(finding.RuleChecksum) || len(finding.EvidenceReferences) == 0 {
			return CheckSnapshot{}, ErrInvalidSnapshot
		}
	}
	return snapshot, nil
}

type FindingFeedbackStatus string

const (
	FeedbackConfirmed     FindingFeedbackStatus = "CONFIRMED"
	FeedbackFalsePositive FindingFeedbackStatus = "FALSE_POSITIVE"
	FeedbackNeedsReview   FindingFeedbackStatus = "NEEDS_REVIEW"
)

type FindingFeedback struct {
	FindingID   string
	Status      FindingFeedbackStatus
	ActorID     string
	ActorRole   string
	UpdatedAt   time.Time
	LockVersion int64
}

func NewFindingFeedback(findingID string, status FindingFeedbackStatus, actor SnapshotActor, now time.Time) (FindingFeedback, error) {
	feedback := FindingFeedback{FindingID: findingID, Status: status, ActorID: actor.Subject, ActorRole: actor.Role, UpdatedAt: now.UTC(), LockVersion: 1}
	if err := feedback.validate(); err != nil {
		return FindingFeedback{}, err
	}
	return feedback, nil
}

func (feedback *FindingFeedback) Reclassify(status FindingFeedbackStatus, actor SnapshotActor, expectedVersion int64, now time.Time) error {
	if feedback.LockVersion != expectedVersion {
		return ErrFeedbackConflict
	}
	feedback.Status = status
	feedback.ActorID = actor.Subject
	feedback.ActorRole = actor.Role
	feedback.UpdatedAt = now.UTC()
	feedback.LockVersion++
	return feedback.validate()
}

func (feedback FindingFeedback) validate() error {
	if strings.TrimSpace(feedback.FindingID) == "" || strings.TrimSpace(feedback.ActorID) == "" ||
		!snapshotActorRole(feedback.ActorRole) || feedback.UpdatedAt.IsZero() || feedback.LockVersion < 1 ||
		(feedback.Status != FeedbackConfirmed && feedback.Status != FeedbackFalsePositive && feedback.Status != FeedbackNeedsReview) {
		return ErrInvalidFindingFeedback
	}
	return nil
}

func snapshotActorRole(role string) bool {
	return role == "INVESTIGATOR" || role == "ADMIN"
}
