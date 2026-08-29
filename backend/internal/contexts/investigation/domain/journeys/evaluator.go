package journeys

import (
	"sort"
	"time"
)

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

type Event struct {
	EventID       string
	EventType     string
	OccurredAt    string
	Producer      string
	AggregateType string
	AggregateID   string
	TraceID       *string
}

type EventReference struct {
	EventID       string  `json:"event_id"`
	EventType     string  `json:"event_type"`
	OccurredAt    string  `json:"occurred_at"`
	Producer      string  `json:"producer"`
	AggregateType string  `json:"aggregate_type"`
	AggregateID   string  `json:"aggregate_id"`
	TraceID       *string `json:"trace_id"`
}

type EvaluatedMilestone struct {
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

type Evaluation struct {
	CorrelationID           string               `json:"correlation_id"`
	ProfileID               string               `json:"profile_id"`
	ProfileVersion          int                  `json:"profile_version"`
	ProfileTitle            string               `json:"profile_title"`
	From                    time.Time            `json:"from"`
	To                      time.Time            `json:"to"`
	Status                  Status               `json:"status"`
	EventCount              int                  `json:"event_count"`
	CompletedMilestoneCount int                  `json:"completed_milestone_count"`
	TotalMilestoneCount     int                  `json:"total_milestone_count"`
	CurrentMilestoneID      *string              `json:"current_milestone_id"`
	NextMilestoneID         *string              `json:"next_milestone_id"`
	NextExpectedEventTypes  []string             `json:"next_expected_event_types"`
	TraceIDs                []string             `json:"trace_ids"`
	StartedAt               *string              `json:"started_at"`
	EndedAt                 *string              `json:"ended_at"`
	DurationMS              *int64               `json:"duration_ms"`
	Milestones              []EvaluatedMilestone `json:"milestones"`
	Anomalies               []Anomaly            `json:"anomalies"`
	UnmappedEventCount      int                  `json:"unmapped_event_count"`
}

// Evaluate interprets observed facts with an immutable Journey Profile. It is
// deliberately pure: it does not read events, publish commands or advance a
// workflow, preserving Business Journey as a diagnostic projection.
func Evaluate(correlationID string, from, to time.Time, observed []Event, profile Profile) Evaluation {
	events := append([]Event(nil), observed...)
	sort.SliceStable(events, func(left, right int) bool { return events[left].OccurredAt < events[right].OccurredAt })
	result := Evaluation{
		CorrelationID: correlationID, ProfileID: profile.ID, ProfileVersion: profile.Version, ProfileTitle: profile.Title,
		From: from, To: to, Status: StatusEmpty, EventCount: len(events),
		Milestones: make([]EvaluatedMilestone, 0, len(profile.Milestones)), TotalMilestoneCount: len(profile.Milestones),
		NextExpectedEventTypes: []string{}, TraceIDs: []string{}, Anomalies: []Anomaly{},
	}
	byType := make(map[string][]Event)
	seenTraceIDs := make(map[string]bool)
	for _, event := range events {
		byType[event.EventType] = append(byType[event.EventType], event)
		if event.TraceID != nil && *event.TraceID != "" && !seenTraceIDs[*event.TraceID] {
			seenTraceIDs[*event.TraceID] = true
			result.TraceIDs = append(result.TraceIDs, *event.TraceID)
		}
	}

	var previousAt *time.Time
	for _, definition := range profile.Milestones {
		milestone := EvaluatedMilestone{
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
			if occurredAt, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err == nil && firstAt == nil {
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
		result.Milestones = append(result.Milestones, milestone)
	}

	knownTypes := make(map[string]bool)
	for _, definition := range profile.Milestones {
		for _, eventType := range definition.ExpectedEventTypes {
			knownTypes[eventType] = true
		}
	}
	for _, event := range events {
		if !knownTypes[event.EventType] {
			result.UnmappedEventCount++
		}
	}
	if len(events) > 0 {
		result.StartedAt = stringPointer(events[0].OccurredAt)
		result.EndedAt = stringPointer(events[len(events)-1].OccurredAt)
		if started, err := time.Parse(time.RFC3339Nano, events[0].OccurredAt); err == nil {
			if ended, err := time.Parse(time.RFC3339Nano, events[len(events)-1].OccurredAt); err == nil {
				duration := ended.Sub(started).Milliseconds()
				result.DurationMS = &duration
			}
		}
	}
	result.Anomalies = detectAnomalies(to, events, byType, profile)
	result.Status = journeyStatus(events, byType, profile.JourneyStateRules)
	deriveProgress(&result)
	return result
}

func deriveProgress(result *Evaluation) {
	currentIndex := -1
	for index, milestone := range result.Milestones {
		if milestone.State == MilestoneCompleted {
			result.CompletedMilestoneCount++
		}
		if currentIndex == -1 && milestone.State == MilestoneInProgress {
			currentIndex = index
		}
	}
	if currentIndex == -1 && (result.Status == StatusFailed || result.Status == StatusCompensated) {
		for index, milestone := range result.Milestones {
			if milestone.State == MilestoneFailed || milestone.State == MilestoneCompensated {
				currentIndex = index
				break
			}
		}
	}
	if currentIndex == -1 {
		return
	}
	current := result.Milestones[currentIndex]
	result.CurrentMilestoneID = stringPointer(current.ID)
	result.NextExpectedEventTypes = append([]string(nil), current.ExpectedEventTypes...)
	if currentIndex+1 < len(result.Milestones) {
		result.NextMilestoneID = stringPointer(result.Milestones[currentIndex+1].ID)
	}
}

func milestoneState(rules []StateRule, byType map[string][]Event) MilestoneState {
	for _, rule := range rules {
		if stateRuleMatches(rule, byType) {
			return MilestoneState(rule.State)
		}
	}
	return MilestoneNotApplicable
}

func journeyStatus(events []Event, byType map[string][]Event, rules []StateRule) Status {
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

func stateRuleMatches(rule StateRule, byType map[string][]Event) bool {
	return hasAny(byType, rule.WhenAnyEventTypes) && !hasAny(byType, rule.UnlessAnyEventTypes)
}

func detectAnomalies(windowEnd time.Time, events []Event, byType map[string][]Event, profile Profile) []Anomaly {
	result := make([]Anomaly, 0)
	for _, rule := range profile.AnomalyRules {
		trigger, found := firstEventOfTypes(events, rule.TriggerEventTypes)
		if !found || hasAny(byType, rule.RequiredAnyEventTypes) || !graceElapsed(trigger, windowEnd, rule.GracePeriodSeconds) {
			continue
		}
		result = append(result, Anomaly{Code: rule.Code, Severity: rule.Severity, Message: rule.Message, EventIDs: idsForTypes(events, rule.EvidenceEventTypes)})
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
			result = append(result, Anomaly{Code: "DUPLICATE_EVENT_ID", Severity: "MEDIUM", Message: "同一 event ID 在查詢結果中出現多次。", EventIDs: duplicates})
		}
	}
	return result
}

func graceElapsed(event Event, windowEnd time.Time, seconds int) bool {
	occurredAt, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
	return err == nil && !windowEnd.Before(occurredAt.Add(time.Duration(seconds)*time.Second))
}

func firstEventOfTypes(events []Event, eventTypes []string) (Event, bool) {
	for _, event := range events {
		if contains(eventTypes, event.EventType) {
			return event, true
		}
	}
	return Event{}, false
}

func idsForTypes(events []Event, eventTypes []string) []string {
	result := make([]string, 0)
	for _, event := range events {
		if contains(eventTypes, event.EventType) {
			result = append(result, event.EventID)
		}
	}
	return result
}

func eventReference(event Event) EventReference {
	return EventReference{EventID: event.EventID, EventType: event.EventType, OccurredAt: event.OccurredAt, Producer: event.Producer, AggregateType: event.AggregateType, AggregateID: event.AggregateID, TraceID: event.TraceID}
}

func hasAny(byType map[string][]Event, eventTypes []string) bool {
	for _, eventType := range eventTypes {
		if len(byType[eventType]) > 0 {
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

func stringPointer(value string) *string { return &value }
