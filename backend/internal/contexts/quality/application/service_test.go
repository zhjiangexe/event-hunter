package application

import (
	"context"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/quality/domain"
)

type recordingAggregator struct {
	windows []domain.Window
}

func (aggregator *recordingAggregator) Aggregate(_ context.Context, window domain.Window) error {
	aggregator.windows = append(aggregator.windows, window)
	return nil
}

func TestBackfillRunsEachWindowInOrder(t *testing.T) {
	start := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	aggregator := &recordingAggregator{}
	service := NewService(aggregator)
	if err := service.Backfill(context.Background(), start, start.Add(150*time.Second), time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(aggregator.windows) != 3 || !aggregator.windows[2].To.Equal(start.Add(150*time.Second)) {
		t.Fatalf("windows = %#v", aggregator.windows)
	}
}
