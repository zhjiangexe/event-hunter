package application

import (
	"context"
	"errors"
	"time"

	"event-hunter/backend/internal/contexts/quality/domain"
	"event-hunter/backend/internal/contexts/quality/ports"
)

type Schedule struct {
	Interval time.Duration
	Window   time.Duration
	Grace    time.Duration
}

type ScheduleResult struct {
	Window domain.Window
	Err    error
}

type Service struct {
	aggregator ports.Aggregator
}

func NewService(aggregator ports.Aggregator) *Service {
	if aggregator == nil {
		panic("quality aggregator is required")
	}
	return &Service{aggregator: aggregator}
}

func (service *Service) Aggregate(ctx context.Context, window domain.Window) error {
	return service.aggregator.Aggregate(ctx, window)
}

func (service *Service) Backfill(ctx context.Context, from, to time.Time, duration time.Duration) error {
	windows, err := domain.Split(from, to, duration)
	if err != nil {
		return err
	}
	for _, window := range windows {
		if err := service.Aggregate(ctx, window); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) RunSchedule(ctx context.Context, schedule Schedule, report func(ScheduleResult)) error {
	if schedule.Interval <= 0 || schedule.Window <= 0 || schedule.Grace < 0 {
		return errors.New("interval and window must be positive; late-grace must not be negative")
	}
	if report == nil {
		report = func(ScheduleResult) {}
	}
	ticker := time.NewTicker(schedule.Interval)
	defer ticker.Stop()
	var lastWindowEnd time.Time
	for {
		window, err := domain.Eligible(time.Now(), schedule.Window, schedule.Grace)
		if err != nil {
			return err
		}
		if window.To.After(lastWindowEnd) {
			err = service.Aggregate(ctx, window)
			report(ScheduleResult{Window: window, Err: err})
			if err == nil {
				lastWindowEnd = window.To
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
