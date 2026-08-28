package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type RelationType string

const (
	RelationSeed            RelationType = "SEED"
	RelationSameCorrelation RelationType = "SAME_CORRELATION"
	RelationSameAggregate   RelationType = "SAME_AGGREGATE"
	RelationCausation       RelationType = "CAUSATION"
	RelationBusinessKey     RelationType = "BUSINESS_KEY"
	RelationParentChild     RelationType = "PARENT_CHILD"
	RelationCustomInclude   RelationType = "CUSTOM_INCLUDE"
)

var (
	ErrInvalidScope       = errors.New("invalid event scope")
	ErrScopeLimitExceeded = errors.New("event scope limit exceeded")
	ErrSeedNotFound       = errors.New("scope seed event not found")
)

type ScopeAdjustment struct {
	EventID string
	Reason  string
}

type ScopeInput struct {
	From         time.Time
	To           time.Time
	SeedEventIDs []string
	Candidates   []Event
	Policy       ScopePolicy
	ModelID      string
	Include      []ScopeAdjustment
	Exclude      []ScopeAdjustment
}

type ScopeMode string

const (
	ScopeStandard ScopeMode = "STANDARD_SCOPE"
	ScopeCustom   ScopeMode = "CUSTOM_SCOPE"
)

type ExcludedEvent struct {
	Event  Event
	Reason string
}

type Relationship struct {
	Ordinal       int
	FromEventID   *string
	ToEventID     string
	Type          RelationType
	SourceField   *string
	SourceModelID *string
	SourceRuleID  *string
}

type ResolvedScope struct {
	Mode                ScopeMode
	SeedEventIDs        []string
	Events              []Event
	IncludedAdjustments []ScopeAdjustment
	ExcludedEvents      []ExcludedEvent
	Relationships       []Relationship
}

func ResolveScope(input ScopeInput) (ResolvedScope, error) {
	if err := validateScopeInput(input); err != nil {
		return ResolvedScope{}, err
	}
	ordered := append([]Event(nil), input.Candidates...)
	sortEvents(ordered)
	withinWindow := make([]Event, 0, len(ordered))
	byID := make(map[string][]Event)
	for _, event := range ordered {
		if event.OccurredAt.Before(input.From) || !event.OccurredAt.Before(input.To) {
			continue
		}
		withinWindow = append(withinWindow, event)
		byID[event.ID] = append(byID[event.ID], event)
	}

	excludedReasons := make(map[string]string, len(input.Exclude))
	for _, adjustment := range input.Exclude {
		if strings.TrimSpace(adjustment.EventID) == "" || strings.TrimSpace(adjustment.Reason) == "" {
			return ResolvedScope{}, fmt.Errorf("%w: exclusion requires event ID and reason", ErrInvalidScope)
		}
		excludedReasons[adjustment.EventID] = adjustment.Reason
	}
	includeReasons := make(map[string]string, len(input.Include))
	for _, adjustment := range input.Include {
		if strings.TrimSpace(adjustment.EventID) == "" || strings.TrimSpace(adjustment.Reason) == "" {
			return ResolvedScope{}, fmt.Errorf("%w: inclusion requires event ID and reason", ErrInvalidScope)
		}
		if _, excluded := excludedReasons[adjustment.EventID]; excluded {
			return ResolvedScope{}, fmt.Errorf("%w: event %s is both included and excluded", ErrInvalidScope, adjustment.EventID)
		}
		includeReasons[adjustment.EventID] = adjustment.Reason
	}

	included := make(map[string]bool)
	depth := make(map[string]int)
	queue := make([]string, 0, len(input.SeedEventIDs))
	relations := make([]Relationship, 0)
	for _, seedID := range input.SeedEventIDs {
		if len(byID[seedID]) == 0 {
			return ResolvedScope{}, fmt.Errorf("%w: %s", ErrSeedNotFound, seedID)
		}
		if _, excluded := excludedReasons[seedID]; excluded {
			return ResolvedScope{}, fmt.Errorf("%w: seed %s cannot be excluded", ErrInvalidScope, seedID)
		}
		if included[seedID] {
			continue
		}
		included[seedID] = true
		depth[seedID] = 0
		queue = append(queue, seedID)
		relations = append(relations, Relationship{Ordinal: len(relations), ToEventID: seedID, Type: RelationSeed})
	}

	for len(queue) > 0 {
		fromID := queue[0]
		queue = queue[1:]
		if depth[fromID] >= input.Policy.MaxRelationshipDepth {
			continue
		}
		from := byID[fromID][0]
		for _, candidate := range withinWindow {
			if included[candidate.ID] || excludedReasons[candidate.ID] != "" {
				continue
			}
			relation, sourceField, sourceRule, ok := explainRelation(from, candidate, input.Policy)
			if !ok {
				continue
			}
			included[candidate.ID] = true
			depth[candidate.ID] = depth[fromID] + 1
			queue = append(queue, candidate.ID)
			fromCopy := fromID
			modelCopy := input.ModelID
			relationship := Relationship{
				Ordinal: len(relations), FromEventID: &fromCopy, ToEventID: candidate.ID,
				Type: relation, SourceField: sourceField,
			}
			if input.ModelID != "" && (relation == RelationBusinessKey || relation == RelationParentChild) {
				relationship.SourceModelID = &modelCopy
				relationship.SourceRuleID = sourceRule
			}
			relations = append(relations, relationship)
		}
	}

	for eventID := range includeReasons {
		if len(byID[eventID]) == 0 {
			return ResolvedScope{}, fmt.Errorf("%w: custom include event %s not found", ErrInvalidScope, eventID)
		}
		if included[eventID] {
			continue
		}
		included[eventID] = true
		relations = append(relations, Relationship{
			Ordinal: len(relations), ToEventID: eventID, Type: RelationCustomInclude,
		})
	}

	events := make([]Event, 0)
	excluded := make([]ExcludedEvent, 0)
	for _, event := range withinWindow {
		if reason := excludedReasons[event.ID]; reason != "" {
			excluded = append(excluded, ExcludedEvent{Event: event, Reason: reason})
			continue
		}
		if included[event.ID] {
			events = append(events, event)
		}
	}
	if err := validateResolvedLimits(events, input.Policy); err != nil {
		return ResolvedScope{}, err
	}
	mode := ScopeStandard
	if len(input.Include) > 0 || len(input.Exclude) > 0 {
		mode = ScopeCustom
	}
	return ResolvedScope{
		Mode: mode, SeedEventIDs: append([]string(nil), input.SeedEventIDs...), Events: events,
		IncludedAdjustments: append([]ScopeAdjustment(nil), input.Include...),
		ExcludedEvents:      excluded, Relationships: relations,
	}, nil
}

func validateScopeInput(input ScopeInput) error {
	if input.From.IsZero() || input.To.IsZero() || !input.From.Before(input.To) {
		return fmt.Errorf("%w: from must be before to", ErrInvalidScope)
	}
	if input.To.Sub(input.From) > PlatformMaxDuration || input.To.Sub(input.From) > input.Policy.MaxDuration() {
		return fmt.Errorf("%w: time window", ErrScopeLimitExceeded)
	}
	if input.Policy.MaxDurationSeconds <= 0 || input.Policy.MaxDuration() > PlatformMaxDuration ||
		input.Policy.MaxEvents <= 0 || input.Policy.MaxEvents > PlatformMaxEvents ||
		input.Policy.MaxCorrelations <= 0 || input.Policy.MaxCorrelations > PlatformMaxCorrelations ||
		input.Policy.MaxRelationshipDepth < 0 || input.Policy.MaxRelationshipDepth > PlatformMaxRelationshipDepth {
		return fmt.Errorf("%w: model policy exceeds platform bounds", ErrInvalidScope)
	}
	if len(input.SeedEventIDs) == 0 {
		return fmt.Errorf("%w: at least one seed is required", ErrInvalidScope)
	}
	return nil
}

func validateResolvedLimits(events []Event, policy ScopePolicy) error {
	if len(events) > policy.MaxEvents {
		return fmt.Errorf("%w: events %d > %d", ErrScopeLimitExceeded, len(events), policy.MaxEvents)
	}
	correlations := make(map[string]struct{})
	for _, event := range events {
		correlations[event.CorrelationID] = struct{}{}
	}
	if len(correlations) > policy.MaxCorrelations {
		return fmt.Errorf("%w: correlations %d > %d", ErrScopeLimitExceeded, len(correlations), policy.MaxCorrelations)
	}
	return nil
}

func explainRelation(left, right Event, policy ScopePolicy) (RelationType, *string, *string, bool) {
	if relationEnabled(policy, RelationCausation) {
		if right.CausationID != nil && *right.CausationID == left.ID {
			field := "causation_id"
			return RelationCausation, &field, nil, true
		}
		if left.CausationID != nil && *left.CausationID == right.ID {
			field := "causation_id"
			return RelationCausation, &field, nil, true
		}
	}
	if relationEnabled(policy, RelationParentChild) {
		for _, relation := range policy.ParentChildRelations {
			if !aggregatePairMatches(left, right, relation) {
				continue
			}
			key, ok := businessKeyByName(policy.BusinessKeys, relation.BusinessKey)
			if !ok || !sameBusinessKey(left, right, key) {
				continue
			}
			field := key.JSONPointer
			rule := relation.ID
			return RelationParentChild, &field, &rule, true
		}
	}
	if relationEnabled(policy, RelationBusinessKey) {
		for _, key := range policy.BusinessKeys {
			if sameBusinessKey(left, right, key) {
				field := key.JSONPointer
				rule := key.Name
				return RelationBusinessKey, &field, &rule, true
			}
		}
	}
	if relationEnabled(policy, RelationSameAggregate) && left.AggregateType == right.AggregateType && left.AggregateID == right.AggregateID {
		field := "aggregate_type+aggregate_id"
		return RelationSameAggregate, &field, nil, true
	}
	if relationEnabled(policy, RelationSameCorrelation) && left.CorrelationID != "" && left.CorrelationID == right.CorrelationID {
		field := "correlation_id"
		return RelationSameCorrelation, &field, nil, true
	}
	return "", nil, nil, false
}

func relationEnabled(policy ScopePolicy, relation RelationType) bool {
	for _, candidate := range policy.Relations {
		if candidate == relation {
			return true
		}
	}
	return false
}

func aggregatePairMatches(left, right Event, relation ParentChildRelation) bool {
	return (left.AggregateType == relation.ParentAggregateType && right.AggregateType == relation.ChildAggregateType) ||
		(right.AggregateType == relation.ParentAggregateType && left.AggregateType == relation.ChildAggregateType)
}

func businessKeyByName(keys []BusinessKey, name string) (BusinessKey, bool) {
	for _, key := range keys {
		if key.Name == name {
			return key, true
		}
	}
	return BusinessKey{}, false
}

func sameBusinessKey(left, right Event, key BusinessKey) bool {
	if !containsString(key.EventTypes, left.Type) || !containsString(key.EventTypes, right.Type) {
		return false
	}
	leftValue, leftOK := payloadPointer(left.Payload, key.JSONPointer)
	rightValue, rightOK := payloadPointer(right.Payload, key.JSONPointer)
	return leftOK && rightOK && leftValue != "" && leftValue == rightValue
}

func payloadPointer(payload map[string]any, pointer string) (string, bool) {
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	if len(parts) == 0 {
		return "", false
	}
	if parts[0] == "payload" {
		parts = parts[1:]
	}
	var current any = payload
	for _, raw := range parts {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = object[part]
		if !ok {
			return "", false
		}
	}
	value, ok := current.(string)
	return value, ok
}

func sortEvents(events []Event) {
	sort.SliceStable(events, func(left, right int) bool {
		l, r := events[left], events[right]
		if !l.OccurredAt.Equal(r.OccurredAt) {
			return l.OccurredAt.Before(r.OccurredAt)
		}
		if l.AggregateType != r.AggregateType {
			return l.AggregateType < r.AggregateType
		}
		if l.AggregateID != r.AggregateID {
			return l.AggregateID < r.AggregateID
		}
		if l.Sequence != r.Sequence {
			return l.Sequence < r.Sequence
		}
		if l.KafkaTopic != r.KafkaTopic {
			return l.KafkaTopic < r.KafkaTopic
		}
		if l.KafkaPartition != r.KafkaPartition {
			return l.KafkaPartition < r.KafkaPartition
		}
		if l.KafkaOffset != r.KafkaOffset {
			return l.KafkaOffset < r.KafkaOffset
		}
		return l.ID < r.ID
	})
}
