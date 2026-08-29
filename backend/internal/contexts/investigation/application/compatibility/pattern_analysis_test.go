package compatibility

import (
	"context"
	"errors"
	"testing"
	"time"

	forensics "event-hunter/backend/internal/contexts/investigation/application/search"
	"event-hunter/backend/internal/contexts/investigation/domain"
	domainpatterns "event-hunter/backend/internal/contexts/investigation/domain/patterns"
)

func TestPatternServicePersistsTriggerRelativeFindingAndEvidence(t *testing.T) {
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	traceID := "trace-payment-1"
	readModel := &forensicsReadModelFake{firstOccurredAt: triggeredAt.Add(-time.Minute), lastOccurredAt: triggeredAt, eventCount: 2, events: []forensics.ForensicsEvent{{
		EventID: "payment-1", EventType: "PaymentCompleted", OccurredAt: triggeredAt.Format(time.RFC3339), TraceID: &traceID,
	}}}
	details := &patternDetailsFake{}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", CorrelationID: "ORDER-1"}},
		details,
		forensics.NewForensicsService(readModel),
		readModel,
		directUnitOfWork{},
	)
	service.now = func() time.Time { return triggeredAt.Add(6 * time.Minute) }

	result, err := service.Analyze(t.Context(), "case-1", nil, Actor{Subject: "demo", Role: "INVESTIGATOR"}, "request-1")
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(result.ExecutedPatternIDs) != 1 || result.ExecutedPatternIDs[0] != "payment-completed-without-shipment" {
		t.Fatalf("executed patterns = %#v", result.ExecutedPatternIDs)
	}
	findings := result.Findings
	if len(findings) != 1 || !findings[0].WindowFrom.Equal(triggeredAt) || !findings[0].WindowTo.Equal(triggeredAt.Add(5*time.Minute)) {
		t.Fatalf("findings = %#v", findings)
	}
	if got := readModel.lastFilter.To.Sub(readModel.lastFilter.From); got != maxAnalysisWindow {
		t.Fatalf("source query window = %s, want 7 days", got)
	}
	if result.EffectiveWindow == nil || !result.EffectiveWindow.From.Equal(triggeredAt.Add(-time.Minute)) || result.EffectiveWindow.Anchor != "EARLIEST_CORRELATION_EVENT" || result.EffectiveWindow.SourceEventCount != 2 {
		t.Fatalf("effective window = %#v", result.EffectiveWindow)
	}
	if len(details.evidence) != 3 || details.evidence[0].evidenceType != "EVENT" || details.evidence[1].evidenceType != "TRACE" || details.evidence[2].evidenceType != "PATTERN_FINDING" {
		t.Fatalf("evidence = %#v", details.evidence)
	}
	if details.auditAction != "ANALYZE_INVESTIGATION" {
		t.Fatalf("audit action = %q", details.auditAction)
	}
}

func TestPatternServiceDoesNotMatchShipmentInsideFiveMinutes(t *testing.T) {
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	readModel := &forensicsReadModelFake{firstOccurredAt: triggeredAt.Add(-time.Minute), lastOccurredAt: triggeredAt.Add(4 * time.Minute), eventCount: 3, events: []ForensicsEvent{
		{EventID: "payment-1", EventType: "PaymentCompleted", OccurredAt: triggeredAt.Format(time.RFC3339)},
		{EventID: "shipment-1", EventType: "ShipmentCreated", OccurredAt: triggeredAt.Add(4 * time.Minute).Format(time.RFC3339)},
	}}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", CorrelationID: "ORDER-1"}},
		&patternDetailsFake{},
		forensics.NewForensicsService(readModel),
		readModel,
		directUnitOfWork{},
	)
	service.now = func() time.Time { return triggeredAt.Add(6 * time.Minute) }

	result, err := service.Analyze(t.Context(), "case-1", []string{"payment-completed-without-shipment"}, Actor{}, "")
	if err != nil || len(result.Findings) != 0 {
		t.Fatalf("findings = %#v, error = %v", result.Findings, err)
	}
}

func TestPatternServiceRejectsUnknownPattern(t *testing.T) {
	readModel := &forensicsReadModelFake{}
	service := NewPatternService(&caseRepositoryFake{}, &patternDetailsFake{}, forensics.NewForensicsService(readModel), readModel, directUnitOfWork{})
	_, err := service.Analyze(t.Context(), "case-1", []string{"unknown-pattern"}, Actor{}, "")
	if !errors.Is(err, ErrUnknownPattern) {
		t.Fatalf("Analyze() error = %v", err)
	}
}

func TestPatternServiceRejectsClosedInvestigationBeforeReadingEvents(t *testing.T) {
	readModel := &forensicsReadModelFake{}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusClosed, CorrelationID: "ORDER-CLOSED"}},
		&patternDetailsFake{}, forensics.NewForensicsService(readModel), readModel, directUnitOfWork{},
	)

	_, err := service.Analyze(t.Context(), "case-1", nil, Actor{}, "")
	if !errors.Is(err, domain.ErrInvalidTransition) || readModel.searchCalls != 0 {
		t.Fatalf("Analyze() error = %v, search calls = %d", err, readModel.searchCalls)
	}
}

func TestPatternServiceRollsBackFindingsAndEvidenceWhenAuditFails(t *testing.T) {
	triggeredAt := time.Date(2026, 8, 20, 11, 1, 0, 0, time.UTC)
	auditFailure := errors.New("audit unavailable")
	details := &patternDetailsFake{auditErr: auditFailure}
	unit := &patternRollbackUnitOfWorkFake{details: details}
	readModel := &forensicsReadModelFake{firstOccurredAt: triggeredAt, lastOccurredAt: triggeredAt, eventCount: 1, events: []forensics.ForensicsEvent{{
		EventID: "payment-1", EventType: "PaymentCompleted", OccurredAt: triggeredAt.Format(time.RFC3339),
	}}}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", CorrelationID: "ORDER-1"}},
		details,
		forensics.NewForensicsService(readModel),
		readModel,
		unit,
	)
	service.now = func() time.Time { return triggeredAt.Add(6 * time.Minute) }

	_, err := service.Analyze(t.Context(), "case-1", nil, Actor{Subject: "demo"}, "request-1")
	if !errors.Is(err, auditFailure) {
		t.Fatalf("Analyze() error = %v, want audit failure", err)
	}
	if details.finding.PatternID != "" || len(details.evidence) != 0 {
		t.Fatalf("pattern state persisted despite rollback: finding=%#v evidence=%#v", details.finding, details.evidence)
	}
}

func TestPatternServiceUsesTheSameHistoricalWindowAcrossDelayedReruns(t *testing.T) {
	first := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	paymentAt := first.Add(time.Minute)
	readModel := &forensicsReadModelFake{
		firstOccurredAt: first, lastOccurredAt: paymentAt, eventCount: 2,
		events: []ForensicsEvent{{EventID: "payment-historical", EventType: "PaymentCompleted", OccurredAt: paymentAt.Format(time.RFC3339)}},
	}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", CorrelationID: "ORDER-HISTORICAL"}},
		&patternDetailsFake{}, forensics.NewForensicsService(readModel), readModel, directUnitOfWork{},
	)
	currentTime := first.Add(30 * 24 * time.Hour)
	service.now = func() time.Time { return currentTime }

	firstResult, err := service.Analyze(t.Context(), "case-1", nil, Actor{}, "first-run")
	if err != nil {
		t.Fatalf("first Analyze() error = %v", err)
	}
	currentTime = first.Add(365 * 24 * time.Hour)
	secondResult, err := service.Analyze(t.Context(), "case-1", nil, Actor{}, "second-run")
	if err != nil {
		t.Fatalf("second Analyze() error = %v", err)
	}
	if firstResult.EffectiveWindow == nil || secondResult.EffectiveWindow == nil || firstResult.EffectiveWindow.From != secondResult.EffectiveWindow.From || firstResult.EffectiveWindow.To != secondResult.EffectiveWindow.To {
		t.Fatalf("rerun windows differ: %#v / %#v", firstResult.EffectiveWindow, secondResult.EffectiveWindow)
	}
	if len(firstResult.Findings) != 1 || len(secondResult.Findings) != 1 || firstResult.Findings[0].IdempotencyKey != secondResult.Findings[0].IdempotencyKey {
		t.Fatalf("rerun findings differ: %#v / %#v", firstResult.Findings, secondResult.Findings)
	}
}

func TestPatternServiceReturnsExplicitNoEventsStatus(t *testing.T) {
	readModel := &forensicsReadModelFake{}
	details := &patternDetailsFake{}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", CorrelationID: "ORDER-EMPTY"}},
		details, forensics.NewForensicsService(readModel), readModel, directUnitOfWork{},
	)
	result, err := service.Analyze(t.Context(), "case-1", nil, Actor{}, "empty-run")
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.AnalysisStatus != "NO_EVENTS" || result.EffectiveWindow != nil || len(result.Findings) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if readModel.searchCalls != 0 || details.auditMetadata["analysis_status"] != "NO_EVENTS" {
		t.Fatalf("search calls = %d, audit = %#v", readModel.searchCalls, details.auditMetadata)
	}
	if len(result.ExecutedPatternIDs) != len(domainpatterns.Registry()) {
		t.Fatalf("default executed patterns = %#v, registry = %#v", result.ExecutedPatternIDs, domainpatterns.Registry())
	}
}

func TestPatternServiceRejectsCorrelationSpanningBeyondSevenDays(t *testing.T) {
	first := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	readModel := &forensicsReadModelFake{firstOccurredAt: first, lastOccurredAt: first.Add(maxAnalysisWindow), eventCount: 2}
	service := NewPatternService(
		&caseRepositoryFake{current: domain.InvestigationCase{ID: "case-1", CorrelationID: "ORDER-LONG"}},
		&patternDetailsFake{}, forensics.NewForensicsService(readModel), readModel, directUnitOfWork{},
	)
	_, err := service.Analyze(t.Context(), "case-1", nil, Actor{}, "")
	var windowErr AnalysisWindowError
	if !errors.As(err, &windowErr) || readModel.searchCalls != 0 {
		t.Fatalf("Analyze() error = %v, search calls = %d", err, readModel.searchCalls)
	}
}

type forensicsReadModelFake struct {
	events          []forensics.ForensicsEvent
	err             error
	lastFilter      EventSearchFilter
	firstOccurredAt time.Time
	lastOccurredAt  time.Time
	eventCount      int
	windowErr       error
	searchCalls     int
}

type caseRepositoryFake struct {
	current domain.InvestigationCase
}

func (repository *caseRepositoryFake) Create(context.Context, domain.InvestigationCase) (domain.InvestigationCase, error) {
	return repository.current, nil
}

func (repository *caseRepositoryFake) Get(context.Context, string) (domain.InvestigationCase, error) {
	return repository.current, nil
}

func (repository *caseRepositoryFake) Update(context.Context, domain.InvestigationCase) (domain.InvestigationCase, error) {
	return repository.current, nil
}

func (repository *caseRepositoryFake) AppendNote(context.Context, string, int64, domain.CaseNote, string, time.Time) (domain.InvestigationCase, error) {
	return repository.current, nil
}

func (repository *caseRepositoryFake) List(context.Context, domain.CaseFilter) (domain.CasePage, error) {
	return domain.CasePage{Items: []domain.InvestigationCase{repository.current}}, nil
}

func (model *forensicsReadModelFake) Search(_ context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error) {
	model.searchCalls++
	model.lastFilter = filter
	return model.events, model.err
}
func (model *forensicsReadModelFake) CorrelationEventWindow(context.Context, string) (time.Time, time.Time, int, error) {
	return model.firstOccurredAt, model.lastOccurredAt, model.eventCount, model.windowErr
}
func (*forensicsReadModelFake) ProcessingSummaries(context.Context, []string) (map[string]ProcessingSummary, error) {
	return nil, nil
}

type directUnitOfWork struct{}

func (directUnitOfWork) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

type savedEvidence struct {
	evidenceType string
	reference    string
}

type patternDetailsFake struct {
	finding       PatternFinding
	evidence      []savedEvidence
	auditAction   string
	auditErr      error
	auditMetadata map[string]any
}

func (repository *patternDetailsFake) RecordAudit(_ context.Context, _ Actor, action, _, _ string, metadata map[string]any) error {
	repository.auditAction = action
	repository.auditMetadata = metadata
	return repository.auditErr
}
func (*patternDetailsFake) Audit(context.Context, string) ([]AuditEntry, error) { return nil, nil }
func (*patternDetailsFake) Findings(context.Context, string) ([]PatternFinding, error) {
	return nil, nil
}
func (*patternDetailsFake) Evidence(context.Context, string) ([]Evidence, error)     { return nil, nil }
func (*patternDetailsFake) Notes(context.Context, string) ([]domain.CaseNote, error) { return nil, nil }
func (repository *patternDetailsFake) SaveFinding(_ context.Context, _ string, finding PatternFinding) error {
	repository.finding = finding
	return nil
}
func (repository *patternDetailsFake) SaveEvidence(_ context.Context, _, evidenceType, reference, _ string) error {
	repository.evidence = append(repository.evidence, savedEvidence{evidenceType: evidenceType, reference: reference})
	return nil
}

type patternRollbackUnitOfWorkFake struct {
	details *patternDetailsFake
}

func (unit *patternRollbackUnitOfWorkFake) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	beforeFinding := unit.details.finding
	beforeEvidence := append([]savedEvidence(nil), unit.details.evidence...)
	beforeAudit := unit.details.auditAction
	if err := operation(ctx); err != nil {
		unit.details.finding = beforeFinding
		unit.details.evidence = beforeEvidence
		unit.details.auditAction = beforeAudit
		return err
	}
	return nil
}
