package caselifecycle

import (
	"context"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

// Commands is the write boundary exposed to inbound adapters. Keeping it
// separate from Queries prevents a handler from accidentally depending on the
// whole case lifecycle service.
type Commands interface {
	Create(ctx context.Context, title string, severity domain.Severity, correlationID string, incidentWindow domain.IncidentWindow, actor Actor, requestID string) (domain.InvestigationCase, error)
	Update(ctx context.Context, id string, expectedVersion int64, patch CasePatch, actor Actor, requestID string) (domain.InvestigationCase, error)
	AddNote(ctx context.Context, id string, expectedVersion int64, body string, actor Actor, requestID string) (domain.InvestigationCase, domain.CaseNote, error)
	Close(ctx context.Context, id string, expectedVersion int64, rootCause, resolutionSummary string, fixedVersion *string, actor Actor, requestID string) (domain.InvestigationCase, error)
}

// Queries is the read boundary exposed to inbound adapters.
type Queries interface {
	List(ctx context.Context, filter CaseFilter) (CasePage, error)
	Get(ctx context.Context, id string) (domain.InvestigationCase, error)
	GetDetails(ctx context.Context, id string) (CaseDetails, error)
	GetSummaryDetails(ctx context.Context, id string) (CaseSummaryDetails, error)
	Audit(ctx context.Context, id string) ([]AuditEntry, error)
	Findings(ctx context.Context, id string) ([]PatternFinding, error)
	Evidence(ctx context.Context, id string) ([]Evidence, error)
	Notes(ctx context.Context, id string) ([]domain.CaseNote, error)
}

var _ Commands = (*Service)(nil)
var _ Queries = (*Service)(nil)
