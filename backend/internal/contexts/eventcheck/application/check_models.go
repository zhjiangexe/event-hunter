package application

import (
	"errors"
	"sort"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
)

var ErrCheckModelNotFound = errors.New("Check Model not found")

type CheckModelQueries struct {
	registry []domain.RegistryEntry
}

func NewCheckModelQueries() *CheckModelQueries {
	return &CheckModelQueries{registry: domain.Registry()}
}

func (service *CheckModelQueries) List() []domain.RegistryEntry {
	result := append([]domain.RegistryEntry(nil), service.registry...)
	sort.Slice(result, func(left, right int) bool {
		if result[left].Model.ID == result[right].Model.ID {
			return result[left].Model.Version > result[right].Model.Version
		}
		return result[left].Model.ID < result[right].Model.ID
	})
	return result
}

func (service *CheckModelQueries) Get(id string, version int) (domain.RegistryEntry, error) {
	for _, entry := range service.registry {
		if entry.Model.ID == id && entry.Model.Version == version {
			return entry, nil
		}
	}
	return domain.RegistryEntry{}, ErrCheckModelNotFound
}

func (service *CheckModelQueries) GetSource(id string, version int) (domain.ModelSourceDocument, error) {
	source, ok := domain.LookupModelSource(id, version)
	if !ok {
		return domain.ModelSourceDocument{}, ErrCheckModelNotFound
	}
	return source, nil
}
