package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ingestionhealth "event-hunter/backend/internal/contexts/ingestion/adapters/inbound/health"
	ingestionclickhouse "event-hunter/backend/internal/contexts/ingestion/adapters/outbound/clickhouse"
	ingestionkafka "event-hunter/backend/internal/contexts/ingestion/adapters/outbound/kafka"
	ingestionlogging "event-hunter/backend/internal/contexts/ingestion/adapters/outbound/logging"
	"event-hunter/backend/internal/contexts/ingestion/application"
	"event-hunter/backend/internal/contexts/ingestion/ports"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultDLQTopic = "event-hunter.poc-clickhouse-sink.dlq"
	defaultGroupID  = "event-hunter-technical-dlq-projector-v1"
)

type config struct {
	brokers        []string
	topic          string
	groupID        string
	healthAddress  string
	clickHouseURL  string
	clickHouseDB   string
	clickHouseUser string
	clickHousePass string
}

func main() {
	settings := loadConfig()
	client, err := kgo.NewClient(
		kgo.SeedBrokers(settings.brokers...),
		kgo.ClientID("event-hunter-technical-dlq-projector"),
		kgo.ConsumerGroup(settings.groupID),
		kgo.ConsumeTopics(settings.topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		slog.Error("create Kafka client", "error", err)
		os.Exit(1)
	}
	source := ingestionkafka.NewSource(client)
	defer source.Close()
	repository := ingestionclickhouse.NewFailureRepository(ingestionclickhouse.Config{
		URL: settings.clickHouseURL, Database: settings.clickHouseDB,
		User: settings.clickHouseUser, Password: settings.clickHousePass,
	}, &http.Client{Timeout: 3 * time.Second})
	projector := application.NewProjector(source, repository, ingestionlogging.Reporter{}, 250*time.Millisecond)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := checkReady(ctx, source, repository); err != nil {
		slog.Error("technical DLQ projector dependencies are not ready", "error", err)
		os.Exit(1)
	}

	server := &http.Server{Addr: settings.healthAddress, Handler: ingestionhealth.Handler(source, repository), ReadHeaderTimeout: 2 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	workerErrors := make(chan error, 1)
	go func() { workerErrors <- projector.Run(ctx) }()

	slog.Info("technical DLQ projector started", "topic", settings.topic, "consumer_group", settings.groupID)
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
	source.CloseAllowingRebalance()
}

func checkReady(ctx context.Context, source ports.Source, repository ports.FailureRepository) error {
	if err := source.Ping(ctx); err != nil {
		return fmt.Errorf("Kafka: %w", err)
	}
	return repository.Ping(ctx)
}

func loadConfig() config {
	return config{
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
