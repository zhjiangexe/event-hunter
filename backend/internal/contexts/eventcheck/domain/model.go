package domain

import "time"

const (
	PlatformMaxDuration          = 7 * 24 * time.Hour
	PlatformMaxEvents            = 10_000
	PlatformMaxCorrelations      = 20
	PlatformMaxRelationshipDepth = 3
)

type ModelKind string

const (
	ModelKindFlow        ModelKind = "FLOW"
	ModelKindGlobalCheck ModelKind = "GLOBAL_CHECK"
)

type ModelStatus string

const (
	ModelStatusDraft   ModelStatus = "DRAFT"
	ModelStatusActive  ModelStatus = "ACTIVE"
	ModelStatusRetired ModelStatus = "RETIRED"
)

type RegistryDocument struct {
	ContractVersion int             `json:"registry_contract_version"`
	Models          []RegistryEntry `json:"models"`
}

type RegistryEntry struct {
	SourcePath string     `json:"source_path"`
	Checksum   string     `json:"checksum"`
	Model      CheckModel `json:"model"`
}

type CheckModel struct {
	ContractVersion     int                 `json:"contract_version"`
	ID                  string              `json:"model_id"`
	Version             int                 `json:"version"`
	Kind                ModelKind           `json:"kind"`
	Status              ModelStatus         `json:"status"`
	Title               string              `json:"title"`
	Description         string              `json:"description"`
	Domain              string              `json:"domain"`
	Source              ModelSource         `json:"source"`
	AppliesTo           AppliesTo           `json:"applies_to"`
	Scope               ScopePolicy         `json:"scope"`
	Nodes               []FlowNode          `json:"nodes,omitempty"`
	Paths               []FlowPath          `json:"paths,omitempty"`
	Expectations        []Expectation       `json:"expectations,omitempty"`
	ChildModels         []ChildModel        `json:"child_models,omitempty"`
	UnmappedEventPolicy UnmappedEventPolicy `json:"unmapped_event_policy,omitempty"`
	Rules               []GlobalRule        `json:"rules,omitempty"`
	Fixtures            ModelFixtures       `json:"fixtures"`
}

type ModelSource struct {
	Authoring        string `json:"authoring"`
	MutableAtRuntime bool   `json:"mutable_at_runtime"`
}

type AppliesTo struct {
	AggregateTypes    []string              `json:"aggregate_types"`
	TriggerEventTypes []string              `json:"trigger_event_types"`
	EventVersions     []EventVersionSupport `json:"event_versions"`
}

type EventVersionSupport struct {
	EventType string `json:"event_type"`
	Versions  []int  `json:"versions"`
}

type ScopePolicy struct {
	MaxDurationSeconds   int                   `json:"max_duration_seconds"`
	MaxEvents            int                   `json:"max_events"`
	MaxCorrelations      int                   `json:"max_correlations"`
	MaxRelationshipDepth int                   `json:"max_relationship_depth"`
	Relations            []RelationType        `json:"relations"`
	BusinessKeys         []BusinessKey         `json:"business_keys"`
	ParentChildRelations []ParentChildRelation `json:"parent_child_relations"`
}

func (policy ScopePolicy) MaxDuration() time.Duration {
	return time.Duration(policy.MaxDurationSeconds) * time.Second
}

type BusinessKey struct {
	Name        string   `json:"name"`
	JSONPointer string   `json:"json_pointer"`
	EventTypes  []string `json:"event_types"`
}

type ParentChildRelation struct {
	ID                  string `json:"id"`
	ParentAggregateType string `json:"parent_aggregate_type"`
	ChildAggregateType  string `json:"child_aggregate_type"`
	BusinessKey         string `json:"business_key"`
}

type EventMatcher struct {
	EventTypes []string `json:"event_types"`
}

type FlowNode struct {
	ID        string       `json:"id"`
	Label     string       `json:"label"`
	Event     EventMatcher `json:"event"`
	MinOccurs int          `json:"min_occurs"`
	MaxOccurs int          `json:"max_occurs"`
}

type Outcome struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Category string `json:"category"`
}

type FlowPath struct {
	ID                  string   `json:"id"`
	Label               string   `json:"label"`
	Nodes               []string `json:"nodes"`
	ForbiddenEventTypes []string `json:"forbidden_event_types,omitempty"`
	Terminal            bool     `json:"terminal"`
	Outcome             Outcome  `json:"outcome"`
}

type TemporalRelation string

const (
	TemporalBeforeOrAt TemporalRelation = "BEFORE_OR_AT"
	TemporalAfterOrAt  TemporalRelation = "AFTER_OR_AT"
)

type Expectation struct {
	ID                         string           `json:"id"`
	Label                      string           `json:"label"`
	Trigger                    EventMatcher     `json:"trigger"`
	Expected                   EventMatcher     `json:"expected"`
	TemporalRelation           TemporalRelation `json:"temporal_relation"`
	ReminderAfterSeconds       int              `json:"reminder_after_seconds"`
	DeadlineSeconds            int              `json:"deadline_seconds"`
	Exclusions                 EventExclusions  `json:"exclusions"`
	Severity                   string           `json:"severity"`
	FindingCode                string           `json:"finding_code"`
	RecommendedQueryTemplateID string           `json:"recommended_query_template_id"`
}

type EventExclusions struct {
	AnyEventTypes []string `json:"any_event_types"`
}

type ChildModel struct {
	ModelID      string       `json:"model_id"`
	Version      int          `json:"version"`
	ActivateWhen EventMatcher `json:"activate_when"`
	Relation     RelationType `json:"relation"`
}

type UnmappedEventPolicy struct {
	Default            string   `json:"default"`
	EscalateEventTypes []string `json:"escalate_event_types"`
}

type GlobalRule struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	FindingCode string `json:"finding_code"`
}

type ModelFixtures struct {
	ScenarioFile string   `json:"scenario_file"`
	CaseIDs      []string `json:"case_ids"`
}

type Event struct {
	ID             string
	Type           string
	Version        int
	OccurredAt     time.Time
	Producer       string
	CorrelationID  string
	CausationID    *string
	TraceID        *string
	AggregateType  string
	AggregateID    string
	Sequence       uint64
	KafkaTopic     string
	KafkaPartition uint32
	KafkaOffset    uint64
	Payload        map[string]any
}

type SourceHealthStatus string

const (
	SourceHealthy     SourceHealthStatus = "HEALTHY"
	SourceStale       SourceHealthStatus = "STALE"
	SourcePartial     SourceHealthStatus = "PARTIAL"
	SourceUnavailable SourceHealthStatus = "UNAVAILABLE"
)

type SourceHealth struct {
	Status    SourceHealthStatus
	Truncated bool
}

func (health SourceHealth) SupportsAbsenceConclusion() bool {
	return health.Status == SourceHealthy && !health.Truncated
}

type CheckStatus string

const (
	CheckNoData       CheckStatus = "NO_DATA"
	CheckInProgress   CheckStatus = "IN_PROGRESS"
	CheckConformant   CheckStatus = "CONFORMANT"
	CheckDeviated     CheckStatus = "DEVIATED"
	CheckInconclusive CheckStatus = "INCONCLUSIVE"
	CheckAmbiguous    CheckStatus = "AMBIGUOUS"
)

type ExpectationState string

const (
	ExpectationSatisfied     ExpectationState = "SATISFIED"
	ExpectationWaiting       ExpectationState = "WAITING"
	ExpectationReminder      ExpectationState = "REMINDER"
	ExpectationViolated      ExpectationState = "VIOLATED"
	ExpectationLateSatisfied ExpectationState = "LATE_SATISFIED"
)

type Finding struct {
	ModelID                    string
	RuleKind                   string
	RuleID                     string
	RuleVersion                int
	RuleChecksum               string
	Severity                   string
	Code                       string
	ExpectationState           *ExpectationState
	EvidenceEventIDs           []string
	RecommendedQueryTemplateID string
}

type ExpectationResult struct {
	ID                 string
	State              ExpectationState
	TriggerEventIDs    []string
	SatisfyingEventIDs []string
	ReminderAt         *time.Time
	DeadlineAt         *time.Time
	Inconclusive       bool
}

type FlowResult struct {
	ModelID          string
	ModelVersion     int
	Role             string
	Status           CheckStatus
	CandidatePathIDs []string
	MatchedPathID    *string
	Outcome          *Outcome
}

type GlobalCheckResult struct {
	ModelID      string
	ModelVersion int
	Status       CheckStatus
	FindingCodes []string
}

type EvaluationResult struct {
	CheckStatus      CheckStatus
	BusinessOutcome  *Outcome
	Expectations     []ExpectationResult
	Flows            []FlowResult
	GlobalChecks     []GlobalCheckResult
	Findings         []Finding
	UnmappedEventIDs []string
}
