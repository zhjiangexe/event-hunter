package clickhouse

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCorrelationEventWindowUsesAnUnboundedQualifiedAggregate(t *testing.T) {
	var statement string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		statement = string(body)
		_, _ = writer.Write([]byte(`{"first_occurred_at":"2025-01-02 10:00:00.000","last_occurred_at":"2025-01-02 10:01:00.000","event_count":2}`))
	}))
	defer server.Close()
	model := NewHTTPReadModel(HTTPReadModelConfig{URL: server.URL, QueryTimeout: time.Second, Client: server.Client()})

	first, last, count, err := model.CorrelationEventWindow(t.Context(), "ORDER'HISTORICAL")
	if err != nil {
		t.Fatalf("CorrelationEventWindow() error = %v", err)
	}
	if want := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC); !first.Equal(want) {
		t.Fatalf("first = %s, want %s", first, want)
	}
	if want := first.Add(time.Minute); !last.Equal(want) || count != 2 {
		t.Fatalf("last/count = %s/%d, want %s/2", last, count, want)
	}
	if !strings.Contains(statement, "WHERE correlation_id='ORDER''HISTORICAL'") || strings.Contains(statement, "occurred_at >=") {
		t.Fatalf("unexpected aggregate query: %s", statement)
	}
}

func TestDecodeCorrelationEventWindowKeepsNoEventsExplicit(t *testing.T) {
	first, last, count, err := decodeCorrelationEventWindow([]byte(`{"first_occurred_at":"1970-01-01 00:00:00.000","last_occurred_at":"1970-01-01 00:00:00.000","event_count":0}`))
	if err != nil || !first.IsZero() || !last.IsZero() || count != 0 {
		t.Fatalf("window = %s/%s/%d, error = %v", first, last, count, err)
	}
}

func TestValidateTimelineQuery(t *testing.T) {
	from := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	valid := TimelineQuery{
		CorrelationID: "ORDER-1001",
		From:          from,
		To:            from.Add(time.Hour),
		Limit:         1000,
	}
	if err := ValidateTimelineQuery(valid); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}

	cases := []struct {
		name string
		edit func(*TimelineQuery)
		want string
	}{
		{"missing correlation", func(query *TimelineQuery) { query.CorrelationID = "" }, "correlation ID is required"},
		{"missing time window", func(query *TimelineQuery) { query.From = time.Time{} }, "positive time window"},
		{"too wide", func(query *TimelineQuery) { query.To = query.From.Add(8 * 24 * time.Hour) }, "exceeds 7 days"},
		{"too many rows", func(query *TimelineQuery) { query.Limit = MaxTimelineLimit + 1 }, "between 0 and"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			query := valid
			testCase.edit(&query)
			err := ValidateTimelineQuery(query)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want text %q", err, testCase.want)
			}
		})
	}
}

func TestNormalizedLimit(t *testing.T) {
	if got := normalizedLimit(0); got != DefaultTimelineLimit {
		t.Fatalf("normalizedLimit(0) = %d, want %d", got, DefaultTimelineLimit)
	}
}

func TestDecodeEventsMapsClickHouseSnakeCaseFields(t *testing.T) {
	data := []byte(`{"event_id":"evt-1","event_type":"PaymentCompleted","event_version":1,"occurred_at":"2026-08-20 11:01:00.000","producer":"payment-service","correlation_id":"ORDER-2001","causation_id":"evt-0","trace_id":"11111111111111111111111111111111","aggregate_type":"Payment","aggregate_id":"ORDER-2001","sequence":2,"kafka_topic":"payment.events","kafka_partition":3,"kafka_offset":42,"service_version":"1.0.0","admission_status":"SEARCHABLE_WITH_WARNINGS","quality_flags":["UNKNOWN_EVENT_VERSION"],"admission_profile":"minimum-envelope-v1","ingested_at":"2026-08-20 11:01:01.000"}` + "\n")

	events, err := decodeEvents(data)
	if err != nil {
		t.Fatalf("decodeEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].EventID != "evt-1" || events[0].EventType != "PaymentCompleted" || events[0].KafkaTopic != "payment.events" || events[0].KafkaOffset != 42 {
		t.Fatalf("decoded events = %#v", events)
	}
	if events[0].OccurredAt != "2026-08-20T11:01:00Z" || events[0].IngestedAt != "2026-08-20T11:01:01Z" {
		t.Fatalf("timestamps = %q / %q, want RFC3339 UTC", events[0].OccurredAt, events[0].IngestedAt)
	}
	if events[0].AdmissionStatus != "SEARCHABLE_WITH_WARNINGS" || len(events[0].QualityFlags) != 1 || events[0].AdmissionProfile != "minimum-envelope-v1" {
		t.Fatalf("admission metadata = %#v", events[0])
	}
}

func TestNormalizeClickHouseTimestampPreservesExplicitOffsetAsUTC(t *testing.T) {
	got, err := normalizeClickHouseTimestamp("2026-08-20T19:01:00.125+08:00")
	if err != nil {
		t.Fatalf("normalizeClickHouseTimestamp() error = %v", err)
	}
	if want := "2026-08-20T11:01:00.125Z"; got != want {
		t.Fatalf("normalizeClickHouseTimestamp() = %q, want %q", got, want)
	}
}

func TestQuoteEscapesClickHouseStringLiteral(t *testing.T) {
	if got, want := quote("ORDER' OR 1=1 --"), "'ORDER'' OR 1=1 --'"; got != want {
		t.Fatalf("quote() = %q, want %q", got, want)
	}
}

func TestQuotedUniqueBuildsSafeQualifierValues(t *testing.T) {
	got := quotedUnique([]string{"ORDER-1", "ORDER'2", "ORDER-1", ""})
	want := []string{"'ORDER-1'", "'ORDER''2'"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("quotedUnique() = %#v, want %#v", got, want)
	}
}
