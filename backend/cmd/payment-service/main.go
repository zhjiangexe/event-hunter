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
	"event-hunter/backend/internal/demo/payment"
	"event-hunter/backend/internal/demo/telemetry"
	"event-hunter/backend/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Payment Service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceVersion := getenv("SERVICE_VERSION", "payment-service-dev")
	telemetryRuntime, err := observability.New(ctx, "payment-service", serviceVersion)
	if err != nil {
		return err
	}
	defer shutdownTelemetry(telemetryRuntime)

	db, err := sql.Open("pgx", postgresURL())
	if err != nil {
		return fmt.Errorf("open payment database: %w", err)
	}
	defer db.Close()

	const consumerGroupID = "payment-service-v1"
	kafkaTracer, kafkaHooks := telemetryRuntime.Kafka("payment-service", consumerGroupID)
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(getenv("KAFKA_BROKERS", "localhost:28319"), ",")...),
		kgo.ClientID("payment-service"),
		kgo.ConsumerGroup(consumerGroupID),
		kgo.ConsumeTopics("order.events", "shipping.events"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.WithHooks(kafkaHooks...),
	)
	if err != nil {
		return fmt.Errorf("create payment Kafka consumer: %w", err)
	}
	defer consumer.Close()

	eventEmissionDelay, err := parseEventEmissionDelay()
	if err != nil {
		return err
	}
	service := payment.NewService(db, serviceVersion, eventEmissionDelay)
	telemetryPublisher := telemetry.NewPublisher(consumer)
	slog.InfoContext(ctx, "Payment Service consuming", "topics", "order.events,shipping.events")
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
				slog.ErrorContext(ctx, "consume order event", "error", fetchError.Err, "topic", fetchError.Topic, "partition", fetchError.Partition)
			}
			continue
		}
		fetches.EachRecord(func(record *kgo.Record) {
			processCtx, processSpan := kafkaTracer.WithProcessSpan(record)
			defer processSpan.End()
			envelope, decodeErr := payment.DecodeConsumableEvent(record.Value)
			if decodeErr != nil {
				processSpan.RecordError(decodeErr)
				processSpan.SetStatus(codes.Error, decodeErr.Error())
				slog.ErrorContext(processCtx, "decode order event", "error", decodeErr, "offset", record.Offset)
				return
			}
			processSpan.SetAttributes(
				attribute.String("event.id", envelope.EventID),
				attribute.String("event.type", envelope.EventType),
				attribute.String("correlation.id", envelope.CorrelationID),
			)
			startedAt := time.Now().UTC()
			attemptNumber := telemetryPublisher.NextAttempt(envelope.EventID, consumerGroupID)
			if err := emitAttemptNumber(processCtx, telemetryPublisher, envelope, record, consumerGroupID, "STARTED", startedAt, nil, nil, nil, attemptNumber); err != nil {
				slog.ErrorContext(processCtx, "emit payment processing start", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
			}
			var handleErr error
			switch envelope.EventType {
			case "OrderCreated":
				handleErr = service.HandleOrderCreated(processCtx, envelope)
			case "ReturnReceived":
				handleErr = service.HandleReturnReceived(processCtx, envelope)
			}
			if handleErr != nil {
				processSpan.RecordError(handleErr)
				processSpan.SetStatus(codes.Error, handleErr.Error())
				slog.ErrorContext(processCtx, "handle order event", append([]any{"error", handleErr}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				reason := handleErr.Error()
				completedAt := time.Now().UTC()
				if err := emitAttemptNumber(processCtx, telemetryPublisher, envelope, record, consumerGroupID, "FAILED", startedAt, &completedAt, &reason, nil, attemptNumber); err != nil {
					slog.ErrorContext(processCtx, "emit payment processing failure", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				}
				return
			}
			completedAt := time.Now().UTC()
			if err := emitAttemptNumber(processCtx, telemetryPublisher, envelope, record, consumerGroupID, "SUCCEEDED", startedAt, &completedAt, nil, nil, attemptNumber); err != nil {
				slog.ErrorContext(processCtx, "emit payment processing success", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
			}
			if commitErr := consumer.CommitRecords(processCtx, record); commitErr != nil {
				processSpan.RecordError(commitErr)
				processSpan.SetStatus(codes.Error, commitErr.Error())
				slog.ErrorContext(processCtx, "commit order event", append([]any{"error", commitErr}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				return
			}
			slog.InfoContext(processCtx, "processed event", telemetry.ConsumerLogAttrs(envelope, record)...)
		})
		time.Sleep(10 * time.Millisecond)
	}
}

func shutdownTelemetry(runtime *observability.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		slog.Error("flush Payment Service telemetry", "error", err)
	}
}

func emitAttemptNumber(ctx context.Context, publisher *telemetry.Publisher, envelope event.Envelope, record *kgo.Record, groupID, status string, startedAt time.Time, completedAt *time.Time, reason, retryTopic *string, number int) error {
	attempt, err := telemetry.NewAttempt(envelope, groupID, "payment-service", record, number, status, startedAt, completedAt, reason, retryTopic)
	if err != nil {
		return err
	}
	return publisher.Emit(ctx, attempt)
}

func postgresURL() string {
	return "postgres://" + getenv("DEMO_PAYMENT_POSTGRES_USER", "demo_payment") + ":" + getenv("DEMO_PAYMENT_POSTGRES_PASSWORD", "demo_payment_local_only") + "@" + getenv("DEMO_PAYMENT_POSTGRES_HOST", "localhost") + ":" + getenv("DEMO_PAYMENT_POSTGRES_PORT", "28315") + "/demo_payment?sslmode=disable"
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
