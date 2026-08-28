package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	defaultWindowDuration = time.Minute
	defaultLateGrace      = 2 * time.Minute
	maxBackfillDuration   = 31 * 24 * time.Hour
)

var clickHouseClient = &http.Client{Timeout: 5 * time.Second}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "aggregate":
		runAggregate(os.Args[2:])
	case "backfill":
		runBackfill(os.Args[2:])
	case "schedule":
		runSchedule(os.Args[2:])
	default:
		usage()
	}
}

func runBackfill(args []string) {
	flags := flag.NewFlagSet("backfill", flag.ExitOnError)
	fromValue := flags.String("from", "", "UTC backfill start, RFC3339")
	toValue := flags.String("to", "", "UTC backfill end, RFC3339")
	window := flags.Duration("window", defaultWindowDuration, "tumbling window duration")
	_ = flags.Parse(args)

	from, err := time.Parse(time.RFC3339, *fromValue)
	if err != nil {
		fatal("parse --from", err)
	}
	to, err := time.Parse(time.RFC3339, *toValue)
	if err != nil {
		fatal("parse --to", err)
	}
	if err := validateBackfillWindow(from, to); err != nil {
		fatal("validate window", err)
	}
	windows, err := backfillWindows(from.UTC(), to.UTC(), *window)
	if err != nil {
		fatal("build backfill windows", err)
	}
	for _, boundaries := range windows {
		if err := aggregate(context.Background(), boundaries[0], boundaries[1]); err != nil {
			fatal(fmt.Sprintf("aggregate quality window %s..%s", boundaries[0].Format(time.RFC3339), boundaries[1].Format(time.RFC3339)), err)
		}
	}
}

func runAggregate(args []string) {
	flags := flag.NewFlagSet("aggregate", flag.ExitOnError)
	fromValue := flags.String("from", "", "UTC window start, RFC3339")
	toValue := flags.String("to", "", "UTC window end, RFC3339")
	_ = flags.Parse(args)

	from, err := time.Parse(time.RFC3339, *fromValue)
	if err != nil {
		fatal("parse --from", err)
	}
	to, err := time.Parse(time.RFC3339, *toValue)
	if err != nil {
		fatal("parse --to", err)
	}
	if err := validateBackfillWindow(from, to); err != nil {
		fatal("validate window", err)
	}
	if err := aggregate(context.Background(), from.UTC(), to.UTC()); err != nil {
		fatal("aggregate quality metrics", err)
	}
}

func runSchedule(args []string) {
	flags := flag.NewFlagSet("schedule", flag.ExitOnError)
	interval := flags.Duration("interval", time.Minute, "scheduler polling interval")
	windowDuration := flags.Duration("window", defaultWindowDuration, "tumbling window duration")
	lateGrace := flags.Duration("late-grace", defaultLateGrace, "delay before a closed window is eligible")
	healthAddress := flags.String("health-addr", ":28338", "health server listen address; empty disables it")
	_ = flags.Parse(args)
	if *interval <= 0 || *windowDuration <= 0 || *lateGrace < 0 {
		fatal("validate schedule", errors.New("interval and window must be positive; late-grace must not be negative"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var ready atomic.Bool
	if *healthAddress != "" {
		server := startHealthServer(*healthAddress, &ready)
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		}()
	}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	var lastWindowEnd time.Time
	for {
		from, to := eligibleWindow(time.Now().UTC(), *windowDuration, *lateGrace)
		if to.After(lastWindowEnd) {
			if err := aggregate(ctx, from, to); err != nil {
				ready.Store(false)
				log.Printf("quality window %s..%s failed: %v", from.Format(time.RFC3339), to.Format(time.RFC3339), err)
			} else {
				lastWindowEnd = to
				ready.Store(true)
				log.Printf("quality window %s..%s completed", from.Format(time.RFC3339), to.Format(time.RFC3339))
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func validateBackfillWindow(from, to time.Time) error {
	if !to.After(from) || to.Sub(from) > maxBackfillDuration {
		return fmt.Errorf("to must be after from and no wider than %d days", int(maxBackfillDuration.Hours()/24))
	}
	return nil
}

func backfillWindows(from, to time.Time, duration time.Duration) ([][2]time.Time, error) {
	if duration <= 0 {
		return nil, errors.New("window duration must be positive")
	}
	result := make([][2]time.Time, 0, int(to.Sub(from)/duration)+1)
	for start := from; start.Before(to); start = start.Add(duration) {
		end := start.Add(duration)
		if end.After(to) {
			end = to
		}
		result = append(result, [2]time.Time{start, end})
	}
	return result, nil
}

func eligibleWindow(now time.Time, duration, grace time.Duration) (time.Time, time.Time) {
	end := now.UTC().Add(-grace).Truncate(duration)
	return end.Add(-duration), end
}

func startHealthServer(address string, ready *atomic.Bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("live\n"))
	})
	mux.HandleFunc("/health/ready", func(response http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(response, "quality worker has not completed the latest eligible window", http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusOK)
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

func aggregate(ctx context.Context, from, to time.Time) error {
	query := fmt.Sprintf(`
INSERT INTO event_quality_metrics
(window_start, window_end, topic_name, kafka_partition, consumer_group_id,
 event_count, duplicate_count, schema_violation_count, out_of_order_count,
 dlq_count, max_event_delay_ms, consumer_lag_messages, max_processing_latency_ms, source)
WITH
  toDateTime64('%s', 3, 'UTC') AS ws,
  toDateTime64('%s', 3, 'UTC') AS we,
  deliveries AS (
    SELECT kafka_topic, kafka_partition, kafka_offset, any(event_id) AS event_id,
           any(aggregate_type) AS aggregate_type, any(aggregate_id) AS aggregate_id,
           any(sequence) AS sequence, max(greatest(toInt64(dateDiff('millisecond', occurred_at, ingested_at)), 0)) AS delay_ms
    FROM canonical_forensics_events
    WHERE ingested_at >= ws AND ingested_at < we
    GROUP BY kafka_topic, kafka_partition, kafka_offset
  ),
  ordered AS (
    SELECT *, min(kafka_offset) OVER (PARTITION BY event_id) AS first_offset,
           max(sequence) OVER (PARTITION BY aggregate_type, aggregate_id ORDER BY kafka_topic, kafka_partition, kafka_offset ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING) AS prior_sequence
    FROM deliveries
  ),
  grouped AS (
    SELECT kafka_topic, kafka_partition,
           count() AS event_count,
           uniqExactIf(event_id, event_id IN (SELECT event_id FROM deliveries GROUP BY event_id HAVING count() > 1)) AS duplicate_count,
           countIf(kafka_offset = first_offset AND isNotNull(prior_sequence) AND sequence <= prior_sequence) AS out_of_order_count,
           max(delay_ms) AS max_event_delay_ms
    FROM ordered
    GROUP BY kafka_topic, kafka_partition
  ),
  dimensions AS (
    SELECT kafka_topic, kafka_partition, '' AS consumer_group_id FROM grouped
    UNION DISTINCT
    SELECT topic_name, kafka_partition, consumer_group_id
    FROM redpanda_consumer_group_metrics
    WHERE sampled_at >= ws AND sampled_at <= we
    UNION DISTINCT
    SELECT source_topic, source_partition, '' AS consumer_group_id
    FROM event_ingestion_failures
    WHERE failed_at >= ws AND failed_at < we
    UNION DISTINCT
    SELECT kafka_topic, kafka_partition, consumer_group_id
    FROM canonical_event_processing_attempts
    WHERE observed_at >= ws AND observed_at < we
  )
SELECT ws, we, dimensions.kafka_topic, dimensions.kafka_partition, dimensions.consumer_group_id, event_count, duplicate_count,
       (SELECT countDistinct(tuple(source_topic, source_partition, source_offset))
        FROM event_ingestion_failures
        WHERE error_type = 'SCHEMA_VIOLATION' AND failed_at >= ws AND failed_at < we),
       out_of_order_count,
       (SELECT countDistinct(tuple(source_topic, source_partition, source_offset))
        FROM event_ingestion_failures
        WHERE failed_at >= ws AND failed_at < we)
       + (SELECT countDistinct(tuple(event_id, consumer_group_id))
          FROM canonical_event_processing_attempts
          WHERE processing_status = 'DLQ' AND observed_at >= ws AND observed_at < we),
       max_event_delay_ms,
       (SELECT max(lag_messages) FROM redpanda_consumer_group_metrics
        WHERE topic_name = dimensions.kafka_topic AND kafka_partition = dimensions.kafka_partition
          AND consumer_group_id = dimensions.consumer_group_id AND sampled_at >= ws AND sampled_at <= we),
       (SELECT max(dateDiff('millisecond', started_at, completed_at))
        FROM canonical_event_processing_attempts
        WHERE completed_at IS NOT NULL AND started_at >= ws AND started_at < we),
       'quality-worker-v1'
FROM dimensions LEFT JOIN grouped USING (kafka_topic, kafka_partition)`, from.Format("2006-01-02 15:04:05.000"), to.Format("2006-01-02 15:04:05.000"))

	url := getenv("CLICKHOUSE_URL", "http://localhost:28317")
	database := getenv("CLICKHOUSE_DB", "event_hunter")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(url, "/")+"/?database="+database, bytes.NewBufferString(query))
	if err != nil {
		return err
	}
	request.SetBasicAuth(getenv("CLICKHOUSE_USER", "event_hunter"), getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only"))
	request.Header.Set("Content-Type", "text/plain")
	response, err := clickHouseClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("ClickHouse returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
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
