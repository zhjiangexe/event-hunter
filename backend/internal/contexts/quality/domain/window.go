package domain

import (
	"errors"
	"fmt"
	"time"
)

const MaxBackfillDuration = 31 * 24 * time.Hour

type Window struct {
	From time.Time
	To   time.Time
}

func NewWindow(from, to time.Time) (Window, error) {
	from = from.UTC()
	to = to.UTC()
	if !to.After(from) || to.Sub(from) > MaxBackfillDuration {
		return Window{}, fmt.Errorf("to must be after from and no wider than %d days", int(MaxBackfillDuration.Hours()/24))
	}
	return Window{From: from, To: to}, nil
}

func Split(from, to time.Time, duration time.Duration) ([]Window, error) {
	if duration <= 0 {
		return nil, errors.New("window duration must be positive")
	}
	if _, err := NewWindow(from, to); err != nil {
		return nil, err
	}
	result := make([]Window, 0, int(to.Sub(from)/duration)+1)
	for start := from.UTC(); start.Before(to.UTC()); start = start.Add(duration) {
		end := start.Add(duration)
		if end.After(to.UTC()) {
			end = to.UTC()
		}
		result = append(result, Window{From: start, To: end})
	}
	return result, nil
}

func Eligible(now time.Time, duration, grace time.Duration) (Window, error) {
	if duration <= 0 {
		return Window{}, errors.New("window duration must be positive")
	}
	if grace < 0 {
		return Window{}, errors.New("late grace must not be negative")
	}
	end := now.UTC().Add(-grace).Truncate(duration)
	return Window{From: end.Add(-duration), To: end}, nil
}
