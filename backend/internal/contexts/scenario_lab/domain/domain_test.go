package domain

import (
	"fmt"
	"testing"
)

func TestCatalogHasStableS1ThroughS14(t *testing.T) {
	items := Catalog()
	if len(items) != 14 {
		t.Fatalf("catalog length = %d", len(items))
	}
	for index, item := range items {
		want := fmt.Sprintf("S%d", index+1)
		if item.ID != want {
			t.Fatalf("catalog[%d] = %s", index, item.ID)
		}
		live := item.ID == "S1" || item.ID == "S12" || item.ID == "S13" || item.ID == "S14"
		if live && (item.ExecutionMode != LiveServices || item.Synthetic) {
			t.Fatalf("%s live contract changed", item.ID)
		}
		if !live && (item.ExecutionMode != LabInjection || !item.Synthetic) {
			t.Fatalf("%s injection contract changed", item.ID)
		}
	}
}
func TestEvaluateUsesObservedValues(t *testing.T) {
	actual := EmptyActual()
	actual.EventTypes = []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}
	actual.EventCount = 4
	actual.DuplicateEventIDs = []string{"event-payment"}
	if !ChecksPassed(Evaluate("S3", []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}, actual)) {
		t.Fatal("observed duplicate did not pass")
	}
	actual.DuplicateEventIDs = []string{}
	if ChecksPassed(Evaluate("S3", []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}, actual)) {
		t.Fatal("missing duplicate passed")
	}
}
func TestSchemaViolationRequiresFailureAndNoTimelineEvent(t *testing.T) {
	actual := EmptyActual()
	actual.IngestionFailureCount = 1
	actual.IngestionFailureTypes = []string{"SCHEMA_VIOLATION"}
	if !ChecksPassed(Evaluate("S6", nil, actual)) {
		t.Fatal("schema violation did not pass")
	}
	actual.EventCount = 1
	if ChecksPassed(Evaluate("S6", nil, actual)) {
		t.Fatal("schema violation passed with timeline event")
	}
}
