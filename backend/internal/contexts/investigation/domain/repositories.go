package domain

import (
	"context"
	"time"
)

// CaseRepository is the persistence port owned by the Investigation domain.
// Implementations belong to infrastructure adapters; the aggregate does not
// know whether the store is PostgreSQL, an in-memory repository, or another
// persistence mechanism.
type CaseRepository interface {
	Create(ctx context.Context, investigationCase InvestigationCase) (InvestigationCase, error)
	Get(ctx context.Context, id string) (InvestigationCase, error)
	Update(ctx context.Context, investigationCase InvestigationCase) (InvestigationCase, error)
	AppendNote(ctx context.Context, id string, expectedVersion int64, note CaseNote, lastUpdatedBy string, updatedAt time.Time) (InvestigationCase, error)
	List(ctx context.Context, filter CaseFilter) (CasePage, error)
}

// CaseEvidenceRepository is the narrow write port for append-only evidence.
// Implementations must keep the evidence insert, related-correlation update,
// and optimistic lock-version change in one transaction.
type CaseEvidenceRepository interface {
	Get(ctx context.Context, id string) (InvestigationCase, error)
	AppendEvidence(ctx context.Context, investigationCase InvestigationCase, expectedVersion int64, evidence CaseEvidence) (InvestigationCase, CaseEvidence, bool, error)
}

type CaseFilter struct {
	Query         string
	Status        string
	Severity      string
	Assignee      string
	Priority      string
	Tag           string
	CorrelationID string
	SortBy        string
	SortOrder     string
	PageSize      int
	BeforeTime    *time.Time
	BeforeID      string
}

type CasePage struct {
	Items   []InvestigationCase
	HasMore bool
}

type SavedSearchRepository interface {
	Create(ctx context.Context, search SavedSearch) (SavedSearch, error)
	ListByOwner(ctx context.Context, ownerSubject string) ([]SavedSearch, error)
	DeleteByOwner(ctx context.Context, id, ownerSubject string) error
}
