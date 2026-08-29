package operations

import (
	"context"
	"strings"
	"sync"
	"time"
)

type SourceName string

const defaultOverviewWindow = 72 * time.Hour

const (
	SourcePostgres   SourceName = "postgresql"
	SourceClickHouse SourceName = "clickhouse"
	SourceTempo      SourceName = "tempo"
	SourceLoki       SourceName = "loki"
	SourceGrafana    SourceName = "grafana"
)

type SourceState string

const (
	SourceFresh       SourceState = "fresh"
	SourceStale       SourceState = "stale"
	SourcePartial     SourceState = "partial"
	SourceUnavailable SourceState = "unavailable"
)

type CountByKey struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type CaseCounts struct {
	Open          int64 `json:"open"`
	Investigating int64 `json:"investigating"`
	Closed        int64 `json:"closed"`
}

type SeverityCounts struct {
	Low      int64 `json:"low"`
	Medium   int64 `json:"medium"`
	High     int64 `json:"high"`
	Critical int64 `json:"critical"`
}

type RecentActivity struct {
	CasesCreated     int64 `json:"cases_created"`
	CasesClosed      int64 `json:"cases_closed"`
	GrafanaAlerts    int64 `json:"grafana_alerts"`
	ScenarioPassed   int64 `json:"scenario_passed"`
	ScenarioFailed   int64 `json:"scenario_failed"`
	ScenarioTimedOut int64 `json:"scenario_timed_out"`
}

type ControlPlaneSnapshot struct {
	Cases       CaseCounts     `json:"cases"`
	Severity    SeverityCounts `json:"severity"`
	Activity    RecentActivity `json:"activity"`
	TopPatterns []CountByKey   `json:"top_patterns"`
}

type EventSnapshot struct {
	EventCount                int64        `json:"event_count"`
	LatestEventAt             *time.Time   `json:"latest_event_at"`
	LatestProcessingAttemptAt *time.Time   `json:"latest_processing_attempt_at"`
	TopProducers              []CountByKey `json:"top_producers"`
	TopEventTypes             []CountByKey `json:"top_event_types"`
}

type Window struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type SourceHealth struct {
	Name          SourceName  `json:"name"`
	State         SourceState `json:"state"`
	LastSuccessAt *time.Time  `json:"last_success_at"`
	LagMS         *int64      `json:"lag_ms"`
	Reason        *string     `json:"reason"`
}

type Summary struct {
	GeneratedAt  time.Time             `json:"generated_at"`
	Window       Window                `json:"window"`
	Partial      bool                  `json:"partial"`
	Warnings     []string              `json:"warnings"`
	ControlPlane *ControlPlaneSnapshot `json:"control_plane"`
	Events       *EventSnapshot        `json:"events"`
	Sources      []SourceHealth        `json:"sources"`
}

type ControlPlaneReader interface {
	Overview(context.Context, time.Time, time.Time) (ControlPlaneSnapshot, error)
}

type EventReader interface {
	Overview(context.Context, time.Time, time.Time) (EventSnapshot, error)
}

type SourceProbe interface {
	Name() SourceName
	Check(context.Context) error
}

type Service struct {
	controlPlane ControlPlaneReader
	events       EventReader
	probes       []SourceProbe
	now          func() time.Time
	staleAfter   time.Duration
}

func NewService(controlPlane ControlPlaneReader, events EventReader, probes ...SourceProbe) *Service {
	return &Service{
		controlPlane: controlPlane,
		events:       events,
		probes:       probes,
		now:          time.Now,
		staleAfter:   5 * time.Minute,
	}
}

func (service *Service) Get(ctx context.Context) Summary {
	generatedAt := service.now().UTC()
	from := generatedAt.Add(-defaultOverviewWindow)
	result := Summary{
		GeneratedAt: generatedAt,
		Window:      Window{From: from, To: generatedAt},
		Warnings:    []string{},
		Sources:     make([]SourceHealth, 2+len(service.probes)),
	}

	var controlSnapshot ControlPlaneSnapshot
	var eventSnapshot EventSnapshot
	var controlErr error
	var eventErr error
	probeErrors := make([]error, len(service.probes))

	var wait sync.WaitGroup
	wait.Add(2 + len(service.probes))
	go func() {
		defer wait.Done()
		controlSnapshot, controlErr = service.controlPlane.Overview(ctx, from, generatedAt)
	}()
	go func() {
		defer wait.Done()
		eventSnapshot, eventErr = service.events.Overview(ctx, from, generatedAt)
	}()
	for index, probe := range service.probes {
		go func(index int, probe SourceProbe) {
			defer wait.Done()
			probeErrors[index] = probe.Check(ctx)
		}(index, probe)
	}
	wait.Wait()

	result.Sources[0] = currentSource(SourcePostgres, generatedAt, controlErr, "POSTGRES_QUERY_FAILED")
	if controlErr == nil {
		result.ControlPlane = &controlSnapshot
	} else {
		result.Partial = true
		result.Warnings = append(result.Warnings, "POSTGRES_OVERVIEW_UNAVAILABLE")
	}

	result.Sources[1] = service.eventSourceHealth(generatedAt, eventSnapshot, eventErr)
	if eventErr == nil {
		result.Events = &eventSnapshot
	} else {
		result.Partial = true
		result.Warnings = append(result.Warnings, "CLICKHOUSE_OVERVIEW_UNAVAILABLE")
	}

	for index, probe := range service.probes {
		health := currentSource(probe.Name(), generatedAt, probeErrors[index], "SOURCE_UNREACHABLE")
		result.Sources[index+2] = health
		if health.State != SourceFresh {
			result.Partial = true
			result.Warnings = append(result.Warnings, strings.ToUpper(string(probe.Name()))+"_UNAVAILABLE")
		}
	}
	if result.Sources[1].State != SourceFresh {
		result.Partial = true
		if eventErr == nil && result.Sources[1].Reason != nil {
			result.Warnings = append(result.Warnings, *result.Sources[1].Reason)
		}
	}
	return result
}

func currentSource(name SourceName, now time.Time, err error, reasonCode string) SourceHealth {
	if err != nil {
		reason := reasonCode
		return SourceHealth{Name: name, State: SourceUnavailable, Reason: &reason}
	}
	return SourceHealth{Name: name, State: SourceFresh, LastSuccessAt: &now}
}

func (service *Service) eventSourceHealth(now time.Time, snapshot EventSnapshot, err error) SourceHealth {
	if err != nil {
		return currentSource(SourceClickHouse, now, err, "CLICKHOUSE_QUERY_FAILED")
	}
	if snapshot.LatestEventAt == nil {
		reason := "NO_EVENTS_IN_WINDOW"
		return SourceHealth{Name: SourceClickHouse, State: SourcePartial, LastSuccessAt: &now, Reason: &reason}
	}
	eventLag := nonNegativeLag(now, *snapshot.LatestEventAt)
	health := SourceHealth{Name: SourceClickHouse, State: SourceFresh, LastSuccessAt: &now, LagMS: &eventLag}
	if time.Duration(eventLag)*time.Millisecond > service.staleAfter {
		reason := "EVENT_INGESTION_STALE"
		health.State = SourceStale
		health.Reason = &reason
		return health
	}
	if snapshot.LatestProcessingAttemptAt == nil && snapshot.EventCount > 0 {
		reason := "NO_PROCESSING_ATTEMPTS_IN_WINDOW"
		health.State = SourcePartial
		health.Reason = &reason
		return health
	}
	if snapshot.LatestProcessingAttemptAt != nil {
		processingLag := nonNegativeLag(now, *snapshot.LatestProcessingAttemptAt)
		if processingLag > eventLag {
			health.LagMS = &processingLag
		}
		if time.Duration(processingLag)*time.Millisecond > service.staleAfter {
			reason := "PROCESSING_ATTEMPTS_STALE"
			health.State = SourceStale
			health.Reason = &reason
		}
	}
	return health
}

func nonNegativeLag(now, observedAt time.Time) int64 {
	lag := now.Sub(observedAt).Milliseconds()
	if lag < 0 {
		return 0
	}
	return lag
}
