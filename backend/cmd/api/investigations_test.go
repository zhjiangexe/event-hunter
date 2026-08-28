package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

func TestValidTransitionMatchesStateMachine(t *testing.T) {
	cases := []struct {
		from, to string
		valid    bool
	}{
		{"OPEN", "INVESTIGATING", true},
		{"INVESTIGATING", "WAITING_APPROVAL", true},
		{"INVESTIGATING", "RESOLVED", true},
		{"WAITING_APPROVAL", "INVESTIGATING", true},
		{"WAITING_APPROVAL", "RESOLVED", true},
		{"RESOLVED", "INVESTIGATING", true},
		{"OPEN", "RESOLVED", false},
		{"CLOSED", "OPEN", false},
		{"RESOLVED", "CLOSED", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.from+"_to_"+testCase.to, func(t *testing.T) {
			if got := validTransition(testCase.from, testCase.to); got != testCase.valid {
				t.Fatalf("validTransition(%q, %q) = %v, want %v", testCase.from, testCase.to, got, testCase.valid)
			}
		})
	}
}

func TestQueryWindowDefaultsAndBounds(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/summary", nil)
	from, to, err := queryWindow(request)
	if err != nil {
		t.Fatalf("default window error = %v", err)
	}
	if !to.After(from) || to.Sub(from) != 72*time.Hour {
		t.Fatalf("default window = %s to %s, want 72 hours", from, to)
	}
	tooLarge := httptest.NewRequest(http.MethodGet, "/summary?from=2026-08-01T00:00:00Z&to=2026-08-20T00:00:00Z", nil)
	if _, _, err := queryWindow(tooLarge); err == nil {
		t.Fatal("query window over seven days was accepted")
	}
}

func TestQueryWindowUsesPersistedIncidentWindowUntilExplicitlyOverridden(t *testing.T) {
	baseline := domain.IncidentWindow{
		From:   time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		To:     time.Date(2026, 8, 20, 11, 6, 0, 0, time.UTC),
		Source: domain.IncidentWindowTimelineSearch,
	}
	from, to, err := queryWindow(httptest.NewRequest(http.MethodGet, "/summary", nil), baseline)
	if err != nil || !from.Equal(baseline.From) || !to.Equal(baseline.To) {
		t.Fatalf("baseline query window = %s to %s, err %v", from, to, err)
	}

	overridden := httptest.NewRequest(http.MethodGet, "/summary?from=2026-08-20T12:00:00Z&to=2026-08-20T13:00:00Z", nil)
	from, to, err = queryWindow(overridden, baseline)
	if err != nil || from.Hour() != 12 || to.Hour() != 13 {
		t.Fatalf("explicit query window = %s to %s, err %v", from, to, err)
	}
	if !baseline.From.Equal(time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)) {
		t.Fatal("explicit query mutated the persisted baseline")
	}
}

func TestManualIncidentWindowDefaultsAndValidatesExplicitPair(t *testing.T) {
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	window, err := incidentWindowForManualCase("", "", now)
	if err != nil || window.Source != domain.IncidentWindowManualDefault || window.To.Sub(window.From) != 72*time.Hour {
		t.Fatalf("default incident window = %#v, err %v", window, err)
	}
	explicit, err := incidentWindowForManualCase("2026-08-20T11:00:00Z", "2026-08-20T11:06:00Z", now)
	if err != nil || explicit.Source != domain.IncidentWindowTimelineSearch {
		t.Fatalf("explicit incident window = %#v, err %v", explicit, err)
	}
	if _, err := incidentWindowForManualCase("2026-08-20T11:00:00Z", "", now); !errors.Is(err, domain.ErrInvalidIncidentWindow) {
		t.Fatalf("one-sided incident window error = %v", err)
	}
}

func TestClickHouseSummaryFailureKeepsPartialCaseSemantics(t *testing.T) {
	status, warnings := clickHouseSummaryFailure(context.DeadlineExceeded)
	if status != "TIMEOUT" || len(warnings) != 1 || warnings[0] != "CLICKHOUSE_TIMEOUT" {
		t.Fatalf("timeout classification = %q %#v", status, warnings)
	}
	status, warnings = clickHouseSummaryFailure(errors.New("dial failed"))
	if status != "UNAVAILABLE" || warnings[0] != "CLICKHOUSE_UNAVAILABLE" {
		t.Fatalf("unavailable classification = %q %#v", status, warnings)
	}
	timeline := unavailableTimeline("ORDER-1", time.Now().Add(-time.Hour), time.Now())
	if timeline["correlation_id"] != "ORDER-1" || timeline["event_count"] != 0 {
		t.Fatalf("partial timeline = %#v", timeline)
	}
}

func TestQueryLimitClampsAndDefaults(t *testing.T) {
	if got := queryLimit(httptest.NewRequest(http.MethodGet, "/summary", nil)); got != 1000 {
		t.Fatalf("default limit = %d, want 1000", got)
	}
	if got := queryLimit(httptest.NewRequest(http.MethodGet, "/summary?limit=20000", nil)); got != 10000 {
		t.Fatalf("maximum limit = %d, want 10000", got)
	}
	if got := queryLimit(httptest.NewRequest(http.MethodGet, "/summary?limit=-1", nil)); got != 1 {
		t.Fatalf("negative limit = %d, want 1", got)
	}
}

func TestInvestigationCursorBindsSortAndPreservesTimestamp(t *testing.T) {
	want := investigationCursor{SortBy: "updated_at", SortOrder: "asc", Time: time.Date(2026, 8, 24, 10, 11, 12, 345, time.UTC), ID: "11111111-1111-4111-8111-111111111111"}
	decoded, err := decodeInvestigationCursor(encodeInvestigationCursor(want))
	if err != nil {
		t.Fatalf("decodeInvestigationCursor() error = %v", err)
	}
	if decoded.SortBy != want.SortBy || decoded.SortOrder != want.SortOrder || decoded.ID != want.ID || !decoded.Time.Equal(want.Time) {
		t.Fatalf("decoded = %#v, want %#v", decoded, want)
	}
	if _, err := decodeInvestigationCursor("not-a-cursor"); err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

func TestEvidenceManifestStateReportsMissingChecksum(t *testing.T) {
	partial, warnings := evidenceManifestState([]map[string]any{
		{"id": "complete", "checksum": "abc"},
		{"id": "missing", "checksum": nil},
	})
	if !partial {
		t.Fatal("manifest with a missing checksum was not marked partial")
	}
	if len(warnings) != 1 || warnings[0] != "EVIDENCE_CHECKSUM_MISSING:missing" {
		t.Fatalf("warnings = %#v, want missing-checksum warning", warnings)
	}
}

func TestEvidenceSourceUsesOpenActionAllowlist(t *testing.T) {
	tests := map[string][2]string{
		"EVENT":           {"CLICKHOUSE", "GRAFANA_EVENT"},
		"TRACE":           {"TEMPO", "GRAFANA_TEMPO"},
		"LOG":             {"LOKI", "GRAFANA_LOKI"},
		"PATTERN_FINDING": {"PATTERN_ENGINE", "PATTERN_LIBRARY"},
		"unexpected":      {"UNKNOWN", "NONE"},
	}
	for evidenceType, expected := range tests {
		source, action := evidenceSource(evidenceType)
		if source != expected[0] || action != expected[1] {
			t.Fatalf("evidenceSource(%q) = %q, %q; want %q, %q", evidenceType, source, action, expected[0], expected[1])
		}
	}
}

func TestGrafanaAlertSourcePathAcceptsOnlyRuleDetailPaths(t *testing.T) {
	tests := []struct {
		value string
		path  string
		valid bool
	}{
		{"http://grafana:3000/alerting/grafana/rule_uid/view?orgId=1", "/alerting/grafana/rule_uid/view", true},
		{`http:\/\/grafana:3000\/alerting\/grafana\/rule_uid\/view`, "/alerting/grafana/rule_uid/view", true},
		{"https://grafana.example/alerting/rule-uid/edit", "/alerting/rule-uid/edit", true},
		{"https://attacker.example/login", "", false},
		{"javascript:alert(1)", "", false},
		{"https://user:password@grafana.example/alerting/rule/view", "", false},
		{"https://grafana.example/alerting/grafana/../admin/view", "", false},
	}
	for _, testCase := range tests {
		path, valid := grafanaAlertSourcePath(testCase.value)
		if valid != testCase.valid || path != testCase.path {
			t.Fatalf("grafanaAlertSourcePath(%q) = %q, %v; want %q, %v", testCase.value, path, valid, testCase.path, testCase.valid)
		}
	}
}
