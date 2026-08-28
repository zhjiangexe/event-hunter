package caselifecycle

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

func TestUpdateAppliesStateTransitionAndRecordsAudit(t *testing.T) {
	repository := &caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusOpen, Severity: domain.SeverityHigh, CorrelationID: "ORDER-1", LockVersion: 2}}
	details := &detailsRepositoryFake{}
	service := NewService(repository, details)
	service.now = func() time.Time { return time.Date(2026, 8, 21, 6, 0, 0, 0, time.UTC) }
	status := domain.StatusInvestigating
	assignee := "operator-1"

	updated, err := service.Update(t.Context(), "case-1", 2, CasePatch{Status: &status, Assignee: &assignee}, Actor{Subject: "demo", Role: "INVESTIGATOR"}, "request-1")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Status != domain.StatusInvestigating || updated.Assignee == nil || *updated.Assignee != assignee || updated.LockVersion != 3 {
		t.Fatalf("updated case = %#v", updated)
	}
	if details.lastAction != "UPDATE_INVESTIGATION" || details.lastMetadata["from_status"] != "OPEN" || details.lastMetadata["to_status"] != "INVESTIGATING" {
		t.Fatalf("audit = %q %#v", details.lastAction, details.lastMetadata)
	}
}

func TestCreatePreservesIncidentWindowInAggregateAndAudit(t *testing.T) {
	repository := &caseRepositoryFake{}
	details := &detailsRepositoryFake{}
	service := NewService(repository, details)
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	window := domain.IncidentWindow{From: now.Add(-time.Hour), To: now, Source: domain.IncidentWindowTimelineSearch}

	created, err := service.Create(t.Context(), "checkout failed", domain.SeverityHigh, "ORDER-1", window, Actor{Subject: "tester", Role: "INVESTIGATOR"}, "request-create")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !created.IncidentWindow.From.Equal(window.From) || !created.IncidentWindow.To.Equal(window.To) || created.IncidentWindow.Source != window.Source {
		t.Fatalf("created incident window = %#v", created.IncidentWindow)
	}
	if details.lastMetadata["incident_window_source"] != string(domain.IncidentWindowTimelineSearch) {
		t.Fatalf("create audit metadata = %#v", details.lastMetadata)
	}
}

func TestUpdateRejectsInvalidAndIncompleteResolvedTransitions(t *testing.T) {
	for _, test := range []struct {
		name    string
		current domain.CaseStatus
		target  domain.CaseStatus
		want    error
	}{
		{name: "open to resolved", current: domain.StatusOpen, target: domain.StatusResolved, want: ErrInvalidTransition},
		{name: "resolved fields missing", current: domain.StatusInvestigating, target: domain.StatusResolved, want: ErrResolutionFields},
		{name: "closed uses close operation", current: domain.StatusInvestigating, target: domain.StatusClosed, want: ErrCloseRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: test.current, LockVersion: 1}}
			service := NewService(repository, &detailsRepositoryFake{})
			_, err := service.Update(t.Context(), "case-1", 1, CasePatch{Status: &test.target}, Actor{}, "")
			if !errors.Is(err, test.want) {
				t.Fatalf("Update() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestUpdateReturnsCurrentVersionOnOptimisticConflict(t *testing.T) {
	repository := &caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusOpen, LockVersion: 4}, conflict: true}
	service := NewService(repository, &detailsRepositoryFake{})
	status := domain.StatusInvestigating

	_, err := service.Update(t.Context(), "case-1", 3, CasePatch{Status: &status}, Actor{}, "")
	var conflict VersionConflictError
	if !errors.As(err, &conflict) || conflict.CurrentVersion != 4 {
		t.Fatalf("Update() error = %#v, want current version 4", err)
	}
}

func TestCloseSetsResolutionAndClosedTimestamp(t *testing.T) {
	repository := &caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusResolved, LockVersion: 7}}
	details := &detailsRepositoryFake{}
	service := NewService(repository, details)
	closedAt := time.Date(2026, 8, 21, 7, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return closedAt }

	closed, err := service.Close(t.Context(), "case-1", 7, "root cause", "resolution", nil, Actor{}, "request-2")
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closed.Status != domain.StatusClosed || closed.ClosedAt == nil || !closed.ClosedAt.Equal(closedAt) || closed.RootCause == nil || *closed.RootCause != "root cause" {
		t.Fatalf("closed case = %#v", closed)
	}
	if details.lastAction != "CLOSE_INVESTIGATION" {
		t.Fatalf("audit action = %q", details.lastAction)
	}
}

func TestAddNoteAppendsWithOptimisticLockAndAudit(t *testing.T) {
	repository := &caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusInvestigating, LockVersion: 5}}
	details := &detailsRepositoryFake{}
	service := NewService(repository, details)
	now := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	updated, note, err := service.AddNote(t.Context(), "case-1", 5, "consumer replay completed", Actor{Subject: "demo", Role: "INVESTIGATOR"}, "request-note")
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if updated.LockVersion != 6 || updated.LastUpdatedBy != "demo" || note.Body != "consumer replay completed" || note.AuthorID != "demo" {
		t.Fatalf("AddNote() = %#v %#v", updated, note)
	}
	if details.lastAction != "ADD_INVESTIGATION_NOTE" || details.lastMetadata["note_id"] != note.ID {
		t.Fatalf("audit = %q %#v", details.lastAction, details.lastMetadata)
	}
}

func TestGetDetailsAndSummaryFailWhenRequiredChildReadFails(t *testing.T) {
	childFailure := errors.New("child repository unavailable")
	for _, testCase := range []struct {
		name    string
		details *detailsRepositoryFake
		summary bool
	}{
		{name: "findings", details: &detailsRepositoryFake{findingsErr: childFailure}},
		{name: "evidence", details: &detailsRepositoryFake{evidenceErr: childFailure}},
		{name: "notes", details: &detailsRepositoryFake{notesErr: childFailure}},
		{name: "audit", details: &detailsRepositoryFake{auditErr: childFailure}, summary: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewService(&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1"}}, testCase.details)
			var err error
			if testCase.summary {
				_, err = service.GetSummaryDetails(t.Context(), "case-1")
			} else {
				_, err = service.GetDetails(t.Context(), "case-1")
			}
			if !errors.Is(err, childFailure) {
				t.Fatalf("read error = %v, want child repository failure", err)
			}
		})
	}
}

func TestCaseMutationsRollBackWhenAuditFails(t *testing.T) {
	auditFailure := errors.New("audit unavailable")
	for _, testCase := range []struct {
		name    string
		current domain.InvestigationCase
		mutate  func(*Service) error
	}{
		{
			name:    "create",
			current: domain.InvestigationCase{ID: "existing", Status: domain.StatusOpen},
			mutate: func(service *Service) error {
				_, err := service.Create(t.Context(), "new case", domain.SeverityHigh, "ORDER-2", domain.IncidentWindow{
					From: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
					Source: domain.IncidentWindowTimelineSearch,
				}, Actor{Subject: "demo"}, "request-create")
				return err
			},
		},
		{
			name:    "update",
			current: domain.InvestigationCase{ID: "case-1", Title: "before", Status: domain.StatusOpen, LockVersion: 2},
			mutate: func(service *Service) error {
				title := "after"
				_, err := service.Update(t.Context(), "case-1", 2, CasePatch{Title: &title}, Actor{Subject: "demo"}, "request-update")
				return err
			},
		},
		{
			name:    "close",
			current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusResolved, LockVersion: 2},
			mutate: func(service *Service) error {
				_, err := service.Close(t.Context(), "case-1", 2, "root", "resolution", nil, Actor{Subject: "demo"}, "request-close")
				return err
			},
		},
		{
			name:    "note",
			current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusInvestigating, LockVersion: 2},
			mutate: func(service *Service) error {
				_, _, err := service.AddNote(t.Context(), "case-1", 2, "note", Actor{Subject: "demo", Role: "INVESTIGATOR"}, "request-note")
				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &caseRepositoryFake{current: testCase.current}
			details := &detailsRepositoryFake{auditErr: auditFailure}
			unit := &rollbackUnitOfWorkFake{cases: repository}
			service := NewService(repository, details, unit)
			if err := testCase.mutate(service); !errors.Is(err, auditFailure) {
				t.Fatalf("mutation error = %v, want audit failure", err)
			}
			if !reflect.DeepEqual(repository.current, testCase.current) {
				t.Fatalf("case changed despite audit rollback: got %#v want %#v", repository.current, testCase.current)
			}
		})
	}
}

type caseRepositoryFake struct {
	current  domain.InvestigationCase
	conflict bool
}

func (repository *caseRepositoryFake) Create(_ context.Context, value domain.InvestigationCase) (domain.InvestigationCase, error) {
	value.ID = "case-created"
	repository.current = value
	return value, nil
}

func (repository *caseRepositoryFake) Get(context.Context, string) (domain.InvestigationCase, error) {
	return repository.current, nil
}

func (repository *caseRepositoryFake) Update(_ context.Context, value domain.InvestigationCase) (domain.InvestigationCase, error) {
	if repository.conflict {
		return domain.InvestigationCase{}, domain.ErrOptimisticConflict
	}
	value.LockVersion++
	repository.current = value
	return value, nil
}

func (repository *caseRepositoryFake) AppendNote(_ context.Context, _ string, expectedVersion int64, _ domain.CaseNote, lastUpdatedBy string, updatedAt time.Time) (domain.InvestigationCase, error) {
	if repository.conflict || expectedVersion != repository.current.LockVersion {
		return domain.InvestigationCase{}, domain.ErrOptimisticConflict
	}
	repository.current.LockVersion++
	repository.current.LastUpdatedBy = lastUpdatedBy
	repository.current.UpdatedAt = updatedAt
	return repository.current, nil
}

func (repository *caseRepositoryFake) List(context.Context, CaseFilter) (CasePage, error) {
	return CasePage{Items: []domain.InvestigationCase{repository.current}}, nil
}

type detailsRepositoryFake struct {
	lastAction   string
	lastMetadata map[string]any
	auditErr     error
	findingsErr  error
	evidenceErr  error
	notesErr     error
}

func (repository *detailsRepositoryFake) RecordAudit(_ context.Context, _ Actor, action, _, _ string, metadata map[string]any) error {
	repository.lastAction = action
	repository.lastMetadata = metadata
	return repository.auditErr
}

func (repository *detailsRepositoryFake) Audit(context.Context, string) ([]AuditEntry, error) {
	return nil, repository.auditErr
}
func (repository *detailsRepositoryFake) Findings(context.Context, string) ([]PatternFinding, error) {
	return nil, repository.findingsErr
}
func (repository *detailsRepositoryFake) Evidence(context.Context, string) ([]Evidence, error) {
	return nil, repository.evidenceErr
}
func (repository *detailsRepositoryFake) Notes(context.Context, string) ([]domain.CaseNote, error) {
	return nil, repository.notesErr
}
func (*detailsRepositoryFake) SaveFinding(context.Context, string, PatternFinding) error { return nil }
func (*detailsRepositoryFake) SaveEvidence(context.Context, string, string, string, string) error {
	return nil
}

type rollbackUnitOfWorkFake struct {
	cases *caseRepositoryFake
}

func (unit *rollbackUnitOfWorkFake) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	before := unit.cases.current
	if err := operation(ctx); err != nil {
		unit.cases.current = before
		return err
	}
	return nil
}
