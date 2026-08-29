package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
)

type snapshotRepository struct {
	snapshot domain.CheckSnapshot
	feedback []domain.FindingFeedback
}

func (repository snapshotRepository) Create(context.Context, domain.CheckSnapshot) (domain.CheckSnapshot, bool, error) {
	panic("not used")
}

func (repository snapshotRepository) Get(context.Context, string) (domain.CheckSnapshot, error) {
	return repository.snapshot, nil
}

func (repository snapshotRepository) ListFeedback(context.Context, string) ([]domain.FindingFeedback, error) {
	return repository.feedback, nil
}

func (repository snapshotRepository) FindFeedback(context.Context, string) (domain.FindingFeedback, bool, error) {
	panic("not used")
}

func (repository snapshotRepository) SaveFeedback(context.Context, domain.FindingFeedback, int64) error {
	panic("not used")
}

func TestGetAddsMutableFeedbackProjectionWithoutChangingResult(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	findingID := "22222222-2222-4222-8222-222222222222"
	result := json.RawMessage(`{
		"check_status":"DEVIATED","business_outcome":null,"expectations":[],"flows":[],"global_checks":[{"model":{"id":"event-integrity","version":1,"kind":"GLOBAL_CHECK","source_path":"contracts/check-models/event-integrity.yaml","checksum":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},"status":"CONFORMANT","finding_codes":null}],
		"findings":[{"id":"22222222-2222-4222-8222-222222222222","rule_kind":"FLOW_EXPECTATION","rule_id":"shipment-required","rule_version":1,"rule_checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","severity":"HIGH","code":"MISSING_SHIPMENT","expectation_state":"VIOLATED","evidence_references":[],"recommended_query_template_id":null}],
		"unmapped_event_ids":null,"evaluator_contract_version":1,"evaluator_build_version":"test"
	}`)
	repository := snapshotRepository{
		snapshot: domain.CheckSnapshot{
			ID: "11111111-1111-4111-8111-111111111111", Provenance: "LIVE_EVALUATION", CreatedBy: "investigator-1",
			CreatedByRole: "INVESTIGATOR", CreatedAt: now, EvaluationRequest: json.RawMessage(`{"identifier":{"type":"CORRELATION_ID","value":"ORDER-1"},"from":"2026-08-28T00:00:00Z","to":"2026-08-28T01:00:00Z"}`),
			AsOf: now, SourceHealth: json.RawMessage(`{"status":"HEALTHY","checked_at":"2026-08-29T00:00:00Z","coverage_from":"2026-08-28T00:00:00Z","coverage_to":"2026-08-28T01:00:00Z","watermark":null,"truncated":false,"components":[]}`),
			Model:  domain.SnapshotModel{ID: "order-fulfillment", Version: 2, Kind: "FLOW", SourcePath: "contracts/check-models/order-fulfillment.yaml", Checksum: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			Result: result, EventSetHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", EvaluationHash: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", ResultSchemaVersion: 1,
		},
		feedback: []domain.FindingFeedback{{FindingID: findingID, Status: domain.FeedbackConfirmed, ActorID: "investigator-2", ActorRole: "INVESTIGATOR", UpdatedAt: now, LockVersion: 2}},
	}

	response, err := NewGetSnapshotHandler(repository).Get(context.Background(), repository.snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.FindingFeedback) != 1 || response.FindingFeedback[0].Status != "CONFIRMED" || response.FindingFeedback[0].LockVersion != 2 {
		t.Fatalf("feedback projection = %#v", response.FindingFeedback)
	}
	if response.Result.UnmappedEventIDs == nil || response.Result.GlobalChecks[0].FindingCodes == nil {
		t.Fatalf("legacy null collections were not normalized: %#v", response.Result)
	}
	if string(repository.snapshot.Result) != string(result) {
		t.Fatal("immutable result was changed")
	}
}
