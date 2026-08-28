package list_check_snapshots

import (
	"context"
	"errors"
	"testing"
	"time"
)

type snapshotReadModel struct {
	filter Filter
	items  []Summary
}

func (model *snapshotReadModel) ListCheckSnapshotSummaries(_ context.Context, filter Filter) ([]Summary, error) {
	model.filter = filter
	return model.items, nil
}

func TestListUsesBoundedLookAheadAndStableCursor(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 10, 11, 12, 345, time.UTC)
	model := &snapshotReadModel{items: []Summary{
		{ID: "11111111-1111-4111-8111-111111111111", CreatedAt: createdAt},
		{ID: "22222222-2222-4222-8222-222222222222", CreatedAt: createdAt.Add(-time.Second)},
		{ID: "33333333-3333-4333-8333-333333333333", CreatedAt: createdAt.Add(-2 * time.Second)},
	}}
	page, err := NewService(model).List(context.Background(), Filter{Identifier: " ORDER-1 ", CheckStatus: "deviated", PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if model.filter.PageSize != 3 || model.filter.Identifier != "ORDER-1" || model.filter.CheckStatus != "DEVIATED" {
		t.Fatalf("read filter = %#v", model.filter)
	}
	if len(page.Items) != 2 || page.NextCursor == nil || page.PageSize != 2 {
		t.Fatalf("page = %#v", page)
	}
	cursor, err := DecodeCursor(*page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ID != page.Items[1].ID || !cursor.CreatedAt.Equal(page.Items[1].CreatedAt) {
		t.Fatalf("cursor = %#v, last = %#v", cursor, page.Items[1])
	}
}

func TestListRejectsInvalidInputs(t *testing.T) {
	service := NewService(&snapshotReadModel{})
	for _, filter := range []Filter{{PageSize: 101}, {PageSize: 20, CheckStatus: "FAILED"}} {
		if _, err := service.List(context.Background(), filter); !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("List(%#v) error = %v", filter, err)
		}
	}
	if _, err := DecodeCursor("not-a-cursor"); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
}
