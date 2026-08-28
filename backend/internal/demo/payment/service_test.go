package payment

import (
	"testing"
)

func TestDecodeOrderCreated(t *testing.T) {
	decoded, err := DecodeOrderCreated([]byte(`{"eventId":"11111111-1111-1111-1111-111111111111","eventType":"OrderCreated","eventVersion":1,"occurredAt":"2026-08-20T10:00:00Z","producer":"order-service","correlationId":"ORDER-1","causationId":null,"traceId":null,"aggregateType":"Order","aggregateId":"ORDER-1","sequence":1,"payload":{"orderId":"ORDER-1","customerId":"CUSTOMER-1","totalAmount":1280,"currency":"TWD"}}`))
	if err != nil {
		t.Fatalf("DecodeOrderCreated() error = %v", err)
	}
	if decoded.EventType != "OrderCreated" || decoded.Payload["orderId"] != "ORDER-1" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestDecodeRejectsOtherEventTypes(t *testing.T) {
	_, err := DecodeOrderCreated([]byte(`{"eventType":"PaymentCompleted"}`))
	if err == nil {
		t.Fatal("DecodeOrderCreated() error = nil")
	}
}
