package kafka

import (
	"context"
	"fmt"

	"event-hunter/backend/internal/contexts/ingestion/domain"
	"event-hunter/backend/internal/contexts/ingestion/ports"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Source struct {
	client *kgo.Client
}

type sourceRecord struct {
	record *kgo.Record
}

func NewSource(client *kgo.Client) *Source {
	if client == nil {
		panic("Kafka client is required")
	}
	return &Source{client: client}
}

func (source *Source) Poll(ctx context.Context) ([]ports.SourceRecord, error) {
	fetches := source.client.PollFetches(ctx)
	if err := fetches.Err(); err != nil {
		return nil, err
	}
	records := fetches.Records()
	result := make([]ports.SourceRecord, 0, len(records))
	for _, record := range records {
		result = append(result, sourceRecord{record: record})
	}
	return result, nil
}

func (source *Source) Commit(ctx context.Context, candidate ports.SourceRecord) error {
	record, ok := candidate.(sourceRecord)
	if !ok || record.record == nil {
		return fmt.Errorf("unsupported technical DLQ source record %T", candidate)
	}
	return source.client.CommitRecords(ctx, record.record)
}

func (source *Source) Ping(ctx context.Context) error {
	return source.client.Ping(ctx)
}

func (source *Source) Close() {
	source.client.Close()
}

func (source *Source) CloseAllowingRebalance() {
	source.client.CloseAllowingRebalance()
}

func (record sourceRecord) FailureRecord() domain.DLQRecord {
	headers := make([]domain.Header, 0, len(record.record.Headers))
	for _, header := range record.record.Headers {
		headers = append(headers, domain.Header{Key: header.Key, Value: header.Value})
	}
	return domain.DLQRecord{
		Topic:     record.record.Topic,
		Partition: record.record.Partition,
		Offset:    record.record.Offset,
		Timestamp: record.record.Timestamp,
		Payload:   record.record.Value,
		Headers:   headers,
	}
}
