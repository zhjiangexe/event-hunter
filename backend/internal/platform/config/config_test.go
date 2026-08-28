package config

import (
	"testing"
	"time"
)

func TestLoadDefaultsTemporalDisabled(t *testing.T) {
	t.Setenv("EVENT_HUNTER_API_PORT", "")
	t.Setenv("TEMPORAL_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddress != ":28333" {
		t.Fatalf("HTTPAddress = %q, want :28333", cfg.HTTPAddress)
	}
	if cfg.TemporalEnabled {
		t.Fatal("TemporalEnabled = true, want false")
	}
	if cfg.HTTPRequestTimeout != 5*time.Second || cfg.PostgresQueryTimeout != 2*time.Second || cfg.ClickHouseQueryTimeout != 3*time.Second {
		t.Fatalf("unexpected timeout defaults: %#v", cfg)
	}
	if cfg.RateLimitWindow != time.Minute || cfg.RateLimitRequests != 300 {
		t.Fatalf("unexpected rate limit defaults: %#v", cfg)
	}
	if cfg.ClickHouseMaxResultRows != 10000 || cfg.ClickHouseMaxThreads != 4 {
		t.Fatalf("unexpected ClickHouse budget defaults: %#v", cfg)
	}
}

func TestLoadReadsProtectionOverrides(t *testing.T) {
	t.Setenv("EVENT_HUNTER_HTTP_REQUEST_TIMEOUT", "750ms")
	t.Setenv("EVENT_HUNTER_POSTGRES_QUERY_TIMEOUT", "900ms")
	t.Setenv("EVENT_HUNTER_CLICKHOUSE_QUERY_TIMEOUT", "600ms")
	t.Setenv("EVENT_HUNTER_RATE_LIMIT_WINDOW", "10s")
	t.Setenv("EVENT_HUNTER_RATE_LIMIT_REQUESTS", "42")
	t.Setenv("EVENT_HUNTER_CLICKHOUSE_MAX_RESULT_ROWS", "500")
	t.Setenv("EVENT_HUNTER_CLICKHOUSE_MAX_RESULT_BYTES", "1024")
	t.Setenv("EVENT_HUNTER_CLICKHOUSE_MAX_ROWS_TO_READ", "2000")
	t.Setenv("EVENT_HUNTER_CLICKHOUSE_MAX_BYTES_TO_READ", "4096")
	t.Setenv("EVENT_HUNTER_CLICKHOUSE_MAX_THREADS", "2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPRequestTimeout != 750*time.Millisecond || cfg.PostgresQueryTimeout != 900*time.Millisecond || cfg.ClickHouseQueryTimeout != 600*time.Millisecond {
		t.Fatalf("unexpected timeout overrides: %#v", cfg)
	}
	if cfg.RateLimitWindow != 10*time.Second || cfg.RateLimitRequests != 42 {
		t.Fatalf("unexpected rate limit overrides: %#v", cfg)
	}
	if cfg.ClickHouseMaxResultRows != 500 || cfg.ClickHouseMaxResultBytes != 1024 || cfg.ClickHouseMaxRowsToRead != 2000 || cfg.ClickHouseMaxBytesToRead != 4096 || cfg.ClickHouseMaxThreads != 2 {
		t.Fatalf("unexpected ClickHouse budget overrides: %#v", cfg)
	}
}

func TestLoadRejectsInvalidProtectionValues(t *testing.T) {
	for _, test := range []struct {
		key   string
		value string
	}{
		{key: "EVENT_HUNTER_HTTP_REQUEST_TIMEOUT", value: "never"},
		{key: "EVENT_HUNTER_RATE_LIMIT_REQUESTS", value: "0"},
		{key: "EVENT_HUNTER_CLICKHOUSE_MAX_BYTES_TO_READ", value: "-1"},
	} {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() error = nil for %s=%s", test.key, test.value)
			}
		})
	}
}

func TestLoadRejectsInvalidTemporalFlag(t *testing.T) {
	t.Setenv("TEMPORAL_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid boolean error")
	}
}
