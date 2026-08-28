package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"
)

type fixtureSet struct {
	SourceHealthProfiles map[string]fixtureSourceHealth `json:"source_health_profiles"`
	Cases                []fixtureCase                  `json:"cases"`
}

type fixtureSourceHealth struct {
	Status    SourceHealthStatus `json:"status"`
	Truncated bool               `json:"truncated"`
}

type fixtureCase struct {
	ID                  string          `json:"id"`
	Request             fixtureRequest  `json:"request"`
	SourceHealthProfile string          `json:"source_health_profile"`
	Events              []fixtureEvent  `json:"events"`
	Expected            fixtureExpected `json:"expected"`
}

type fixtureRequest struct {
	Identifier struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"identifier"`
	From  string          `json:"from"`
	To    string          `json:"to"`
	Model *RequestedModel `json:"model"`
}

type fixtureEvent struct {
	EventID       string         `json:"eventId"`
	EventType     string         `json:"eventType"`
	EventVersion  int            `json:"eventVersion"`
	OccurredAt    string         `json:"occurredAt"`
	Producer      string         `json:"producer"`
	CorrelationID string         `json:"correlationId"`
	CausationID   *string        `json:"causationId"`
	TraceID       *string        `json:"traceId"`
	AggregateType string         `json:"aggregateType"`
	AggregateID   string         `json:"aggregateId"`
	Sequence      uint64         `json:"sequence"`
	Payload       map[string]any `json:"payload"`
}

type fixtureExpected struct {
	ResolutionStatus        ResolutionStatus            `json:"resolution_status"`
	CheckStatus             *CheckStatus                `json:"check_status"`
	BusinessOutcomeCategory *string                     `json:"business_outcome_category"`
	ExpectationStates       map[string]ExpectationState `json:"expectation_states"`
	FindingCodes            []string                    `json:"finding_codes"`
	ActivatedChildModels    []string                    `json:"activated_child_models"`
	GlobalFindingCodes      []string                    `json:"global_finding_codes"`
	UnmappedEventTypes      []string                    `json:"unmapped_event_types"`
}

func TestContractFixtures(t *testing.T) {
	fixtures := loadContractFixtures(t)
	engine := NewEngine(ActiveRegistry())
	for _, fixture := range fixtures.Cases {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			asOf := mustParseTime(t, fixture.Request.To)
			health := fixtures.SourceHealthProfiles[fixture.SourceHealthProfile]
			events := fixtureEvents(t, fixture.Events)
			actual, err := engine.Execute(ExecuteInput{
				IdentifierType: fixture.Request.Identifier.Type, IdentifierValue: fixture.Request.Identifier.Value,
				RequestedModel: fixture.Request.Model, Events: events, AsOf: asOf,
				SourceHealth: SourceHealth{Status: health.Status, Truncated: health.Truncated},
			})
			if err != nil {
				t.Fatalf("execute fixture: %v", err)
			}
			if actual.ResolutionStatus != fixture.Expected.ResolutionStatus {
				t.Fatalf("resolution status = %s, want %s", actual.ResolutionStatus, fixture.Expected.ResolutionStatus)
			}
			if actual.ResolutionStatus != ResolutionEvaluated {
				return
			}
			if actual.Result == nil {
				t.Fatal("evaluated result is nil")
			}
			if fixture.Expected.CheckStatus == nil || actual.Result.CheckStatus != *fixture.Expected.CheckStatus {
				t.Errorf("check status = %s, want %v", actual.Result.CheckStatus, fixture.Expected.CheckStatus)
			}
			assertOutcome(t, actual.Result.BusinessOutcome, fixture.Expected.BusinessOutcomeCategory)
			assertExpectationStates(t, actual.Result.Expectations, fixture.Expected.ExpectationStates)
			assertStringSet(t, "finding codes", FindingCodes(actual.Result.Findings), fixture.Expected.FindingCodes)
			assertStringSet(t, "child models", childModelIDs(actual.Result.Flows), fixture.Expected.ActivatedChildModels)
			assertStringSet(t, "global finding codes", globalFindingCodes(actual.Result.GlobalChecks), fixture.Expected.GlobalFindingCodes)
			assertStringSet(t, "unmapped event types", unmappedTypes(actual.Result.UnmappedEventIDs, events), fixture.Expected.UnmappedEventTypes)
		})
	}
}

func TestRegistryIsImmutableAndVersionAddressable(t *testing.T) {
	entries := Registry()
	if len(entries) != 4 {
		t.Fatalf("registry size = %d, want 4", len(entries))
	}
	entry, ok := LookupModel("order-fulfillment", 2)
	if !ok {
		t.Fatal("order-fulfillment@2 not found")
	}
	if entry.SourcePath != "contracts/check-models/order-fulfillment.yaml" || len(entry.Checksum) != 64 {
		t.Fatalf("unexpected registry provenance: %+v", entry)
	}
	entries[0].Model.Title = "mutated"
	again := Registry()
	if again[0].Model.Title == "mutated" {
		t.Fatal("Registry returned mutable shared state")
	}
}

func TestCandidateResolverRequiresSelectionForShipmentSeed(t *testing.T) {
	event := Event{ID: "EVT-1", Type: "ShipmentCreated", Version: 1, AggregateType: "Shipment"}
	candidates := ResolveModelCandidates(ActiveRegistry(), []Event{event}, []Event{event})
	if len(candidates) < 2 || candidates[0].Confidence != ConfidenceHigh || candidates[1].Confidence != ConfidenceHigh {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	if _, ok := RecommendedCandidate(candidates); ok {
		t.Fatal("two HIGH candidates must not be silently selected")
	}
}

func TestCandidateResolverRejectsUnsupportedEventVersion(t *testing.T) {
	event := Event{ID: "EVT-1", Type: "OrderCreated", Version: 99, AggregateType: "Order"}
	if candidates := ResolveModelCandidates(ActiveRegistry(), []Event{event}, []Event{event}); len(candidates) != 0 {
		t.Fatalf("unsupported schema version produced candidates: %+v", candidates)
	}
}

func TestExplicitIdentifierNeverFallsBackToUnrelatedEvent(t *testing.T) {
	engine := NewEngine(ActiveRegistry())
	actual, err := engine.Execute(ExecuteInput{
		IdentifierType: "EVENT_ID", IdentifierValue: "MISSING",
		Events: []Event{{ID: "UNRELATED", Type: "OrderCreated", Version: 1, AggregateType: "Order"}},
		AsOf:   time.Now(), SourceHealth: SourceHealth{Status: SourceHealthy},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if actual.ResolutionStatus != ResolutionNoData {
		t.Fatalf("resolution = %s, want NO_DATA", actual.ResolutionStatus)
	}
}

func loadContractFixtures(t *testing.T) fixtureSet {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../../contracts/event-check/fixtures/check-model-scenarios.json"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures fixtureSet
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	return fixtures
}

func fixtureEvents(t *testing.T, fixtures []fixtureEvent) []Event {
	t.Helper()
	events := make([]Event, 0, len(fixtures))
	for _, event := range fixtures {
		events = append(events, Event{
			ID: event.EventID, Type: event.EventType, Version: event.EventVersion,
			OccurredAt: mustParseTime(t, event.OccurredAt), Producer: event.Producer,
			CorrelationID: event.CorrelationID, CausationID: event.CausationID, TraceID: event.TraceID,
			AggregateType: event.AggregateType, AggregateID: event.AggregateID,
			Sequence: event.Sequence, Payload: event.Payload,
		})
	}
	return events
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func assertOutcome(t *testing.T, actual *Outcome, expected *string) {
	t.Helper()
	if expected == nil {
		if actual != nil {
			t.Errorf("business outcome = %+v, want nil", actual)
		}
		return
	}
	if actual == nil || actual.Category != *expected {
		t.Errorf("business outcome = %+v, want category %s", actual, *expected)
	}
}

func assertExpectationStates(t *testing.T, actual []ExpectationResult, expected map[string]ExpectationState) {
	t.Helper()
	states := make(map[string]ExpectationState, len(actual))
	for _, result := range actual {
		states[result.ID] = result.State
	}
	if !reflect.DeepEqual(states, expected) {
		t.Errorf("expectation states = %v, want %v", states, expected)
	}
}

func assertStringSet(t *testing.T, label string, actual, expected []string) {
	t.Helper()
	actual = append([]string(nil), actual...)
	expected = append([]string(nil), expected...)
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("%s = %v, want %v", label, actual, expected)
	}
}

func childModelIDs(flows []FlowResult) []string {
	ids := make([]string, 0)
	for _, flow := range flows {
		if flow.Role == "CHILD" {
			ids = append(ids, flow.ModelID)
		}
	}
	return ids
}

func globalFindingCodes(results []GlobalCheckResult) []string {
	codes := make([]string, 0)
	for _, result := range results {
		codes = append(codes, result.FindingCodes...)
	}
	return codes
}

func unmappedTypes(eventIDs []string, events []Event) []string {
	types := make([]string, 0)
	seen := make(map[string]bool)
	for _, eventID := range eventIDs {
		for _, event := range events {
			if event.ID == eventID && !seen[event.Type] {
				seen[event.Type] = true
				types = append(types, event.Type)
			}
		}
	}
	return types
}
