package savedsearch

import (
	"context"
	"strings"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

type repositoryStub struct {
	created SavedSearch
	owner   string
	deleted string
}

func (stub *repositoryStub) Create(_ context.Context, search SavedSearch) (SavedSearch, error) {
	stub.created = search
	search.ID = "saved-1"
	return search, nil
}
func (stub *repositoryStub) ListByOwner(_ context.Context, owner string) ([]SavedSearch, error) {
	stub.owner = owner
	return []SavedSearch{stub.created}, nil
}
func (stub *repositoryStub) DeleteByOwner(_ context.Context, id, owner string) error {
	stub.deleted = id + ":" + owner
	return nil
}

func TestServiceScopesPersistenceToAuthenticatedSubject(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) }
	query := Query{From: service.now().Add(-time.Hour), To: service.now(), CorrelationID: "ORDER-2001"}
	created, err := service.Create(context.Background(), "demo-viewer", "我的訂單", TargetJourney, query)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "saved-1" || repository.created.OwnerSubject != "demo-viewer" {
		t.Fatalf("created = %#v", created)
	}
	_, _ = service.List(context.Background(), "demo-viewer")
	_ = service.Delete(context.Background(), "saved-1", "demo-viewer")
	if repository.owner != "demo-viewer" || repository.deleted != "saved-1:demo-viewer" {
		t.Fatalf("scope lost: owner=%q delete=%q", repository.owner, repository.deleted)
	}
}

func TestPresetsUseOneBoundedWindowAndAllowlistedEventTypes(t *testing.T) {
	service := NewService(&repositoryStub{})
	service.now = func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) }
	presets := service.Presets()
	if len(presets) != 4 {
		t.Fatalf("presets = %#v", presets)
	}
	if presets[0].ID != "payment-failed-72h" || presets[0].OpenURL == "" {
		t.Fatalf("first preset = %#v", presets[0])
	}
	if _, err := domain.NewSavedSearch("demo-viewer", "copy", domain.SavedSearchTimeline, Query{
		From: service.now().Add(-72 * time.Hour), To: service.now(), EventType: "PaymentFailed",
	}, service.now()); err != nil {
		t.Fatal(err)
	}
}

func TestListRefreshesRelativeURLsAtReadTime(t *testing.T) {
	repository := &repositoryStub{}
	createdAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	window := uint32(60 * 60)
	search, err := domain.NewSavedSearch("demo-viewer", "最近一小時", domain.SavedSearchTimeline, Query{
		TimeMode: domain.SavedSearchRelative, RelativeWindowSeconds: &window,
		From: createdAt.Add(-time.Hour), To: createdAt, EventType: "PaymentFailed",
	}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	repository.created = search
	service := NewService(repository)
	service.now = func() time.Time { return createdAt.Add(24 * time.Hour) }

	items, err := service.List(context.Background(), "demo-viewer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(items[0].OpenURL, "from=2026-08-21T11%3A00%3A00Z") {
		t.Fatalf("relative URL was not refreshed: %s", items[0].OpenURL)
	}
}
