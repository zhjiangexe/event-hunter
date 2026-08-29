package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	ErrOptimisticConflict    = errors.New("investigation case was modified by another writer")
	ErrCaseNotFound          = errors.New("investigation case not found")
	ErrInvalidTransition     = errors.New("invalid investigation state transition")
	ErrCloseRequired         = errors.New("closed status requires close operation")
	ErrResolutionFields      = errors.New("resolved status requires root cause and resolution summary")
	ErrInvalidOwner          = errors.New("invalid investigation owner")
	ErrInvalidPriority       = errors.New("invalid investigation priority")
	ErrInvalidTags           = errors.New("invalid investigation tags")
	ErrInvalidRelatedIDs     = errors.New("invalid related correlation ids")
	ErrInvalidCaseNote       = errors.New("invalid investigation case note")
	ErrInvalidEvidence       = errors.New("invalid investigation evidence")
	ErrInvalidIncidentWindow = errors.New("invalid investigation incident window")
	ErrInvalidCase           = errors.New("invalid investigation case")
	ErrInvalidCaseTitle      = errors.New("invalid investigation case title")
	ErrInvalidCaseSeverity   = errors.New("invalid investigation case severity")
	ErrInvalidCaseStatus     = errors.New("invalid investigation case status")
)

const MaximumIncidentWindow = 7 * 24 * time.Hour

type IncidentWindowSource string

const (
	IncidentWindowTimelineSearch IncidentWindowSource = "TIMELINE_SEARCH"
	IncidentWindowManualDefault  IncidentWindowSource = "MANUAL_DEFAULT"
	IncidentWindowGrafanaAlert   IncidentWindowSource = "GRAFANA_ALERT"
	IncidentWindowLegacyCreated  IncidentWindowSource = "LEGACY_CREATED_AT"
)

type IncidentWindow struct {
	From   time.Time
	To     time.Time
	Source IncidentWindowSource
}

func NewIncidentWindow(from, to time.Time, source IncidentWindowSource) (IncidentWindow, error) {
	from, to = from.UTC(), to.UTC()
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > MaximumIncidentWindow || !source.Valid() {
		return IncidentWindow{}, ErrInvalidIncidentWindow
	}
	return IncidentWindow{From: from, To: to, Source: source}, nil
}

func (source IncidentWindowSource) Valid() bool {
	return source == IncidentWindowTimelineSearch || source == IncidentWindowManualDefault || source == IncidentWindowGrafanaAlert || source == IncidentWindowLegacyCreated
}

type Severity string

const (
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

type CaseStatus string

const (
	StatusOpen            CaseStatus = "OPEN"
	StatusInvestigating   CaseStatus = "INVESTIGATING"
	StatusWaitingApproval CaseStatus = "WAITING_APPROVAL"
	StatusResolved        CaseStatus = "RESOLVED"
	StatusClosed          CaseStatus = "CLOSED"
)

type CasePriority string

const (
	PriorityP0 CasePriority = "P0"
	PriorityP1 CasePriority = "P1"
	PriorityP2 CasePriority = "P2"
	PriorityP3 CasePriority = "P3"
)

type SLAStatus string

const (
	SLAOnTrack   SLAStatus = "ON_TRACK"
	SLADueSoon   SLAStatus = "DUE_SOON"
	SLABreached  SLAStatus = "BREACHED"
	SLACompleted SLAStatus = "COMPLETED"
)

type CaseNote struct {
	ID         string
	Body       string
	AuthorID   string
	AuthorRole string
	CreatedAt  time.Time
}

type CaseEvidence struct {
	ID           string
	EvidenceType string
	Reference    string
	Checksum     string
	CollectedAt  time.Time
}

type InvestigationCase struct {
	ID                    string
	CaseNo                string
	Title                 string
	Severity              Severity
	Status                CaseStatus
	CorrelationID         string
	IncidentWindow        IncidentWindow
	Assignee              *string
	Priority              CasePriority
	Tags                  []string
	RelatedCorrelationIDs []string
	LastUpdatedBy         string
	RootCause             *string
	ResolutionSummary     *string
	FixedVersion          *string
	Notes                 *string
	WorkflowID            *string
	LockVersion           int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ClosedAt              *time.Time
}

func NewInvestigationCase(title string, severity Severity, correlationID string, incidentWindow IncidentWindow, actor string, now time.Time) (InvestigationCase, error) {
	title = strings.TrimSpace(title)
	correlationID = strings.TrimSpace(correlationID)
	actor = strings.TrimSpace(actor)
	if err := validateCaseTitle(title); err != nil {
		return InvestigationCase{}, err
	}
	if !severity.Valid() {
		return InvestigationCase{}, ErrInvalidCaseSeverity
	}
	if correlationID == "" || len(correlationID) > 200 || actor == "" {
		return InvestigationCase{}, ErrInvalidCase
	}
	validatedWindow, err := NewIncidentWindow(incidentWindow.From, incidentWindow.To, incidentWindow.Source)
	if err != nil {
		return InvestigationCase{}, err
	}
	now = now.UTC()
	result := InvestigationCase{
		Title: title, Severity: severity, Status: StatusOpen, CorrelationID: correlationID,
		IncidentWindow: validatedWindow, Priority: DefaultPriority(severity), Tags: []string{},
		RelatedCorrelationIDs: []string{}, LastUpdatedBy: actor, CreatedAt: now, UpdatedAt: now,
	}
	if err := result.Validate(); err != nil {
		return InvestigationCase{}, err
	}
	return result, nil
}

// RehydrateInvestigationCase is the only persistence entry point for a fully
// materialized aggregate. It normalizes owned strings and slices, then rejects
// persisted state that violates the same invariants used by commands.
func RehydrateInvestigationCase(candidate InvestigationCase) (InvestigationCase, error) {
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.CaseNo = strings.TrimSpace(candidate.CaseNo)
	candidate.Title = strings.TrimSpace(candidate.Title)
	candidate.CorrelationID = strings.TrimSpace(candidate.CorrelationID)
	candidate.LastUpdatedBy = strings.TrimSpace(candidate.LastUpdatedBy)
	candidate.Assignee = normalizedOptional(candidate.Assignee)
	candidate.RootCause = normalizedOptional(candidate.RootCause)
	candidate.ResolutionSummary = normalizedOptional(candidate.ResolutionSummary)
	candidate.FixedVersion = normalizedOptional(candidate.FixedVersion)
	candidate.Notes = cloneOptional(candidate.Notes)
	candidate.WorkflowID = cloneOptional(candidate.WorkflowID)
	candidate.Tags = slices.Clone(candidate.Tags)
	candidate.RelatedCorrelationIDs = slices.Clone(candidate.RelatedCorrelationIDs)
	if candidate.ID == "" || candidate.CaseNo == "" {
		return InvestigationCase{}, fmt.Errorf("%w: persisted identity is missing", ErrInvalidCase)
	}
	if err := candidate.Validate(); err != nil {
		return InvestigationCase{}, err
	}
	return candidate, nil
}

// Validate protects the aggregate boundary before persistence. IDs and CaseNo
// are database-assigned during Create, so persisted identity is enforced by
// RehydrateInvestigationCase rather than here.
func (investigationCase InvestigationCase) Validate() error {
	if err := validateCaseTitle(investigationCase.Title); err != nil {
		return err
	}
	if investigationCase.Title != strings.TrimSpace(investigationCase.Title) {
		return ErrInvalidCaseTitle
	}
	if !investigationCase.Severity.Valid() {
		return ErrInvalidCaseSeverity
	}
	if !investigationCase.Status.Valid() {
		return ErrInvalidCaseStatus
	}
	if investigationCase.CorrelationID == "" || investigationCase.CorrelationID != strings.TrimSpace(investigationCase.CorrelationID) || len(investigationCase.CorrelationID) > 200 {
		return fmt.Errorf("%w: invalid correlation id", ErrInvalidCase)
	}
	if _, err := NewIncidentWindow(investigationCase.IncidentWindow.From, investigationCase.IncidentWindow.To, investigationCase.IncidentWindow.Source); err != nil {
		return err
	}
	if !investigationCase.Priority.Valid() {
		return ErrInvalidPriority
	}
	if investigationCase.LastUpdatedBy == "" || investigationCase.LastUpdatedBy != strings.TrimSpace(investigationCase.LastUpdatedBy) || len(investigationCase.LastUpdatedBy) > 200 {
		return fmt.Errorf("%w: invalid last-updated actor", ErrInvalidCase)
	}
	if investigationCase.CreatedAt.IsZero() || investigationCase.UpdatedAt.IsZero() || investigationCase.UpdatedAt.Before(investigationCase.CreatedAt) || investigationCase.LockVersion < 0 {
		return fmt.Errorf("%w: invalid version or timestamps", ErrInvalidCase)
	}
	if investigationCase.Assignee != nil && (strings.TrimSpace(*investigationCase.Assignee) == "" || len(strings.TrimSpace(*investigationCase.Assignee)) > 200) {
		return ErrInvalidOwner
	}
	normalizedTags, err := normalizeUniqueValues(investigationCase.Tags, 10, 50, true)
	if err != nil || !slices.Equal(normalizedTags, investigationCase.Tags) {
		return ErrInvalidTags
	}
	normalizedRelated, err := normalizeUniqueValues(investigationCase.RelatedCorrelationIDs, 20, 200, false)
	if err != nil || !slices.Equal(normalizedRelated, investigationCase.RelatedCorrelationIDs) || slices.Contains(normalizedRelated, investigationCase.CorrelationID) {
		return ErrInvalidRelatedIDs
	}
	if investigationCase.Status == StatusResolved || investigationCase.Status == StatusClosed {
		if !optionalHasText(investigationCase.RootCause) || !optionalHasText(investigationCase.ResolutionSummary) {
			return ErrResolutionFields
		}
	}
	if (investigationCase.Status == StatusClosed) != (investigationCase.ClosedAt != nil) {
		return fmt.Errorf("%w: closed status and timestamp disagree", ErrInvalidCase)
	}
	return nil
}

// TransitionTo is the aggregate's state transition entry point. Callers must
// not mutate Status directly because resolution has mandatory business data
// and CLOSED has a dedicated operation.
func (investigationCase *InvestigationCase) TransitionTo(to CaseStatus, rootCause, resolutionSummary *string) error {
	if !to.Valid() {
		return ErrInvalidCaseStatus
	}
	if to == StatusClosed {
		return ErrCloseRequired
	}
	if investigationCase.Status == StatusClosed || !investigationCase.CanTransitionTo(to) {
		return ErrInvalidTransition
	}
	if to == StatusResolved && (rootCause == nil || strings.TrimSpace(*rootCause) == "" || resolutionSummary == nil || strings.TrimSpace(*resolutionSummary) == "") {
		return ErrResolutionFields
	}
	investigationCase.Status = to
	if rootCause != nil {
		investigationCase.RootCause = normalizedOptional(rootCause)
	}
	if resolutionSummary != nil {
		investigationCase.ResolutionSummary = normalizedOptional(resolutionSummary)
	}
	return nil
}

// Close is the aggregate's only path to CLOSED. The application layer still
// owns persistence and audit, while the aggregate owns the state invariant.
func (investigationCase *InvestigationCase) Close(now time.Time, rootCause, resolutionSummary string, fixedVersion *string) error {
	if investigationCase.Status == StatusClosed {
		return ErrInvalidTransition
	}
	if strings.TrimSpace(rootCause) == "" || strings.TrimSpace(resolutionSummary) == "" {
		return ErrResolutionFields
	}
	now = now.UTC()
	rootCause = strings.TrimSpace(rootCause)
	resolutionSummary = strings.TrimSpace(resolutionSummary)
	investigationCase.Status = StatusClosed
	investigationCase.RootCause = &rootCause
	investigationCase.ResolutionSummary = &resolutionSummary
	investigationCase.FixedVersion = normalizedOptional(fixedVersion)
	investigationCase.ClosedAt = &now
	investigationCase.UpdatedAt = now
	return nil
}

func (investigationCase *InvestigationCase) ChangeTitle(title string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if err := validateCaseTitle(title); err != nil {
		return err
	}
	investigationCase.Title = title
	return nil
}

func (investigationCase *InvestigationCase) ChangeSeverity(severity Severity) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	if !severity.Valid() {
		return ErrInvalidCaseSeverity
	}
	investigationCase.Severity = severity
	return nil
}

func (investigationCase *InvestigationCase) Assign(assignee *string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	if assignee == nil || strings.TrimSpace(*assignee) == "" {
		investigationCase.Assignee = nil
		return nil
	}
	normalized := strings.TrimSpace(*assignee)
	if len(normalized) > 200 {
		return fmt.Errorf("%w: owner exceeds 200 characters", ErrInvalidOwner)
	}
	investigationCase.Assignee = &normalized
	return nil
}

func (investigationCase *InvestigationCase) ChangePriority(priority CasePriority) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	if !priority.Valid() {
		return ErrInvalidPriority
	}
	investigationCase.Priority = priority
	return nil
}

func (investigationCase *InvestigationCase) ReplaceTags(tags []string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	normalized, err := normalizeUniqueValues(tags, 10, 50, true)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTags, err)
	}
	investigationCase.Tags = normalized
	return nil
}

func (investigationCase *InvestigationCase) ReplaceRelatedCorrelationIDs(values []string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	normalized, err := normalizeUniqueValues(values, 20, 200, false)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRelatedIDs, err)
	}
	if slices.Contains(normalized, investigationCase.CorrelationID) {
		return fmt.Errorf("%w: primary correlation id cannot be related to itself", ErrInvalidRelatedIDs)
	}
	investigationCase.RelatedCorrelationIDs = normalized
	return nil
}

func (investigationCase *InvestigationCase) WriteNote(id, body, authorID, authorRole string, now time.Time) (CaseNote, error) {
	if err := investigationCase.ensureMutable(); err != nil {
		return CaseNote{}, err
	}
	body = strings.TrimSpace(body)
	authorID = strings.TrimSpace(authorID)
	authorRole = strings.TrimSpace(authorRole)
	if strings.TrimSpace(id) == "" || body == "" || len(body) > 2000 || authorID == "" || len(authorID) > 200 || authorRole == "" || len(authorRole) > 50 {
		return CaseNote{}, ErrInvalidCaseNote
	}
	return CaseNote{ID: id, Body: body, AuthorID: authorID, AuthorRole: authorRole, CreatedAt: now.UTC()}, nil
}

// AttachEvent validates the case-side invariants for linking an existing
// canonical event. The event payload remains in ClickHouse; the aggregate only
// keeps a stable reference and, when needed, the source correlation relation.
func (investigationCase *InvestigationCase) AttachEvent(id, eventID, sourceCorrelationID, checksum string, now time.Time) (CaseEvidence, error) {
	if err := investigationCase.ensureMutable(); err != nil {
		return CaseEvidence{}, err
	}
	id = strings.TrimSpace(id)
	eventID = strings.TrimSpace(eventID)
	sourceCorrelationID = strings.TrimSpace(sourceCorrelationID)
	checksum = strings.TrimSpace(checksum)
	if id == "" || eventID == "" || len(eventID) > 200 || sourceCorrelationID == "" || len(sourceCorrelationID) > 200 || !validSHA256(checksum) {
		return CaseEvidence{}, ErrInvalidEvidence
	}
	if sourceCorrelationID != investigationCase.CorrelationID && !slices.Contains(investigationCase.RelatedCorrelationIDs, sourceCorrelationID) {
		related := append(slices.Clone(investigationCase.RelatedCorrelationIDs), sourceCorrelationID)
		if err := investigationCase.ReplaceRelatedCorrelationIDs(related); err != nil {
			return CaseEvidence{}, err
		}
	}
	return CaseEvidence{ID: id, EvidenceType: "EVENT", Reference: eventID, Checksum: checksum, CollectedAt: now.UTC()}, nil
}

func validSHA256(value string) bool {
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

func (investigationCase InvestigationCase) SLA(now time.Time) (time.Time, SLAStatus) {
	dueAt := investigationCase.CreatedAt.UTC().Add(investigationCase.Priority.slaDuration())
	if investigationCase.Status == StatusResolved || investigationCase.Status == StatusClosed {
		return dueAt, SLACompleted
	}
	now = now.UTC()
	if now.After(dueAt) {
		return dueAt, SLABreached
	}
	if !now.Add(time.Hour).Before(dueAt) {
		return dueAt, SLADueSoon
	}
	return dueAt, SLAOnTrack
}

func DefaultPriority(severity Severity) CasePriority {
	switch severity {
	case SeverityCritical:
		return PriorityP0
	case SeverityHigh:
		return PriorityP1
	case SeverityMedium:
		return PriorityP2
	default:
		return PriorityP3
	}
}

func (severity Severity) Valid() bool {
	return severity == SeverityLow || severity == SeverityMedium || severity == SeverityHigh || severity == SeverityCritical
}

func (status CaseStatus) Valid() bool {
	return status == StatusOpen || status == StatusInvestigating || status == StatusWaitingApproval || status == StatusResolved || status == StatusClosed
}

func (priority CasePriority) Valid() bool {
	return priority == PriorityP0 || priority == PriorityP1 || priority == PriorityP2 || priority == PriorityP3
}

func (priority CasePriority) slaDuration() time.Duration {
	switch priority {
	case PriorityP0:
		return time.Hour
	case PriorityP1:
		return 4 * time.Hour
	case PriorityP3:
		return 72 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func normalizeUniqueValues(values []string, maxItems, maxLength int, lower bool) ([]string, error) {
	if len(values) > maxItems {
		return nil, fmt.Errorf("at most %d values are allowed", maxItems)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" || len(value) > maxLength {
			return nil, fmt.Errorf("each value must contain 1-%d characters", maxLength)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func (investigationCase *InvestigationCase) SetRootCause(rootCause *string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	normalized := normalizedOptional(rootCause)
	if (investigationCase.Status == StatusResolved || investigationCase.Status == StatusClosed) && !optionalHasText(normalized) {
		return ErrResolutionFields
	}
	investigationCase.RootCause = normalized
	return nil
}

func (investigationCase *InvestigationCase) SetResolutionSummary(summary *string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	normalized := normalizedOptional(summary)
	if (investigationCase.Status == StatusResolved || investigationCase.Status == StatusClosed) && !optionalHasText(normalized) {
		return ErrResolutionFields
	}
	investigationCase.ResolutionSummary = normalized
	return nil
}

func (investigationCase *InvestigationCase) SetFixedVersion(version *string) error {
	if err := investigationCase.ensureMutable(); err != nil {
		return err
	}
	investigationCase.FixedVersion = normalizedOptional(version)
	return nil
}

func (investigationCase *InvestigationCase) CanTransitionTo(to CaseStatus) bool {
	return allowedTransitions[investigationCase.Status][to]
}

// AllowedTransitions is the read contract for human actions. CLOSED is
// included for every mutable case even though it uses the dedicated Close
// command rather than TransitionTo.
func (investigationCase InvestigationCase) AllowedTransitions() []CaseStatus {
	if investigationCase.Status == StatusClosed {
		return []CaseStatus{}
	}
	result := make([]CaseStatus, 0, 3)
	for _, candidate := range []CaseStatus{StatusInvestigating, StatusWaitingApproval, StatusResolved} {
		if investigationCase.CanTransitionTo(candidate) {
			result = append(result, candidate)
		}
	}
	return append(result, StatusClosed)
}

func CanTransition(from, to CaseStatus) bool {
	return allowedTransitions[from][to]
}

var allowedTransitions = map[CaseStatus]map[CaseStatus]bool{
	StatusOpen:            {StatusInvestigating: true},
	StatusInvestigating:   {StatusWaitingApproval: true, StatusResolved: true},
	StatusWaitingApproval: {StatusInvestigating: true, StatusResolved: true},
	StatusResolved:        {StatusInvestigating: true},
}

func (investigationCase *InvestigationCase) ensureMutable() error {
	if investigationCase.Status == StatusClosed {
		return ErrInvalidTransition
	}
	return nil
}

func validateCaseTitle(title string) error {
	if strings.TrimSpace(title) == "" || len(strings.TrimSpace(title)) > 300 {
		return ErrInvalidCaseTitle
	}
	return nil
}

func normalizedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func cloneOptional(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func optionalHasText(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}
