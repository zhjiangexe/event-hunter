package search

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubReadModel struct {
	rows   []Issue
	filter Filter
	err    error
}

func (stub *stubReadModel) SearchIngestionIssues(_ context.Context, filter Filter) ([]Issue, error) {
	stub.filter = filter
	return stub.rows, stub.err
}

func TestSearchUsesKeysetCursorWithoutReturningLookaheadRow(t *testing.T) {
	reader := &stubReadModel{rows: []Issue{
		{ID: "issue-3", OccurredAt: "2026-08-27T03:00:00Z"},
		{ID: "issue-2", OccurredAt: "2026-08-27T02:00:00Z"},
		{ID: "issue-1", OccurredAt: "2026-08-27T01:00:00Z"},
	}}
	service := NewIngestionIssueService(reader)
	page, err := service.Search(t.Context(), Filter{
		From: time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 8, 27, 4, 0, 0, 0, time.UTC), PageSize: 2,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == nil || reader.filter.PageSize != 2 {
		t.Fatalf("page = %#v, reader filter = %#v", page, reader.filter)
	}
	cursor, err := DecodeCursor(*page.NextCursor)
	if err != nil || cursor.IssueID != "issue-2" || cursor.OccurredAt.Hour() != 2 {
		t.Fatalf("cursor = %#v, error = %v", cursor, err)
	}
}

func TestSearchRejectsUnsafeWindowAndKind(t *testing.T) {
	service := NewIngestionIssueService(&stubReadModel{})
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, filter := range []Filter{
		{From: from, To: from.Add(8 * 24 * time.Hour)},
		{From: from, To: from.Add(time.Hour), Kind: "UNKNOWN"},
	} {
		if _, err := service.Search(t.Context(), filter); !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("Search(%#v) error = %v, want ErrInvalidFilter", filter, err)
		}
	}
}

func TestDecodeCursorRejectsArbitraryInput(t *testing.T) {
	if _, err := DecodeCursor("raw-sql-or-garbage"); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("DecodeCursor() error = %v, want ErrInvalidFilter", err)
	}
}
