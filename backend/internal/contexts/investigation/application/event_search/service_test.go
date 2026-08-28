package eventsearch

import (
	"context"
	"errors"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
)

func TestEventSearchPatternRestrictsEvidenceEventTypes(t *testing.T) {
	readModel := &eventSearchReadModelFake{}
	service := NewEventSearchService(readModel, nil)

	_, err := service.Search(t.Context(), AdvancedEventSearchFilter{
		EventSearchFilter: validEventSearchFilter(),
		PatternID:         "payment-completed-without-shipment",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	for _, eventType := range []string{"PaymentCompleted", "ShipmentCreated", "OrderCancelled", "PaymentRefunded", "PaymentVoided"} {
		if !containsString(readModel.filter.EventTypes, eventType) {
			t.Errorf("pattern qualifier omitted %s: %#v", eventType, readModel.filter.EventTypes)
		}
	}
}

func TestEventSearchRejectsUnknownPatternWithoutReadingClickHouse(t *testing.T) {
	readModel := &eventSearchReadModelFake{}
	service := NewEventSearchService(readModel, nil)

	_, err := service.Search(t.Context(), AdvancedEventSearchFilter{
		EventSearchFilter: validEventSearchFilter(), PatternID: "runtime-pattern",
	})
	if !errors.Is(err, ErrUnknownPattern) || readModel.calls != 0 {
		t.Fatalf("error = %v, ClickHouse calls = %d", err, readModel.calls)
	}
}

func TestEventSearchIntersectsAlertAndSeverityCorrelations(t *testing.T) {
	readModel := &eventSearchReadModelFake{}
	qualifiers := &eventSearchQualifierFake{
		alerts:     []string{"ORDER-1", "ORDER-2"},
		severities: []string{"ORDER-2", "ORDER-3"},
	}
	service := NewEventSearchService(readModel, qualifiers)

	_, err := service.Search(t.Context(), AdvancedEventSearchFilter{
		EventSearchFilter: validEventSearchFilter(), AlertID: "fingerprint-1", MinimumSeverity: "high",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got := readModel.filter.CorrelationIDs; len(got) != 1 || got[0] != "ORDER-2" {
		t.Fatalf("qualified correlations = %#v, want [ORDER-2]", got)
	}
	if qualifiers.alertID != "fingerprint-1" || qualifiers.severity != "HIGH" {
		t.Fatalf("qualifier inputs = alert %q severity %q", qualifiers.alertID, qualifiers.severity)
	}
}

func TestEventSearchReturnsEmptyBeforeClickHouseWhenQualifiersDoNotMatch(t *testing.T) {
	readModel := &eventSearchReadModelFake{}
	service := NewEventSearchService(readModel, &eventSearchQualifierFake{})

	result, err := service.Search(t.Context(), AdvancedEventSearchFilter{
		EventSearchFilter: validEventSearchFilter(), AlertID: "missing",
	})
	if err != nil || len(result) != 0 || readModel.calls != 0 {
		t.Fatalf("result = %#v, error = %v, ClickHouse calls = %d", result, err, readModel.calls)
	}
}

func TestEventSearchRejectsInvalidMinimumSeverity(t *testing.T) {
	service := NewEventSearchService(&eventSearchReadModelFake{}, &eventSearchQualifierFake{})
	_, err := service.Search(t.Context(), AdvancedEventSearchFilter{
		EventSearchFilter: validEventSearchFilter(), MinimumSeverity: "urgent",
	})
	if !errors.Is(err, ErrInvalidSeverity) {
		t.Fatalf("error = %v, want ErrInvalidSeverity", err)
	}
}

func TestEventSearchPreservesQualifierDeadline(t *testing.T) {
	service := NewEventSearchService(&eventSearchReadModelFake{}, &eventSearchQualifierFake{err: context.DeadlineExceeded})
	_, err := service.Search(t.Context(), AdvancedEventSearchFilter{
		EventSearchFilter: validEventSearchFilter(), AlertID: "fingerprint-1",
	})
	if !errors.Is(err, ErrSearchQualifierSource) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want qualifier source and deadline errors", err)
	}
}

func validEventSearchFilter() forensics.EventSearchFilter {
	return forensics.EventSearchFilter{
		From:  time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 8, 20, 11, 6, 0, 0, time.UTC),
		Limit: 100,
	}
}

type eventSearchReadModelFake struct {
	filter EventSearchFilter
	calls  int
}

func (fake *eventSearchReadModelFake) Search(_ context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error) {
	fake.calls++
	fake.filter = filter
	return []forensics.ForensicsEvent{}, nil
}

func (fake *eventSearchReadModelFake) ProcessingSummaries(context.Context, []string) (map[string]forensics.ProcessingSummary, error) {
	return map[string]forensics.ProcessingSummary{}, nil
}

type eventSearchQualifierFake struct {
	alerts     []string
	severities []string
	alertID    string
	severity   string
	err        error
}

func (fake *eventSearchQualifierFake) CorrelationsByAlertFingerprint(_ context.Context, fingerprint string) ([]string, error) {
	fake.alertID = fingerprint
	return fake.alerts, fake.err
}

func (fake *eventSearchQualifierFake) CorrelationsByMinimumSeverity(_ context.Context, severity string) ([]string, error) {
	fake.severity = severity
	return fake.severities, fake.err
}
