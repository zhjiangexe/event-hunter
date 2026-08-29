package kafka

import (
	"context"
	"event-hunter/backend/internal/contexts/scenario_lab/ports"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Publisher struct{ Client *kgo.Client }

func (publisher Publisher) Publish(ctx context.Context, topic, key string, value []byte) (ports.PublishedRecord, error) {
	results := publisher.Client.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte(key), Value: value})
	if err := results.FirstErr(); err != nil {
		return ports.PublishedRecord{}, err
	}
	return ports.PublishedRecord{Partition: results[0].Record.Partition, Offset: results[0].Record.Offset}, nil
}
