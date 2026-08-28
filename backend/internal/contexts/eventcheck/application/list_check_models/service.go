package list_check_models

import (
	"errors"
	"sort"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
)

var ErrNotFound = errors.New("Check Model not found")

type Service struct {
	registry []domain.RegistryEntry
}

func NewService() *Service {
	return &Service{registry: domain.Registry()}
}

func (service *Service) List() []domain.RegistryEntry {
	result := append([]domain.RegistryEntry(nil), service.registry...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Model.ID == result[right].Model.ID {
			return result[left].Model.Version > result[right].Model.Version
		}
		return result[left].Model.ID < result[right].Model.ID
	})
	return result
}

func (service *Service) Get(id string, version int) (domain.RegistryEntry, error) {
	for _, entry := range service.registry {
		if entry.Model.ID == id && entry.Model.Version == version {
			return entry, nil
		}
	}
	return domain.RegistryEntry{}, ErrNotFound
}
