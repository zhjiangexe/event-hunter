package logging

import (
	"context"
	"log/slog"

	"event-hunter/backend/internal/contexts/ingestion/domain"
)

type Reporter struct{}

func (Reporter) PollFailed(ctx context.Context, err error) {
	slog.WarnContext(ctx, "poll technical DLQ", "error", err)
}

func (Reporter) ProjectionFailed(ctx context.Context, record domain.DLQRecord, err error) {
	slog.ErrorContext(ctx, "project technical DLQ record", "dlq_topic", record.Topic, "dlq_partition", record.Partition, "dlq_offset", record.Offset, "error", err)
}

func (Reporter) CommitFailed(ctx context.Context, record domain.DLQRecord, err error) {
	slog.ErrorContext(ctx, "commit technical DLQ record", "dlq_topic", record.Topic, "dlq_partition", record.Partition, "dlq_offset", record.Offset, "error", err)
}
