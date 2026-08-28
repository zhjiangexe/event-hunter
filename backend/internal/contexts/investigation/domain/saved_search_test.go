package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSavedSearchBuildsBoundedAllowlistedTimelineURL(t *testing.T) {
	from := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	search, err := NewSavedSearch("demo-viewer", "付款失敗", SavedSearchTimeline, SavedSearchQuery{
		From: from, To: from.Add(time.Hour), EventType: "PaymentFailed", IncludeProcessingAttempts: true,
	}, from)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"/timeline?", "event_type=PaymentFailed", "from=2026-08-20T11%3A00%3A00Z", "include_processing_attempts=true"} {
		if !strings.Contains(search.OpenURL, part) {
			t.Fatalf("open URL %q does not contain %q", search.OpenURL, part)
		}
	}
}

func TestSavedSearchRejectsUnboundedOrEmptyQueries(t *testing.T) {
	from := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	for _, query := range []SavedSearchQuery{
		{From: from, To: from.Add(8 * 24 * time.Hour), EventType: "PaymentFailed"},
		{From: from, To: from.Add(time.Hour)},
	} {
		if _, err := NewSavedSearch("demo-viewer", "invalid", SavedSearchTimeline, query, from); !errors.Is(err, ErrInvalidSavedSearch) {
			t.Fatalf("expected invalid search, got %v", err)
		}
	}
}

func TestJourneySavedSearchRequiresCorrelationID(t *testing.T) {
	from := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	_, err := NewSavedSearch("demo-viewer", "journey", SavedSearchJourney, SavedSearchQuery{From: from, To: from.Add(time.Hour)}, from)
	if !errors.Is(err, ErrInvalidSavedSearch) {
		t.Fatalf("expected invalid search, got %v", err)
	}
}

func TestRelativeSavedSearchRefreshesItsWindowAtOpenTime(t *testing.T) {
	createdAt := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	openedAt := createdAt.Add(48 * time.Hour)
	window := uint32(24 * 60 * 60)
	search, err := NewSavedSearch("demo-viewer", "最近付款失敗", SavedSearchTimeline, SavedSearchQuery{
		TimeMode: SavedSearchRelative, RelativeWindowSeconds: &window,
		From: createdAt.Add(-24 * time.Hour), To: createdAt, EventType: "PaymentFailed",
	}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	refreshed := search.RefreshOpenURL(openedAt)
	for _, part := range []string{"from=2026-08-21T11%3A00%3A00Z", "to=2026-08-22T11%3A00%3A00Z"} {
		if !strings.Contains(refreshed.OpenURL, part) {
			t.Fatalf("refreshed URL %q does not contain %q", refreshed.OpenURL, part)
		}
	}
}

func TestSavedSearchRejectsInvalidTimeModeCombinations(t *testing.T) {
	from := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	window := uint32(30)
	for _, query := range []SavedSearchQuery{
		{TimeMode: "SLIDING", From: from, To: from.Add(time.Hour), EventType: "PaymentFailed"},
		{TimeMode: SavedSearchRelative, From: from, To: from.Add(time.Hour), EventType: "PaymentFailed"},
		{TimeMode: SavedSearchRelative, RelativeWindowSeconds: &window, From: from, To: from.Add(time.Hour), EventType: "PaymentFailed"},
		{TimeMode: SavedSearchAbsolute, RelativeWindowSeconds: &window, From: from, To: from.Add(time.Hour), EventType: "PaymentFailed"},
	} {
		if _, err := NewSavedSearch("demo-viewer", "invalid", SavedSearchTimeline, query, from); !errors.Is(err, ErrInvalidSavedSearch) {
			t.Fatalf("expected invalid search for %#v, got %v", query, err)
		}
	}
}
