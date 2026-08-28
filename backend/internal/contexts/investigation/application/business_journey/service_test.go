package businessjourney

import (
	"context"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/contexts/investigation/domain/journeys"
)

type eventReaderStub struct {
	events []forensics.ForensicsEvent
}

func TestGetUsesInjectedJourneyProfileInsteadOfLogisticsConstants(t *testing.T) {
	profile := journeys.Profile{
		ID: "custom-flow", Version: 7, Title: "Custom Flow",
		Milestones: []journeys.Milestone{{
			ID: "CUSTOM", Label: "自訂步驟", ExpectedEventTypes: []string{"CustomCompleted"},
			StateRules: []journeys.StateRule{{State: "COMPLETED", WhenAnyEventTypes: []string{"CustomCompleted"}}},
		}},
		JourneyStateRules: []journeys.StateRule{{State: "COMPLETED", WhenAnyEventTypes: []string{"CustomCompleted"}}},
		AnomalyRules: []journeys.AnomalyRule{{
			Code: "CUSTOM_SUCCESSOR_MISSING", Severity: "MEDIUM", Message: "缺少自訂後續事件。",
			TriggerEventTypes: []string{"CustomCompleted"}, RequiredAnyEventTypes: []string{"CustomArchived"},
			EvidenceEventTypes: []string{"CustomCompleted"}, GracePeriodSeconds: 0,
		}},
	}
	service := NewServiceWithProfile(eventReaderStub{events: []forensics.ForensicsEvent{
		event("custom-1", "CustomCompleted", "2026-08-20T11:00:00Z", "custom-service"),
	}}, profile)

	result, err := service.Get(context.Background(), journeyQuery())
	if err != nil {
		t.Fatal(err)
	}
	if result.ProfileID != "custom-flow" || result.ProfileVersion != 7 || result.Status != StatusCompleted {
		t.Fatalf("unexpected profile result: %#v", result)
	}
	if len(result.Milestones) != 1 || result.Milestones[0].ID != "CUSTOM" || result.Milestones[0].State != MilestoneCompleted {
		t.Fatalf("unexpected custom milestone: %#v", result.Milestones)
	}
	if len(result.Anomalies) != 1 || result.Anomalies[0].Code != "CUSTOM_SUCCESSOR_MISSING" {
		t.Fatalf("unexpected custom anomalies: %#v", result.Anomalies)
	}
}

func (stub eventReaderStub) Search(context.Context, forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error) {
	return stub.events, nil
}

func TestGetBuildsCompletedDeliveredJourney(t *testing.T) {
	service := NewService(eventReaderStub{events: []forensics.ForensicsEvent{
		event("delivered-1", "ShipmentDelivered", "2026-08-20T10:05:00Z", "shipping-service"),
		event("shipment-1", "ShipmentCreated", "2026-08-20T10:03:00Z", "shipping-service"),
		event("order-1", "OrderCreated", "2026-08-20T10:00:00Z", "order-service"),
		event("payment-1", "PaymentCompleted", "2026-08-20T10:00:30Z", "payment-service"),
	}})
	result, err := service.Get(context.Background(), journeyQuery())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || result.EventCount != 4 || len(result.Anomalies) != 0 {
		t.Fatalf("unexpected journey: %#v", result)
	}
	if result.Milestones[2].State != MilestoneCompleted || *result.Milestones[2].DurationFromPreviousMS != 150000 {
		t.Fatalf("unexpected shipping milestone: %#v", result.Milestones[2])
	}
}

func TestGetKeepsCreatedShipmentInProgressUntilDelivery(t *testing.T) {
	service := NewService(eventReaderStub{events: []forensics.ForensicsEvent{
		event("order-1", "OrderCreated", "2026-08-20T10:00:00Z", "order-service"),
		event("payment-1", "PaymentCompleted", "2026-08-20T10:00:30Z", "payment-service"),
		event("shipment-1", "ShipmentCreated", "2026-08-20T10:03:00Z", "shipping-service"),
	}})
	result, err := service.Get(context.Background(), journeyQuery())
	if err != nil || result.Status != StatusInProgress || result.Milestones[3].State != MilestoneInProgress {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestGetReportsOverdueShipmentWithoutPretendingJourneyCompleted(t *testing.T) {
	service := NewService(eventReaderStub{events: []forensics.ForensicsEvent{
		event("order-1", "OrderCreated", "2026-08-20T11:00:00Z", "order-service"),
		event("payment-1", "PaymentCompleted", "2026-08-20T11:00:30Z", "payment-service"),
	}})
	result, err := service.Get(context.Background(), journeyQuery())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusInProgress || result.Milestones[2].State != MilestoneInProgress {
		t.Fatalf("unexpected journey state: %#v", result)
	}
	if len(result.Anomalies) != 1 || result.Anomalies[0].Code != "MISSING_SHIPMENT_AFTER_PAYMENT" {
		t.Fatalf("unexpected anomalies: %#v", result.Anomalies)
	}
}

func TestGetReportsFailedAndCompensatedOutcomes(t *testing.T) {
	failed := NewService(eventReaderStub{events: []forensics.ForensicsEvent{
		event("order-1", "OrderCreated", "2026-08-20T11:00:00Z", "order-service"),
		event("payment-1", "PaymentFailed", "2026-08-20T11:00:30Z", "payment-service"),
		event("order-2", "OrderCancelled", "2026-08-20T11:01:00Z", "order-service"),
	}})
	failedResult, err := failed.Get(context.Background(), journeyQuery())
	if err != nil || failedResult.Status != StatusFailed {
		t.Fatalf("failed result = %#v, err = %v", failedResult, err)
	}

	compensated := NewService(eventReaderStub{events: []forensics.ForensicsEvent{
		event("order-1", "OrderCreated", "2026-08-20T11:00:00Z", "order-service"),
		event("payment-1", "PaymentCompleted", "2026-08-20T11:00:30Z", "payment-service"),
		event("refund-1", "PaymentRefunded", "2026-08-20T11:05:00Z", "payment-service"),
	}})
	compensatedResult, err := compensated.Get(context.Background(), journeyQuery())
	if err != nil || compensatedResult.Status != StatusCompensated {
		t.Fatalf("compensated result = %#v, err = %v", compensatedResult, err)
	}
}

func event(id, eventType, occurredAt, producer string) forensics.ForensicsEvent {
	return forensics.ForensicsEvent{
		EventID: id, EventType: eventType, OccurredAt: occurredAt, Producer: producer,
		CorrelationID: "ORDER-1", AggregateType: "Order", AggregateID: "ORDER-1",
	}
}

func journeyQuery() Query {
	return Query{
		CorrelationID: "ORDER-1",
		From:          time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		To:            time.Date(2026, 8, 20, 11, 6, 0, 0, time.UTC),
	}
}
