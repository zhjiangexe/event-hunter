package telemetry

import (
	"github.com/twmb/franz-go/pkg/kgo"

	"event-hunter/backend/internal/demo/event"
)

// ConsumerLogAttrs returns the canonical event and Kafka coordinates used by
// context-aware service logs. Dotted keys follow the same semantic shape used
// on spans; Loki's OTLP ingestion exposes them as underscore-normalized
// structured metadata (for example correlation_id and kafka_offset).
func ConsumerLogAttrs(envelope event.Envelope, record *kgo.Record) []any {
	return []any{
		"event.id", envelope.EventID,
		"event.type", envelope.EventType,
		"correlation.id", envelope.CorrelationID,
		"kafka.topic", record.Topic,
		"kafka.partition", record.Partition,
		"kafka.offset", record.Offset,
		"kafka.position.known", true,
	}
}

// ProducerLogAttrs records the intended outbox route. Kafka assigns partition
// and offset only after Debezium publishes the committed row, so -1 plus the
// explicit known=false marker avoids presenting fabricated broker positions.
func ProducerLogAttrs(envelope event.Envelope, topic string) []any {
	return []any{
		"event.id", envelope.EventID,
		"event.type", envelope.EventType,
		"event.producer", envelope.Producer,
		"correlation.id", envelope.CorrelationID,
		"aggregate.type", envelope.AggregateType,
		"aggregate.id", envelope.AggregateID,
		"event.sequence", envelope.Sequence,
		"kafka.topic", topic,
		"kafka.partition", int32(-1),
		"kafka.offset", int64(-1),
		"kafka.position.known", false,
	}
}
