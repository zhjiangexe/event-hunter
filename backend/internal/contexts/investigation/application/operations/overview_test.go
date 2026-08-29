package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

type controlReaderStub struct {
	value ControlPlaneSnapshot
	err   error
}

func (stub controlReaderStub) Overview(context.Context, time.Time, time.Time) (ControlPlaneSnapshot, error) {
	return stub.value, stub.err
}

type eventReaderStub struct {
	value EventSnapshot
	err   error
}

func (stub eventReaderStub) Overview(context.Context, time.Time, time.Time) (EventSnapshot, error) {
	return stub.value, stub.err
}

type probeStub struct {
	name SourceName
	err  error
}

func (stub probeStub) Name() SourceName            { return stub.name }
func (stub probeStub) Check(context.Context) error { return stub.err }

func TestGetReturnsTraceableAggregateAndFreshSources(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	latest := now.Add(-time.Minute)
	service := NewService(
		controlReaderStub{value: ControlPlaneSnapshot{Cases: CaseCounts{Open: 3}, Activity: RecentActivity{GrafanaAlerts: 2}}},
		eventReaderStub{value: EventSnapshot{EventCount: 9, LatestEventAt: &latest, LatestProcessingAttemptAt: &latest}},
		probeStub{name: SourceTempo}, probeStub{name: SourceLoki}, probeStub{name: SourceGrafana},
	)
	service.now = func() time.Time { return now }

	result := service.Get(context.Background())

	if result.Partial {
		t.Fatal("expected complete overview")
	}
	if result.ControlPlane == nil || result.ControlPlane.Cases.Open != 3 {
		t.Fatalf("unexpected control-plane result: %#v", result.ControlPlane)
	}
	if result.Events == nil || result.Events.EventCount != 9 {
		t.Fatalf("unexpected event result: %#v", result.Events)
	}
	if result.Window.To.Sub(result.Window.From) != 72*time.Hour {
		t.Fatalf("overview window = %s to %s, want 72 hours", result.Window.From, result.Window.To)
	}
	for _, source := range result.Sources {
		if source.State != SourceFresh {
			t.Fatalf("expected %s to be fresh, got %s", source.Name, source.State)
		}
	}
}

func TestGetUsesNullBlocksWhenDependenciesFail(t *testing.T) {
	service := NewService(
		controlReaderStub{err: errors.New("postgres down")},
		eventReaderStub{err: errors.New("clickhouse down")},
		probeStub{name: SourceTempo, err: errors.New("tempo down")},
	)

	result := service.Get(context.Background())

	if !result.Partial {
		t.Fatal("expected partial overview")
	}
	if result.ControlPlane != nil || result.Events != nil {
		t.Fatalf("failed sources must remain null, got control=%#v events=%#v", result.ControlPlane, result.Events)
	}
	if result.Sources[0].State != SourceUnavailable || result.Sources[1].State != SourceUnavailable {
		t.Fatalf("unexpected source states: %#v", result.Sources)
	}
}

func TestGetMarksOldClickHouseDataStale(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	latest := now.Add(-10 * time.Minute)
	service := NewService(controlReaderStub{}, eventReaderStub{value: EventSnapshot{LatestEventAt: &latest}})
	service.now = func() time.Time { return now }

	result := service.Get(context.Background())

	if result.Sources[1].State != SourceStale || !result.Partial {
		t.Fatalf("expected stale partial ClickHouse source, got %#v", result.Sources[1])
	}
}

func TestGetMarksOldProcessingAttemptsStaleEvenWhenEventsAreFresh(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	latestEvent := now.Add(-time.Minute)
	latestAttempt := now.Add(-10 * time.Minute)
	service := NewService(controlReaderStub{}, eventReaderStub{value: EventSnapshot{
		EventCount: 2, LatestEventAt: &latestEvent, LatestProcessingAttemptAt: &latestAttempt,
	}})
	service.now = func() time.Time { return now }

	result := service.Get(context.Background())

	if result.Sources[1].State != SourceStale || result.Sources[1].Reason == nil || *result.Sources[1].Reason != "PROCESSING_ATTEMPTS_STALE" {
		t.Fatalf("expected stale processing attempts, got %#v", result.Sources[1])
	}
}
