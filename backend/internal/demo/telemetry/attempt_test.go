package telemetry

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"event-hunter/backend/internal/demo/event"
)

func TestNextAttemptIsScopedByEventAndConsumerGroup(t *testing.T) {
	publisher := NewPublisher(nil)
	if got := publisher.NextAttempt("event-1", "consumer-a"); got != 1 {
		t.Fatalf("first attempt = %d, want 1", got)
	}
	if got := publisher.NextAttempt("event-1", "consumer-a"); got != 2 {
		t.Fatalf("second attempt = %d, want 2", got)
	}
	if got := publisher.NextAttempt("event-1", "consumer-b"); got != 1 {
		t.Fatalf("different consumer group attempt = %d, want 1", got)
	}
}

func TestAttemptUsesContractJSONNames(t *testing.T) {
	envelope := event.Envelope{
		EventID: "event-1", EventType: "OrderCreated", EventVersion: 1,
		OccurredAt: time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		Producer:   "order-service", CorrelationID: "ORDER-1", AggregateType: "Order", AggregateID: "ORDER-1", Sequence: 1,
	}
	attempt, err := NewAttempt(envelope, "payment-service-v1", "payment-service", &kgo.Record{Topic: "order.events", Partition: 2, Offset: 100}, 1, "SUCCEEDED", envelope.OccurredAt, timePtr(envelope.OccurredAt.Add(50*time.Millisecond)), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(attempt)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"attemptId", "eventId", "consumerGroupId", "processingStatus", "kafkaPartition", "startedAt", "completedAt", "observedAt"} {
		if !bytes.Contains(data, []byte("\""+field+"\"")) {
			t.Fatalf("JSON missing contract field %q: %s", field, data)
		}
	}
}

func timePtr(value time.Time) *time.Time { return &value }
