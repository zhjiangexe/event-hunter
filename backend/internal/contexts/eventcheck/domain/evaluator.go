package domain

import (
	"fmt"
	"sort"
	"time"
)

type EvaluateInput struct {
	PrimaryModel RegistryEntry
	Registry     []RegistryEntry
	Events       []Event
	AsOf         time.Time
	SourceHealth SourceHealth
}

type modelEvaluation struct {
	flow         FlowResult
	expectations []ExpectationResult
	findings     []Finding
	inconclusive bool
	deviated     bool
}

func Evaluate(input EvaluateInput) (EvaluationResult, error) {
	if input.PrimaryModel.Model.Kind != ModelKindFlow || input.PrimaryModel.Model.Status != ModelStatusActive {
		return EvaluationResult{}, fmt.Errorf("primary model %s@%d is not an active FLOW model", input.PrimaryModel.Model.ID, input.PrimaryModel.Model.Version)
	}
	if input.AsOf.IsZero() {
		return EvaluationResult{}, fmt.Errorf("evaluation as-of is required")
	}
	events := append([]Event(nil), input.Events...)
	sortEvents(events)
	root := evaluateModel(input.PrimaryModel, events, input.AsOf, input.SourceHealth, "ROOT")
	modelEvaluations := []modelEvaluation{root}
	activeModels := []RegistryEntry{input.PrimaryModel}

	for _, childRef := range input.PrimaryModel.Model.ChildModels {
		if !anyEventType(events, childRef.ActivateWhen.EventTypes) {
			continue
		}
		child, ok := lookupInRegistry(input.Registry, childRef.ModelID, childRef.Version)
		if !ok || child.Model.Status != ModelStatusActive || child.Model.Kind != ModelKindFlow {
			return EvaluationResult{}, fmt.Errorf("pinned child model %s@%d unavailable", childRef.ModelID, childRef.Version)
		}
		activeModels = append(activeModels, child)
		modelEvaluations = append(modelEvaluations, evaluateModel(child, events, input.AsOf, input.SourceHealth, "CHILD"))
	}

	result := EvaluationResult{
		CheckStatus: CheckInProgress, Expectations: []ExpectationResult{}, Flows: []FlowResult{},
		GlobalChecks: []GlobalCheckResult{}, Findings: []Finding{}, UnmappedEventIDs: []string{},
	}
	ambiguous := false
	inconclusive := false
	deviated := false
	allFlowsComplete := true
	for _, evaluation := range modelEvaluations {
		result.Flows = append(result.Flows, evaluation.flow)
		result.Expectations = append(result.Expectations, evaluation.expectations...)
		result.Findings = append(result.Findings, evaluation.findings...)
		ambiguous = ambiguous || evaluation.flow.Status == CheckAmbiguous
		inconclusive = inconclusive || evaluation.inconclusive
		deviated = deviated || evaluation.deviated
		allFlowsComplete = allFlowsComplete && evaluation.flow.Outcome != nil
		if evaluation.flow.Outcome != nil {
			result.BusinessOutcome = cloneOutcome(evaluation.flow.Outcome)
		}
	}

	for _, global := range input.Registry {
		if global.Model.Status != ModelStatusActive || global.Model.Kind != ModelKindGlobalCheck || !globalApplies(global.Model, events) {
			continue
		}
		globalResult, findings := evaluateGlobalCheck(global, events, input.SourceHealth)
		result.GlobalChecks = append(result.GlobalChecks, globalResult)
		result.Findings = append(result.Findings, findings...)
		if globalResult.Status == CheckDeviated {
			deviated = true
		}
		if globalResult.Status == CheckInconclusive {
			inconclusive = true
		}
	}

	result.UnmappedEventIDs = unmappedEventIDs(events, activeModels)
	if !ambiguous && result.BusinessOutcome == nil {
		result.BusinessOutcome = &Outcome{Code: "INCOMPLETE", Label: "尚未完成", Category: "INCOMPLETE"}
	}
	switch {
	case ambiguous:
		result.CheckStatus = CheckAmbiguous
		result.BusinessOutcome = nil
	case deviated:
		result.CheckStatus = CheckDeviated
	case inconclusive:
		result.CheckStatus = CheckInconclusive
	case allFlowsComplete:
		result.CheckStatus = CheckConformant
	default:
		result.CheckStatus = CheckInProgress
	}

	for index := range result.Flows {
		if result.Flows[index].Status == CheckAmbiguous {
			continue
		}
		if modelHasDeviation(result.Flows[index].ModelID, result.Findings) {
			result.Flows[index].Status = CheckDeviated
		} else if modelHasInconclusive(result.Flows[index].ModelID, modelEvaluations) {
			result.Flows[index].Status = CheckInconclusive
		}
	}
	sort.Slice(result.Findings, func(left, right int) bool {
		if result.Findings[left].Code == result.Findings[right].Code {
			return result.Findings[left].RuleID < result.Findings[right].RuleID
		}
		return result.Findings[left].Code < result.Findings[right].Code
	})
	return result, nil
}

func evaluateModel(entry RegistryEntry, events []Event, asOf time.Time, health SourceHealth, role string) modelEvaluation {
	flow := evaluatePaths(entry, events, role)
	evaluation := modelEvaluation{flow: flow, expectations: []ExpectationResult{}, findings: []Finding{}}
	for _, expectation := range entry.Model.Expectations {
		result, finding, triggered, inconclusive := evaluateExpectation(entry, expectation, events, asOf, health)
		if !triggered {
			continue
		}
		evaluation.expectations = append(evaluation.expectations, result)
		evaluation.inconclusive = evaluation.inconclusive || inconclusive
		if finding != nil {
			evaluation.findings = append(evaluation.findings, *finding)
			if result.State == ExpectationViolated || result.State == ExpectationLateSatisfied {
				evaluation.deviated = true
			}
		}
	}
	if flow.Status == CheckAmbiguous {
		return evaluation
	}
	if evaluation.deviated {
		evaluation.flow.Status = CheckDeviated
	} else if evaluation.inconclusive {
		evaluation.flow.Status = CheckInconclusive
	}
	return evaluation
}

func evaluatePaths(entry RegistryEntry, events []Event, role string) FlowResult {
	model := entry.Model
	nodes := make(map[string]FlowNode, len(model.Nodes))
	for _, node := range model.Nodes {
		nodes[node.ID] = node
	}
	candidates := make([]string, 0, len(model.Paths))
	completed := make([]FlowPath, 0)
	for _, path := range model.Paths {
		if anyEventType(events, path.ForbiddenEventTypes) {
			continue
		}
		candidates = append(candidates, path.ID)
		if pathComplete(path, nodes, events) {
			completed = append(completed, path)
		}
	}
	result := FlowResult{
		ModelID: model.ID, ModelVersion: model.Version, Role: role,
		Status: CheckInProgress, CandidatePathIDs: candidates,
	}
	if len(completed) > 1 {
		result.Status = CheckAmbiguous
		return result
	}
	if len(completed) == 1 {
		matched := completed[0].ID
		outcome := completed[0].Outcome
		result.MatchedPathID = &matched
		result.Outcome = &outcome
		result.Status = CheckConformant
	}
	return result
}

func pathComplete(path FlowPath, nodes map[string]FlowNode, events []Event) bool {
	cursor := -1
	for _, nodeID := range path.Nodes {
		node := nodes[nodeID]
		matches := make([]int, 0)
		for index := cursor + 1; index < len(events); index++ {
			if containsString(node.Event.EventTypes, events[index].Type) {
				matches = append(matches, index)
			}
		}
		if len(matches) < node.MinOccurs || len(matches) > node.MaxOccurs {
			return false
		}
		if len(matches) > 0 {
			cursor = matches[len(matches)-1]
		}
	}
	return true
}

func evaluateExpectation(entry RegistryEntry, expectation Expectation, events []Event, asOf time.Time, health SourceHealth) (ExpectationResult, *Finding, bool, bool) {
	triggers := eventsOfTypes(events, expectation.Trigger.EventTypes)
	if len(triggers) == 0 {
		return ExpectationResult{}, nil, false, false
	}
	trigger := triggers[0]
	result := ExpectationResult{ID: expectation.ID, TriggerEventIDs: []string{trigger.ID}, SatisfyingEventIDs: []string{}}
	reminderAt := trigger.OccurredAt.Add(time.Duration(expectation.ReminderAfterSeconds) * time.Second)
	deadlineAt := trigger.OccurredAt.Add(time.Duration(expectation.DeadlineSeconds) * time.Second)
	result.ReminderAt = &reminderAt
	result.DeadlineAt = &deadlineAt
	expected, found := findExpectedEvent(events, expectation, trigger)
	if found {
		result.SatisfyingEventIDs = []string{expected.ID}
		if expectation.TemporalRelation == TemporalAfterOrAt && expected.OccurredAt.After(deadlineAt) {
			result.State = ExpectationLateSatisfied
			return result, expectationFinding(entry, expectation, result.State, []string{trigger.ID, expected.ID}), true, false
		}
		result.State = ExpectationSatisfied
		return result, nil, true, false
	}
	if excludedAfterTrigger(events, expectation.Exclusions.AnyEventTypes, trigger.OccurredAt) {
		return ExpectationResult{}, nil, false, false
	}
	if !asOf.Before(deadlineAt) {
		if !health.SupportsAbsenceConclusion() {
			result.State = ExpectationWaiting
			result.Inconclusive = true
			return result, nil, true, true
		}
		result.State = ExpectationViolated
		return result, expectationFinding(entry, expectation, result.State, []string{trigger.ID}), true, false
	}
	if !asOf.Before(reminderAt) {
		result.State = ExpectationReminder
		return result, expectationFinding(entry, expectation, result.State, []string{trigger.ID}), true, false
	}
	result.State = ExpectationWaiting
	return result, nil, true, false
}

func expectationFinding(entry RegistryEntry, expectation Expectation, state ExpectationState, eventIDs []string) *Finding {
	stateCopy := state
	return &Finding{
		ModelID: entry.Model.ID, RuleKind: "FLOW_EXPECTATION", RuleID: expectation.ID, RuleVersion: entry.Model.Version,
		RuleChecksum: entry.Checksum, Severity: expectation.Severity, Code: expectation.FindingCode,
		ExpectationState: &stateCopy, EvidenceEventIDs: eventIDs,
		RecommendedQueryTemplateID: expectation.RecommendedQueryTemplateID,
	}
}

func findExpectedEvent(events []Event, expectation Expectation, trigger Event) (Event, bool) {
	for _, event := range events {
		if !containsString(expectation.Expected.EventTypes, event.Type) {
			continue
		}
		switch expectation.TemporalRelation {
		case TemporalBeforeOrAt:
			if !event.OccurredAt.After(trigger.OccurredAt) {
				return event, true
			}
		case TemporalAfterOrAt:
			if !event.OccurredAt.Before(trigger.OccurredAt) {
				return event, true
			}
		}
	}
	return Event{}, false
}

func excludedAfterTrigger(events []Event, exclusionTypes []string, triggerAt time.Time) bool {
	for _, event := range events {
		if containsString(exclusionTypes, event.Type) && !event.OccurredAt.Before(triggerAt) {
			return true
		}
	}
	return false
}

func evaluateGlobalCheck(entry RegistryEntry, events []Event, health SourceHealth) (GlobalCheckResult, []Finding) {
	result := GlobalCheckResult{ModelID: entry.Model.ID, ModelVersion: entry.Model.Version, Status: CheckConformant, FindingCodes: []string{}}
	findings := make([]Finding, 0)
	for _, rule := range entry.Model.Rules {
		evidence := globalRuleEvidence(rule.Type, events)
		if len(evidence) == 0 {
			continue
		}
		result.Status = CheckDeviated
		result.FindingCodes = append(result.FindingCodes, rule.FindingCode)
		findings = append(findings, Finding{
			ModelID: entry.Model.ID, RuleKind: "GLOBAL_CHECK", RuleID: rule.ID, RuleVersion: entry.Model.Version,
			RuleChecksum: entry.Checksum, Severity: rule.Severity, Code: rule.FindingCode,
			EvidenceEventIDs: evidence,
		})
	}
	if health.Status == SourceUnavailable {
		result.Status = CheckInconclusive
	}
	return result, findings
}

func globalRuleEvidence(ruleType string, events []Event) []string {
	switch ruleType {
	case "DUPLICATE_EVENT_ID":
		seen := make(map[string]bool)
		duplicates := make([]string, 0)
		for _, event := range events {
			if seen[event.ID] && !containsString(duplicates, event.ID) {
				duplicates = append(duplicates, event.ID)
			}
			seen[event.ID] = true
		}
		return duplicates
	case "NON_MONOTONIC_AGGREGATE_SEQUENCE":
		last := make(map[string]uint64)
		seenAggregate := make(map[string]bool)
		seenEventIDs := make(map[string]bool)
		violations := make([]string, 0)
		for _, event := range events {
			if seenEventIDs[event.ID] {
				continue
			}
			seenEventIDs[event.ID] = true
			key := event.AggregateType + "\x00" + event.AggregateID
			if seenAggregate[key] && event.Sequence <= last[key] {
				violations = append(violations, event.ID)
			}
			last[key] = event.Sequence
			seenAggregate[key] = true
		}
		return violations
	case "MISSING_TRACE_CONTEXT":
		missing := make([]string, 0)
		for _, event := range events {
			if event.TraceID == nil || *event.TraceID == "" {
				missing = append(missing, event.ID)
			}
		}
		return missing
	default:
		return nil
	}
}

func globalApplies(model CheckModel, events []Event) bool {
	return anyAggregateType(events, model.AppliesTo.AggregateTypes) && anySupportedTrigger(events, model)
}

func unmappedEventIDs(events []Event, models []RegistryEntry) []string {
	mappedTypes := make(map[string]bool)
	for _, entry := range models {
		for _, node := range entry.Model.Nodes {
			for _, eventType := range node.Event.EventTypes {
				mappedTypes[eventType] = true
			}
		}
		for _, expectation := range entry.Model.Expectations {
			for _, eventType := range expectation.Trigger.EventTypes {
				mappedTypes[eventType] = true
			}
			for _, eventType := range expectation.Expected.EventTypes {
				mappedTypes[eventType] = true
			}
			for _, eventType := range expectation.Exclusions.AnyEventTypes {
				mappedTypes[eventType] = true
			}
		}
	}
	ids := make([]string, 0)
	for _, event := range events {
		if !mappedTypes[event.Type] {
			ids = append(ids, event.ID)
		}
	}
	return ids
}

func eventsOfTypes(events []Event, eventTypes []string) []Event {
	result := make([]Event, 0)
	for _, event := range events {
		if containsString(eventTypes, event.Type) {
			result = append(result, event)
		}
	}
	return result
}

func lookupInRegistry(entries []RegistryEntry, id string, version int) (RegistryEntry, bool) {
	for _, entry := range entries {
		if entry.Model.ID == id && entry.Model.Version == version {
			return entry, true
		}
	}
	return RegistryEntry{}, false
}

func cloneOutcome(outcome *Outcome) *Outcome {
	if outcome == nil {
		return nil
	}
	copy := *outcome
	return &copy
}

func modelHasDeviation(modelID string, findings []Finding) bool {
	for _, finding := range findings {
		if finding.ModelID == modelID && finding.RuleKind == "FLOW_EXPECTATION" && finding.RuleChecksum != "" &&
			(finding.ExpectationState != nil && (*finding.ExpectationState == ExpectationViolated || *finding.ExpectationState == ExpectationLateSatisfied)) {
			return true
		}
	}
	return false
}

func modelHasInconclusive(modelID string, evaluations []modelEvaluation) bool {
	for _, evaluation := range evaluations {
		if evaluation.flow.ModelID == modelID && evaluation.inconclusive {
			return true
		}
	}
	return false
}
