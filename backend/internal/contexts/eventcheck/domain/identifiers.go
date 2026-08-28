package domain

import "sort"

type IdentifierQuery struct {
	Type            string
	Value           string
	AggregateType   string
	BusinessKeyName string
}

type IdentifierCandidate struct {
	Type       string
	ReasonCode string
}

type IdentifierResolution struct {
	SeedEvents        []Event
	Candidates        []IdentifierCandidate
	SelectionRequired bool
}

func ResolveIdentifier(query IdentifierQuery, registry []RegistryEntry, events []Event) IdentifierResolution {
	if query.Type != "AUTO" {
		matched := eventsForIdentifier(query, registry, events)
		sortEvents(matched)
		if len(matched) > 1 && query.Type != "EVENT_ID" {
			matched = matched[:1]
		}
		return IdentifierResolution{SeedEvents: matched}
	}
	types := []string{"EVENT_ID", "TRACE_ID", "CORRELATION_ID", "AGGREGATE_ID", "BUSINESS_KEY"}
	candidates := make([]IdentifierCandidate, 0)
	matches := make(map[string][]Event)
	for _, identifierType := range types {
		candidateQuery := query
		candidateQuery.Type = identifierType
		matched := eventsForIdentifier(candidateQuery, registry, events)
		if len(matched) == 0 {
			continue
		}
		matches[identifierType] = matched
		candidates = append(candidates, IdentifierCandidate{Type: identifierType, ReasonCode: "EXACT_INDEX_MATCH"})
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].Type < candidates[right].Type })
	if len(candidates) != 1 {
		return IdentifierResolution{Candidates: candidates, SelectionRequired: len(candidates) > 1}
	}
	matched := matches[candidates[0].Type]
	sortEvents(matched)
	if len(matched) > 1 && candidates[0].Type != "EVENT_ID" {
		matched = matched[:1]
	}
	return IdentifierResolution{SeedEvents: matched, Candidates: candidates}
}

func eventsForIdentifier(query IdentifierQuery, registry []RegistryEntry, events []Event) []Event {
	matched := make([]Event, 0)
	for _, event := range events {
		switch query.Type {
		case "EVENT_ID":
			if event.ID == query.Value {
				matched = append(matched, event)
			}
		case "TRACE_ID":
			if event.TraceID != nil && *event.TraceID == query.Value {
				matched = append(matched, event)
			}
		case "CORRELATION_ID":
			if event.CorrelationID == query.Value {
				matched = append(matched, event)
			}
		case "AGGREGATE_ID":
			if event.AggregateID == query.Value && (query.AggregateType == "" || event.AggregateType == query.AggregateType) {
				matched = append(matched, event)
			}
		case "BUSINESS_KEY":
			if eventHasBusinessKey(event, query.BusinessKeyName, query.Value, registry) {
				matched = append(matched, event)
			}
		}
	}
	return matched
}

func eventHasBusinessKey(event Event, keyName, value string, registry []RegistryEntry) bool {
	for _, entry := range registry {
		if entry.Model.Status != ModelStatusActive || entry.Model.Kind != ModelKindFlow {
			continue
		}
		for _, key := range entry.Model.Scope.BusinessKeys {
			if keyName != "" && key.Name != keyName {
				continue
			}
			if !containsString(key.EventTypes, event.Type) {
				continue
			}
			actual, ok := payloadPointer(event.Payload, key.JSONPointer)
			if ok && actual == value {
				return true
			}
		}
	}
	return false
}
