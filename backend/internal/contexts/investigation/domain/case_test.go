package domain

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestInvestigationCaseOwnsImmutableIncidentWindowAtCreation(t *testing.T) {
	from := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	window, err := NewIncidentWindow(from, from.Add(time.Hour), IncidentWindowTimelineSearch)
	if err != nil {
		t.Fatalf("NewIncidentWindow() error = %v", err)
	}
	created, err := NewInvestigationCase("Case", SeverityHigh, "ORDER-1", window, "demo", from.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("NewInvestigationCase() error = %v", err)
	}
	if created.IncidentWindow != window || created.Priority != PriorityP1 || created.Status != StatusOpen {
		t.Fatalf("created case = %#v", created)
	}
	if _, err := NewIncidentWindow(from, from.Add(MaximumIncidentWindow+time.Second), IncidentWindowTimelineSearch); !errors.Is(err, ErrInvalidIncidentWindow) {
		t.Fatalf("oversized incident window error = %v, want %v", err, ErrInvalidIncidentWindow)
	}
}

func TestInvestigationCaseTransitionOwnsResolutionInvariant(t *testing.T) {
	investigationCase := InvestigationCase{Status: StatusInvestigating}

	if err := investigationCase.TransitionTo(StatusResolved, nil, nil); !errors.Is(err, ErrResolutionFields) {
		t.Fatalf("TransitionTo() error = %v, want %v", err, ErrResolutionFields)
	}

	rootCause := "consumer was stalled"
	resolution := "restarted consumer and replayed the failed partition"
	if err := investigationCase.TransitionTo(StatusResolved, &rootCause, &resolution); err != nil {
		t.Fatalf("TransitionTo() error = %v", err)
	}
	if investigationCase.Status != StatusResolved || investigationCase.RootCause == nil || investigationCase.ResolutionSummary == nil {
		t.Fatalf("resolved aggregate = %#v", investigationCase)
	}
}

func TestInvestigationCaseRejectsInvalidMutableFields(t *testing.T) {
	investigationCase := InvestigationCase{Status: StatusOpen, Title: "valid"}
	if err := investigationCase.ChangeTitle("   "); !errors.Is(err, ErrInvalidCaseTitle) {
		t.Fatalf("ChangeTitle() error = %v, want %v", err, ErrInvalidCaseTitle)
	}
	if err := investigationCase.ChangeSeverity(Severity("URGENT")); !errors.Is(err, ErrInvalidCaseSeverity) {
		t.Fatalf("ChangeSeverity() error = %v, want %v", err, ErrInvalidCaseSeverity)
	}
	if err := investigationCase.TransitionTo(CaseStatus("PAUSED"), nil, nil); !errors.Is(err, ErrInvalidCaseStatus) {
		t.Fatalf("TransitionTo() error = %v, want %v", err, ErrInvalidCaseStatus)
	}
	if investigationCase.Title != "valid" {
		t.Fatalf("invalid title changed aggregate to %q", investigationCase.Title)
	}
}

func TestResolvedInvestigationCaseCannotClearRequiredResolutionFields(t *testing.T) {
	rootCause := "consumer stalled"
	resolution := "consumer restarted"
	investigationCase := InvestigationCase{Status: StatusResolved, RootCause: &rootCause, ResolutionSummary: &resolution}
	empty := "  "
	if err := investigationCase.SetRootCause(&empty); !errors.Is(err, ErrResolutionFields) {
		t.Fatalf("SetRootCause() error = %v, want %v", err, ErrResolutionFields)
	}
	if err := investigationCase.SetResolutionSummary(nil); !errors.Is(err, ErrResolutionFields) {
		t.Fatalf("SetResolutionSummary() error = %v, want %v", err, ErrResolutionFields)
	}
}

func TestRehydrateInvestigationCaseValidatesPersistedState(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	valid := InvestigationCase{
		ID: "case-1", CaseNo: "EH-1", Title: "  valid case  ", Severity: SeverityHigh, Status: StatusOpen,
		CorrelationID: "ORDER-1", IncidentWindow: IncidentWindow{From: now.Add(-time.Hour), To: now, Source: IncidentWindowManualDefault},
		Priority: PriorityP1, Tags: []string{}, RelatedCorrelationIDs: []string{}, LastUpdatedBy: "tester",
		CreatedAt: now, UpdatedAt: now,
	}
	rehydrated, err := RehydrateInvestigationCase(valid)
	if err != nil {
		t.Fatalf("RehydrateInvestigationCase() error = %v", err)
	}
	if rehydrated.Title != "valid case" {
		t.Fatalf("rehydrated title = %q", rehydrated.Title)
	}

	invalid := valid
	invalid.Status = StatusResolved
	if _, err := RehydrateInvestigationCase(invalid); !errors.Is(err, ErrResolutionFields) {
		t.Fatalf("invalid persisted state error = %v, want %v", err, ErrResolutionFields)
	}
	invalid = valid
	invalid.Severity = Severity("URGENT")
	if _, err := RehydrateInvestigationCase(invalid); !errors.Is(err, ErrInvalidCaseSeverity) {
		t.Fatalf("invalid persisted severity error = %v, want %v", err, ErrInvalidCaseSeverity)
	}
}

func TestAllowedTransitionsIncludesDedicatedCloseAction(t *testing.T) {
	tests := []struct {
		status CaseStatus
		want   []CaseStatus
	}{
		{StatusOpen, []CaseStatus{StatusInvestigating, StatusClosed}},
		{StatusInvestigating, []CaseStatus{StatusWaitingApproval, StatusResolved, StatusClosed}},
		{StatusWaitingApproval, []CaseStatus{StatusInvestigating, StatusResolved, StatusClosed}},
		{StatusResolved, []CaseStatus{StatusInvestigating, StatusClosed}},
		{StatusClosed, []CaseStatus{}},
	}
	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			investigationCase := InvestigationCase{Status: test.status}
			if got := investigationCase.AllowedTransitions(); !slices.Equal(got, test.want) {
				t.Fatalf("AllowedTransitions() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestInvestigationCaseCloseOwnsClosedState(t *testing.T) {
	closedAt := time.Date(2026, 8, 21, 7, 0, 0, 0, time.UTC)
	investigationCase := InvestigationCase{Status: StatusResolved}
	if err := investigationCase.Close(closedAt, "root cause", "resolution", nil); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if investigationCase.Status != StatusClosed || investigationCase.ClosedAt == nil || !investigationCase.ClosedAt.Equal(closedAt) {
		t.Fatalf("closed aggregate = %#v", investigationCase)
	}
	if err := investigationCase.ChangeTitle("new title"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ChangeTitle() error = %v, want %v", err, ErrInvalidTransition)
	}
}

func TestInvestigationCaseNormalizesCollaborationMetadata(t *testing.T) {
	investigationCase := InvestigationCase{Status: StatusOpen, CorrelationID: "ORDER-1"}
	if err := investigationCase.ReplaceTags([]string{" Payments ", "payments", "VIP"}); err != nil {
		t.Fatalf("ReplaceTags() error = %v", err)
	}
	if err := investigationCase.ReplaceRelatedCorrelationIDs([]string{"SHIPMENT-1", " PAYMENT-1 "}); err != nil {
		t.Fatalf("ReplaceRelatedCorrelationIDs() error = %v", err)
	}
	if len(investigationCase.Tags) != 2 || investigationCase.Tags[0] != "payments" || investigationCase.Tags[1] != "vip" {
		t.Fatalf("tags = %#v", investigationCase.Tags)
	}
	if len(investigationCase.RelatedCorrelationIDs) != 2 || investigationCase.RelatedCorrelationIDs[1] != "PAYMENT-1" {
		t.Fatalf("related correlations = %#v", investigationCase.RelatedCorrelationIDs)
	}
	if err := investigationCase.ReplaceRelatedCorrelationIDs([]string{"ORDER-1"}); !errors.Is(err, ErrInvalidRelatedIDs) {
		t.Fatalf("ReplaceRelatedCorrelationIDs() error = %v, want %v", err, ErrInvalidRelatedIDs)
	}
}

func TestInvestigationCaseComputesPrioritySLA(t *testing.T) {
	createdAt := time.Date(2026, 8, 21, 7, 0, 0, 0, time.UTC)
	investigationCase := InvestigationCase{Status: StatusOpen, Priority: PriorityP1, CreatedAt: createdAt}
	dueAt, status := investigationCase.SLA(createdAt.Add(2 * time.Hour))
	if !dueAt.Equal(createdAt.Add(4*time.Hour)) || status != SLAOnTrack {
		t.Fatalf("SLA() = %s %s", dueAt, status)
	}
	_, status = investigationCase.SLA(createdAt.Add(3*time.Hour + 30*time.Minute))
	if status != SLADueSoon {
		t.Fatalf("SLA() status = %s, want %s", status, SLADueSoon)
	}
	_, status = investigationCase.SLA(createdAt.Add(5 * time.Hour))
	if status != SLABreached {
		t.Fatalf("SLA() status = %s, want %s", status, SLABreached)
	}
	investigationCase.Status = StatusResolved
	_, status = investigationCase.SLA(createdAt.Add(5 * time.Hour))
	if status != SLACompleted {
		t.Fatalf("resolved SLA() status = %s, want %s", status, SLACompleted)
	}
}

func TestInvestigationCaseWritesAppendOnlyNoteValue(t *testing.T) {
	now := time.Date(2026, 8, 21, 7, 0, 0, 0, time.UTC)
	investigationCase := InvestigationCase{Status: StatusInvestigating}
	note, err := investigationCase.WriteNote("note-1", "  confirmed consumer lag  ", "demo", "INVESTIGATOR", now)
	if err != nil || note.Body != "confirmed consumer lag" || !note.CreatedAt.Equal(now) {
		t.Fatalf("WriteNote() = %#v, %v", note, err)
	}
	if _, err := investigationCase.WriteNote("note-2", "", "demo", "INVESTIGATOR", now); !errors.Is(err, ErrInvalidCaseNote) {
		t.Fatalf("WriteNote() error = %v, want %v", err, ErrInvalidCaseNote)
	}
}

func TestInvestigationCaseAttachesEventAndTracksSourceCorrelation(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	investigationCase := InvestigationCase{
		Status:                StatusInvestigating,
		CorrelationID:         "ORDER-1001",
		RelatedCorrelationIDs: []string{"PAYMENT-1001"},
	}

	evidence, err := investigationCase.AttachEvent(
		"evidence-1",
		"event-1",
		"SHIPMENT-1001",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		now,
	)
	if err != nil {
		t.Fatalf("AttachEvent() error = %v", err)
	}
	if evidence.EvidenceType != "EVENT" || evidence.Reference != "event-1" || !evidence.CollectedAt.Equal(now) {
		t.Fatalf("evidence = %#v", evidence)
	}
	if !slices.Equal(investigationCase.RelatedCorrelationIDs, []string{"PAYMENT-1001", "SHIPMENT-1001"}) {
		t.Fatalf("related correlations = %#v", investigationCase.RelatedCorrelationIDs)
	}
}

func TestInvestigationCaseRejectsInvalidOrClosedEventAttachment(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	investigationCase := InvestigationCase{Status: StatusInvestigating, CorrelationID: "ORDER-1001"}
	if _, err := investigationCase.AttachEvent("evidence-1", "event-1", "ORDER-1001", "bad", now); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("AttachEvent() error = %v, want %v", err, ErrInvalidEvidence)
	}
	investigationCase.Status = StatusClosed
	if _, err := investigationCase.AttachEvent("evidence-1", "event-1", "ORDER-1001", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("AttachEvent() closed error = %v, want %v", err, ErrInvalidTransition)
	}
}
