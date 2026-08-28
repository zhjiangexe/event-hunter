package fixtures

import (
	"path/filepath"
	"testing"
)

func TestLoadFixtures(t *testing.T) {
	fixtureRoot := filepath.Join("..", "..", "..", "..", "contracts", "fixtures")
	events, err := LoadFile(filepath.Join(fixtureRoot, "normal-order-flow.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("loaded %d events, want 3", len(events))
	}
	if events[0].EventType != "OrderCreated" || events[0].CorrelationID != "ORDER-1001" {
		t.Fatalf("first event = %#v, want OrderCreated / ORDER-1001", events[0])
	}

	allEvents, err := LoadDirectory(fixtureRoot)
	if err != nil {
		t.Fatalf("LoadDirectory() error = %v", err)
	}
	if len(allEvents) != 38 {
		t.Fatalf("loaded %d events, want 38", len(allEvents))
	}

	extendedEvents, err := LoadFile(filepath.Join(fixtureRoot, "extended-order-scenarios.json"))
	if err != nil {
		t.Fatalf("LoadFile(extended) error = %v", err)
	}
	if len(extendedEvents) != 23 {
		t.Fatalf("loaded %d extended events, want 23", len(extendedEvents))
	}
	eventTypes := map[string]bool{}
	for _, event := range extendedEvents {
		eventTypes[event.EventType] = true
	}
	for _, expected := range []string{
		"PaymentFailed", "ShipmentDispatchFailed", "ShipmentDispatched", "ShipmentInTransit",
		"ShipmentDelivered", "ReturnRequested", "ReturnReceived",
	} {
		if !eventTypes[expected] {
			t.Fatalf("extended fixtures do not contain %s", expected)
		}
	}
}
