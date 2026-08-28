package evaluate_event_check

import (
	"context"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type fakeEventSource struct {
	result ports.CanonicalEventResult
	err    error
}

func (source fakeEventSource) FindCanonicalEvents(context.Context, ports.CanonicalEventQuery) (ports.CanonicalEventResult, error) {
	return source.result, source.err
}

func TestEvaluateResolvesScopeAndProducesStableHashes(t *testing.T) {
	from := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	events := fulfillmentEvents(from)
	watermark := from.Add(6 * time.Minute)
	service := NewService(fakeEventSource{result: ports.CanonicalEventResult{Events: events, Watermark: &watermark}})
	request := Request{
		Identifier: Identifier{Type: "CORRELATION_ID", Value: "ORDER-CHECK-01"},
		From:       from.Format(time.RFC3339), To: from.Add(10 * time.Minute).Format(time.RFC3339),
	}
	first, err := service.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ResolutionStatus != "EVALUATED" || first.Result == nil || first.Result.CheckStatus != "CONFORMANT" {
		t.Fatalf("unexpected evaluation: %#v", first)
	}
	if first.Model == nil || first.Model.ID != "order-fulfillment" || first.Model.Version != 2 {
		t.Fatalf("unexpected model: %#v", first.Model)
	}
	if len(first.Scope.Events) != len(events) || len(first.Scope.Relationships) != len(events) {
		t.Fatalf("unexpected scope: %#v", first.Scope)
	}
	if first.EventSetHash == nil || first.EvaluationHash == nil ||
		*first.EventSetHash != *second.EventSetHash || *first.EvaluationHash != *second.EvaluationHash {
		t.Fatalf("hashes are not stable: %#v / %#v", first, second)
	}
	if first.NormalizedRequest.Model == nil || first.NormalizedRequest.Model.ID != "order-fulfillment" {
		t.Fatalf("normalized request did not pin selected model: %#v", first.NormalizedRequest)
	}
	if first.Result.UnmappedEventIDs == nil {
		t.Fatal("empty unmapped_event_ids must be encoded as [] instead of null")
	}
	for _, global := range first.Result.GlobalChecks {
		if global.FindingCodes == nil {
			t.Fatal("empty finding_codes must be encoded as [] instead of null")
		}
	}
}

func TestEvaluateNoDataIsAProductStateWithoutHashes(t *testing.T) {
	from := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	service := NewService(fakeEventSource{result: ports.CanonicalEventResult{Events: []domain.Event{}}})
	result, err := service.Evaluate(context.Background(), Request{
		Identifier: Identifier{Type: "EVENT_ID", Value: "MISSING"},
		From:       from.Format(time.RFC3339), To: from.Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResolutionStatus != "NO_DATA" || result.EventSetHash != nil || result.EvaluationHash != nil {
		t.Fatalf("unexpected no-data result: %#v", result)
	}
	if result.Scope.Seeds == nil || result.Scope.Events == nil || result.IdentifierCandidates == nil || result.ModelCandidates == nil || result.Warnings == nil {
		t.Fatalf("required empty collections must not be nil: %#v", result)
	}
}

func TestEvaluateCustomExclusionPreservesReasonAndChangesOutcome(t *testing.T) {
	from := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	service := NewService(fakeEventSource{result: ports.CanonicalEventResult{Events: fulfillmentEvents(from)}})
	result, err := service.Evaluate(context.Background(), Request{
		Identifier: Identifier{Type: "CORRELATION_ID", Value: "ORDER-CHECK-01"},
		From:       from.Format(time.RFC3339), To: from.Add(10 * time.Minute).Format(time.RFC3339),
		Model:            &RequestedModel{ID: "order-fulfillment", Version: 2},
		ScopeAdjustments: &ScopeAdjustments{Include: []ScopeAdjustment{}, Exclude: []ScopeAdjustment{{EventID: "EVT-SHIPMENT", Reason: "測試不同調查假設"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scope.Mode != "CUSTOM_SCOPE" || len(result.Scope.ExcludedEvents) != 1 || result.Scope.ExcludedEvents[0].Reason != "測試不同調查假設" {
		t.Fatalf("custom scope provenance missing: %#v", result.Scope)
	}
	if result.Result == nil || result.Result.CheckStatus != "DEVIATED" {
		t.Fatalf("excluded shipment should mature the expectation as deviated: %#v", result.Result)
	}
}

func fulfillmentEvents(start time.Time) []domain.Event {
	traceID := "11111111111111111111111111111111"
	types := []struct {
		id, eventType, aggregateType, aggregateID string
		sequence                                  uint64
	}{
		{"EVT-ORDER", "OrderCreated", "Order", "ORDER-CHECK-01", 1},
		{"EVT-PAYMENT", "PaymentCompleted", "Payment", "PAY-CHECK-01", 1},
		{"EVT-SHIPMENT", "ShipmentCreated", "Shipment", "SHIP-CHECK-01", 1},
		{"EVT-DISPATCH", "ShipmentDispatched", "Shipment", "SHIP-CHECK-01", 2},
		{"EVT-DELIVERED", "ShipmentDelivered", "Shipment", "SHIP-CHECK-01", 3},
	}
	result := make([]domain.Event, 0, len(types))
	for index, value := range types {
		result = append(result, domain.Event{
			ID: value.id, Type: value.eventType, Version: 1, OccurredAt: start.Add(time.Duration(index) * time.Minute),
			Producer: "test-service", CorrelationID: "ORDER-CHECK-01", TraceID: &traceID,
			AggregateType: value.aggregateType, AggregateID: value.aggregateID, Sequence: value.sequence,
			KafkaTopic: "test.events", KafkaPartition: 0, KafkaOffset: uint64(index),
			Payload: map[string]any{"orderId": "ORDER-CHECK-01"},
		})
	}
	return result
}
