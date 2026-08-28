package patterns

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDefinitionJSONExposesOnlyRegistryMetadata(t *testing.T) {
	definition, _ := Lookup("payment-completed-without-shipment")
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "trigger_event_type") || !strings.Contains(string(encoded), `"window":"PT5M"`) ||
		!strings.Contains(string(encoded), `"source_path":"contracts/patterns/payment-completed-without-shipment.yaml"`) ||
		!strings.Contains(string(encoded), `"fixture_coverage":{"match_count":1,"non_match_count":2,"total":3}`) {
		t.Fatalf("public definition JSON = %s", encoded)
	}
	if len(definition.Checksum) != 64 {
		t.Fatalf("checksum = %q", definition.Checksum)
	}
}

func TestEvaluateUsesTriggerRelativeWindow(t *testing.T) {
	definition, ok := Lookup("payment-completed-without-shipment")
	if !ok {
		t.Fatal("generated pattern is not registered")
	}
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	traceID := "trace-1"

	match, err := Evaluate(definition, []Event{{ID: "payment-1", Type: "PaymentCompleted", OccurredAt: triggeredAt, TraceID: &traceID}}, triggeredAt.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if match == nil || match.TriggerEvent.ID != "payment-1" || !match.WindowFrom.Equal(triggeredAt) || !match.WindowTo.Equal(triggeredAt.Add(5*time.Minute)) {
		t.Fatalf("match = %#v", match)
	}
}

func TestEvaluateDoesNotMatchBeforeWindowMatures(t *testing.T) {
	definition, _ := Lookup("payment-completed-without-shipment")
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	match, err := Evaluate(definition, []Event{{ID: "payment-1", Type: "PaymentCompleted", OccurredAt: triggeredAt}}, triggeredAt.Add(4*time.Minute))
	if err != nil || match != nil {
		t.Fatalf("match = %#v, error = %v", match, err)
	}
}

func TestEvaluateTreatsExpectedEventInsideWindowAsSuccess(t *testing.T) {
	definition, _ := Lookup("payment-completed-without-shipment")
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	events := []Event{
		{ID: "payment-1", Type: "PaymentCompleted", OccurredAt: triggeredAt},
		{ID: "shipment-1", Type: "ShipmentCreated", OccurredAt: triggeredAt.Add(4*time.Minute + 59*time.Second)},
	}
	match, err := Evaluate(definition, events, triggeredAt.Add(6*time.Minute))
	if err != nil || match != nil {
		t.Fatalf("match = %#v, error = %v", match, err)
	}
}

func TestEvaluateTreatsLateExpectedEventAsWindowViolation(t *testing.T) {
	definition, _ := Lookup("payment-completed-without-shipment")
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	events := []Event{
		{ID: "payment-1", Type: "PaymentCompleted", OccurredAt: triggeredAt},
		{ID: "shipment-1", Type: "ShipmentCreated", OccurredAt: triggeredAt.Add(5*time.Minute + time.Second)},
	}
	match, err := Evaluate(definition, events, triggeredAt.Add(6*time.Minute))
	if err != nil || match == nil {
		t.Fatalf("match = %#v, error = %v", match, err)
	}
}

func TestEvaluateExcludesTerminatedFlow(t *testing.T) {
	definition, _ := Lookup("payment-completed-without-shipment")
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	events := []Event{
		{ID: "payment-1", Type: "PaymentCompleted", OccurredAt: triggeredAt},
		{ID: "refund-1", Type: "PaymentRefunded", OccurredAt: triggeredAt.Add(time.Minute)},
	}
	match, err := Evaluate(definition, events, triggeredAt.Add(6*time.Minute))
	if err != nil || match != nil {
		t.Fatalf("match = %#v, error = %v", match, err)
	}
}
