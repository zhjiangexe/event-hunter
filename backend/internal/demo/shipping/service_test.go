package shipping

import "testing"

func TestDecodePaymentCompleted(t *testing.T) {
	decoded, err := DecodePaymentCompleted([]byte(`{"eventId":"22222222-2222-2222-2222-222222222222","eventType":"PaymentCompleted","eventVersion":1,"occurredAt":"2026-08-20T10:00:30Z","producer":"payment-service","correlationId":"ORDER-1","causationId":"11111111-1111-1111-1111-111111111111","traceId":null,"aggregateType":"Payment","aggregateId":"PAYMENT-1","sequence":1,"payload":{"paymentId":"PAYMENT-1","orderId":"ORDER-1","amount":1280,"currency":"TWD"}}`))
	if err != nil {
		t.Fatalf("DecodePaymentCompleted() error = %v", err)
	}
	if decoded.EventType != "PaymentCompleted" || decoded.Payload["orderId"] != "ORDER-1" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestDecodeRejectsOtherEventTypes(t *testing.T) {
	if _, err := DecodePaymentCompleted([]byte(`{"eventType":"OrderCreated"}`)); err == nil {
		t.Fatal("DecodePaymentCompleted() error = nil")
	}
}
