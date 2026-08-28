package ports

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

// Actor is the authenticated principal used for append-only audit records.
// It is kept in the context port layer because persistence needs the identity,
// while the domain aggregate does not depend on transport authentication.
type Actor struct {
	Subject string
	Role    string
}

type AuditEntry struct {
	ID        string
	ActorID   string
	ActorRole string
	Action    string
	RequestID string
	TraceID   *string
	Metadata  map[string]any
	CreatedAt time.Time
}

type PatternFinding struct {
	ID                   string
	PatternID            string
	PatternVersion       int
	Severity             string
	MatchedConditions    []string
	EvidenceReferences   []string
	RecommendedNextQuery string
	QueryTemplateID      string
	WindowFrom           time.Time
	WindowTo             time.Time
	IdempotencyKey       string
	FeedbackStatus       domain.PatternFeedbackStatus
	FeedbackActorID      string
	FeedbackActorRole    string
	FeedbackUpdatedAt    *time.Time
	FeedbackLockVersion  int64
}

type Evidence struct {
	ID           string
	EvidenceType string
	Reference    string
	Checksum     *string
	CollectedAt  time.Time
	GeneratorURL *string
	GrafanaOrgID *int64
}

type InvestigationDetailsRepository interface {
	RecordAudit(ctx context.Context, actor Actor, action, resourceID, requestID string, metadata map[string]any) error
	Audit(ctx context.Context, investigationID string) ([]AuditEntry, error)
	Findings(ctx context.Context, investigationID string) ([]PatternFinding, error)
	Evidence(ctx context.Context, investigationID string) ([]Evidence, error)
	Notes(ctx context.Context, investigationID string) ([]domain.CaseNote, error)
	SaveFinding(ctx context.Context, investigationID string, finding PatternFinding) error
	SaveEvidence(ctx context.Context, investigationID, evidenceType, reference, checksum string) error
}

// UnitOfWork keeps one application mutation and its append-only audit record
// in the same persistence transaction. Repository adapters obtain the active
// transaction from ctx; application and domain code remain storage agnostic.
type UnitOfWork interface {
	WithinTransaction(ctx context.Context, operation func(context.Context) error) error
}

type PatternFeedbackRepository interface {
	FindPatternFeedback(ctx context.Context, investigationID, findingID string) (domain.PatternFindingFeedback, error)
	SavePatternFeedback(ctx context.Context, feedback domain.PatternFindingFeedback, expectedVersion int64) error
}
