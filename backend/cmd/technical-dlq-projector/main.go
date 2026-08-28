package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultDLQTopic = "event-hunter.poc-clickhouse-sink.dlq"
	defaultGroupID  = "event-hunter-technical-dlq-projector-v1"
)

type projectorConfig struct {
	brokers        []string
	topic          string
	groupID        string
	healthAddress  string
	clickHouseURL  string
	clickHouseDB   string
	clickHouseUser string
	clickHousePass string
}

type technicalFailure struct {
	FailureID       string  `json:"failure_id"`
	DLQTopic        string  `json:"dlq_topic"`
	DLQPartition    uint32  `json:"dlq_partition"`
	DLQOffset       uint64  `json:"dlq_offset"`
	SourceTopic     *string `json:"source_topic"`
	SourcePartition *uint32 `json:"source_partition"`
	SourceOffset    *uint64 `json:"source_offset"`
	ConnectorName   *string `json:"connector_name"`
	ConnectorTask   *uint32 `json:"connector_task"`
	FailureStage    *string `json:"failure_stage"`
	ExceptionClass  *string `json:"exception_class"`
	PayloadSHA256   string  `json:"payload_sha256"`
	ObservedAt      string  `json:"observed_at"`
}

type projector struct {
	config projectorConfig
	kafka  *kgo.Client
	http   *http.Client
}

func main() {
	config := loadConfig()
	kafka, err := kgo.NewClient(
		kgo.SeedBrokers(config.brokers...),
		kgo.ClientID("event-hunter-technical-dlq-projector"),
		kgo.ConsumerGroup(config.groupID),
		kgo.ConsumeTopics(config.topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		slog.Error("create Kafka client", "error", err)
		os.Exit(1)
	}
	defer kafka.Close()

	worker := &projector{config: config, kafka: kafka, http: &http.Client{Timeout: 3 * time.Second}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := worker.checkReady(ctx); err != nil {
		slog.Error("technical DLQ projector dependencies are not ready", "error", err)
		os.Exit(1)
	}

	server := &http.Server{Addr: config.healthAddress, Handler: worker.healthHandler(), ReadHeaderTimeout: 2 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	workerErrors := make(chan error, 1)
	go func() { workerErrors <- worker.run(ctx) }()

	slog.Info("technical DLQ projector started", "topic", config.topic, "consumer_group", config.groupID)
	select {
	case <-ctx.Done():
		slog.Info("technical DLQ projector shutdown requested")
	case err := <-workerErrors:
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("technical DLQ projector stopped", "error", err)
		}
		stop()
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("technical DLQ health server stopped", "error", err)
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	kafka.CloseAllowingRebalance()
}

func loadConfig() projectorConfig {
	return projectorConfig{
		brokers:        splitNonEmpty(getenv("KAFKA_BROKERS", "localhost:28319")),
		topic:          getenv("TECHNICAL_DLQ_TOPIC", defaultDLQTopic),
		groupID:        getenv("TECHNICAL_DLQ_CONSUMER_GROUP", defaultGroupID),
		healthAddress:  getenv("TECHNICAL_DLQ_HEALTH_ADDRESS", ":8080"),
		clickHouseURL:  getenv("CLICKHOUSE_URL", "http://localhost:28317"),
		clickHouseDB:   getenv("CLICKHOUSE_DB", "event_hunter"),
		clickHouseUser: getenv("CLICKHOUSE_USER", "event_hunter"),
		clickHousePass: getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only"),
	}
}

func (worker *projector) run(ctx context.Context) error {
	for ctx.Err() == nil {
		fetches := worker.kafka.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "poll technical DLQ", "error", err)
			continue
		}
		for _, record := range fetches.Records() {
			if ctx.Err() != nil {
				break
			}
			failure := summarizeRecord(record)
			for ctx.Err() == nil {
				if err := worker.insert(ctx, failure); err != nil {
					slog.ErrorContext(ctx, "project technical DLQ record", "dlq_topic", record.Topic, "dlq_partition", record.Partition, "dlq_offset", record.Offset, "error", err)
					if !waitForRetry(ctx, 250*time.Millisecond) {
						break
					}
					continue
				}
				break
			}
			if ctx.Err() != nil {
				break
			}
			for ctx.Err() == nil {
				if err := worker.kafka.CommitRecords(ctx, record); err != nil {
					slog.ErrorContext(ctx, "commit technical DLQ record", "dlq_topic", record.Topic, "dlq_partition", record.Partition, "dlq_offset", record.Offset, "error", err)
					if !waitForRetry(ctx, 250*time.Millisecond) {
						break
					}
					continue
				}
				break
			}
		}
	}
	return ctx.Err()
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func summarizeRecord(record *kgo.Record) technicalFailure {
	payloadHash := sha256.Sum256(record.Value)
	failureHash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", record.Topic, record.Partition, record.Offset)))
	observedAt := record.Timestamp.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return technicalFailure{
		FailureID:       hex.EncodeToString(failureHash[:]),
		DLQTopic:        record.Topic,
		DLQPartition:    uint32(record.Partition),
		DLQOffset:       uint64(record.Offset),
		SourceTopic:     headerString(record, "__connect.errors.topic", 249),
		SourcePartition: headerUint32(record, "__connect.errors.partition"),
		SourceOffset:    headerUint64(record, "__connect.errors.offset"),
		ConnectorName:   headerString(record, "__connect.errors.connector.name", 255),
		ConnectorTask:   headerUint32(record, "__connect.errors.task.id"),
		FailureStage:    headerString(record, "__connect.errors.stage", 255),
		ExceptionClass:  headerString(record, "__connect.errors.exception.class.name", 512),
		PayloadSHA256:   hex.EncodeToString(payloadHash[:]),
		ObservedAt:      observedAt.Format("2006-01-02 15:04:05.000"),
	}
}

func (worker *projector) insert(ctx context.Context, failure technicalFailure) error {
	body, err := json.Marshal(failure)
	if err != nil {
		return err
	}
	statement := `INSERT INTO ingestion_technical_failures
(failure_id,dlq_topic,dlq_partition,dlq_offset,source_topic,source_partition,source_offset,connector_name,connector_task,failure_stage,exception_class,payload_sha256,observed_at)
FORMAT JSONEachRow
`
	endpoint, err := url.Parse(worker.config.clickHouseURL)
	if err != nil {
		return fmt.Errorf("parse ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", worker.config.clickHouseDB)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), io.MultiReader(strings.NewReader(statement), bytes.NewReader(body), strings.NewReader("\n")))
	if err != nil {
		return err
	}
	request.SetBasicAuth(worker.config.clickHouseUser, worker.config.clickHousePass)
	response, err := worker.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("ClickHouse insert returned %s", response.Status)
	}
	return nil
}

func (worker *projector) healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) {
		writeHealth(writer, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := worker.checkReady(ctx); err != nil {
			writeHealth(writer, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeHealth(writer, http.StatusOK, "ready")
	})
	return mux
}

func (worker *projector) checkReady(ctx context.Context) error {
	if err := worker.kafka.Ping(ctx); err != nil {
		return fmt.Errorf("Kafka: %w", err)
	}
	endpoint := strings.TrimRight(worker.config.clickHouseURL, "/") + "/ping"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(worker.config.clickHouseUser, worker.config.clickHousePass)
	response, err := worker.http.Do(request)
	if err != nil {
		return fmt.Errorf("ClickHouse: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("ClickHouse returned %s", response.Status)
	}
	return nil
}

func headerString(record *kgo.Record, key string, maxLength int) *string {
	value := headerValue(record, key)
	if len(value) == 0 {
		return nil
	}
	text := strings.TrimSpace(string(value))
	if text == "" {
		return nil
	}
	if len(text) > maxLength {
		text = text[:maxLength]
	}
	return &text
}

func headerUint32(record *kgo.Record, key string) *uint32 {
	value := headerValue(record, key)
	if len(value) == 4 {
		parsed := binary.BigEndian.Uint32(value)
		return &parsed
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 32)
	if err != nil {
		return nil
	}
	result := uint32(parsed)
	return &result
}

func headerUint64(record *kgo.Record, key string) *uint64 {
	value := headerValue(record, key)
	if len(value) == 8 {
		parsed := binary.BigEndian.Uint64(value)
		return &parsed
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func headerValue(record *kgo.Record, key string) []byte {
	for index := len(record.Headers) - 1; index >= 0; index-- {
		if record.Headers[index].Key == key {
			return record.Headers[index].Value
		}
	}
	return nil
}

func splitNonEmpty(value string) []string {
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func writeHealth(writer http.ResponseWriter, status int, value string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": value})
}
