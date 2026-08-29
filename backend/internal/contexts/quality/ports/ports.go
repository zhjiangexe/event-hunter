package ports

import (
	"context"

	"event-hunter/backend/internal/contexts/quality/domain"
)

type Aggregator interface {
	Aggregate(context.Context, domain.Window) error
}
