package getinvestigationsummary

import (
	"context"
	"errors"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/caseview"
	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/contexts/investigation/domain"
)

func TestGetReturnsCaseAndEventSourceResult(t *testing.T) {
	generatedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	reader := &summaryEventReaderFake{events: []forensics.ForensicsEvent{{EventID: "event-1"}}, summaries: map[string]forensics.ProcessingSummary{"event-1": {AttemptCount: 2}}}
	service := NewService(summaryCaseReaderFake{details: caseview.SummaryDetails{CaseDetails: caseview.Details{Case: summaryCase("case-1", "ORDER-1")}}}, reader)
	service.now = func() time.Time { return generatedAt }

	from := generatedAt.Add(-time.Hour)
	result, err := service.Get(t.Context(), Request{InvestigationID: "case-1", From: &from, To: &generatedAt, Limit: 1000})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Partial || result.ClickHouseStatus != "OK" || len(result.Events) != 1 || result.ProcessingSummaries["event-1"].AttemptCount != 2 {
		t.Fatalf("result = %#v", result)
	}
	if reader.filter.CorrelationID != "ORDER-1" || reader.ids[0] != "event-1" || result.ClickHouseLastSuccessAt == nil {
		t.Fatalf("reader = %#v, result = %#v", reader, result)
	}
}

func TestGetKeepsPostgresDetailsWhenEventSourceTimesOut(t *testing.T) {
	service := NewService(
		summaryCaseReaderFake{details: caseview.SummaryDetails{CaseDetails: caseview.Details{Case: summaryCase("case-1", "ORDER-1")}}},
		&summaryEventReaderFake{searchErr: context.DeadlineExceeded},
	)
	result, err := service.Get(t.Context(), Request{InvestigationID: "case-1", Limit: 1000})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !result.Partial || result.ClickHouseStatus != "TIMEOUT" || len(result.Warnings) != 1 || result.Details.Case.ID != "case-1" {
		t.Fatalf("result = %#v", result)
	}
}

func summaryCase(id, correlationID string) domain.InvestigationCase {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	return domain.InvestigationCase{ID: id, CorrelationID: correlationID, IncidentWindow: domain.IncidentWindow{From: now.Add(-time.Hour), To: now, Source: domain.IncidentWindowTimelineSearch}}
}

func TestGetReturnsRequiredCaseReadFailure(t *testing.T) {
	want := errors.New("postgres unavailable")
	service := NewService(summaryCaseReaderFake{err: want}, &summaryEventReaderFake{})
	if _, err := service.Get(t.Context(), Request{}); !errors.Is(err, want) {
		t.Fatalf("Get() error = %v, want %v", err, want)
	}
}

type summaryCaseReaderFake struct {
	details caseview.SummaryDetails
	err     error
}

func (fake summaryCaseReaderFake) GetSummaryDetails(context.Context, string) (caseview.SummaryDetails, error) {
	return fake.details, fake.err
}

type summaryEventReaderFake struct {
	events     []forensics.ForensicsEvent
	summaries  map[string]forensics.ProcessingSummary
	searchErr  error
	summaryErr error
	filter     forensics.EventSearchFilter
	ids        []string
}

func (fake *summaryEventReaderFake) Search(_ context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error) {
	fake.filter = filter
	return fake.events, fake.searchErr
}

func (fake *summaryEventReaderFake) ProcessingSummaries(_ context.Context, ids []string) (map[string]forensics.ProcessingSummary, error) {
	fake.ids = ids
	return fake.summaries, fake.summaryErr
}
