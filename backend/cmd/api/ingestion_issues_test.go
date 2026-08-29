package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ingestionissues "event-hunter/backend/internal/contexts/investigation/application/search"
)

type stubIngestionIssueSearcher struct {
	filter ingestionissues.Filter
	page   ingestionissues.Page
	err    error
}

func (stub *stubIngestionIssueSearcher) Search(_ context.Context, filter ingestionissues.Filter) (ingestionissues.Page, error) {
	stub.filter = filter
	return stub.page, stub.err
}

func TestListIngestionIssuesDefaultsToLast72Hours(t *testing.T) {
	fixedNow := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	stub := &stubIngestionIssueSearcher{page: ingestionissues.Page{Items: []ingestionissues.Issue{}, PageSize: 20}}
	handler := ingestionIssuesAPI{service: stub, now: func() time.Time { return fixedNow }}
	response := httptest.NewRecorder()
	handler.list(response, httptest.NewRequest(http.MethodGet, "/api/v1/ingestion-issues?kind=TECHNICAL_DLQ&source_topic=order.events", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !stub.filter.To.Equal(fixedNow) || !stub.filter.From.Equal(fixedNow.Add(-72*time.Hour)) || stub.filter.Kind != ingestionissues.KindTechnicalDLQ || stub.filter.SourceTopic != "order.events" {
		t.Fatalf("filter = %#v", stub.filter)
	}
}

func TestListIngestionIssuesRejectsPartialWindowAndMalformedCursor(t *testing.T) {
	stub := &stubIngestionIssueSearcher{}
	handler := ingestionIssuesAPI{service: stub}
	for _, target := range []string{
		"/api/v1/ingestion-issues?from=2026-08-27T00:00:00Z",
		"/api/v1/ingestion-issues?cursor=raw-sql",
	} {
		response := httptest.NewRecorder()
		handler.list(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("target %s status = %d, body = %s", target, response.Code, response.Body.String())
		}
	}
}

func TestIngestionIssueJSONNeverIncludesUnsafePayloadFields(t *testing.T) {
	stub := &stubIngestionIssueSearcher{page: ingestionissues.Page{Items: []ingestionissues.Issue{{
		ID: "failure-1", Kind: ingestionissues.KindContractValidation, OccurredAt: "2026-08-27T12:00:00Z",
		Pipeline: "redpanda-connect/domain-events", ErrorCode: "SCHEMA_VIOLATION", PayloadSHA256: strings.Repeat("a", 64),
	}}, PageSize: 20}}
	handler := ingestionIssuesAPI{service: stub}
	response := httptest.NewRecorder()
	handler.list(response, httptest.NewRequest(http.MethodGet, "/api/v1/ingestion-issues", nil))
	body := strings.ToLower(response.Body.String())
	for _, unsafe := range []string{"raw_payload", "error_summary", "exception_message", "stacktrace"} {
		if strings.Contains(body, unsafe) {
			t.Fatalf("response exposes %q: %s", unsafe, body)
		}
	}
}
