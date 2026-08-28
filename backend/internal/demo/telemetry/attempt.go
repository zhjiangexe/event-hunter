package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"event-hunter/backend/internal/demo/event"
)

const Topic = "event-hunter.processing-attempts"

type Attempt struct {
	AttemptID        string     `json:"attemptId"`
	EventID          string     `json:"eventId"`
	EventType        string     `json:"eventType"`
	CorrelationID    string     `json:"correlationId"`
	TraceID          *string    `json:"traceId"`
	ConsumerGroupID  string     `json:"consumerGroupId"`
	ConsumerService  string     `json:"consumerService"`
	Attempt          int        `json:"attempt"`
	ProcessingStatus string     `json:"processingStatus"`
	RetryReason      *string    `json:"retryReason"`
	RetryTopic       *string    `json:"retryTopic"`
	KafkaTopic       string     `json:"kafkaTopic"`
	KafkaPartition   int32      `json:"kafkaPartition"`
	KafkaOffset      int64      `json:"kafkaOffset"`
	StartedAt        time.Time  `json:"startedAt"`
	CompletedAt      *time.Time `json:"completedAt"`
	ObservedAt       time.Time  `json:"observedAt"`
}

type Publisher struct {
	client *kgo.Client
	mu     sync.Mutex
	seen   map[string]int
}

func NewPublisher(client *kgo.Client) *Publisher {
	return &Publisher{client: client, seen: make(map[string]int)}
}

func (publisher *Publisher) NextAttempt(eventID, consumerGroupID string) int {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	key := eventID + "\x00" + consumerGroupID
	publisher.seen[key]++
	return publisher.seen[key]
}

func (publisher *Publisher) Emit(ctx context.Context, attempt Attempt) error {
	if attempt.AttemptID == "" {
		return fmt.Errorf("attempt ID is required")
	}
	data, err := json.Marshal(attempt)
	if err != nil {
		return fmt.Errorf("marshal processing attempt: %w", err)
	}
	result := publisher.client.ProduceSync(ctx, &kgo.Record{
		Topic:   Topic,
		Key:     []byte(attempt.EventID + "\x00" + attempt.ConsumerGroupID),
		Value:   data,
		Context: ctx,
	})
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("publish processing attempt: %w", err)
	}
	return nil
}

func NewAttempt(envelope event.Envelope, groupID, service string, record *kgo.Record, number int, status string, started time.Time, completed *time.Time, reason, retryTopic *string) (Attempt, error) {
	attemptID, err := newID()
	if err != nil {
		return Attempt{}, err
	}
	return Attempt{
		AttemptID: attemptID, EventID: envelope.EventID, EventType: envelope.EventType,
		CorrelationID: envelope.CorrelationID, TraceID: envelope.TraceID,
		ConsumerGroupID: groupID, ConsumerService: service, Attempt: number,
		ProcessingStatus: status, RetryReason: reason, RetryTopic: retryTopic,
		KafkaTopic: record.Topic, KafkaPartition: record.Partition, KafkaOffset: record.Offset,
		StartedAt: started.UTC(), CompletedAt: completed, ObservedAt: time.Now().UTC(),
	}, nil
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate attempt ID: %w", err)
	}
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
