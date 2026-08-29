package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"event-hunter/backend/internal/contexts/scenario_lab/adapters/inbound/httpapi"
	scenarioclickhouse "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/clickhouse"
	scenarioclock "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/clock"
	scenarioemission "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/emission"
	scenariokafka "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/kafka"
	scenariolinks "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/links"
	scenarioorderapi "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/orderapi"
	scenariopostgres "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/postgres"
	scenariotelemetry "event-hunter/backend/internal/contexts/scenario_lab/adapters/outbound/telemetry"
	scenarioapplication "event-hunter/backend/internal/contexts/scenario_lab/application"
	"event-hunter/backend/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Scenario Lab stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	telemetryRuntime, err := observability.New(ctx, "event-lab", getenv("SERVICE_VERSION", "event-lab-dev"))
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := telemetryRuntime.Shutdown(shutdownCtx); err != nil {
			slog.Error("flush Scenario Lab telemetry", "error", err)
		}
	}()

	db, err := sql.Open("pgx", postgresURL())
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping Scenario Lab database: %w", err)
	}

	_, hooks := telemetryRuntime.Kafka("event-lab", "")
	kafka, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(getenv("KAFKA_BROKERS", "localhost:28319"), ",")...),
		kgo.ClientID("event-lab"), kgo.WithHooks(hooks...),
	)
	if err != nil {
		return err
	}
	defer kafka.Close()
	if err := kafka.Ping(ctx); err != nil {
		return fmt.Errorf("ping Kafka: %w", err)
	}

	scenarioTelemetry := &scenariotelemetry.Adapter{}
	runner := &scenarioapplication.Runner{
		Repository: scenariopostgres.New(db), Publisher: scenariokafka.Publisher{Client: kafka},
		Observer: &scenarioclickhouse.Observer{
			URL: getenv("CLICKHOUSE_URL", "http://localhost:28317"), Database: getenv("CLICKHOUSE_DB", "event_hunter"),
			User: getenv("CLICKHOUSE_USER", "event_hunter"), Password: getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only"),
		},
		OrderStarter: &scenarioorderapi.Starter{URL: getenv("DEMO_ORDER_API_URL", "http://localhost:28335")},
		Emissions:    scenarioemission.Builder{},
		Links:        scenariolinks.Builder{EventHunterURL: getenv("EVENT_HUNTER_UI_URL", "http://localhost:28334"), GrafanaURL: getenv("GRAFANA_URL", "http://localhost:28332")},
		Traces:       scenarioTelemetry, Telemetry: scenarioTelemetry, Clock: scenarioclock.System{}, Timeout: 45 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, request *http.Request) {
		if db.PingContext(request.Context()) != nil {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	})
	httpapi.New(runner).Register(mux)

	server := &http.Server{
		Addr: ":" + getenv("EVENT_LAB_PORT", "28343"), Handler: otelhttp.NewHandler(mux, "event-lab.http"),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	slog.InfoContext(ctx, "Scenario Lab listening", "address", server.Addr)
	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func postgresURL() string {
	return "postgres://" + getenv("POSTGRES_USER", "event_hunter") + ":" + getenv("POSTGRES_PASSWORD", "event_hunter_local_only") + "@" + getenv("POSTGRES_HOST", "localhost") + ":" + getenv("POSTGRES_PORT", "28313") + "/" + getenv("POSTGRES_DB", "event_hunter") + "?sslmode=disable"
}
func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
