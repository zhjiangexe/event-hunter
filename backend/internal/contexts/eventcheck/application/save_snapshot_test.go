package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

type mutableSource struct {
	result ports.CanonicalEventResult
}

func (source *mutableSource) FindCanonicalEvents(context.Context, ports.CanonicalEventQuery) (ports.CanonicalEventResult, error) {
	return source.result, nil
}

type memorySnapshotRepository struct {
	byID          map[string]domain.CheckSnapshot
	byIdempotency map[string]string
	audits        []ports.AuditRecord
}

func newMemorySnapshotRepository() *memorySnapshotRepository {
	return &memorySnapshotRepository{byID: map[string]domain.CheckSnapshot{}, byIdempotency: map[string]string{}}
}

func (repository *memorySnapshotRepository) Create(_ context.Context, snapshot domain.CheckSnapshot) (domain.CheckSnapshot, bool, error) {
	key := snapshot.IdempotencyActor + "\x00" + snapshot.IdempotencyKey
	if id := repository.byIdempotency[key]; id != "" {
		return repository.byID[id], false, nil
	}
	repository.byID[snapshot.ID] = snapshot
	repository.byIdempotency[key] = snapshot.ID
	return snapshot, true, nil
}

func (repository *memorySnapshotRepository) Get(_ context.Context, id string) (domain.CheckSnapshot, error) {
	value, ok := repository.byID[id]
	if !ok {
		return domain.CheckSnapshot{}, domain.ErrSnapshotNotFound
	}
	return value, nil
}

func (*memorySnapshotRepository) FindFeedback(context.Context, string) (domain.FindingFeedback, bool, error) {
	panic("not used")
}

func (*memorySnapshotRepository) ListFeedback(context.Context, string) ([]domain.FindingFeedback, error) {
	return []domain.FindingFeedback{}, nil
}

func (*memorySnapshotRepository) SaveFeedback(context.Context, domain.FindingFeedback, int64) error {
	panic("not used")
}

func (repository *memorySnapshotRepository) RecordEventCheckAudit(_ context.Context, record ports.AuditRecord) error {
	repository.audits = append(repository.audits, record)
	return nil
}

type snapshotUnitOfWork struct{}

func (snapshotUnitOfWork) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func TestSaveReevaluatesAndPersistsImmutableEvidenceReferences(t *testing.T) {
	start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	source := &mutableSource{result: ports.CanonicalEventResult{Events: deviatedEvents(start)}}
	evaluator := NewEvaluateEventCheckHandler(source)
	evaluationRequest := EvaluateRequest{
		Identifier: Identifier{Type: "CORRELATION_ID", Value: "ORDER-SNAPSHOT-01"},
		From:       start.Format(time.RFC3339), To: start.Add(10 * time.Minute).Format(time.RFC3339),
	}
	evaluation, err := evaluator.Evaluate(context.Background(), evaluationRequest)
	if err != nil {
		t.Fatal(err)
	}
	repository := newMemorySnapshotRepository()
	service := NewSaveSnapshotHandler(evaluator, repository, repository, snapshotUnitOfWork{})
	service.now = func() time.Time { return start.Add(11 * time.Minute) }
	created, wasCreated, err := service.Save(context.Background(), SaveSnapshotRequest{
		EvaluationRequest: evaluation.NormalizedRequest, ExpectedEventSetHash: *evaluation.EventSetHash,
		ExpectedEvaluationHash: *evaluation.EvaluationHash,
	}, Actor{Subject: "investigator-1", Role: "INVESTIGATOR"}, "save-1", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if !wasCreated || created.Result.CheckStatus != "DEVIATED" || len(created.Result.Findings) == 0 || created.Result.Findings[0].ID == nil {
		t.Fatalf("unexpected saved Snapshot: %#v", created)
	}
	if len(created.FindingFeedback) != len(created.Result.Findings) || created.FindingFeedback[0].Status != "UNREVIEWED" || created.FindingFeedback[0].LockVersion != 0 {
		t.Fatalf("unexpected default feedback projection: %#v", created.FindingFeedback)
	}
	if len(created.EventReferences) != 2 || len(repository.audits) != 1 {
		t.Fatalf("missing references or audit: %#v / %#v", created.EventReferences, repository.audits)
	}
	for _, reference := range created.EventReferences {
		if reference.PayloadSHA256 == "" || !reference.SourceAvailable {
			t.Fatalf("invalid evidence reference: %#v", reference)
		}
	}
	if string(repository.byID[created.ID].Result) == "" {
		t.Fatal("deterministic result was not persisted")
	}
}

func TestSaveRejectsLateEventInsteadOfSilentlyChangingSnapshot(t *testing.T) {
	start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	source := &mutableSource{result: ports.CanonicalEventResult{Events: deviatedEvents(start)}}
	evaluator := NewEvaluateEventCheckHandler(source)
	request := EvaluateRequest{
		Identifier: Identifier{Type: "CORRELATION_ID", Value: "ORDER-SNAPSHOT-01"},
		From:       start.Format(time.RFC3339), To: start.Add(10 * time.Minute).Format(time.RFC3339),
	}
	old, err := evaluator.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	source.result.Events = append(source.result.Events, domain.Event{
		ID: "EVT-LATE-SHIPMENT", Type: "ShipmentCreated", Version: 1, OccurredAt: start.Add(8 * time.Minute),
		Producer: "shipping-service", CorrelationID: "ORDER-SNAPSHOT-01", AggregateType: "Shipment",
		AggregateID: "SHIP-SNAPSHOT-01", Sequence: 1, Payload: map[string]any{"orderId": "ORDER-SNAPSHOT-01"},
	})
	repository := newMemorySnapshotRepository()
	service := NewSaveSnapshotHandler(evaluator, repository, repository, snapshotUnitOfWork{})
	_, _, err = service.Save(context.Background(), SaveSnapshotRequest{
		EvaluationRequest: old.NormalizedRequest, ExpectedEventSetHash: *old.EventSetHash, ExpectedEvaluationHash: *old.EvaluationHash,
	}, Actor{Subject: "investigator-1", Role: "INVESTIGATOR"}, "save-late", "request-late")
	if !errors.Is(err, ErrEvaluationChanged) {
		t.Fatalf("error = %v, want EVALUATION_CHANGED", err)
	}
	if len(repository.byID) != 0 {
		t.Fatal("changed evaluation was persisted")
	}
}

func TestSaveIsIdempotentAndRejectsKeyReuse(t *testing.T) {
	start := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	source := &mutableSource{result: ports.CanonicalEventResult{Events: deviatedEvents(start)}}
	evaluator := NewEvaluateEventCheckHandler(source)
	evaluation, err := evaluator.Evaluate(context.Background(), EvaluateRequest{
		Identifier: Identifier{Type: "CORRELATION_ID", Value: "ORDER-SNAPSHOT-01"},
		From:       start.Format(time.RFC3339), To: start.Add(10 * time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := newMemorySnapshotRepository()
	service := NewSaveSnapshotHandler(evaluator, repository, repository, snapshotUnitOfWork{})
	input := SaveSnapshotRequest{EvaluationRequest: evaluation.NormalizedRequest, ExpectedEventSetHash: *evaluation.EventSetHash, ExpectedEvaluationHash: *evaluation.EvaluationHash}
	first, created, err := service.Save(context.Background(), input, Actor{Subject: "investigator-1", Role: "ADMIN"}, "same-key", "request-1")
	if err != nil || !created {
		t.Fatalf("first save = %#v, %v, %v", first, created, err)
	}
	second, created, err := service.Save(context.Background(), input, Actor{Subject: "investigator-1", Role: "ADMIN"}, "same-key", "request-2")
	if err != nil || created || second.ID != first.ID || len(repository.audits) != 1 {
		t.Fatalf("idempotent save = %#v, %v, %v, audits=%d", second, created, err, len(repository.audits))
	}
	input.RetentionProfile = &RetentionProfile{ID: "legal-hold", Version: 1}
	_, _, err = service.Save(context.Background(), input, Actor{Subject: "investigator-1", Role: "ADMIN"}, "same-key", "request-3")
	if !errors.Is(err, ErrIdempotencyKeyReused) {
		t.Fatalf("key reuse error = %v", err)
	}
}

func deviatedEvents(start time.Time) []domain.Event {
	traceID := "22222222222222222222222222222222"
	return []domain.Event{
		{ID: "EVT-ORDER", Type: "OrderCreated", Version: 1, OccurredAt: start, Producer: "order-service", CorrelationID: "ORDER-SNAPSHOT-01", TraceID: &traceID, AggregateType: "Order", AggregateID: "ORDER-SNAPSHOT-01", Sequence: 1, Payload: map[string]any{"orderId": "ORDER-SNAPSHOT-01"}},
		{ID: "EVT-PAYMENT", Type: "PaymentCompleted", Version: 1, OccurredAt: start.Add(time.Minute), Producer: "payment-service", CorrelationID: "ORDER-SNAPSHOT-01", TraceID: &traceID, AggregateType: "Payment", AggregateID: "PAY-SNAPSHOT-01", Sequence: 1, Payload: map[string]any{"orderId": "ORDER-SNAPSHOT-01"}},
	}
}
