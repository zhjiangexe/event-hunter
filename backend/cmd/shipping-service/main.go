package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"event-hunter/backend/internal/demo/event"
	"event-hunter/backend/internal/demo/shipping"
	"event-hunter/backend/internal/demo/telemetry"
	"event-hunter/backend/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Shipping Service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceVersion := getenv("SERVICE_VERSION", "shipping-service-dev")
	telemetryRuntime, err := observability.New(ctx, "shipping-service", serviceVersion)
	if err != nil {
		return err
	}
	defer shutdownTelemetry(telemetryRuntime)

	db, err := sql.Open("pgx", postgresURL())
	if err != nil {
		return fmt.Errorf("open shipping database: %w", err)
	}
	defer db.Close()

	const consumerGroupID = "shipping-service-v1"
	kafkaTracer, kafkaHooks := telemetryRuntime.Kafka("shipping-service", consumerGroupID)
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(getenv("KAFKA_BROKERS", "localhost:28319"), ",")...),
		kgo.ClientID("shipping-service"),
		kgo.ConsumerGroup(consumerGroupID),
		kgo.ConsumeTopics("payment.events"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.WithHooks(kafkaHooks...),
	)
	if err != nil {
		return fmt.Errorf("create shipping Kafka consumer: %w", err)
	}
	defer consumer.Close()

	eventEmissionDelay, err := parseEventEmissionDelay()
	if err != nil {
		return err
	}
	service := shipping.NewService(db, serviceVersion, eventEmissionDelay)
	telemetryPublisher := telemetry.NewPublisher(consumer)
	slog.InfoContext(ctx, "Shipping Service consuming", "topic", "payment.events")
	for {
		fetches := consumer.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if fetches.IsClientClosed() {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, fetchError := range errs {
				slog.ErrorContext(ctx, "consume payment event", "error", fetchError.Err, "topic", fetchError.Topic, "partition", fetchError.Partition)
			}
			continue
		}
		fetches.EachRecord(func(record *kgo.Record) {
			if record.Topic != "payment.events" {
				return
			}
			processCtx, processSpan := kafkaTracer.WithProcessSpan(record)
			defer processSpan.End()
			envelope, decodeErr := shipping.DecodeConsumableEvent(record.Value)
			if decodeErr != nil {
				processSpan.RecordError(decodeErr)
				processSpan.SetStatus(codes.Error, decodeErr.Error())
				slog.ErrorContext(processCtx, "decode payment event", "error", decodeErr, "offset", record.Offset)
				return
			}
			processSpan.SetAttributes(
				attribute.String("event.id", envelope.EventID),
				attribute.String("event.type", envelope.EventType),
				attribute.String("correlation.id", envelope.CorrelationID),
			)
			startedAt := time.Now().UTC()
			attemptNumber := telemetryPublisher.NextAttempt(envelope.EventID, consumerGroupID)
			if err := emitAttempt(processCtx, telemetryPublisher, envelope, record, consumerGroupID, "STARTED", startedAt, nil, nil, nil, attemptNumber); err != nil {
				slog.ErrorContext(processCtx, "emit shipping processing start", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
			}
			var handleErr error
			if envelope.EventType == "PaymentCompleted" {
				handleErr = service.HandlePaymentCompleted(processCtx, envelope)
			}
			if handleErr != nil {
				processSpan.RecordError(handleErr)
				processSpan.SetStatus(codes.Error, handleErr.Error())
				slog.ErrorContext(processCtx, "handle payment event", append([]any{"error", handleErr}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				reason := handleErr.Error()
				completedAt := time.Now().UTC()
				if err := emitAttempt(processCtx, telemetryPublisher, envelope, record, consumerGroupID, "FAILED", startedAt, &completedAt, &reason, nil, attemptNumber); err != nil {
					slog.ErrorContext(processCtx, "emit shipping processing failure", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				}
				return
			}
			completedAt := time.Now().UTC()
			if err := emitAttempt(processCtx, telemetryPublisher, envelope, record, consumerGroupID, "SUCCEEDED", startedAt, &completedAt, nil, nil, attemptNumber); err != nil {
				slog.ErrorContext(processCtx, "emit shipping processing success", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
			}
			if commitErr := consumer.CommitRecords(processCtx, record); commitErr != nil {
				processSpan.RecordError(commitErr)
				processSpan.SetStatus(codes.Error, commitErr.Error())
				slog.ErrorContext(processCtx, "commit payment event", append([]any{"error", commitErr}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				return
			}
			slog.InfoContext(processCtx, fmt.Sprintf("processed domain event: %s", envelope.EventType), telemetry.ConsumerLogAttrs(envelope, record)...)
		})
		time.Sleep(10 * time.Millisecond)
	}
}

func shutdownTelemetry(runtime *observability.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		slog.Error("flush Shipping Service telemetry", "error", err)
	}
}

func emitAttempt(ctx context.Context, publisher *telemetry.Publisher, envelope event.Envelope, record *kgo.Record, groupID, status string, startedAt time.Time, completedAt *time.Time, reason, retryTopic *string, number int) error {
	attempt, err := telemetry.NewAttempt(envelope, groupID, "shipping-service", record, number, status, startedAt, completedAt, reason, retryTopic)
	if err != nil {
		return err
	}
	return publisher.Emit(ctx, attempt)
}

func postgresURL() string {
	return "postgres://" + getenv("DEMO_SHIPPING_POSTGRES_USER", "demo_shipping") + ":" + getenv("DEMO_SHIPPING_POSTGRES_PASSWORD", "demo_shipping_local_only") + "@" + getenv("DEMO_SHIPPING_POSTGRES_HOST", "localhost") + ":" + getenv("DEMO_SHIPPING_POSTGRES_PORT", "28316") + "/demo_shipping?sslmode=disable"
}

func parseEventEmissionDelay() (time.Duration, error) {
	value := getenv("DEMO_EVENT_EMISSION_DELAY", "2s")
	delay, err := time.ParseDuration(value)
	if err != nil || delay < 0 {
		return 0, fmt.Errorf("DEMO_EVENT_EMISSION_DELAY must be a non-negative duration: %q", value)
	}
	return delay, nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
