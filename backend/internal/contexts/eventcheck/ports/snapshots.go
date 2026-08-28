package ports

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
)

type UnitOfWork interface {
	WithinTransaction(ctx context.Context, operation func(context.Context) error) error
}

type AuditRecord struct {
	Actor        domain.SnapshotActor
	Action       string
	ResourceType string
	ResourceID   string
	RequestID    string
	Metadata     map[string]any
	CreatedAt    time.Time
}

type AuditWriter interface {
	RecordEventCheckAudit(ctx context.Context, record AuditRecord) error
}

type SnapshotRepository interface {
	Create(ctx context.Context, snapshot domain.CheckSnapshot) (persisted domain.CheckSnapshot, created bool, err error)
	Get(ctx context.Context, id string) (domain.CheckSnapshot, error)
	ListFeedback(ctx context.Context, snapshotID string) ([]domain.FindingFeedback, error)
	FindFeedback(ctx context.Context, findingID string) (domain.FindingFeedback, bool, error)
	SaveFeedback(ctx context.Context, feedback domain.FindingFeedback, expectedVersion int64) error
}

type InvestigationSnapshotLink struct {
	InvestigationID string
	SnapshotID      string
	LinkedBy        string
	LinkedByRole    string
	LinkedAt        time.Time
	CaseLockVersion int64
}

type InvestigationSnapshotRepository interface {
	Attach(ctx context.Context, investigationID, snapshotID string, expectedVersion int64, actor domain.SnapshotActor, linkedAt time.Time) (InvestigationSnapshotLink, bool, error)
	List(ctx context.Context, investigationID string) ([]InvestigationSnapshotLink, error)
}
