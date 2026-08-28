package savedsearch

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

type Target = domain.SavedSearchTarget
type Query = domain.SavedSearchQuery
type SavedSearch = domain.SavedSearch
type Repository = domain.SavedSearchRepository

const (
	TargetTimeline             = domain.SavedSearchTimeline
	TargetJourney              = domain.SavedSearchJourney
	defaultPresetWindowSeconds = 72 * 60 * 60
)

type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OpenURL     string `json:"open_url"`
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (service *Service) Create(ctx context.Context, ownerSubject, name string, target Target, query Query) (SavedSearch, error) {
	search, err := domain.NewSavedSearch(ownerSubject, name, target, query, service.now())
	if err != nil {
		return SavedSearch{}, err
	}
	return service.repository.Create(ctx, search)
}

func (service *Service) List(ctx context.Context, ownerSubject string) ([]SavedSearch, error) {
	items, err := service.repository.ListByOwner(ctx, ownerSubject)
	if err != nil {
		return nil, err
	}
	now := service.now().UTC()
	for index := range items {
		items[index] = items[index].RefreshOpenURL(now)
	}
	return items, nil
}

func (service *Service) Delete(ctx context.Context, id, ownerSubject string) error {
	return service.repository.DeleteByOwner(ctx, id, ownerSubject)
}

func (service *Service) Presets() []Preset {
	now := service.now().UTC()
	from := now.Add(-time.Duration(defaultPresetWindowSeconds) * time.Second)
	definitions := []struct {
		id, name, description, eventType string
	}{
		{id: "payment-failed-72h", name: "最近付款失敗", description: "最近 72 小時的 PaymentFailed events。", eventType: "PaymentFailed"},
		{id: "shipment-dispatch-failed-72h", name: "最近派送失敗", description: "最近 72 小時的 ShipmentDispatchFailed events。", eventType: "ShipmentDispatchFailed"},
		{id: "order-cancelled-72h", name: "最近取消訂單", description: "最近 72 小時的 OrderCancelled events。", eventType: "OrderCancelled"},
		{id: "payment-refunded-72h", name: "最近付款退款", description: "最近 72 小時的 PaymentRefunded events。", eventType: "PaymentRefunded"},
	}
	result := make([]Preset, 0, len(definitions))
	for _, definition := range definitions {
		windowSeconds := uint32(defaultPresetWindowSeconds)
		search, _ := domain.NewSavedSearch("builtin", definition.name, domain.SavedSearchTimeline, domain.SavedSearchQuery{
			TimeMode: domain.SavedSearchRelative, RelativeWindowSeconds: &windowSeconds,
			From: from, To: now, EventType: definition.eventType, IncludeProcessingAttempts: true,
		}, now)
		result = append(result, Preset{ID: definition.id, Name: definition.name, Description: definition.description, OpenURL: search.OpenURL})
	}
	return result
}
