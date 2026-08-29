package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"event-hunter/backend/internal/contexts/eventcheck/application/internal/canonicaljson"
	"event-hunter/backend/internal/contexts/eventcheck/domain"
	"event-hunter/backend/internal/contexts/eventcheck/ports"
)

var (
	ErrInvalidSaveRequest   = errors.New("invalid Check Snapshot save request")
	ErrEvaluationChanged    = errors.New("Event Check evaluation changed")
	ErrIdempotencyKeyReused = errors.New("idempotency key reused with different request")
)

type SaveSnapshotRequest struct {
	EvaluationRequest      EvaluateRequest   `json:"evaluation_request"`
	ExpectedEventSetHash   string            `json:"expected_event_set_hash"`
	ExpectedEvaluationHash string            `json:"expected_evaluation_hash"`
	RetentionProfile       *RetentionProfile `json:"retention_profile,omitempty"`
}

type Actor = domain.SnapshotActor

type EvaluationChangedError struct {
	CurrentEventSetHash   *string
	CurrentEvaluationHash *string
}

type EventCheckEvaluator interface {
	Evaluate(context.Context, EvaluateRequest) (EvaluateResponse, error)
}

func (err EvaluationChangedError) Error() string { return ErrEvaluationChanged.Error() }
func (err EvaluationChangedError) Unwrap() error { return ErrEvaluationChanged }

type SaveSnapshotHandler struct {
	evaluator  EventCheckEvaluator
	repository ports.SnapshotRepository
	audit      ports.AuditWriter
	unitOfWork ports.UnitOfWork
	now        func() time.Time
	newID      func() string
}

func NewSaveSnapshotHandler(evaluator EventCheckEvaluator, repository ports.SnapshotRepository, audit ports.AuditWriter, unitOfWork ports.UnitOfWork) *SaveSnapshotHandler {
	return &SaveSnapshotHandler{evaluator: evaluator, repository: repository, audit: audit, unitOfWork: unitOfWork, now: time.Now, newID: func() string { return uuid.NewString() }}
}

func (service *SaveSnapshotHandler) Save(ctx context.Context, input SaveSnapshotRequest, actor Actor, idempotencyKey, requestID string) (Snapshot, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if strings.TrimSpace(actor.Subject) == "" || (actor.Role != "INVESTIGATOR" && actor.Role != "ADMIN") ||
		idempotencyKey == "" || len(idempotencyKey) > 200 || !validHash(input.ExpectedEventSetHash) ||
		!validHash(input.ExpectedEvaluationHash) || (input.RetentionProfile != nil && (strings.TrimSpace(input.RetentionProfile.ID) == "" || input.RetentionProfile.Version < 1)) {
		return Snapshot{}, false, ErrInvalidSaveRequest
	}
	evaluation, err := service.evaluator.Evaluate(ctx, input.EvaluationRequest)
	if err != nil {
		return Snapshot{}, false, err
	}
	if evaluation.ResolutionStatus != "EVALUATED" || evaluation.Model == nil || evaluation.Result == nil ||
		evaluation.EventSetHash == nil || evaluation.EvaluationHash == nil {
		return Snapshot{}, false, ErrInvalidSaveRequest
	}
	if *evaluation.EventSetHash != input.ExpectedEventSetHash || *evaluation.EvaluationHash != input.ExpectedEvaluationHash {
		return Snapshot{}, false, EvaluationChangedError{CurrentEventSetHash: evaluation.EventSetHash, CurrentEvaluationHash: evaluation.EvaluationHash}
	}
	requestHash, err := canonicaljson.SHA256(input)
	if err != nil {
		return Snapshot{}, false, err
	}
	now := service.now().UTC()
	result := *evaluation.Result
	findings := make([]domain.SnapshotFinding, 0, len(result.Findings))
	for index := range result.Findings {
		findingID := service.newID()
		result.Findings[index].ID = &findingID
		evidence, err := json.Marshal(result.Findings[index].EvidenceReferences)
		if err != nil {
			return Snapshot{}, false, err
		}
		findings = append(findings, domain.SnapshotFinding{
			ID: findingID, RuleKind: result.Findings[index].RuleKind, RuleID: result.Findings[index].RuleID,
			RuleVersion: result.Findings[index].RuleVersion, RuleChecksum: result.Findings[index].RuleChecksum,
			Severity: result.Findings[index].Severity, Code: result.Findings[index].Code,
			ExpectationState: result.Findings[index].ExpectationState, EvidenceReferences: evidence,
			RecommendedQueryTemplateID: result.Findings[index].RecommendedQueryTemplateID,
		})
	}
	requestJSON, healthJSON, resultJSON, retentionJSON, err := snapshotJSON(evaluation.NormalizedRequest, evaluation.SourceHealth, result, input.RetentionProfile)
	if err != nil {
		return Snapshot{}, false, err
	}
	asOf, err := time.Parse(time.RFC3339Nano, evaluation.NormalizedRequest.To)
	if err != nil {
		return Snapshot{}, false, err
	}
	snapshot, err := domain.NewCheckSnapshot(domain.CheckSnapshot{
		ID: service.newID(), Provenance: "LIVE_EVALUATION", CreatedBy: actor.Subject, CreatedByRole: actor.Role,
		CreatedAt: now, EvaluationRequest: requestJSON, AsOf: asOf, SourceHealth: healthJSON,
		Model:  domain.SnapshotModel{ID: evaluation.Model.ID, Version: evaluation.Model.Version, Kind: evaluation.Model.Kind, SourcePath: evaluation.Model.SourcePath, Checksum: evaluation.Model.Checksum},
		Result: resultJSON, EventReferences: snapshotEventReferences(evaluation), Relationships: snapshotRelationships(evaluation.Scope.Relationships), Findings: findings,
		EventSetHash: *evaluation.EventSetHash, EvaluationHash: *evaluation.EvaluationHash, ResultSchemaVersion: 1,
		RetentionProfile: retentionJSON, IdempotencyActor: actor.Subject, IdempotencyKey: idempotencyKey, IdempotencyRequestHash: requestHash,
	})
	if err != nil {
		return Snapshot{}, false, err
	}
	var persisted domain.CheckSnapshot
	created := false
	err = service.unitOfWork.WithinTransaction(ctx, func(transactionContext context.Context) error {
		var persistErr error
		persisted, created, persistErr = service.repository.Create(transactionContext, snapshot)
		if persistErr != nil {
			return persistErr
		}
		if !created {
			if persisted.IdempotencyRequestHash != requestHash {
				return ErrIdempotencyKeyReused
			}
			return nil
		}
		return service.audit.RecordEventCheckAudit(transactionContext, ports.AuditRecord{
			Actor: actor, Action: "CREATE_CHECK_SNAPSHOT", ResourceType: "CHECK_SNAPSHOT", ResourceID: snapshot.ID,
			RequestID: requestID, Metadata: map[string]any{"model_id": snapshot.Model.ID, "model_version": snapshot.Model.Version, "event_set_hash": snapshot.EventSetHash}, CreatedAt: now,
		})
	})
	if err != nil {
		return Snapshot{}, false, err
	}
	response, err := FromDomain(persisted)
	return response, created, err
}

func ToResponse(snapshot domain.CheckSnapshot) (Snapshot, error) {
	return FromDomain(snapshot)
}

func snapshotEventReferences(evaluation EvaluateResponse) []domain.SnapshotEventReference {
	result := make([]domain.SnapshotEventReference, 0, len(evaluation.Scope.Events)+len(evaluation.Scope.ExcludedEvents))
	includeReasons := map[string]string{}
	if evaluation.NormalizedRequest.ScopeAdjustments != nil {
		for _, adjustment := range evaluation.NormalizedRequest.ScopeAdjustments.Include {
			includeReasons[adjustment.EventID] = adjustment.Reason
		}
	}
	for _, reference := range evaluation.Scope.Events {
		var reason *string
		if value := includeReasons[reference.EventID]; value != "" {
			reason = &value
		}
		result = append(result, domain.SnapshotEventReference{
			EventID: reference.EventID, EventType: reference.EventType, EventVersion: reference.EventVersion,
			OccurredAt: mustTime(reference.OccurredAt), Producer: reference.Producer, AggregateType: reference.AggregateType,
			AggregateID: reference.AggregateID, Sequence: reference.Sequence, CorrelationID: reference.CorrelationID,
			TraceID: reference.TraceID, PayloadSHA256: reference.PayloadSHA256, Ordinal: reference.Ordinal,
			Disposition: "INCLUDED", AdjustmentReason: reason, SourceAvailable: true,
		})
	}
	for _, reference := range evaluation.Scope.ExcludedEvents {
		reason := reference.Reason
		result = append(result, domain.SnapshotEventReference{
			EventID: reference.EventID, EventType: reference.EventType, EventVersion: reference.EventVersion,
			OccurredAt: mustTime(reference.OccurredAt), Producer: reference.Producer, AggregateType: reference.AggregateType,
			AggregateID: reference.AggregateID, Sequence: reference.Sequence, CorrelationID: reference.CorrelationID,
			TraceID: reference.TraceID, PayloadSHA256: reference.PayloadSHA256, Ordinal: reference.Ordinal,
			Disposition: "EXCLUDED", AdjustmentReason: &reason, SourceAvailable: true,
		})
	}
	return result
}

func snapshotRelationships(values []Relationship) []domain.SnapshotRelationship {
	result := make([]domain.SnapshotRelationship, 0, len(values))
	for _, value := range values {
		result = append(result, domain.SnapshotRelationship{
			Ordinal: value.Ordinal, FromEventID: value.FromEventID, ToEventID: value.ToEventID,
			RelationType: value.RelationType, SourceField: value.SourceField, SourceModelID: value.SourceModelID, SourceRuleID: value.SourceRuleID,
		})
	}
	return result
}

func snapshotJSON(request EvaluateRequest, health SourceHealth, result CheckResult, retention *RetentionProfile) (json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, error) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	healthJSON, err := json.Marshal(health)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var retentionJSON json.RawMessage
	if retention != nil {
		retentionJSON, err = json.Marshal(retention)
	}
	return requestJSON, healthJSON, resultJSON, retentionJSON, err
}

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(fmt.Sprintf("normalized Event Check timestamp %q: %v", value, err))
	}
	return parsed.UTC()
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
