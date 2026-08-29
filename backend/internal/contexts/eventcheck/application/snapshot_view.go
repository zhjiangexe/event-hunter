package application

import (
	"encoding/json"
	"time"

	"event-hunter/backend/internal/contexts/eventcheck/domain"
)

type RetentionProfile struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type SnapshotEventReference struct {
	EventID          string  `json:"event_id"`
	EventType        string  `json:"event_type"`
	OccurredAt       string  `json:"occurred_at"`
	Producer         string  `json:"producer"`
	AggregateType    string  `json:"aggregate_type"`
	AggregateID      string  `json:"aggregate_id"`
	CorrelationID    string  `json:"correlation_id"`
	TraceID          *string `json:"trace_id"`
	PayloadSHA256    string  `json:"payload_sha256"`
	Ordinal          int     `json:"ordinal"`
	Disposition      string  `json:"disposition"`
	AdjustmentReason *string `json:"adjustment_reason"`
	SourceAvailable  bool    `json:"source_available"`
}

type FindingFeedback struct {
	FindingID   string  `json:"finding_id"`
	Status      string  `json:"status"`
	ActorID     string  `json:"actor_id"`
	ActorRole   string  `json:"actor_role"`
	UpdatedAt   *string `json:"updated_at"`
	LockVersion int64   `json:"lock_version"`
}

type Snapshot struct {
	ID                  string                   `json:"id"`
	Provenance          string                   `json:"provenance"`
	CreatedBy           string                   `json:"created_by"`
	CreatedByRole       string                   `json:"created_by_role"`
	CreatedAt           string                   `json:"created_at"`
	EvaluationRequest   EvaluateRequest          `json:"evaluation_request"`
	AsOf                string                   `json:"as_of"`
	SourceHealth        SourceHealth             `json:"source_health"`
	Model               ModelRef                 `json:"model"`
	Result              CheckResult              `json:"result"`
	EventReferences     []SnapshotEventReference `json:"event_references"`
	Relationships       []Relationship           `json:"relationships"`
	FindingFeedback     []FindingFeedback        `json:"finding_feedback"`
	EventSetHash        string                   `json:"event_set_hash"`
	EvaluationHash      string                   `json:"evaluation_hash"`
	ResultSchemaVersion int                      `json:"result_schema_version"`
	RetentionProfile    *RetentionProfile        `json:"retention_profile"`
}

func FromDomain(snapshot domain.CheckSnapshot) (Snapshot, error) {
	var request EvaluateRequest
	var health SourceHealth
	var result CheckResult
	if err := json.Unmarshal(snapshot.EvaluationRequest, &request); err != nil {
		return Snapshot{}, err
	}
	if err := json.Unmarshal(snapshot.SourceHealth, &health); err != nil {
		return Snapshot{}, err
	}
	if err := json.Unmarshal(snapshot.Result, &result); err != nil {
		return Snapshot{}, err
	}
	result.EnsureCollections()
	var retention *RetentionProfile
	if len(snapshot.RetentionProfile) > 0 && string(snapshot.RetentionProfile) != "null" {
		retention = &RetentionProfile{}
		if err := json.Unmarshal(snapshot.RetentionProfile, retention); err != nil {
			return Snapshot{}, err
		}
	}
	response := Snapshot{
		ID: snapshot.ID, Provenance: snapshot.Provenance, CreatedBy: snapshot.CreatedBy, CreatedByRole: snapshot.CreatedByRole,
		CreatedAt: snapshot.CreatedAt.UTC().Format(time.RFC3339Nano), EvaluationRequest: request,
		AsOf: snapshot.AsOf.UTC().Format(time.RFC3339Nano), SourceHealth: health,
		Model:  ModelRef{ID: snapshot.Model.ID, Version: snapshot.Model.Version, Kind: snapshot.Model.Kind, SourcePath: snapshot.Model.SourcePath, Checksum: snapshot.Model.Checksum},
		Result: result, EventReferences: []SnapshotEventReference{}, Relationships: []Relationship{}, FindingFeedback: defaultFindingFeedback(result),
		EventSetHash: snapshot.EventSetHash, EvaluationHash: snapshot.EvaluationHash, ResultSchemaVersion: snapshot.ResultSchemaVersion, RetentionProfile: retention,
	}
	for _, reference := range snapshot.EventReferences {
		response.EventReferences = append(response.EventReferences, SnapshotEventReference{
			EventID: reference.EventID, EventType: reference.EventType, OccurredAt: reference.OccurredAt.UTC().Format(time.RFC3339Nano),
			Producer: reference.Producer, AggregateType: reference.AggregateType, AggregateID: reference.AggregateID,
			CorrelationID: reference.CorrelationID, TraceID: reference.TraceID, PayloadSHA256: reference.PayloadSHA256,
			Ordinal: reference.Ordinal, Disposition: reference.Disposition, AdjustmentReason: reference.AdjustmentReason, SourceAvailable: reference.SourceAvailable,
		})
	}
	for _, relation := range snapshot.Relationships {
		response.Relationships = append(response.Relationships, Relationship{
			Ordinal: relation.Ordinal, FromEventID: relation.FromEventID, ToEventID: relation.ToEventID,
			RelationType: relation.RelationType, SourceField: relation.SourceField, SourceModelID: relation.SourceModelID, SourceRuleID: relation.SourceRuleID,
		})
	}
	return response, nil
}

func ApplyFindingFeedback(snapshot *Snapshot, values []domain.FindingFeedback) {
	byFindingID := make(map[string]domain.FindingFeedback, len(values))
	for _, value := range values {
		byFindingID[value.FindingID] = value
	}
	for index, current := range snapshot.FindingFeedback {
		value, ok := byFindingID[current.FindingID]
		if !ok {
			continue
		}
		updatedAt := value.UpdatedAt.UTC().Format(time.RFC3339Nano)
		snapshot.FindingFeedback[index] = FindingFeedback{
			FindingID: value.FindingID, Status: string(value.Status), ActorID: value.ActorID,
			ActorRole: value.ActorRole, UpdatedAt: &updatedAt, LockVersion: value.LockVersion,
		}
	}
}

func defaultFindingFeedback(result CheckResult) []FindingFeedback {
	feedback := make([]FindingFeedback, 0, len(result.Findings))
	for _, finding := range result.Findings {
		if finding.ID == nil {
			continue
		}
		feedback = append(feedback, FindingFeedback{
			FindingID: *finding.ID, Status: "UNREVIEWED", ActorID: "", ActorRole: "", LockVersion: 0,
		})
	}
	return feedback
}
