package event

import (
	"encoding/json"
	"testing"
)

func TestNewEnvelopeUsesCanonicalJSONNames(t *testing.T) {
	envelope, err := NewEnvelope("OrderCreated", "order-service", "ORDER-1", "Order", "ORDER-1", 1, nil, nil, map[string]any{"orderId": "ORDER-1"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	data, err := envelope.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal = %v", err)
	}
	for _, key := range []string{"eventId", "eventType", "eventVersion", "occurredAt", "correlationId", "aggregateType", "aggregateId", "payload"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("canonical field %q missing from %s", key, data)
		}
	}
}
