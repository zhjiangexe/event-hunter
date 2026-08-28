package get_check_snapshot

import (
	"context"

	savechecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/save_check_snapshot"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type Service struct {
	repository ports.SnapshotRepository
}

func NewService(repository ports.SnapshotRepository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Get(ctx context.Context, id string) (savechecksnapshot.Snapshot, error) {
	persisted, err := service.repository.Get(ctx, id)
	if err != nil {
		return savechecksnapshot.Snapshot{}, err
	}
	response, err := savechecksnapshot.ToResponse(persisted)
	if err != nil {
		return savechecksnapshot.Snapshot{}, err
	}
	feedback, err := service.repository.ListFeedback(ctx, id)
	if err != nil {
		return savechecksnapshot.Snapshot{}, err
	}
	savechecksnapshot.ApplyFindingFeedback(&response, feedback)
	return response, nil
}
