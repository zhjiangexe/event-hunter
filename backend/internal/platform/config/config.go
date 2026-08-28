package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contains only process-level settings. External clients and Context
// dependencies are assembled by the composition root after loading Config.
type Config struct {
	HTTPAddress              string
	TemporalEnabled          bool
	HTTPRequestTimeout       time.Duration
	PostgresQueryTimeout     time.Duration
	ClickHouseQueryTimeout   time.Duration
	RateLimitWindow          time.Duration
	RateLimitRequests        int
	ClickHouseMaxResultRows  int
	ClickHouseMaxResultBytes int64
	ClickHouseMaxRowsToRead  int64
	ClickHouseMaxBytesToRead int64
	ClickHouseMaxThreads     int
}

// Defaults is also used by focused handler tests that construct a partial
// Config. Production processes must still call Load so invalid environment
// values fail fast during startup.
func Defaults() Config {
	return Config{
		HTTPAddress:              ":28333",
		HTTPRequestTimeout:       5 * time.Second,
		PostgresQueryTimeout:     2 * time.Second,
		ClickHouseQueryTimeout:   3 * time.Second,
		RateLimitWindow:          time.Minute,
		RateLimitRequests:        300,
		ClickHouseMaxResultRows:  10000,
		ClickHouseMaxResultBytes: 8 * 1024 * 1024,
		ClickHouseMaxRowsToRead:  5_000_000,
		ClickHouseMaxBytesToRead: 512 * 1024 * 1024,
		ClickHouseMaxThreads:     4,
	}
}

func (config Config) WithDefaults() Config {
	defaults := Defaults()
	if config.HTTPAddress == "" {
		config.HTTPAddress = defaults.HTTPAddress
	}
	if config.HTTPRequestTimeout <= 0 {
		config.HTTPRequestTimeout = defaults.HTTPRequestTimeout
	}
	if config.PostgresQueryTimeout <= 0 {
		config.PostgresQueryTimeout = defaults.PostgresQueryTimeout
	}
	if config.ClickHouseQueryTimeout <= 0 {
		config.ClickHouseQueryTimeout = defaults.ClickHouseQueryTimeout
	}
	if config.RateLimitWindow <= 0 {
		config.RateLimitWindow = defaults.RateLimitWindow
	}
	if config.RateLimitRequests <= 0 {
		config.RateLimitRequests = defaults.RateLimitRequests
	}
	if config.ClickHouseMaxResultRows <= 0 {
		config.ClickHouseMaxResultRows = defaults.ClickHouseMaxResultRows
	}
	if config.ClickHouseMaxResultBytes <= 0 {
		config.ClickHouseMaxResultBytes = defaults.ClickHouseMaxResultBytes
	}
	if config.ClickHouseMaxRowsToRead <= 0 {
		config.ClickHouseMaxRowsToRead = defaults.ClickHouseMaxRowsToRead
	}
	if config.ClickHouseMaxBytesToRead <= 0 {
		config.ClickHouseMaxBytesToRead = defaults.ClickHouseMaxBytesToRead
	}
	if config.ClickHouseMaxThreads <= 0 {
		config.ClickHouseMaxThreads = defaults.ClickHouseMaxThreads
	}
	return config
}

func Load() (Config, error) {
	defaults := Defaults()
	port := getenv("EVENT_HUNTER_API_PORT", "28333")
	if _, err := strconv.Atoi(port); err != nil {
		return Config{}, fmt.Errorf("EVENT_HUNTER_API_PORT must be numeric: %w", err)
	}

	temporalEnabled, err := strconv.ParseBool(getenv("TEMPORAL_ENABLED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("TEMPORAL_ENABLED must be boolean: %w", err)
	}
	httpRequestTimeout, err := positiveDuration("EVENT_HUNTER_HTTP_REQUEST_TIMEOUT", defaults.HTTPRequestTimeout)
	if err != nil {
		return Config{}, err
	}
	postgresQueryTimeout, err := positiveDuration("EVENT_HUNTER_POSTGRES_QUERY_TIMEOUT", defaults.PostgresQueryTimeout)
	if err != nil {
		return Config{}, err
	}
	clickHouseQueryTimeout, err := positiveDuration("EVENT_HUNTER_CLICKHOUSE_QUERY_TIMEOUT", defaults.ClickHouseQueryTimeout)
	if err != nil {
		return Config{}, err
	}
	rateLimitWindow, err := positiveDuration("EVENT_HUNTER_RATE_LIMIT_WINDOW", defaults.RateLimitWindow)
	if err != nil {
		return Config{}, err
	}
	rateLimitRequests, err := positiveInt("EVENT_HUNTER_RATE_LIMIT_REQUESTS", defaults.RateLimitRequests)
	if err != nil {
		return Config{}, err
	}
	maxResultRows, err := positiveInt("EVENT_HUNTER_CLICKHOUSE_MAX_RESULT_ROWS", defaults.ClickHouseMaxResultRows)
	if err != nil {
		return Config{}, err
	}
	maxResultBytes, err := positiveInt64("EVENT_HUNTER_CLICKHOUSE_MAX_RESULT_BYTES", defaults.ClickHouseMaxResultBytes)
	if err != nil {
		return Config{}, err
	}
	maxRowsToRead, err := positiveInt64("EVENT_HUNTER_CLICKHOUSE_MAX_ROWS_TO_READ", defaults.ClickHouseMaxRowsToRead)
	if err != nil {
		return Config{}, err
	}
	maxBytesToRead, err := positiveInt64("EVENT_HUNTER_CLICKHOUSE_MAX_BYTES_TO_READ", defaults.ClickHouseMaxBytesToRead)
	if err != nil {
		return Config{}, err
	}
	maxThreads, err := positiveInt("EVENT_HUNTER_CLICKHOUSE_MAX_THREADS", defaults.ClickHouseMaxThreads)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddress:              ":" + port,
		TemporalEnabled:          temporalEnabled,
		HTTPRequestTimeout:       httpRequestTimeout,
		PostgresQueryTimeout:     postgresQueryTimeout,
		ClickHouseQueryTimeout:   clickHouseQueryTimeout,
		RateLimitWindow:          rateLimitWindow,
		RateLimitRequests:        rateLimitRequests,
		ClickHouseMaxResultRows:  maxResultRows,
		ClickHouseMaxResultBytes: maxResultBytes,
		ClickHouseMaxRowsToRead:  maxRowsToRead,
		ClickHouseMaxBytesToRead: maxBytesToRead,
		ClickHouseMaxThreads:     maxThreads,
	}, nil
}

func positiveDuration(key string, fallback time.Duration) (time.Duration, error) {
	value, err := time.ParseDuration(getenv(key, fallback.String()))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}

func positiveInt(key string, fallback int) (int, error) {
	value, err := strconv.Atoi(getenv(key, strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func positiveInt64(key string, fallback int64) (int64, error) {
	value, err := strconv.ParseInt(getenv(key, strconv.FormatInt(fallback, 10)), 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
