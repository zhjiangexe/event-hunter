package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/ingestion/domain"
	"event-hunter/backend/internal/contexts/ingestion/ports"
)

type fakeRecord struct{ value domain.DLQRecord }

func (record fakeRecord) FailureRecord() domain.DLQRecord { return record.value }

type fakeSource struct {
	record      ports.SourceRecord
	delivered   bool
	commitFails int
	operations  *[]string
	cancel      context.CancelFunc
}

func (source *fakeSource) Poll(ctx context.Context) ([]ports.SourceRecord, error) {
	if !source.delivered {
		source.delivered = true
		return []ports.SourceRecord{source.record}, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (source *fakeSource) Commit(_ context.Context, _ ports.SourceRecord) error {
	*source.operations = append(*source.operations, "commit")
	if source.commitFails > 0 {
		source.commitFails--
		return errors.New("commit failed")
	}
	source.cancel()
	return nil
}

func (*fakeSource) Ping(context.Context) error { return nil }

type fakeRepository struct {
	fails      int
	operations *[]string
}

func (repository *fakeRepository) Insert(_ context.Context, _ domain.TechnicalFailure) error {
	*repository.operations = append(*repository.operations, "insert")
	if repository.fails > 0 {
		repository.fails--
		return errors.New("insert failed")
	}
	return nil
}

func (*fakeRepository) Ping(context.Context) error { return nil }

type recordingReporter struct {
	projectionFailures int
	commitFailures     int
}

func (*recordingReporter) PollFailed(context.Context, error) {}
func (reporter *recordingReporter) ProjectionFailed(context.Context, domain.DLQRecord, error) {
	reporter.projectionFailures++
}
func (reporter *recordingReporter) CommitFailed(context.Context, domain.DLQRecord, error) {
	reporter.commitFailures++
}

func TestProjectorPersistsBeforeCommitAndRetriesInPlace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	operations := make([]string, 0)
	source := &fakeSource{
		record:      fakeRecord{value: domain.DLQRecord{Topic: "dlq", Partition: 1, Offset: 9, Timestamp: time.Now()}},
		commitFails: 1,
		operations:  &operations,
		cancel:      cancel,
	}
	repository := &fakeRepository{fails: 2, operations: &operations}
	reporter := &recordingReporter{}
	projector := NewProjector(source, repository, reporter, 0)
	if err := projector.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	want := []string{"insert", "insert", "insert", "commit", "commit"}
	if !reflect.DeepEqual(operations, want) {
		t.Fatalf("operations = %v, want %v", operations, want)
	}
	if reporter.projectionFailures != 2 || reporter.commitFailures != 1 {
		t.Fatalf("failure reports = projection:%d commit:%d", reporter.projectionFailures, reporter.commitFailures)
	}
}
