package cases

import (
	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

// Details is the shared application read model for one investigation. It is
// deliberately transport-neutral so read capabilities do not depend on the
// case lifecycle package merely to exchange data.
type CaseDetails struct {
	Case     domain.InvestigationCase
	Findings []ports.PatternFinding
	Evidence []ports.Evidence
	Notes    []domain.CaseNote
}

type Details = CaseDetails

type SummaryDetails struct {
	CaseDetails
	Audit []ports.AuditEntry
}
