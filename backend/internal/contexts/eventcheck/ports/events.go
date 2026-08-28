package ports

import (
	"context"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
)

// CanonicalEventQuery is deliberately bounded. Application services may
// resolve relationships in memory, but adapters must never expose an
// unbounded ClickHouse scan.
type CanonicalEventQuery struct {
	From  time.Time
	To    time.Time
	Limit int
}

type CanonicalEventResult struct {
	Events    []domain.Event
	Watermark *time.Time
	Truncated bool
}

type CanonicalEventSource interface {
	FindCanonicalEvents(ctx context.Context, query CanonicalEventQuery) (CanonicalEventResult, error)
}
