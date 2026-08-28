package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"event-hunter/backend/internal/demo/event"
	"event-hunter/backend/internal/demo/order"
	"event-hunter/backend/internal/demo/telemetry"
	"event-hunter/backend/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Order Service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceVersion := getenv("SERVICE_VERSION", "order-service-dev")
	telemetryRuntime, err := observability.New(ctx, "order-service", serviceVersion)
	if err != nil {
		return err
	}
	defer shutdownTelemetry(telemetryRuntime)

	db, err := sql.Open("pgx", postgresURL())
	if err != nil {
		return err
	}
	defer db.Close()

	eventEmissionDelay, err := parseEventEmissionDelay()
	if err != nil {
		return err
	}
	service := order.NewService(db, serviceVersion, eventEmissionDelay)
	const consumerGroupID = "order-service-v1"
	kafkaTracer, kafkaHooks := telemetryRuntime.Kafka("order-service", consumerGroupID)
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(getenv("KAFKA_BROKERS", "localhost:28319"), ",")...),
		kgo.ClientID("order-service"),
		kgo.ConsumerGroup(consumerGroupID),
		kgo.ConsumeTopics("payment.events"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.WithHooks(kafkaHooks...),
	)
	if err != nil {
		return fmt.Errorf("create order Kafka consumer: %w", err)
	}
	defer consumer.Close()
	consumerError := make(chan error, 1)
	go func() { consumerError <- consumePaymentEvents(ctx, consumer, kafkaTracer, service, consumerGroupID) }()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/orders", func(writer http.ResponseWriter, request *http.Request) {
		var input order.CreateOrderRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			slog.WarnContext(request.Context(), "invalid order request", "error", err)
			http.Error(writer, "invalid JSON request", http.StatusBadRequest)
			return
		}
		accepted, err := service.CreateOrder(request.Context(), request.Header.Get("Idempotency-Key"), input)
		if err != nil {
			slog.ErrorContext(request.Context(), "create order", "error", err)
			if err == order.ErrIdempotencyConflict {
				http.Error(writer, err.Error(), http.StatusConflict)
				return
			}
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(writer).Encode(accepted)
	})
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, _ *http.Request) {
		if err := db.Ping(); err != nil {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	})

	port, _ := strconv.Atoi(getenv("DEMO_ORDER_API_PORT", "28335"))
	handler := otelhttp.NewHandler(mux, "order-service.http",
		otelhttp.WithFilter(func(request *http.Request) bool {
			return request.URL.Path != "/health/live" && request.URL.Path != "/health/ready"
		}),
	)
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	slog.InfoContext(ctx, "Order Service listening", "port", port)

	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-consumerError:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func consumePaymentEvents(ctx context.Context, consumer *kgo.Client, kafkaTracer *kotel.Tracer, service *order.Service, consumerGroupID string) error {
	publisher := telemetry.NewPublisher(consumer)
	for {
		fetches := consumer.PollFetches(ctx)
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, fetchError := range errs {
				slog.ErrorContext(ctx, "consume payment event", "error", fetchError.Err, "topic", fetchError.Topic, "partition", fetchError.Partition)
			}
			continue
		}
		fetches.EachRecord(func(record *kgo.Record) {
			processCtx, processSpan := kafkaTracer.WithProcessSpan(record)
			defer processSpan.End()
			envelope, decodeErr := event.Decode(record.Value)
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
			attemptNumber := publisher.NextAttempt(envelope.EventID, consumerGroupID)
			_ = emitOrderAttempt(processCtx, publisher, envelope, record, consumerGroupID, "STARTED", startedAt, nil, nil, attemptNumber)
			var handleErr error
			if envelope.EventType == "PaymentFailed" {
				handleErr = service.HandlePaymentFailed(processCtx, envelope)
			}
			completedAt := time.Now().UTC()
			if handleErr != nil {
				processSpan.RecordError(handleErr)
				processSpan.SetStatus(codes.Error, handleErr.Error())
				reason := handleErr.Error()
				_ = emitOrderAttempt(processCtx, publisher, envelope, record, consumerGroupID, "FAILED", startedAt, &completedAt, &reason, attemptNumber)
				slog.ErrorContext(processCtx, "handle payment event", append([]any{"error", handleErr}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				return
			}
			_ = emitOrderAttempt(processCtx, publisher, envelope, record, consumerGroupID, "SUCCEEDED", startedAt, &completedAt, nil, attemptNumber)
			if err := consumer.CommitRecords(processCtx, record); err != nil {
				processSpan.RecordError(err)
				processSpan.SetStatus(codes.Error, err.Error())
				slog.ErrorContext(processCtx, "commit payment event", append([]any{"error", err}, telemetry.ConsumerLogAttrs(envelope, record)...)...)
				return
			}
			slog.InfoContext(processCtx, fmt.Sprintf("processed domain event: %s", envelope.EventType), telemetry.ConsumerLogAttrs(envelope, record)...)
		})
	}
}

func emitOrderAttempt(ctx context.Context, publisher *telemetry.Publisher, envelope event.Envelope, record *kgo.Record, groupID, status string, startedAt time.Time, completedAt *time.Time, reason *string, number int) error {
	attempt, err := telemetry.NewAttempt(envelope, groupID, "order-service", record, number, status, startedAt, completedAt, reason, nil)
	if err != nil {
		return err
	}
	return publisher.Emit(ctx, attempt)
}

func shutdownTelemetry(runtime *observability.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		slog.Error("flush Order Service telemetry", "error", err)
	}
}

func postgresURL() string {
	return "postgres://" + getenv("DEMO_ORDER_POSTGRES_USER", "demo_order") + ":" + getenv("DEMO_ORDER_POSTGRES_PASSWORD", "demo_order_local_only") + "@" + getenv("DEMO_ORDER_POSTGRES_HOST", "localhost") + ":" + getenv("DEMO_ORDER_POSTGRES_PORT", "28314") + "/demo_order?sslmode=disable"
}

func parseEventEmissionDelay() (time.Duration, error) {
	delay, err := time.ParseDuration(getenv("DEMO_EVENT_EMISSION_DELAY", "2s"))
	if err != nil || delay < 0 {
		return 0, fmt.Errorf("DEMO_EVENT_EMISSION_DELAY must be a non-negative duration: %q", getenv("DEMO_EVENT_EMISSION_DELAY", "2s"))
	}
	return delay, nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
