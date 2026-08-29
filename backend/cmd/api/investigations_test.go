package main

import (
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

func TestOptionalQueryWindowParsesOnlyExplicitPairs(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/summary", nil)
	from, to, err := optionalQueryWindow(request)
	if err != nil || from != nil || to != nil {
		t.Fatalf("empty window = %v, %v, %v", from, to, err)
	}
	oneSided := httptest.NewRequest(http.MethodGet, "/summary?from=2026-08-01T00:00:00Z", nil)
	if _, _, err := optionalQueryWindow(oneSided); err == nil {
		t.Fatal("one-sided query window was accepted")
	}
	overridden := httptest.NewRequest(http.MethodGet, "/summary?from=2026-08-20T12:00:00Z&to=2026-08-20T13:00:00Z", nil)
	from, to, err = optionalQueryWindow(overridden)
	if err != nil || from == nil || to == nil || from.Hour() != 12 || to.Hour() != 13 {
		t.Fatalf("explicit query window = %v to %v, err %v", from, to, err)
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
