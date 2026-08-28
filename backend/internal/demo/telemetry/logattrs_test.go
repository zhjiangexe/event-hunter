package telemetry

import (
	"reflect"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"event-hunter/backend/internal/demo/event"
)

func TestConsumerLogAttrsIncludesCanonicalEventAndKafkaCoordinates(t *testing.T) {
	envelope := event.Envelope{EventID: "evt-1", EventType: "OrderCreated", CorrelationID: "ORDER-1"}
	record := &kgo.Record{Topic: "order.events", Partition: 2, Offset: 41}
	want := []any{
		"event.id", "evt-1", "event.type", "OrderCreated", "correlation.id", "ORDER-1",
		"kafka.topic", "order.events", "kafka.partition", int32(2), "kafka.offset", int64(41),
		"kafka.position.known", true,
	}
	if got := ConsumerLogAttrs(envelope, record); !reflect.DeepEqual(got, want) {
		t.Fatalf("ConsumerLogAttrs() = %#v, want %#v", got, want)
	}
}

func TestProducerLogAttrsMarksBrokerPositionUnknown(t *testing.T) {
	envelope := event.Envelope{
		EventID: "evt-1", EventType: "OrderCreated", Producer: "order-service", CorrelationID: "ORDER-1",
		AggregateType: "Order", AggregateID: "ORDER-1", Sequence: 1,
	}
	want := []any{
		"event.id", "evt-1", "event.type", "OrderCreated", "event.producer", "order-service",
		"correlation.id", "ORDER-1", "aggregate.type", "Order", "aggregate.id", "ORDER-1", "event.sequence", uint64(1),
		"kafka.topic", "order.events", "kafka.partition", int32(-1), "kafka.offset", int64(-1),
		"kafka.position.known", false,
	}
	if got := ProducerLogAttrs(envelope, "order.events"); !reflect.DeepEqual(got, want) {
		t.Fatalf("ProducerLogAttrs() = %#v, want %#v", got, want)
	}
}
