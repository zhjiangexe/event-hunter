package domain

import (
	"errors"
	"testing"
	"time"
)

func TestResolveScopeExplainsCrossCorrelationBusinessKey(t *testing.T) {
	model, ok := LookupModel("order-fulfillment", 2)
	if !ok {
		t.Fatal("model not found")
	}
	orderID := "ORDER-SCOPE-1"
	order := scopeEvent("EVT-ORDER", "OrderCreated", "Order", orderID, orderID, time.Minute, map[string]any{"orderId": orderID})
	shipment := scopeEvent("EVT-SHIP", "ShipmentCreated", "Shipment", "SHIP-1", orderID, 2*time.Minute, map[string]any{"orderId": orderID})
	returned := scopeEvent("EVT-RETURN", "ReturnRequested", "Return", "RETURN-1", "RETURN-CORR-1", 3*time.Minute, map[string]any{"orderId": orderID})
	unrelated := scopeEvent("EVT-OTHER", "ReturnRequested", "Return", "RETURN-2", "RETURN-CORR-2", 3*time.Minute, map[string]any{"orderId": "ORDER-OTHER"})
	resolved, err := ResolveScope(ScopeInput{
		From: time.Unix(0, 0).UTC(), To: time.Unix(0, 0).UTC().Add(time.Hour), SeedEventIDs: []string{order.ID},
		Candidates: []Event{unrelated, returned, shipment, order}, Policy: model.Model.Scope, ModelID: model.Model.ID,
	})
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}
	if len(resolved.Events) != 3 {
		t.Fatalf("events = %v, want three related events", eventIDs(resolved.Events))
	}
	if containsString(eventIDs(resolved.Events), unrelated.ID) {
		t.Fatal("time-near unrelated event was included without an explainable relation")
	}
	foundBusinessKey := false
	for _, relation := range resolved.Relationships {
		if relation.ToEventID == returned.ID && relation.Type == RelationBusinessKey && relation.SourceField != nil && *relation.SourceField == "/payload/orderId" {
			foundBusinessKey = true
		}
	}
	if !foundBusinessKey {
		t.Fatalf("missing business-key provenance: %+v", resolved.Relationships)
	}
}

func TestResolveScopePreservesCustomReasons(t *testing.T) {
	model, _ := LookupModel("order-fulfillment", 2)
	base := time.Unix(0, 0).UTC()
	seed := scopeEvent("EVT-SEED", "OrderCreated", "Order", "ORDER-1", "CORR-1", time.Minute, map[string]any{"orderId": "ORDER-1"})
	related := scopeEvent("EVT-RELATED", "PaymentCompleted", "Payment", "PAY-1", "CORR-1", 2*time.Minute, map[string]any{"orderId": "ORDER-1"})
	manual := scopeEvent("EVT-MANUAL", "OrderCreated", "Order", "ORDER-2", "CORR-2", 3*time.Minute, map[string]any{"orderId": "ORDER-2"})
	resolved, err := ResolveScope(ScopeInput{
		From: base, To: base.Add(time.Hour), SeedEventIDs: []string{seed.ID}, Candidates: []Event{seed, related, manual},
		Policy: model.Model.Scope, ModelID: model.Model.ID,
		Include: []ScopeAdjustment{{EventID: manual.ID, Reason: "operator-confirmed external relation"}},
		Exclude: []ScopeAdjustment{{EventID: related.ID, Reason: "known replay duplicate"}},
	})
	if err != nil {
		t.Fatalf("resolve custom scope: %v", err)
	}
	if resolved.Mode != ScopeCustom || len(resolved.IncludedAdjustments) != 1 || resolved.IncludedAdjustments[0].Reason == "" {
		t.Fatalf("custom inclusion provenance lost: %+v", resolved)
	}
	if len(resolved.ExcludedEvents) != 1 || resolved.ExcludedEvents[0].Reason != "known replay duplicate" {
		t.Fatalf("custom exclusion provenance lost: %+v", resolved.ExcludedEvents)
	}
}

func TestResolveScopeRejectsModelLimitExpansion(t *testing.T) {
	policy := ScopePolicy{
		MaxDurationSeconds: int(PlatformMaxDuration/time.Second) + 1,
		MaxEvents:          1, MaxCorrelations: 1, MaxRelationshipDepth: 0,
	}
	_, err := ResolveScope(ScopeInput{
		From: time.Unix(0, 0).UTC(), To: time.Unix(0, 0).UTC().Add(time.Minute),
		SeedEventIDs: []string{"EVT"}, Candidates: []Event{{ID: "EVT", OccurredAt: time.Unix(1, 0).UTC()}}, Policy: policy,
	})
	if !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("error = %v, want ErrInvalidScope", err)
	}
}

func scopeEvent(id, eventType, aggregateType, aggregateID, correlationID string, offset time.Duration, payload map[string]any) Event {
	return Event{
		ID: id, Type: eventType, Version: 1, OccurredAt: time.Unix(0, 0).UTC().Add(offset),
		AggregateType: aggregateType, AggregateID: aggregateID, CorrelationID: correlationID,
		Sequence: 1, Payload: payload,
	}
}

func eventIDs(events []Event) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
