package domain

import "sort"

type CandidateConfidence string

const (
	ConfidenceHigh   CandidateConfidence = "HIGH"
	ConfidenceMedium CandidateConfidence = "MEDIUM"
	ConfidenceLow    CandidateConfidence = "LOW"
)

type ModelCandidate struct {
	Entry       RegistryEntry
	Confidence  CandidateConfidence
	ReasonCodes []string
	Score       int
}

func ResolveModelCandidates(entries []RegistryEntry, seedEvents, scopeEvents []Event) []ModelCandidate {
	candidates := make([]ModelCandidate, 0)
	for _, entry := range entries {
		model := entry.Model
		if model.Status != ModelStatusActive || model.Kind != ModelKindFlow {
			continue
		}
		if !anySupportedTrigger(scopeEvents, model) {
			continue
		}
		score := 0
		reasons := make([]string, 0, 4)
		if anyAggregateType(seedEvents, model.AppliesTo.AggregateTypes) {
			score += 100
			reasons = append(reasons, "SEED_AGGREGATE_MATCH")
		}
		if anySupportedTrigger(seedEvents, model) {
			score += 100
			reasons = append(reasons, "SEED_TRIGGER_MATCH")
		}
		if anyAggregateType(scopeEvents, model.AppliesTo.AggregateTypes) {
			score += 20
			reasons = append(reasons, "SCOPE_AGGREGATE_MATCH")
		}
		if anySupportedTrigger(scopeEvents, model) {
			score += 20
			reasons = append(reasons, "SCOPE_TRIGGER_MATCH")
		}
		confidence := ConfidenceLow
		switch {
		case score >= 200:
			confidence = ConfidenceHigh
		case score >= 100:
			confidence = ConfidenceMedium
		}
		candidates = append(candidates, ModelCandidate{
			Entry: entry, Confidence: confidence, ReasonCodes: reasons, Score: score,
		})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].Score == candidates[right].Score {
			if candidates[left].Entry.Model.ID == candidates[right].Entry.Model.ID {
				return candidates[left].Entry.Model.Version < candidates[right].Entry.Model.Version
			}
			return candidates[left].Entry.Model.ID < candidates[right].Entry.Model.ID
		}
		return candidates[left].Score > candidates[right].Score
	})
	return candidates
}

func anySupportedTrigger(events []Event, model CheckModel) bool {
	for _, event := range events {
		if containsString(model.AppliesTo.TriggerEventTypes, event.Type) && supportsEventVersion(model, event.Type, event.Version) {
			return true
		}
	}
	return false
}

func supportsEventVersion(model CheckModel, eventType string, version int) bool {
	for _, support := range model.AppliesTo.EventVersions {
		if support.EventType != eventType {
			continue
		}
		for _, supportedVersion := range support.Versions {
			if supportedVersion == version {
				return true
			}
		}
	}
	return false
}

func RecommendedCandidate(candidates []ModelCandidate) (RegistryEntry, bool) {
	if len(candidates) == 0 || candidates[0].Confidence != ConfidenceHigh {
		return RegistryEntry{}, false
	}
	highCount := 0
	for _, candidate := range candidates {
		if candidate.Confidence == ConfidenceHigh {
			highCount++
		}
	}
	if highCount != 1 {
		return RegistryEntry{}, false
	}
	return candidates[0].Entry, true
}

func anyEventType(events []Event, allowed []string) bool {
	for _, event := range events {
		if containsString(allowed, event.Type) {
			return true
		}
	}
	return false
}

func anyAggregateType(events []Event, allowed []string) bool {
	for _, event := range events {
		if containsString(allowed, event.AggregateType) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
