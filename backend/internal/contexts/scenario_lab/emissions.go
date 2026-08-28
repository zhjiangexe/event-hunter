package scenariolab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"event-hunter/backend/internal/demo/event"
	"event-hunter/backend/internal/demo/telemetry"
)

const LabTopic = "event-lab.events"

type PublishedRecord struct {
	Partition int32
	Offset    int64
}

type Publisher interface {
	Publish(context.Context, string, string, []byte) (PublishedRecord, error)
}

type Emission struct {
	Topic    string
	Key      string
	Value    []byte
	Envelope *event.Envelope
}

func TraceID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func BuildEmissions(scenarioID, correlationID, traceID string, now time.Time) ([]Emission, error) {
	if scenarioID == "S6" {
		invalid := map[string]any{
			"eventId": uuid.NewString(), "eventType": "PaymentCompleted", "eventVersion": 1,
			"occurredAt": now.UTC(), "producer": "event-lab", "correlationId": correlationID,
			"causationId": nil, "traceId": traceID, "aggregateType": "Payment",
			"aggregateId": "PAYMENT-" + correlationID, "sequence": 1,
			"payload": map[string]any{"paymentId": "PAYMENT-" + correlationID, "orderId": correlationID, "amount": 990},
		}
		value, _ := json.Marshal(invalid)
		return []Emission{{Topic: LabTopic, Key: correlationID, Value: value}}, nil
	}

	base := now.UTC()
	if scenarioID == "S2" {
		base = base.Add(-6 * time.Minute)
	}
	if scenarioID == "S7" {
		base = base.Add(-10 * time.Minute)
	}
	orderID := correlationID
	paymentID := "PAYMENT-" + correlationID
	shipmentID := "SHIPMENT-" + correlationID
	returnID := "RETURN-" + correlationID

	orderCreated, err := scenarioEnvelope("OrderCreated", "order-service", correlationID, "Order", orderID, 1, nil, traceID, base,
		map[string]any{"orderId": orderID, "customerId": "SCENARIO-LAB", "totalAmount": 990, "currency": "TWD"})
	if err != nil {
		return nil, err
	}
	events := []event.Envelope{orderCreated}

	paymentCompleted := func(sequence uint64, causation string, at time.Time) (event.Envelope, error) {
		return scenarioEnvelope("PaymentCompleted", "payment-service", correlationID, "Payment", paymentID, sequence, &causation, traceID, at,
			map[string]any{"paymentId": paymentID, "orderId": orderID, "amount": 990, "currency": "TWD"})
	}
	shipmentCreated := func(causation string, at time.Time) (event.Envelope, error) {
		return scenarioEnvelope("ShipmentCreated", "shipping-service", correlationID, "Shipment", shipmentID, 1, &causation, traceID, at,
			map[string]any{"shipmentId": shipmentID, "orderId": orderID, "provider": "SCENARIO-CARRIER", "status": "CREATED"})
	}

	switch scenarioID {
	case "S2", "S5":
		payment, err := paymentCompleted(1, orderCreated.EventID, base.Add(time.Second))
		if err != nil {
			return nil, err
		}
		events = append(events, payment)
	case "S3":
		payment, err := paymentCompleted(1, orderCreated.EventID, base.Add(time.Second))
		if err != nil {
			return nil, err
		}
		shipment, err := shipmentCreated(payment.EventID, base.Add(3*time.Second))
		if err != nil {
			return nil, err
		}
		events = append(events, payment, payment, shipment)
	case "S4":
		payment2, err := paymentCompleted(2, orderCreated.EventID, base.Add(time.Second))
		if err != nil {
			return nil, err
		}
		payment1, err := paymentCompleted(1, orderCreated.EventID, base.Add(2*time.Second))
		if err != nil {
			return nil, err
		}
		events = append(events, payment2, payment1)
	case "S7", "S9", "S10", "S11":
		payment, err := paymentCompleted(1, orderCreated.EventID, base.Add(time.Second))
		if err != nil {
			return nil, err
		}
		shipment, err := shipmentCreated(payment.EventID, base.Add(2*time.Second))
		if err != nil {
			return nil, err
		}
		events = append(events, payment, shipment)
		if scenarioID == "S7" {
			break
		}
		lastID := shipment.EventID
		sequence := uint64(2)
		if scenarioID == "S10" {
			failed, err := scenarioEnvelope("ShipmentDispatchFailed", "shipping-service", correlationID, "Shipment", shipmentID, sequence, &lastID, traceID, base.Add(3*time.Second),
				map[string]any{"shipmentId": shipmentID, "orderId": orderID, "provider": "SCENARIO-CARRIER", "reasonCode": "CAPACITY_UNAVAILABLE", "retryable": true, "status": "DISPATCH_FAILED"})
			if err != nil {
				return nil, err
			}
			events = append(events, failed)
			lastID = failed.EventID
			sequence++
		}
		dispatched, err := scenarioEnvelope("ShipmentDispatched", "shipping-service", correlationID, "Shipment", shipmentID, sequence, &lastID, traceID, base.Add(4*time.Second),
			map[string]any{"shipmentId": shipmentID, "orderId": orderID, "provider": "SCENARIO-CARRIER", "trackingNumber": "LAB-TRACKING", "status": "DISPATCHED"})
		if err != nil {
			return nil, err
		}
		events = append(events, dispatched)
		lastID = dispatched.EventID
		sequence++
		if scenarioID == "S9" {
			transit, err := scenarioEnvelope("ShipmentInTransit", "shipping-service", correlationID, "Shipment", shipmentID, sequence, &lastID, traceID, base.Add(5*time.Second),
				map[string]any{"shipmentId": shipmentID, "orderId": orderID, "location": "Taipei Hub", "status": "IN_TRANSIT"})
			if err != nil {
				return nil, err
			}
			events = append(events, transit)
			lastID = transit.EventID
			sequence++
		}
		deliveredAt := base.Add(6 * time.Second)
		delivered, err := scenarioEnvelope("ShipmentDelivered", "shipping-service", correlationID, "Shipment", shipmentID, sequence, &lastID, traceID, deliveredAt,
			map[string]any{"shipmentId": shipmentID, "orderId": orderID, "deliveredAt": deliveredAt.Format(time.RFC3339Nano), "recipient": "SCENARIO-RECIPIENT", "status": "DELIVERED"})
		if err != nil {
			return nil, err
		}
		events = append(events, delivered)
		if scenarioID == "S11" {
			requested, err := scenarioEnvelope("ReturnRequested", "shipping-service", correlationID, "Return", returnID, 1, &delivered.EventID, traceID, base.Add(7*time.Second),
				map[string]any{"returnId": returnID, "orderId": orderID, "shipmentId": shipmentID, "reason": "DAMAGED_ITEM", "status": "REQUESTED"})
			if err != nil {
				return nil, err
			}
			receivedAt := base.Add(8 * time.Second)
			received, err := scenarioEnvelope("ReturnReceived", "shipping-service", correlationID, "Return", returnID, 2, &requested.EventID, traceID, receivedAt,
				map[string]any{"returnId": returnID, "orderId": orderID, "receivedAt": receivedAt.Format(time.RFC3339Nano), "condition": "DAMAGED", "status": "RECEIVED"})
			if err != nil {
				return nil, err
			}
			refunded, err := scenarioEnvelope("PaymentRefunded", "payment-service", correlationID, "Payment", paymentID, 2, &received.EventID, traceID, base.Add(9*time.Second),
				map[string]any{"paymentId": paymentID, "orderId": orderID, "amount": 990, "reason": "RETURN_RECEIVED"})
			if err != nil {
				return nil, err
			}
			events = append(events, requested, received, refunded)
		}
	case "S8":
		failed, err := scenarioEnvelope("PaymentFailed", "payment-service", correlationID, "Payment", paymentID, 1, &orderCreated.EventID, traceID, base.Add(time.Second),
			map[string]any{"paymentId": paymentID, "orderId": orderID, "reasonCode": "CARD_DECLINED", "retryable": false, "status": "FAILED"})
		if err != nil {
			return nil, err
		}
		cancelled, err := scenarioEnvelope("OrderCancelled", "order-service", correlationID, "Order", orderID, 2, &failed.EventID, traceID, base.Add(2*time.Second),
			map[string]any{"orderId": orderID, "reason": "PAYMENT_FAILED"})
		if err != nil {
			return nil, err
		}
		events = append(events, failed, cancelled)
	default:
		return nil, fmt.Errorf("scenario %s has no lab emissions", scenarioID)
	}

	result := make([]Emission, 0, len(events))
	for index := range events {
		value, err := events[index].JSON()
		if err != nil {
			return nil, err
		}
		result = append(result, Emission{Topic: LabTopic, Key: correlationID, Value: value, Envelope: &events[index]})
	}
	return result, nil
}

func scenarioEnvelope(eventType, producer, correlationID, aggregateType, aggregateID string, sequence uint64, causation *string, traceID string, occurredAt time.Time, payload map[string]any) (event.Envelope, error) {
	envelope, err := event.NewEnvelope(eventType, producer, correlationID, aggregateType, aggregateID, sequence, causation, &traceID, payload)
	if err != nil {
		return event.Envelope{}, err
	}
	envelope.OccurredAt = occurredAt.UTC()
	return envelope, nil
}

func AttemptEmissions(envelope event.Envelope, record PublishedRecord, now time.Time) ([]Emission, error) {
	statuses := []string{"FAILED", "RETRY_SCHEDULED", "DLQ"}
	result := make([]Emission, 0, len(statuses))
	for index, status := range statuses {
		reason := "SCENARIO_LAB_FORCED_FAILURE"
		retryTopic := "event-lab.events.retry"
		completed := now.Add(time.Duration(index+1) * time.Second).UTC()
		attempt := telemetry.Attempt{
			AttemptID: uuid.NewString(), EventID: envelope.EventID, EventType: envelope.EventType,
			CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID,
			ConsumerGroupID: "scenario-lab-consumer-v1", ConsumerService: "scenario-lab",
			Attempt: index + 1, ProcessingStatus: status, RetryReason: &reason, RetryTopic: &retryTopic,
			KafkaTopic: LabTopic, KafkaPartition: record.Partition, KafkaOffset: record.Offset,
			StartedAt: now.Add(time.Duration(index) * time.Second).UTC(), CompletedAt: &completed, ObservedAt: completed,
		}
		value, err := json.Marshal(attempt)
		if err != nil {
			return nil, err
		}
		result = append(result, Emission{Topic: telemetry.Topic, Key: envelope.EventID + "\x00scenario-lab-consumer-v1", Value: value})
	}
	return result, nil
}
