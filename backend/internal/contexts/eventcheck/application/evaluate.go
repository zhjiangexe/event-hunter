package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/application/internal/canonicaljson"
	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

var (
	ErrInvalidRequest   = errors.New("invalid Event Check request")
	ErrModelUnavailable = errors.New("Check Model unavailable")
)

type EvaluateEventCheckHandler struct {
	source   ports.CanonicalEventSource
	registry []domain.RegistryEntry
	now      func() time.Time
}

func NewEvaluateEventCheckHandler(source ports.CanonicalEventSource) *EvaluateEventCheckHandler {
	return &EvaluateEventCheckHandler{source: source, registry: domain.ActiveRegistry(), now: time.Now}
}

func (service *EvaluateEventCheckHandler) Evaluate(ctx context.Context, input EvaluateRequest) (EvaluateResponse, error) {
	request, from, to, err := normalizeRequest(input)
	if err != nil {
		return EvaluateResponse{}, err
	}
	loaded, err := service.source.FindCanonicalEvents(ctx, ports.CanonicalEventQuery{
		From: from, To: to, Limit: domain.PlatformMaxEvents,
	})
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("find canonical events: %w", err)
	}
	health := service.sourceHealth(from, to, loaded)
	domainHealth := domain.SourceHealth{Status: domain.SourceHealthStatus(health.Status), Truncated: health.Truncated}
	response := EvaluateResponse{
		NormalizedRequest: request, SourceHealth: health, Scope: emptyScope(platformLimits()),
		IdentifierCandidates: []IdentifierCandidate{}, ModelCandidates: []ModelCandidate{}, Warnings: []string{},
	}
	if loaded.Truncated {
		response.Warnings = append(response.Warnings, "SOURCE_TRUNCATED")
	}

	query := domain.IdentifierQuery{Type: request.Identifier.Type, Value: request.Identifier.Value}
	if request.Identifier.Qualifier != nil {
		query.AggregateType = request.Identifier.Qualifier.AggregateType
		query.BusinessKeyName = request.Identifier.Qualifier.BusinessKeyName
	}
	identifier := domain.ResolveIdentifier(query, service.registry, loaded.Events)
	response.IdentifierCandidates = identifierCandidateDTOs(identifier.Candidates)
	if identifier.SelectionRequired {
		response.ResolutionStatus = string(domain.ResolutionIdentifierSelectionRequired)
		return response, nil
	}
	if len(identifier.SeedEvents) == 0 {
		response.ResolutionStatus = string(domain.ResolutionNoData)
		return response, nil
	}
	seedIDs := eventIDs(identifier.SeedEvents)
	response.Scope.Seeds = seedIDs
	response.Scope.Events, err = eventReferenceDTOs(identifier.SeedEvents)
	if err != nil {
		return EvaluateResponse{}, err
	}

	candidates := domain.ResolveModelCandidates(service.registry, identifier.SeedEvents, loaded.Events)
	response.ModelCandidates = modelCandidateDTOs(candidates)
	selected, status, err := service.selectModel(request.Model, candidates)
	if err != nil {
		return EvaluateResponse{}, err
	}
	if status != domain.ResolutionEvaluated {
		response.ResolutionStatus = string(status)
		return response, nil
	}

	include, exclude := domainAdjustments(request.ScopeAdjustments)
	resolved, err := domain.ResolveScope(domain.ScopeInput{
		From: from, To: to, SeedEventIDs: seedIDs, Candidates: loaded.Events,
		Policy: selected.Model.Scope, ModelID: selected.Model.ID, Include: include, Exclude: exclude,
	})
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	response.Scope, err = resolvedScopeDTO(resolved, selected.Model.Scope)
	if err != nil {
		return EvaluateResponse{}, err
	}
	evaluated, err := domain.Evaluate(domain.EvaluateInput{
		PrimaryModel: selected, Registry: service.registry, Events: resolved.Events,
		AsOf: to, SourceHealth: domainHealth,
	})
	if err != nil {
		return EvaluateResponse{}, err
	}
	model := modelRef(selected)
	result := checkResultDTO(evaluated, service.registry)
	request.Model = &RequestedModel{ID: selected.Model.ID, Version: selected.Model.Version}
	response.NormalizedRequest = request
	response.ResolutionStatus = string(domain.ResolutionEvaluated)
	response.Model = &model
	response.Result = &result
	eventSetHash, err := EventSetHash(response.Scope)
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("hash event scope: %w", err)
	}
	evaluationHash, err := EvaluationHash(request, model, eventSetHash, health, result)
	if err != nil {
		return EvaluateResponse{}, fmt.Errorf("hash evaluation: %w", err)
	}
	response.EventSetHash = &eventSetHash
	response.EvaluationHash = &evaluationHash
	return response, nil
}

func (service *EvaluateEventCheckHandler) selectModel(requested *RequestedModel, candidates []domain.ModelCandidate) (domain.RegistryEntry, domain.ResolutionStatus, error) {
	if requested != nil {
		for _, entry := range service.registry {
			if entry.Model.ID == requested.ID && entry.Model.Version == requested.Version &&
				entry.Model.Status == domain.ModelStatusActive && entry.Model.Kind == domain.ModelKindFlow {
				return entry, domain.ResolutionEvaluated, nil
			}
		}
		return domain.RegistryEntry{}, "", ErrModelUnavailable
	}
	if selected, ok := domain.RecommendedCandidate(candidates); ok {
		return selected, domain.ResolutionEvaluated, nil
	}
	if len(candidates) == 0 {
		return domain.RegistryEntry{}, domain.ResolutionNoApplicableModel, nil
	}
	return domain.RegistryEntry{}, domain.ResolutionModelSelectionRequired, nil
}

func normalizeRequest(input EvaluateRequest) (EvaluateRequest, time.Time, time.Time, error) {
	input.Identifier.Type = strings.ToUpper(strings.TrimSpace(input.Identifier.Type))
	input.Identifier.Value = strings.TrimSpace(input.Identifier.Value)
	validIdentifier := map[string]bool{"EVENT_ID": true, "TRACE_ID": true, "CORRELATION_ID": true, "AGGREGATE_ID": true, "BUSINESS_KEY": true, "AUTO": true}
	if !validIdentifier[input.Identifier.Type] || input.Identifier.Value == "" || len(input.Identifier.Value) > 200 {
		return EvaluateRequest{}, time.Time{}, time.Time{}, fmt.Errorf("%w: identifier", ErrInvalidRequest)
	}
	from, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.From))
	if err != nil {
		return EvaluateRequest{}, time.Time{}, time.Time{}, fmt.Errorf("%w: from", ErrInvalidRequest)
	}
	to, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.To))
	if err != nil || !from.Before(to) || to.Sub(from) > domain.PlatformMaxDuration {
		return EvaluateRequest{}, time.Time{}, time.Time{}, fmt.Errorf("%w: time window", ErrInvalidRequest)
	}
	input.From = from.UTC().Format(time.RFC3339Nano)
	input.To = to.UTC().Format(time.RFC3339Nano)
	if input.Identifier.Qualifier != nil {
		input.Identifier.Qualifier.AggregateType = strings.TrimSpace(input.Identifier.Qualifier.AggregateType)
		input.Identifier.Qualifier.BusinessKeyName = strings.TrimSpace(input.Identifier.Qualifier.BusinessKeyName)
		if input.Identifier.Qualifier.AggregateType == "" && input.Identifier.Qualifier.BusinessKeyName == "" {
			return EvaluateRequest{}, time.Time{}, time.Time{}, fmt.Errorf("%w: empty identifier qualifier", ErrInvalidRequest)
		}
	}
	if input.Model != nil {
		input.Model.ID = strings.TrimSpace(input.Model.ID)
		if input.Model.ID == "" || input.Model.Version < 1 {
			return EvaluateRequest{}, time.Time{}, time.Time{}, fmt.Errorf("%w: model", ErrInvalidRequest)
		}
	}
	if input.ScopeAdjustments != nil {
		if len(input.ScopeAdjustments.Include) > 100 || len(input.ScopeAdjustments.Exclude) > 100 {
			return EvaluateRequest{}, time.Time{}, time.Time{}, fmt.Errorf("%w: scope adjustment limit", ErrInvalidRequest)
		}
		for index := range input.ScopeAdjustments.Include {
			if err := normalizeAdjustment(&input.ScopeAdjustments.Include[index]); err != nil {
				return EvaluateRequest{}, time.Time{}, time.Time{}, err
			}
		}
		for index := range input.ScopeAdjustments.Exclude {
			if err := normalizeAdjustment(&input.ScopeAdjustments.Exclude[index]); err != nil {
				return EvaluateRequest{}, time.Time{}, time.Time{}, err
			}
		}
	}
	return input, from.UTC(), to.UTC(), nil
}

func normalizeAdjustment(adjustment *ScopeAdjustment) error {
	adjustment.EventID = strings.TrimSpace(adjustment.EventID)
	adjustment.Reason = strings.TrimSpace(adjustment.Reason)
	if adjustment.EventID == "" || len(adjustment.EventID) > 200 || adjustment.Reason == "" || len(adjustment.Reason) > 500 {
		return fmt.Errorf("%w: scope adjustment", ErrInvalidRequest)
	}
	return nil
}

func (service *EvaluateEventCheckHandler) sourceHealth(from, to time.Time, source ports.CanonicalEventResult) SourceHealth {
	status := string(domain.SourceHealthy)
	detail := "QUERY_COMPLETE"
	if source.Truncated {
		status = string(domain.SourcePartial)
		detail = "QUERY_LIMIT_REACHED"
	}
	var watermark *string
	watermarkDetail := "NO_RETAINED_EVENTS"
	if source.Watermark != nil {
		formatted := source.Watermark.UTC().Format(time.RFC3339Nano)
		watermark = &formatted
		watermarkDetail = "WATERMARK_OBSERVED"
	}
	return SourceHealth{
		Status: status, CheckedAt: service.now().UTC().Format(time.RFC3339Nano),
		CoverageFrom: from.UTC().Format(time.RFC3339Nano), CoverageTo: to.UTC().Format(time.RFC3339Nano),
		Watermark: watermark, Truncated: source.Truncated,
		Components: []SourceComponentHealth{
			{Component: "CANONICAL_EVENTS", Status: status, DetailCode: detail},
			{Component: "INGESTION_WATERMARK", Status: status, DetailCode: watermarkDetail},
			{Component: "RELATION_INDEX", Status: string(domain.SourceHealthy), DetailCode: "IN_MEMORY_RELATIONS_READY"},
		},
	}
}

func emptyScope(limits ScopeLimits) ResolvedScope {
	return ResolvedScope{Mode: string(domain.ScopeStandard), Seeds: []string{}, Events: []EventReference{}, ExcludedEvents: []ExcludedEventReference{}, Relationships: []Relationship{}, Limits: limits}
}

func domainAdjustments(adjustments *ScopeAdjustments) ([]domain.ScopeAdjustment, []domain.ScopeAdjustment) {
	if adjustments == nil {
		return nil, nil
	}
	convert := func(values []ScopeAdjustment) []domain.ScopeAdjustment {
		result := make([]domain.ScopeAdjustment, 0, len(values))
		for _, value := range values {
			result = append(result, domain.ScopeAdjustment{EventID: value.EventID, Reason: value.Reason})
		}
		return result
	}
	return convert(adjustments.Include), convert(adjustments.Exclude)
}

func eventIDs(events []domain.Event) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, event.ID)
	}
	return result
}

func identifierCandidateDTOs(values []domain.IdentifierCandidate) []IdentifierCandidate {
	result := make([]IdentifierCandidate, 0, len(values))
	for _, value := range values {
		result = append(result, IdentifierCandidate{Type: value.Type, Confidence: "HIGH", ReasonCode: value.ReasonCode})
	}
	return result
}

func modelCandidateDTOs(values []domain.ModelCandidate) []ModelCandidate {
	result := make([]ModelCandidate, 0, len(values))
	for _, value := range values {
		result = append(result, ModelCandidate{Model: modelRef(value.Entry), Confidence: string(value.Confidence), ReasonCodes: append([]string{}, value.ReasonCodes...)})
	}
	return result
}

func modelRef(entry domain.RegistryEntry) ModelRef {
	return ModelRef{ID: entry.Model.ID, Version: entry.Model.Version, Kind: string(entry.Model.Kind), SourcePath: entry.SourcePath, Checksum: entry.Checksum}
}

func eventReferenceDTO(event domain.Event, ordinal int) (EventReference, error) {
	payloadHash, err := canonicaljson.SHA256(event.Payload)
	if err != nil {
		return EventReference{}, fmt.Errorf("hash event %s payload: %w", event.ID, err)
	}
	return EventReference{
		EventID: event.ID, EventType: event.Type, EventVersion: event.Version,
		OccurredAt: event.OccurredAt.UTC().Format(time.RFC3339Nano), Producer: event.Producer,
		AggregateType: event.AggregateType, AggregateID: event.AggregateID, Sequence: event.Sequence,
		CorrelationID: event.CorrelationID, TraceID: event.TraceID, PayloadSHA256: payloadHash, Ordinal: ordinal,
	}, nil
}

func eventReferenceDTOs(events []domain.Event) ([]EventReference, error) {
	result := make([]EventReference, 0, len(events))
	for index, event := range events {
		mapped, err := eventReferenceDTO(event, index)
		if err != nil {
			return nil, err
		}
		result = append(result, mapped)
	}
	return result, nil
}

func resolvedScopeDTO(scope domain.ResolvedScope, policy domain.ScopePolicy) (ResolvedScope, error) {
	result := emptyScope(limitsFromPolicy(policy))
	result.Mode = string(scope.Mode)
	result.Seeds = append([]string{}, scope.SeedEventIDs...)
	var err error
	result.Events, err = eventReferenceDTOs(scope.Events)
	if err != nil {
		return ResolvedScope{}, err
	}
	for _, excluded := range scope.ExcludedEvents {
		mapped, err := eventReferenceDTO(excluded.Event, len(result.ExcludedEvents))
		if err != nil {
			return ResolvedScope{}, err
		}
		result.ExcludedEvents = append(result.ExcludedEvents, ExcludedEventReference{EventReference: mapped, Reason: excluded.Reason})
	}
	for _, relation := range scope.Relationships {
		result.Relationships = append(result.Relationships, Relationship{
			Ordinal: relation.Ordinal, FromEventID: relation.FromEventID, ToEventID: relation.ToEventID,
			RelationType: string(relation.Type), SourceField: relation.SourceField,
			SourceModelID: relation.SourceModelID, SourceRuleID: relation.SourceRuleID,
		})
	}
	return result, nil
}

func checkResultDTO(value domain.EvaluationResult, registry []domain.RegistryEntry) CheckResult {
	result := CheckResult{
		CheckStatus: string(value.CheckStatus), Expectations: []ExpectationResult{}, Flows: []FlowResult{},
		GlobalChecks: []GlobalCheckResult{}, Findings: []Finding{}, UnmappedEventIDs: append([]string{}, value.UnmappedEventIDs...),
		EvaluatorContractVersion: EvaluatorContractVersion, EvaluatorBuildVersion: EvaluatorBuildVersion,
	}
	if value.BusinessOutcome != nil {
		result.BusinessOutcome = &BusinessOutcome{Category: value.BusinessOutcome.Category, Code: value.BusinessOutcome.Code, Label: value.BusinessOutcome.Label}
	}
	for _, expectation := range value.Expectations {
		result.Expectations = append(result.Expectations, ExpectationResult{
			ID: expectation.ID, State: string(expectation.State), TriggerEventIDs: append([]string{}, expectation.TriggerEventIDs...),
			SatisfyingEventIDs: append([]string{}, expectation.SatisfyingEventIDs...), ReminderAt: formatTimePointer(expectation.ReminderAt), DeadlineAt: formatTimePointer(expectation.DeadlineAt),
		})
	}
	for _, flow := range value.Flows {
		entry, _ := lookupRegistry(registry, flow.ModelID, flow.ModelVersion)
		mapped := FlowResult{Model: modelRef(entry), Role: flow.Role, Status: string(flow.Status), CandidatePathIDs: append([]string{}, flow.CandidatePathIDs...), MatchedPathID: flow.MatchedPathID}
		if flow.Outcome != nil {
			mapped.Outcome = &BusinessOutcome{Category: flow.Outcome.Category, Code: flow.Outcome.Code, Label: flow.Outcome.Label}
		}
		result.Flows = append(result.Flows, mapped)
	}
	for _, global := range value.GlobalChecks {
		entry, _ := lookupRegistry(registry, global.ModelID, global.ModelVersion)
		result.GlobalChecks = append(result.GlobalChecks, GlobalCheckResult{Model: modelRef(entry), Status: string(global.Status), FindingCodes: append([]string{}, global.FindingCodes...)})
	}
	for _, finding := range value.Findings {
		var state *string
		if finding.ExpectationState != nil {
			formatted := string(*finding.ExpectationState)
			state = &formatted
		}
		var queryTemplate *string
		if finding.RecommendedQueryTemplateID != "" {
			formatted := finding.RecommendedQueryTemplateID
			queryTemplate = &formatted
		}
		evidence := make([]EvidenceReference, 0, len(finding.EvidenceEventIDs))
		for _, eventID := range finding.EvidenceEventIDs {
			evidence = append(evidence, EvidenceReference{Type: "EVENT", Value: eventID})
		}
		result.Findings = append(result.Findings, Finding{
			RuleKind: finding.RuleKind, RuleID: finding.RuleID, RuleVersion: finding.RuleVersion,
			RuleChecksum: finding.RuleChecksum, Severity: finding.Severity, Code: finding.Code,
			ExpectationState: state, EvidenceReferences: evidence, RecommendedQueryTemplateID: queryTemplate,
		})
	}
	sort.Strings(result.UnmappedEventIDs)
	result.EnsureCollections()
	return result
}

func lookupRegistry(registry []domain.RegistryEntry, id string, version int) (domain.RegistryEntry, bool) {
	for _, entry := range registry {
		if entry.Model.ID == id && entry.Model.Version == version {
			return entry, true
		}
	}
	return domain.RegistryEntry{}, false
}

func formatTimePointer(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func EventSetHash(scope ResolvedScope) (string, error) {
	included := make([]map[string]any, 0, len(scope.Events))
	for _, event := range scope.Events {
		included = append(included, map[string]any{
			"event_id": event.EventID, "event_type": event.EventType, "event_version": event.EventVersion,
			"occurred_at": event.OccurredAt, "producer": event.Producer, "aggregate_type": event.AggregateType,
			"aggregate_id": event.AggregateID, "sequence": event.Sequence, "correlation_id": event.CorrelationID,
			"trace_id": event.TraceID, "payload_sha256": event.PayloadSHA256, "ordinal": event.Ordinal,
		})
	}
	excluded := make([]map[string]any, 0, len(scope.ExcludedEvents))
	for _, event := range scope.ExcludedEvents {
		excluded = append(excluded, map[string]any{
			"event_id": event.EventID, "occurred_at": event.OccurredAt, "payload_sha256": event.PayloadSHA256,
			"ordinal": event.Ordinal, "reason": event.Reason,
		})
	}
	relations := make([]map[string]any, 0, len(scope.Relationships))
	for _, relation := range scope.Relationships {
		edge := map[string]any{
			"ordinal": relation.Ordinal, "from_event_id": relation.FromEventID,
			"to_event_id": relation.ToEventID, "relation_type": relation.RelationType,
		}
		if relation.SourceField != nil {
			edge["source_field"] = relation.SourceField
		}
		if relation.SourceModelID != nil {
			edge["source_model_id"] = relation.SourceModelID
		}
		if relation.SourceRuleID != nil {
			edge["source_rule_id"] = relation.SourceRuleID
		}
		relations = append(relations, edge)
	}
	return canonicaljson.SHA256(map[string]any{
		"scope_contract_version":                        1,
		"included_event_metadata_and_payload_checksums": included,
		"excluded_event_metadata_and_reasons":           excluded,
		"relationship_edges":                            relations,
	})
}

func EvaluationHash(request EvaluateRequest, model ModelRef, eventSetHash string, health SourceHealth, result CheckResult) (string, error) {
	return canonicaljson.SHA256(map[string]any{
		"evaluation_contract_version": EvaluatorContractVersion,
		"normalized_request":          request,
		"model_ref_and_checksum": map[string]any{
			"id": model.ID, "version": model.Version, "checksum": model.Checksum,
		},
		"event_set_hash":       eventSetHash,
		"source_health":        map[string]any{"status": health.Status, "truncated": health.Truncated},
		"deterministic_result": result,
	})
}
