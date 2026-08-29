package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	qualityclickhouse "event-hunter/backend/internal/contexts/quality/adapters/outbound/clickhouse"
	"event-hunter/backend/internal/contexts/quality/application"
	"event-hunter/backend/internal/contexts/quality/domain"
)

const (
	defaultWindowDuration = time.Minute
	defaultLateGrace      = 2 * time.Minute
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	service := newService()
	switch os.Args[1] {
	case "aggregate":
		runAggregate(service, os.Args[2:])
	case "backfill":
		runBackfill(service, os.Args[2:])
	case "schedule":
		runSchedule(service, os.Args[2:])
	default:
		usage()
	}
}

func newService() *application.Service {
	config := qualityclickhouse.Config{
		URL:      getenv("CLICKHOUSE_URL", "http://localhost:28317"),
		Database: getenv("CLICKHOUSE_DB", "event_hunter"),
		User:     getenv("CLICKHOUSE_USER", "event_hunter"),
		Password: getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only"),
	}
	return application.NewService(qualityclickhouse.NewAggregator(config, &http.Client{Timeout: 5 * time.Second}))
}

func runBackfill(service *application.Service, args []string) {
	flags := flag.NewFlagSet("backfill", flag.ExitOnError)
	fromValue := flags.String("from", "", "UTC backfill start, RFC3339")
	toValue := flags.String("to", "", "UTC backfill end, RFC3339")
	window := flags.Duration("window", defaultWindowDuration, "tumbling window duration")
	_ = flags.Parse(args)
	if err := service.Backfill(context.Background(), parseTime("--from", *fromValue), parseTime("--to", *toValue), *window); err != nil {
		fatal("backfill quality metrics", err)
	}
}

func runAggregate(service *application.Service, args []string) {
	flags := flag.NewFlagSet("aggregate", flag.ExitOnError)
	fromValue := flags.String("from", "", "UTC window start, RFC3339")
	toValue := flags.String("to", "", "UTC window end, RFC3339")
	_ = flags.Parse(args)
	window, err := domain.NewWindow(parseTime("--from", *fromValue), parseTime("--to", *toValue))
	if err != nil {
		fatal("validate window", err)
	}
	if err := service.Aggregate(context.Background(), window); err != nil {
		fatal("aggregate quality metrics", err)
	}
}

func runSchedule(service *application.Service, args []string) {
	flags := flag.NewFlagSet("schedule", flag.ExitOnError)
	interval := flags.Duration("interval", time.Minute, "scheduler polling interval")
	windowDuration := flags.Duration("window", defaultWindowDuration, "tumbling window duration")
	lateGrace := flags.Duration("late-grace", defaultLateGrace, "delay before a closed window is eligible")
	healthAddress := flags.String("health-addr", ":28338", "health server listen address; empty disables it")
	_ = flags.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var ready atomic.Bool
	server := startHealthServer(*healthAddress, &ready)
	defer shutdown(server)
	err := service.RunSchedule(ctx, application.Schedule{Interval: *interval, Window: *windowDuration, Grace: *lateGrace}, func(result application.ScheduleResult) {
		ready.Store(result.Err == nil)
		if result.Err != nil {
			log.Printf("quality window %s..%s failed: %v", result.Window.From.Format(time.RFC3339), result.Window.To.Format(time.RFC3339), result.Err)
			return
		}
		log.Printf("quality window %s..%s completed", result.Window.From.Format(time.RFC3339), result.Window.To.Format(time.RFC3339))
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		fatal("run quality schedule", err)
	}
}

func startHealthServer(address string, ready *atomic.Bool) *http.Server {
	if strings.TrimSpace(address) == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte("live\n")) })
	mux.HandleFunc("/health/ready", func(response http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(response, "quality worker has not completed the latest eligible window", http.StatusServiceUnavailable)
			return
		}
		_, _ = response.Write([]byte("ready\n"))
	})
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("quality worker health server failed: %v", err)
		}
	}()
	return server
}

func shutdown(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func parseTime(flagName, value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		fatal("parse "+flagName, err)
	}
	return parsed
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: quality-worker aggregate --from RFC3339 --to RFC3339")
	fmt.Fprintln(os.Stderr, "       quality-worker backfill --from RFC3339 --to RFC3339 [--window 1m]")
	fmt.Fprintln(os.Stderr, "       quality-worker schedule [--interval 1m] [--window 1m] [--late-grace 2m] [--health-addr :28338]")
	os.Exit(2)
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
