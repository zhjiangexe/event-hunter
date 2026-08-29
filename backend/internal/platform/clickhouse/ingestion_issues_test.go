package clickhouse

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ingestionissues "event-hunter/backend/internal/contexts/investigation/application/search"
)

func TestSearchIngestionIssuesUnifiesOnlySafeFailureFields(t *testing.T) {
	var statement string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read query: %v", err)
		}
		statement = string(body)
		_, _ = writer.Write([]byte(`{"id":"tech-1","kind":"TECHNICAL_DLQ","occurred_at":"2026-08-27 02:03:04.000","pipeline":"kafka-connect/clickhouse-sink","error_code":"CONNECTOR_TASK_FAILURE","event_id":null,"event_type":null,"correlation_id":null,"source_topic":"order.events","source_partition":1,"source_offset":42,"dlq_topic":"event-hunter.poc-clickhouse-sink.dlq","dlq_partition":0,"dlq_offset":5,"payload_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","admission_profile":null,"connector_name":"event-hunter-poc-raw-landing","connector_task":0,"failure_stage":"VALUE_CONVERTER","exception_class":"org.example.BadValue"}` + "\n"))
	}))
	defer server.Close()
	model := NewHTTPReadModel(HTTPReadModelConfig{URL: server.URL, QueryTimeout: time.Second, Client: server.Client(), MaxResultRows: 100, MaxResultBytes: 1 << 20, MaxRowsToRead: 1000, MaxBytesToRead: 1 << 20, MaxThreads: 1})
	from := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	issues, err := model.SearchIngestionIssues(t.Context(), ingestionissues.Filter{
		From: from, To: from.Add(4 * time.Hour), PageSize: 20, Kind: ingestionissues.KindTechnicalDLQ, SourceTopic: "order.events",
	})
	if err != nil {
		t.Fatalf("SearchIngestionIssues() error = %v", err)
	}
	if len(issues) != 1 || issues[0].OccurredAt != "2026-08-27T02:03:04Z" || issues[0].SourceOffset == nil || *issues[0].SourceOffset != 42 {
		t.Fatalf("issues = %#v", issues)
	}
	for _, unsafe := range []string{"error_summary", "raw_payload", "exception_message", "exception_stacktrace"} {
		if strings.Contains(strings.ToLower(statement), unsafe) {
			t.Fatalf("query exposes unsafe field %q: %s", unsafe, statement)
		}
	}
	for _, required := range []string{"FROM event_ingestion_failures", "FROM poc_event_admission_failures", "FROM poc_processing_attempt_admission_failures", "FROM ingestion_technical_failures", "kind='TECHNICAL_DLQ'", "source_topic='order.events'"} {
		if !strings.Contains(statement, required) {
			t.Errorf("query does not contain %q: %s", required, statement)
		}
	}
}

func TestSearchIngestionIssuesUsesDescendingKeysetCursor(t *testing.T) {
	var statement string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		statement = string(body)
	}))
	defer server.Close()
	model := NewHTTPReadModel(HTTPReadModelConfig{URL: server.URL, QueryTimeout: time.Second, Client: server.Client(), MaxResultRows: 100, MaxResultBytes: 1 << 20, MaxRowsToRead: 1000, MaxBytesToRead: 1 << 20, MaxThreads: 1})
	from := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	_, err := model.SearchIngestionIssues(t.Context(), ingestionissues.Filter{
		From: from, To: from.Add(time.Hour), PageSize: 20,
		Cursor: &ingestionissues.Cursor{OccurredAt: from.Add(30 * time.Minute), IssueID: "issue'20"},
	})
	if err != nil {
		t.Fatalf("SearchIngestionIssues() error = %v", err)
	}
	if !strings.Contains(statement, "id < 'issue''20'") || !strings.Contains(statement, "ORDER BY occurred_at DESC,id DESC") || !strings.Contains(statement, "LIMIT 21") {
		t.Fatalf("query does not use bounded descending keyset pagination: %s", statement)
	}
}
