package businessjourney

import (
	"context"
	"sort"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/contexts/investigation/domain/journeys"
)

type EventReader interface {
	Search(context.Context, forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error)
}

type Query struct {
	CorrelationID string
	From          time.Time
	To            time.Time
}

type Status string

const (
	StatusEmpty       Status = "EMPTY"
	StatusInProgress  Status = "IN_PROGRESS"
	StatusCompleted   Status = "COMPLETED"
	StatusFailed      Status = "FAILED"
	StatusCompensated Status = "COMPENSATED"
)

type MilestoneState string

const (
	MilestoneCompleted     MilestoneState = "COMPLETED"
	MilestoneInProgress    MilestoneState = "IN_PROGRESS"
	MilestoneFailed        MilestoneState = "FAILED"
	MilestoneCompensated   MilestoneState = "COMPENSATED"
	MilestoneNotApplicable MilestoneState = "NOT_APPLICABLE"
)

type EventReference struct {
	EventID       string  `json:"event_id"`
	EventType     string  `json:"event_type"`
	OccurredAt    string  `json:"occurred_at"`
	Producer      string  `json:"producer"`
	AggregateType string  `json:"aggregate_type"`
	AggregateID   string  `json:"aggregate_id"`
	TraceID       *string `json:"trace_id"`
}

type Milestone struct {
	ID                     string           `json:"id"`
	Label                  string           `json:"label"`
	State                  MilestoneState   `json:"state"`
	ExpectedEventTypes     []string         `json:"expected_event_types"`
	ActualEventTypes       []string         `json:"actual_event_types"`
	FirstEventAt           *string          `json:"first_event_at"`
	DurationFromPreviousMS *int64           `json:"duration_from_previous_ms"`
	Events                 []EventReference `json:"events"`
}

type Anomaly struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	EventIDs []string `json:"event_ids"`
}

type Journey struct {
	CorrelationID      string      `json:"correlation_id"`
	ProfileID          string      `json:"profile_id"`
	ProfileVersion     int         `json:"profile_version"`
	ProfileTitle       string      `json:"profile_title"`
	From               time.Time   `json:"from"`
	To                 time.Time   `json:"to"`
	Status             Status      `json:"status"`
	EventCount         int         `json:"event_count"`
	StartedAt          *string     `json:"started_at"`
	EndedAt            *string     `json:"ended_at"`
	DurationMS         *int64      `json:"duration_ms"`
	Milestones         []Milestone `json:"milestones"`
	Anomalies          []Anomaly   `json:"anomalies"`
	UnmappedEventCount int         `json:"unmapped_event_count"`
}

type Service struct {
	events  EventReader
	profile journeys.Profile
}

func NewService(events EventReader) *Service {
	profile, ok := journeys.Default()
	if !ok {
		panic("business journey requires one active default Journey Profile")
	}
	return NewServiceWithProfile(events, profile)
}

func NewServiceWithProfile(events EventReader, profile journeys.Profile) *Service {
	return &Service{events: events, profile: profile}
}

func (service *Service) Get(ctx context.Context, query Query) (Journey, error) {
	events, err := service.events.Search(ctx, forensics.EventSearchFilter{
		From: query.From, To: query.To, Limit: 1000, CorrelationID: query.CorrelationID,
	})
	if err != nil {
		return Journey{}, err
	}
	sort.SliceStable(events, func(left, right int) bool {
		return events[left].OccurredAt < events[right].OccurredAt
	})
	return build(query, events, service.profile), nil
}

func build(query Query, events []forensics.ForensicsEvent, profile journeys.Profile) Journey {
	journey := Journey{
		CorrelationID: query.CorrelationID, ProfileID: profile.ID, ProfileVersion: profile.Version, ProfileTitle: profile.Title,
		From: query.From, To: query.To,
		Status: StatusEmpty, EventCount: len(events), Milestones: make([]Milestone, 0, len(profile.Milestones)),
		Anomalies: []Anomaly{},
	}
	byType := make(map[string][]forensics.ForensicsEvent)
	for _, event := range events {
		byType[event.EventType] = append(byType[event.EventType], event)
	}

	var previousAt *time.Time
	for _, definition := range profile.Milestones {
		milestone := Milestone{
			ID: definition.ID, Label: definition.Label, State: MilestoneNotApplicable,
			ExpectedEventTypes: append([]string(nil), definition.ExpectedEventTypes...), ActualEventTypes: []string{}, Events: []EventReference{},
		}
		var firstAt *time.Time
		for _, event := range events {
			if !contains(definition.ExpectedEventTypes, event.EventType) {
				continue
			}
			milestone.ActualEventTypes = append(milestone.ActualEventTypes, event.EventType)
			milestone.Events = append(milestone.Events, eventReference(event))
			if occurredAt, parseErr := time.Parse(time.RFC3339Nano, event.OccurredAt); parseErr == nil && firstAt == nil {
				copy := occurredAt
				firstAt = &copy
				value := occurredAt.Format(time.RFC3339Nano)
				milestone.FirstEventAt = &value
			}
		}
		milestone.State = milestoneState(definition.StateRules, byType)
		if firstAt != nil && previousAt != nil {
			duration := firstAt.Sub(*previousAt).Milliseconds()
			milestone.DurationFromPreviousMS = &duration
		}
		if firstAt != nil {
			copy := *firstAt
			previousAt = &copy
		}
		journey.Milestones = append(journey.Milestones, milestone)
	}

	knownTypes := make(map[string]bool)
	for _, definition := range profile.Milestones {
		for _, eventType := range definition.ExpectedEventTypes {
			knownTypes[eventType] = true
		}
	}
	for _, event := range events {
		if !knownTypes[event.EventType] {
			journey.UnmappedEventCount++
		}
	}

	if len(events) > 0 {
		journey.StartedAt = stringPointer(events[0].OccurredAt)
		journey.EndedAt = stringPointer(events[len(events)-1].OccurredAt)
		if started, startErr := time.Parse(time.RFC3339Nano, events[0].OccurredAt); startErr == nil {
			if ended, endErr := time.Parse(time.RFC3339Nano, events[len(events)-1].OccurredAt); endErr == nil {
				duration := ended.Sub(started).Milliseconds()
				journey.DurationMS = &duration
			}
		}
	}

	journey.Anomalies = detectAnomalies(query.To, events, byType, profile)
	journey.Status = journeyStatus(events, byType, profile.JourneyStateRules)
	return journey
}

func milestoneState(rules []journeys.StateRule, byType map[string][]forensics.ForensicsEvent) MilestoneState {
	for _, rule := range rules {
		if stateRuleMatches(rule, byType) {
			return MilestoneState(rule.State)
		}
	}
	return MilestoneNotApplicable
}

func journeyStatus(events []forensics.ForensicsEvent, byType map[string][]forensics.ForensicsEvent, rules []journeys.StateRule) Status {
	if len(events) == 0 {
		return StatusEmpty
	}
	for _, rule := range rules {
		if stateRuleMatches(rule, byType) {
			return Status(rule.State)
		}
	}
	return StatusInProgress
}

func stateRuleMatches(rule journeys.StateRule, byType map[string][]forensics.ForensicsEvent) bool {
	return hasAny(byType, rule.WhenAnyEventTypes) && !hasAny(byType, rule.UnlessAnyEventTypes)
}

func detectAnomalies(windowEnd time.Time, events []forensics.ForensicsEvent, byType map[string][]forensics.ForensicsEvent, profile journeys.Profile) []Anomaly {
	anomalies := make([]Anomaly, 0)
	for _, rule := range profile.AnomalyRules {
		trigger, found := firstEventOfTypes(events, rule.TriggerEventTypes)
		if !found || hasAny(byType, rule.RequiredAnyEventTypes) || !graceElapsed(trigger, windowEnd, rule.GracePeriodSeconds) {
			continue
		}
		anomalies = append(anomalies, anomaly(
			rule.Code,
			rule.Severity,
			rule.Message,
			idsForTypes(events, rule.EvidenceEventTypes),
		))
	}
	if profile.DataQuality.DetectDuplicateEventIDs {
		seen := make(map[string]bool)
		duplicates := make([]string, 0)
		for _, event := range events {
			if seen[event.EventID] {
				duplicates = append(duplicates, event.EventID)
			}
			seen[event.EventID] = true
		}
		if len(duplicates) > 0 {
			anomalies = append(anomalies, anomaly("DUPLICATE_EVENT_ID", "MEDIUM", "同一 event ID 在查詢結果中出現多次。", duplicates))
		}
	}
	return anomalies
}

func graceElapsed(event forensics.ForensicsEvent, windowEnd time.Time, seconds int) bool {
	occurredAt, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
	return err == nil && !windowEnd.Before(occurredAt.Add(time.Duration(seconds)*time.Second))
}

func firstEventOfTypes(events []forensics.ForensicsEvent, eventTypes []string) (forensics.ForensicsEvent, bool) {
	for _, event := range events {
		if contains(eventTypes, event.EventType) {
			return event, true
		}
	}
	return forensics.ForensicsEvent{}, false
}

func idsForTypes(events []forensics.ForensicsEvent, eventTypes []string) []string {
	result := make([]string, 0)
	for _, event := range events {
		if contains(eventTypes, event.EventType) {
			result = append(result, event.EventID)
		}
	}
	return result
}

func eventReference(event forensics.ForensicsEvent) EventReference {
	return EventReference{
		EventID: event.EventID, EventType: event.EventType, OccurredAt: event.OccurredAt,
		Producer: event.Producer, AggregateType: event.AggregateType, AggregateID: event.AggregateID, TraceID: event.TraceID,
	}
}

func anomaly(code, severity, message string, eventIDs []string) Anomaly {
	return Anomaly{Code: code, Severity: severity, Message: message, EventIDs: eventIDs}
}

func has(byType map[string][]forensics.ForensicsEvent, eventType string) bool {
	return len(byType[eventType]) > 0
}

func hasAny(byType map[string][]forensics.ForensicsEvent, eventTypes []string) bool {
	for _, eventType := range eventTypes {
		if has(byType, eventType) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}
