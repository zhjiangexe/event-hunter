package journeyprofiles

import (
	"sort"

	"event-hunter/backend/internal/contexts/investigation/domain/journeys"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

// List returns the immutable profiles compiled into this API build. Runtime
// editing and publishing deliberately remain outside this read-only use case.
func (service *Service) List() []journeys.Profile {
	profiles := journeys.Registry()
	sort.SliceStable(profiles, func(left, right int) bool {
		if profiles[left].Default != profiles[right].Default {
			return profiles[left].Default
		}
		if profiles[left].ID != profiles[right].ID {
			return profiles[left].ID < profiles[right].ID
		}
		return profiles[left].Version > profiles[right].Version
	})
	return profiles
}
