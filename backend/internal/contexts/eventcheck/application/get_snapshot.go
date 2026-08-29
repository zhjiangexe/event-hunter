package application

import (
	"context"

	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type GetSnapshotHandler struct {
	repository ports.SnapshotRepository
}

func NewGetSnapshotHandler(repository ports.SnapshotRepository) *GetSnapshotHandler {
	return &GetSnapshotHandler{repository: repository}
}

func (service *GetSnapshotHandler) Get(ctx context.Context, id string) (Snapshot, error) {
	persisted, err := service.repository.Get(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	response, err := FromDomain(persisted)
	if err != nil {
		return Snapshot{}, err
	}
	feedback, err := service.repository.ListFeedback(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	ApplyFindingFeedback(&response, feedback)
	return response, nil
}
