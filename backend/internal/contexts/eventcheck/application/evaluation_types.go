package application

import "event-hunter/backend/internal/contexts/eventcheck/domain"

const (
	EvaluatorContractVersion = 1
	EvaluatorBuildVersion    = "eventcheck-domain-v1"
)

type IdentifierQualifier struct {
	AggregateType   string `json:"aggregate_type,omitempty"`
	BusinessKeyName string `json:"business_key_name,omitempty"`
}

type Identifier struct {
	Type      string               `json:"type"`
	Value     string               `json:"value"`
	Qualifier *IdentifierQualifier `json:"qualifier,omitempty"`
}

type RequestedModel struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type ScopeAdjustment struct {
	EventID string `json:"event_id"`
	Reason  string `json:"reason"`
}

type ScopeAdjustments struct {
	Include []ScopeAdjustment `json:"include"`
	Exclude []ScopeAdjustment `json:"exclude"`
}

type EvaluateRequest struct {
	Identifier       Identifier        `json:"identifier"`
	From             string            `json:"from"`
	To               string            `json:"to"`
	Model            *RequestedModel   `json:"model,omitempty"`
	ScopeAdjustments *ScopeAdjustments `json:"scope_adjustments,omitempty"`
}

type SourceComponentHealth struct {
	Component  string `json:"component"`
	Status     string `json:"status"`
	DetailCode string `json:"detail_code"`
}

type SourceHealth struct {
	Status       string                  `json:"status"`
	CheckedAt    string                  `json:"checked_at"`
	CoverageFrom string                  `json:"coverage_from"`
	CoverageTo   string                  `json:"coverage_to"`
	Watermark    *string                 `json:"watermark"`
	Truncated    bool                    `json:"truncated"`
	Components   []SourceComponentHealth `json:"components"`
}

type ModelRef struct {
	ID         string `json:"id"`
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	SourcePath string `json:"source_path"`
	Checksum   string `json:"checksum"`
}

type IdentifierCandidate struct {
	Type       string `json:"type"`
	Confidence string `json:"confidence"`
	ReasonCode string `json:"reason_code"`
}

type ModelCandidate struct {
	Model       ModelRef `json:"model"`
	Confidence  string   `json:"confidence"`
	ReasonCodes []string `json:"reason_codes"`
}

type EventReference struct {
	EventID       string  `json:"event_id"`
	EventType     string  `json:"event_type"`
	EventVersion  int     `json:"event_version"`
	OccurredAt    string  `json:"occurred_at"`
	Producer      string  `json:"producer"`
	AggregateType string  `json:"aggregate_type"`
	AggregateID   string  `json:"aggregate_id"`
	Sequence      uint64  `json:"sequence"`
	CorrelationID string  `json:"correlation_id"`
	TraceID       *string `json:"trace_id"`
	PayloadSHA256 string  `json:"payload_sha256"`
	Ordinal       int     `json:"ordinal"`
}

type ExcludedEventReference struct {
	EventReference
	Reason string `json:"reason"`
}

type Relationship struct {
	Ordinal       int     `json:"ordinal"`
	FromEventID   *string `json:"from_event_id"`
	ToEventID     string  `json:"to_event_id"`
	RelationType  string  `json:"relation_type"`
	SourceField   *string `json:"source_field"`
	SourceModelID *string `json:"source_model_id"`
	SourceRuleID  *string `json:"source_rule_id"`
}

type ScopeLimits struct {
	MaxDurationSeconds   int `json:"max_duration_seconds"`
	MaxEvents            int `json:"max_events"`
	MaxCorrelations      int `json:"max_correlations"`
	MaxRelationshipDepth int `json:"max_relationship_depth"`
}

type ResolvedScope struct {
	Mode           string                   `json:"mode"`
	Seeds          []string                 `json:"seeds"`
	Events         []EventReference         `json:"events"`
	ExcludedEvents []ExcludedEventReference `json:"excluded_events"`
	Relationships  []Relationship           `json:"relationships"`
	Limits         ScopeLimits              `json:"limits"`
}

type BusinessOutcome struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Label    string `json:"label"`
}

type ExpectationResult struct {
	ID                 string   `json:"id"`
	State              string   `json:"state"`
	TriggerEventIDs    []string `json:"trigger_event_ids"`
	SatisfyingEventIDs []string `json:"satisfying_event_ids"`
	ReminderAt         *string  `json:"reminder_at"`
	DeadlineAt         *string  `json:"deadline_at"`
}

type FlowResult struct {
	Model            ModelRef         `json:"model"`
	Role             string           `json:"role"`
	Status           string           `json:"status"`
	CandidatePathIDs []string         `json:"candidate_path_ids"`
	MatchedPathID    *string          `json:"matched_path_id"`
	Outcome          *BusinessOutcome `json:"outcome"`
}

type GlobalCheckResult struct {
	Model        ModelRef `json:"model"`
	Status       string   `json:"status"`
	FindingCodes []string `json:"finding_codes"`
}

type EvidenceReference struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Finding struct {
	ID                         *string             `json:"id"`
	RuleKind                   string              `json:"rule_kind"`
	RuleID                     string              `json:"rule_id"`
	RuleVersion                int                 `json:"rule_version"`
	RuleChecksum               string              `json:"rule_checksum"`
	Severity                   string              `json:"severity"`
	Code                       string              `json:"code"`
	ExpectationState           *string             `json:"expectation_state"`
	EvidenceReferences         []EvidenceReference `json:"evidence_references"`
	RecommendedQueryTemplateID *string             `json:"recommended_query_template_id"`
}

type CheckResult struct {
	CheckStatus              string              `json:"check_status"`
	BusinessOutcome          *BusinessOutcome    `json:"business_outcome"`
	Expectations             []ExpectationResult `json:"expectations"`
	Flows                    []FlowResult        `json:"flows"`
	GlobalChecks             []GlobalCheckResult `json:"global_checks"`
	Findings                 []Finding           `json:"findings"`
	UnmappedEventIDs         []string            `json:"unmapped_event_ids"`
	EvaluatorContractVersion int                 `json:"evaluator_contract_version"`
	EvaluatorBuildVersion    string              `json:"evaluator_build_version"`
}

// EnsureCollections keeps API responses compliant with the JSON Schema even
// when an empty Go slice was decoded from a legacy Snapshot as nil. Required
// collection fields must be encoded as [] rather than null.
func (result *CheckResult) EnsureCollections() {
	if result.Expectations == nil {
		result.Expectations = []ExpectationResult{}
	}
	if result.Flows == nil {
		result.Flows = []FlowResult{}
	}
	if result.GlobalChecks == nil {
		result.GlobalChecks = []GlobalCheckResult{}
	}
	if result.Findings == nil {
		result.Findings = []Finding{}
	}
	if result.UnmappedEventIDs == nil {
		result.UnmappedEventIDs = []string{}
	}
	for index := range result.Expectations {
		if result.Expectations[index].TriggerEventIDs == nil {
			result.Expectations[index].TriggerEventIDs = []string{}
		}
		if result.Expectations[index].SatisfyingEventIDs == nil {
			result.Expectations[index].SatisfyingEventIDs = []string{}
		}
	}
	for index := range result.Flows {
		if result.Flows[index].CandidatePathIDs == nil {
			result.Flows[index].CandidatePathIDs = []string{}
		}
	}
	for index := range result.GlobalChecks {
		if result.GlobalChecks[index].FindingCodes == nil {
			result.GlobalChecks[index].FindingCodes = []string{}
		}
	}
	for index := range result.Findings {
		if result.Findings[index].EvidenceReferences == nil {
			result.Findings[index].EvidenceReferences = []EvidenceReference{}
		}
	}
}

type EvaluateResponse struct {
	ResolutionStatus     string                `json:"resolution_status"`
	NormalizedRequest    EvaluateRequest       `json:"normalized_request"`
	SourceHealth         SourceHealth          `json:"source_health"`
	Scope                ResolvedScope         `json:"scope"`
	IdentifierCandidates []IdentifierCandidate `json:"identifier_candidates"`
	ModelCandidates      []ModelCandidate      `json:"model_candidates"`
	Model                *ModelRef             `json:"model"`
	Result               *CheckResult          `json:"result"`
	EventSetHash         *string               `json:"event_set_hash"`
	EvaluationHash       *string               `json:"evaluation_hash"`
	Warnings             []string              `json:"warnings"`
}

func limitsFromPolicy(policy domain.ScopePolicy) ScopeLimits {
	return ScopeLimits{
		MaxDurationSeconds: policy.MaxDurationSeconds, MaxEvents: policy.MaxEvents,
		MaxCorrelations: policy.MaxCorrelations, MaxRelationshipDepth: policy.MaxRelationshipDepth,
	}
}

func platformLimits() ScopeLimits {
	return ScopeLimits{
		MaxDurationSeconds: int(domain.PlatformMaxDuration.Seconds()), MaxEvents: domain.PlatformMaxEvents,
		MaxCorrelations: domain.PlatformMaxCorrelations, MaxRelationshipDepth: domain.PlatformMaxRelationshipDepth,
	}
}
