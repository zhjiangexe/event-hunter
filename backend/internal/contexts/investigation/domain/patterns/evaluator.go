package patterns

import (
	"sort"
	"time"
)

type Event struct {
	ID         string
	Type       string
	OccurredAt time.Time
	TraceID    *string
}

type Match struct {
	TriggerEvent Event
	WindowFrom   time.Time
	WindowTo     time.Time
	Conditions   []string
}

func Evaluate(definition Definition, events []Event, observedAt time.Time) (*Match, error) {
	window, err := definition.WindowDuration()
	if err != nil {
		return nil, err
	}
	ordered := append([]Event(nil), events...)
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].OccurredAt.Before(ordered[right].OccurredAt) })
	exclusions := make(map[string]struct{}, len(definition.ExclusionEventTypes))
	for _, eventType := range definition.ExclusionEventTypes {
		exclusions[eventType] = struct{}{}
	}
	for _, event := range ordered {
		if _, excluded := exclusions[event.Type]; excluded {
			return nil, nil
		}
	}
	for _, trigger := range ordered {
		if trigger.Type != definition.TriggerEventType {
			continue
		}
		deadline := trigger.OccurredAt.Add(window)
		if observedAt.Before(deadline) {
			continue
		}
		expected := false
		for _, event := range ordered {
			if event.Type == definition.ExpectedEventType && !event.OccurredAt.Before(trigger.OccurredAt) && !event.OccurredAt.After(deadline) {
				expected = true
				break
			}
		}
		if !expected {
			return &Match{TriggerEvent: trigger, WindowFrom: trigger.OccurredAt, WindowTo: deadline, Conditions: append([]string(nil), definition.MatchedConditionCodes...)}, nil
		}
	}
	return nil, nil
}
