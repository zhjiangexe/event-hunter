package getinvestigationsummary

import (
	"context"
	"errors"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/caseview"
	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/contexts/investigation/application/investigationwindow"
)

const EventRetention = 90 * 24 * time.Hour

type CaseReader interface {
	GetSummaryDetails(ctx context.Context, id string) (caseview.SummaryDetails, error)
}

type EventReader interface {
	Search(ctx context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error)
	ProcessingSummaries(ctx context.Context, eventIDs []string) (map[string]forensics.ProcessingSummary, error)
}

type Request struct {
	InvestigationID string
	From            *time.Time
	To              *time.Time
	Limit           int
	IncludePayload  bool
}

type Result struct {
	Details                 caseview.SummaryDetails
	Events                  []forensics.ForensicsEvent
	ProcessingSummaries     map[string]forensics.ProcessingSummary
	GeneratedAt             time.Time
	From                    time.Time
	To                      time.Time
	Partial                 bool
	Warnings                []string
	EventRetentionBoundary  time.Time
	PostgresStatus          string
	ClickHouseStatus        string
	PostgresLastSuccessAt   *time.Time
	ClickHouseLastSuccessAt *time.Time
}

type Service struct {
	cases  CaseReader
	events EventReader
	now    func() time.Time
}

func NewService(cases CaseReader, events EventReader) *Service {
	return &Service{cases: cases, events: events, now: time.Now}
}

func (service *Service) Get(ctx context.Context, request Request) (Result, error) {
	details, err := service.cases.GetSummaryDetails(ctx, request.InvestigationID)
	if err != nil {
		return Result{}, err
	}
	generatedAt := service.now().UTC()
	from, to, err := investigationwindow.Resolve(request.From, request.To, details.Case.IncidentWindow)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Details: details, GeneratedAt: generatedAt, From: from, To: to,
		Events: []forensics.ForensicsEvent{}, ProcessingSummaries: map[string]forensics.ProcessingSummary{},
		Warnings: []string{}, EventRetentionBoundary: generatedAt.Add(-EventRetention),
		PostgresStatus: "OK", ClickHouseStatus: "OK", PostgresLastSuccessAt: timePointer(generatedAt),
	}
	events, err := service.events.Search(ctx, forensics.EventSearchFilter{
		From: from, To: to, Limit: request.Limit,
		CorrelationID: details.Case.CorrelationID, IncludePayload: request.IncludePayload,
	})
	if err != nil {
		result.markEventSourceFailure(err)
		return result, nil
	}
	if len(events) == 0 {
		result.ClickHouseLastSuccessAt = timePointer(generatedAt)
		return result, nil
	}
	summaries, err := service.events.ProcessingSummaries(ctx, uniqueEventIDs(events))
	if err != nil {
		result.markEventSourceFailure(err)
		return result, nil
	}
	result.Events = events
	result.ProcessingSummaries = summaries
	result.ClickHouseLastSuccessAt = timePointer(generatedAt)
	return result, nil
}

func (result *Result) markEventSourceFailure(err error) {
	result.Partial = true
	result.Events = []forensics.ForensicsEvent{}
	result.ProcessingSummaries = map[string]forensics.ProcessingSummary{}
	if errors.Is(err, context.DeadlineExceeded) {
		result.ClickHouseStatus = "TIMEOUT"
		result.Warnings = []string{"CLICKHOUSE_TIMEOUT"}
		return
	}
	result.ClickHouseStatus = "UNAVAILABLE"
	result.Warnings = []string{"CLICKHOUSE_UNAVAILABLE"}
}

func uniqueEventIDs(events []forensics.ForensicsEvent) []string {
	if len(events) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if _, exists := seen[event.EventID]; exists {
			continue
		}
		seen[event.EventID] = struct{}{}
		result = append(result, event.EventID)
	}
	return result
}

func timePointer(value time.Time) *time.Time { return &value }
