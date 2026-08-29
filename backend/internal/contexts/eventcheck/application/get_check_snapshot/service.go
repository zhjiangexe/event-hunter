package get_check_snapshot

import (
	"context"

	"event-hunter/backend/internal/contexts/eventcheck/application/snapshotview"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type Service struct {
	repository ports.SnapshotRepository
}

func NewService(repository ports.SnapshotRepository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Get(ctx context.Context, id string) (snapshotview.Snapshot, error) {
	persisted, err := service.repository.Get(ctx, id)
	if err != nil {
		return snapshotview.Snapshot{}, err
	}
	response, err := snapshotview.FromDomain(persisted)
	if err != nil {
		return snapshotview.Snapshot{}, err
	}
	feedback, err := service.repository.ListFeedback(ctx, id)
	if err != nil {
		return snapshotview.Snapshot{}, err
	}
	snapshotview.ApplyFindingFeedback(&response, feedback)
	return response, nil
}
