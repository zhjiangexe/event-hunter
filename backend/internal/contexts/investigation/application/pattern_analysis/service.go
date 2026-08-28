package patternanalysis

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	"event-hunter/backend/internal/contexts/investigation/domain"
	domainpatterns "event-hunter/backend/internal/contexts/investigation/domain/patterns"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

var ErrUnknownPattern = errors.New("unknown or inactive pattern")

const maxAnalysisWindow = 7 * 24 * time.Hour

type Actor = ports.Actor
type AuditEntry = ports.AuditEntry
type Evidence = ports.Evidence
type PatternFinding = ports.PatternFinding
type CaseRepository = domain.CaseRepository
type InvestigationDetailsRepository = ports.InvestigationDetailsRepository
type EventSearchFilter = forensics.EventSearchFilter
type ProcessingSummary = forensics.ProcessingSummary
type ForensicsEvent = forensics.ForensicsEvent

type EventWindowSource interface {
	CorrelationEventWindow(ctx context.Context, correlationID string) (time.Time, time.Time, int, error)
}

type EffectiveAnalysisWindow struct {
	From             time.Time `json:"from"`
	To               time.Time `json:"to"`
	ObservedAt       time.Time `json:"observed_at"`
	Anchor           string    `json:"anchor"`
	SourceEventCount int       `json:"source_event_count"`
}

type AnalysisResult struct {
	AnalyzedAt         time.Time                `json:"analyzed_at"`
	AnalysisStatus     string                   `json:"analysis_status"`
	ExecutedPatternIDs []string                 `json:"executed_pattern_ids"`
	EffectiveWindow    *EffectiveAnalysisWindow `json:"effective_window"`
	Findings           []PatternFinding         `json:"-"`
}

type AnalysisWindowError struct {
	FirstOccurredAt time.Time
	LastOccurredAt  time.Time
}

func (err AnalysisWindowError) Error() string {
	return fmt.Sprintf("correlation event span %s exceeds analysis limit %s", err.LastOccurredAt.Sub(err.FirstOccurredAt), maxAnalysisWindow)
}

type PatternSourceError struct{ Err error }

func (err PatternSourceError) Error() string { return fmt.Sprintf("pattern source: %v", err.Err) }
func (err PatternSourceError) Unwrap() error { return err.Err }

type PatternPersistenceError struct{ Err error }

func (err PatternPersistenceError) Error() string {
	return fmt.Sprintf("pattern persistence: %v", err.Err)
}
func (err PatternPersistenceError) Unwrap() error { return err.Err }

type PatternService struct {
	cases       CaseRepository
	details     InvestigationDetailsRepository
	forensics   *forensics.ForensicsService
	eventWindow EventWindowSource
	unitOfWork  ports.UnitOfWork
	now         func() time.Time
}

func NewPatternService(cases CaseRepository, details InvestigationDetailsRepository, readModel *forensics.ForensicsService, eventWindow EventWindowSource, unitsOfWork ...ports.UnitOfWork) *PatternService {
	var unitOfWork ports.UnitOfWork
	if len(unitsOfWork) > 0 {
		unitOfWork = unitsOfWork[0]
	}
	return &PatternService{cases: cases, details: details, forensics: readModel, eventWindow: eventWindow, unitOfWork: unitOfWork, now: time.Now}
}

func (service *PatternService) Analyze(ctx context.Context, investigationID string, patternIDs []string, actor Actor, requestID string) (AnalysisResult, error) {
	result := AnalysisResult{AnalysisStatus: "EVALUATED", Findings: []PatternFinding{}}
	if len(patternIDs) == 0 {
		for _, definition := range domainpatterns.Registry() {
			if definition.Status == "ACTIVE" {
				patternIDs = append(patternIDs, definition.ID)
			}
		}
	}
	result.ExecutedPatternIDs = append([]string(nil), patternIDs...)
	definitions := make([]domainpatterns.Definition, 0, len(patternIDs))
	for _, patternID := range patternIDs {
		definition, ok := domainpatterns.Lookup(patternID)
		if !ok {
			return AnalysisResult{}, fmt.Errorf("%w: %s", ErrUnknownPattern, patternID)
		}
		definitions = append(definitions, definition)
	}
	investigationCase, err := service.cases.Get(ctx, investigationID)
	if err != nil {
		return AnalysisResult{}, err
	}
	if investigationCase.Status == domain.StatusClosed {
		return AnalysisResult{}, domain.ErrInvalidTransition
	}
	now := service.now().UTC()
	result.AnalyzedAt = now
	firstOccurredAt, lastOccurredAt, sourceEventCount, err := service.eventWindow.CorrelationEventWindow(ctx, investigationCase.CorrelationID)
	if err != nil {
		return AnalysisResult{}, PatternSourceError{Err: err}
	}
	if sourceEventCount == 0 {
		result.AnalysisStatus = "NO_EVENTS"
		if err := service.recordAnalysis(ctx, investigationID, actor, requestID, patternIDs, result); err != nil {
			return AnalysisResult{}, err
		}
		return result, nil
	}
	queryFrom := firstOccurredAt.UTC()
	queryTo := queryFrom.Add(maxAnalysisWindow)
	if !lastOccurredAt.Before(queryTo) {
		return AnalysisResult{}, AnalysisWindowError{FirstOccurredAt: queryFrom, LastOccurredAt: lastOccurredAt.UTC()}
	}
	observedAt := now
	if observedAt.After(queryTo) {
		observedAt = queryTo
	}
	result.EffectiveWindow = &EffectiveAnalysisWindow{
		From: queryFrom, To: queryTo, ObservedAt: observedAt,
		Anchor: "EARLIEST_CORRELATION_EVENT", SourceEventCount: sourceEventCount,
	}
	forensicsEvents, err := service.forensics.Search(ctx, forensics.EventSearchFilter{
		From: queryFrom, To: queryTo, Limit: 10000, CorrelationID: investigationCase.CorrelationID,
	})
	if err != nil {
		return AnalysisResult{}, PatternSourceError{Err: err}
	}
	events, err := domainPatternEvents(forensicsEvents)
	if err != nil {
		return AnalysisResult{}, PatternSourceError{Err: err}
	}
	findings := make([]PatternFinding, 0, len(definitions))
	type pendingEvidence struct {
		evidenceType string
		reference    string
	}
	evidenceToSave := make([]pendingEvidence, 0, len(definitions)*3)
	for _, definition := range definitions {
		match, evaluateErr := domainpatterns.Evaluate(definition, events, observedAt)
		if evaluateErr != nil {
			return AnalysisResult{}, PatternSourceError{Err: evaluateErr}
		}
		if match == nil {
			continue
		}
		findingReference := fmt.Sprintf("%s:v%d:%s", definition.ID, definition.Version, investigationCase.CorrelationID)
		evidenceReferences := []string{match.TriggerEvent.ID}
		if match.TriggerEvent.TraceID != nil && strings.TrimSpace(*match.TriggerEvent.TraceID) != "" {
			evidenceReferences = append(evidenceReferences, *match.TriggerEvent.TraceID)
		}
		evidenceReferences = append(evidenceReferences, findingReference)
		finding := PatternFinding{
			PatternID: definition.ID, PatternVersion: definition.Version, Severity: definition.Severity,
			MatchedConditions: match.Conditions, EvidenceReferences: evidenceReferences,
			RecommendedNextQuery: definition.RecommendedNextQuery, QueryTemplateID: definition.EvidenceQueryTemplateID,
			WindowFrom: match.WindowFrom, WindowTo: match.WindowTo,
			IdempotencyKey: fmt.Sprintf("%s:%s:%d:%s", investigationID, definition.ID, definition.Version, match.TriggerEvent.ID),
		}
		evidenceToSave = append(evidenceToSave, pendingEvidence{evidenceType: "EVENT", reference: match.TriggerEvent.ID})
		if match.TriggerEvent.TraceID != nil && strings.TrimSpace(*match.TriggerEvent.TraceID) != "" {
			evidenceToSave = append(evidenceToSave, pendingEvidence{evidenceType: "TRACE", reference: *match.TriggerEvent.TraceID})
		}
		evidenceToSave = append(evidenceToSave, pendingEvidence{evidenceType: "PATTERN_FINDING", reference: findingReference})
		findings = append(findings, finding)
	}
	result.Findings = findings
	if err := service.withinTransaction(ctx, func(transactionContext context.Context) error {
		for _, finding := range findings {
			if err := service.details.SaveFinding(transactionContext, investigationID, finding); err != nil {
				return err
			}
		}
		for _, evidence := range evidenceToSave {
			if err := service.saveEvidence(transactionContext, investigationID, evidence.evidenceType, evidence.reference); err != nil {
				return err
			}
		}
		return service.details.RecordAudit(transactionContext, actor, "ANALYZE_INVESTIGATION", investigationID, requestID, analysisAuditMetadata(patternIDs, result))
	}); err != nil {
		return AnalysisResult{}, PatternPersistenceError{Err: err}
	}
	return result, nil
}

func (service *PatternService) recordAnalysis(ctx context.Context, investigationID string, actor Actor, requestID string, patternIDs []string, result AnalysisResult) error {
	if err := service.withinTransaction(ctx, func(transactionContext context.Context) error {
		return service.details.RecordAudit(transactionContext, actor, "ANALYZE_INVESTIGATION", investigationID, requestID, analysisAuditMetadata(patternIDs, result))
	}); err != nil {
		return PatternPersistenceError{Err: err}
	}
	return nil
}

func analysisAuditMetadata(patternIDs []string, result AnalysisResult) map[string]any {
	metadata := map[string]any{
		"pattern_ids": patternIDs, "executed_pattern_ids": result.ExecutedPatternIDs,
		"finding_count": len(result.Findings), "analysis_status": result.AnalysisStatus,
	}
	if result.EffectiveWindow != nil {
		metadata["effective_window"] = result.EffectiveWindow
	}
	return metadata
}

func (service *PatternService) saveEvidence(ctx context.Context, investigationID, evidenceType, reference string) error {
	checksum := sha256.Sum256([]byte(evidenceType + ":" + reference))
	if err := service.details.SaveEvidence(ctx, investigationID, evidenceType, reference, fmt.Sprintf("%x", checksum[:])); err != nil {
		return PatternPersistenceError{Err: err}
	}
	return nil
}

func (service *PatternService) withinTransaction(ctx context.Context, operation func(context.Context) error) error {
	if service.unitOfWork == nil {
		return operation(ctx)
	}
	return service.unitOfWork.WithinTransaction(ctx, operation)
}

func domainPatternEvents(events []forensics.ForensicsEvent) ([]domainpatterns.Event, error) {
	result := make([]domainpatterns.Event, 0, len(events))
	for _, event := range events {
		occurredAt, err := parseForensicsTime(event.OccurredAt)
		if err != nil {
			return nil, fmt.Errorf("event %s occurred_at: %w", event.EventID, err)
		}
		result = append(result, domainpatterns.Event{ID: event.EventID, Type: event.EventType, OccurredAt: occurredAt, TraceID: event.TraceID})
	}
	return result, nil
}

func parseForensicsTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}
