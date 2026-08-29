package investigationwindow

import (
	"errors"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

const MaximumDuration = 7 * 24 * time.Hour

var ErrInvalidWindow = errors.New("invalid investigation query window")

// Resolve applies the persisted incident window unless the caller supplied an
// explicit pair. Keeping this rule in the application layer prevents inbound
// adapters from independently choosing a different investigation boundary.
func Resolve(from, to *time.Time, fallback domain.IncidentWindow) (time.Time, time.Time, error) {
	if (from == nil) != (to == nil) {
		return time.Time{}, time.Time{}, ErrInvalidWindow
	}
	if from == nil {
		window, err := domain.NewIncidentWindow(fallback.From, fallback.To, fallback.Source)
		if err != nil || window.To.Sub(window.From) > MaximumDuration {
			return time.Time{}, time.Time{}, ErrInvalidWindow
		}
		return window.From.UTC(), window.To.UTC(), nil
	}
	resolvedFrom := from.UTC()
	resolvedTo := to.UTC()
	if !resolvedTo.After(resolvedFrom) || resolvedTo.Sub(resolvedFrom) > MaximumDuration {
		return time.Time{}, time.Time{}, ErrInvalidWindow
	}
	return resolvedFrom, resolvedTo, nil
}
