package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/platform/config"
)

func TestHealthEndpoints(t *testing.T) {
	server := newServer(config.Config{HTTPAddress: ":28333"})
	for _, path := range []string{"/health/live", "/health/ready"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
		})
	}
}

func TestDefaultJSONContentTypeCoversAPIResponsesOnly(t *testing.T) {
	handler := defaultJSONContentType(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/no-content" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	}))

	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/v1/example", nil))
	if got := apiResponse.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("API content type = %q, want application/json", got)
	}

	noContentResponse := httptest.NewRecorder()
	handler.ServeHTTP(noContentResponse, httptest.NewRequest(http.MethodDelete, "/api/v1/no-content", nil))
	if got := noContentResponse.Header().Get("Content-Type"); got != "" {
		t.Fatalf("204 content type = %q, want empty", got)
	}

	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if got := healthResponse.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("non-API content type = %q, want net/http default", got)
	}
}

func TestSearchEventsUsesAllowlistedFilters(t *testing.T) {
	var clickhouseQuery string
	var clickhouseSettings map[string]string
	clickhouse := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read ClickHouse query: %v", err)
		}
		clickhouseQuery = string(body)
		clickhouseSettings = map[string]string{}
		for key, values := range request.URL.Query() {
			clickhouseSettings[key] = values[0]
		}
		writer.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = writer.Write([]byte("{\"event_id\":\"evt-1\",\"event_type\":\"PaymentCompleted\",\"event_version\":1,\"occurred_at\":\"2026-08-20 11:01:00.000\",\"producer\":\"payment-service\",\"correlation_id\":\"ORDER-2001\",\"aggregate_type\":\"Payment\",\"aggregate_id\":\"ORDER-2001\",\"sequence\":2,\"kafka_topic\":\"payment.events\",\"kafka_partition\":0,\"kafka_offset\":42,\"ingested_at\":\"2026-08-20 11:01:01.000\"}\n"))
	}))
	defer clickhouse.Close()
	t.Setenv("CLICKHOUSE_URL", clickhouse.URL)

	server := newServerWithWebhook(config.Config{HTTPAddress: ":28333"}, http.NotFoundHandler(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?from=2026-08-20T11:00:00Z&to=2026-08-20T11:06:00Z&correlation_id=ORDER-2001&event_type=PaymentCompleted&kafka_partition=0", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, condition := range []string{"correlation_id='ORDER-2001'", "event_type='PaymentCompleted'", "kafka_partition=0"} {
		if !strings.Contains(clickhouseQuery, condition) {
			t.Errorf("ClickHouse query does not contain %q: %s", condition, clickhouseQuery)
		}
	}
	if !strings.Contains(clickhouseQuery, "PARTITION BY kafka_topic,kafka_partition,kafka_offset") || !strings.Contains(clickhouseQuery, "_delivery_rank=1") {
		t.Fatalf("ClickHouse query does not deduplicate sink redelivery by transport identity: %s", clickhouseQuery)
	}
	if !strings.Contains(response.Body.String(), `"count":1`) {
		t.Fatalf("response does not contain one result: %s", response.Body.String())
	}
	for key, want := range map[string]string{
		"database":             "event_hunter",
		"readonly":             "2",
		"max_execution_time":   "3",
		"max_result_rows":      "10000",
		"max_result_bytes":     "8388608",
		"max_rows_to_read":     "5000000",
		"max_bytes_to_read":    "536870912",
		"result_overflow_mode": "throw",
		"max_threads":          "4",
	} {
		if clickhouseSettings[key] != want {
			t.Errorf("ClickHouse setting %s = %q, want %q", key, clickhouseSettings[key], want)
		}
	}
}

func TestProcessingSummariesDeduplicateAttemptIdentity(t *testing.T) {
	var query string
	clickhouse := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read ClickHouse query: %v", err)
		}
		query = string(body)
		_, _ = writer.Write([]byte("{\"event_id\":\"evt-1\",\"attempt_count\":1,\"last_attempt\":1,\"final_status\":\"SUCCEEDED\",\"consumer_groups\":[\"consumer-a\"],\"last_attempt_at\":\"2026-08-20 11:00:01.000\"}\n"))
	}))
	defer clickhouse.Close()
	t.Setenv("CLICKHOUSE_URL", clickhouse.URL)

	events := []timelineEvent{{timelineEventMetadata: timelineEventMetadata{EventID: "evt-1"}}}
	summaries, err := processingSummaries(t.Context(), newForensicsService(config.Config{}), events)
	if err != nil {
		t.Fatalf("processing summaries: %v", err)
	}
	if summaries["evt-1"]["attempt_count"] != 1 {
		t.Fatalf("attempt_count = %#v, want 1", summaries["evt-1"]["attempt_count"])
	}
	if !strings.Contains(query, "GROUP BY attempt_id") {
		t.Fatalf("processing summary query does not deduplicate attempt_id: %s", query)
	}
}

func TestSearchEventsRejectsWindowOverSevenDays(t *testing.T) {
	server := newServerWithWebhook(config.Config{HTTPAddress: ":28333"}, http.NotFoundHandler(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?from=2026-08-01T00:00:00Z&to=2026-08-09T00:00:00Z&event_id=evt-1", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.Code)
	}
	if !strings.Contains(response.Body.String(), "INVALID_TIME_WINDOW") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestSearchEventsRejectsUnknownPattern(t *testing.T) {
	server := newServerWithWebhook(config.Config{HTTPAddress: ":28333"}, http.NotFoundHandler(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?from=2026-08-20T11:00:00Z&to=2026-08-20T11:06:00Z&pattern_id=runtime-pattern", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "UNKNOWN_PATTERN") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSearchEventsRejectsInvalidSeverity(t *testing.T) {
	server := newServerWithWebhook(config.Config{HTTPAddress: ":28333"}, http.NotFoundHandler(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?from=2026-08-20T11:00:00Z&to=2026-08-20T11:06:00Z&severity=URGENT", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "INVALID_SEVERITY") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestDecodeTimelineEventsMasksPayload(t *testing.T) {
	events, err := timelineEventsFromForensics([]forensics.ForensicsEvent{{
		EventID: "evt-1", EventType: "OrderCreated", EventVersion: 1, OccurredAt: "2026-08-20 11:00:00.000",
		Producer: "order-service", CorrelationID: "ORDER-1", AggregateType: "Order", AggregateID: "ORDER-1", Sequence: 1,
		KafkaTopic: "order.events", KafkaPartition: 0, KafkaOffset: 1, IngestedAt: "2026-08-20 11:00:01.000",
		AdmissionStatus: "SEARCHABLE_WITH_WARNINGS", QualityFlags: []string{"UNKNOWN_EVENT_VERSION"}, AdmissionProfile: "minimum-envelope-v1",
		Payload: `{"orderId":"ORDER-1","customerId":"CUSTOMER-1","totalAmount":1280}`,
	}}, true)
	if err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Payload["orderId"] != "ORDER-1" {
		t.Fatalf("internal identifier was unexpectedly masked: %#v", events[0].Payload)
	}
	if events[0].AdmissionStatus != "SEARCHABLE_WITH_WARNINGS" || len(events[0].QualityFlags) != 1 || events[0].AdmissionProfile != "minimum-envelope-v1" {
		t.Fatalf("admission metadata was not preserved: %#v", events[0].timelineEventMetadata)
	}
	if events[0].Payload["totalAmount"] != "[REDACTED_AMOUNT]" {
		t.Fatalf("amount was not masked: %#v", events[0].Payload)
	}
	if !strings.HasPrefix(events[0].Payload["customerId"].(string), "CUSTOMER-***-") {
		t.Fatalf("customer ID was not tokenized: %#v", events[0].Payload)
	}
}

func TestTimelineReadRequiresSession(t *testing.T) {
	manager := sessionManager{secret: []byte("test-secret")}
	server := newServerWithWebhook(config.Config{HTTPAddress: ":28333"}, http.NotFoundHandler(), &manager)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?from=2026-08-20T11:00:00Z&to=2026-08-20T11:06:00Z&event_id=evt-1", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestAPIProtectionAddsDeadline(t *testing.T) {
	cfg := config.Config{HTTPRequestTimeout: 250 * time.Millisecond}
	server := protectAPI(cfg, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) > 250*time.Millisecond {
			t.Fatalf("request deadline = %v, ok = %v", deadline, ok)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/patterns", nil)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestAPIProtectionRateLimitsByRemoteAddress(t *testing.T) {
	cfg := config.Config{RateLimitRequests: 2, RateLimitWindow: time.Minute}
	server := protectAPI(cfg, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	for attempt := 1; attempt <= 3; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/patterns", nil)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if attempt <= 2 && response.Code != http.StatusNoContent {
			t.Fatalf("attempt %d status = %d, want 204", attempt, response.Code)
		}
		if attempt == 3 {
			if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "RATE_LIMIT_EXCEEDED") {
				t.Fatalf("attempt 3 status = %d, body = %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Retry-After") == "" {
				t.Fatal("rate-limited response has no Retry-After header")
			}
		}
	}

	healthRequest := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	healthResponse := httptest.NewRecorder()
	server.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusNoContent {
		t.Fatalf("health status = %d, want 204 without rate limiting", healthResponse.Code)
	}
}

func TestSearchEventsReturnsGatewayTimeoutWhenClickHouseExceedsBudget(t *testing.T) {
	previousClient := clickHouseHTTPClient
	clickHouseHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	defer func() { clickHouseHTTPClient = previousClient }()

	cfg := config.Config{ClickHouseQueryTimeout: 20 * time.Millisecond, HTTPRequestTimeout: time.Second}
	server := newServerWithWebhook(cfg, http.NotFoundHandler(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events/search?from=2026-08-20T11:00:00Z&to=2026-08-20T11:06:00Z&event_id=evt-1", nil)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), "EVENT_SEARCH_TIMEOUT") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestPostgresURLIncludesStatementTimeout(t *testing.T) {
	result := postgresURL(1500 * time.Millisecond)
	if !strings.Contains(result, "statement_timeout=1500") {
		t.Fatalf("postgres URL has no statement timeout: %s", result)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
