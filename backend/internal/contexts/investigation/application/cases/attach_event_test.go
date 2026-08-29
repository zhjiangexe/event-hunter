package cases

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	forensics "event-hunter/backend/internal/contexts/investigation/application/search"
	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

func TestAttachEventValidatesSourceAndPersistsReference(t *testing.T) {
	repository := &attachmentRepositoryFake{current: domain.InvestigationCase{
		ID: "case-1", Status: domain.StatusInvestigating, CorrelationID: "ORDER-1", LockVersion: 3,
	}}
	audit := &auditWriterFake{}
	service := NewEventEvidenceService(repository, eventLookupFake{events: []forensics.ForensicsEvent{{EventID: "event-1", CorrelationID: "SHIPMENT-1"}}}, audit, attachmentDirectUnitOfWork{})
	now := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	result, err := service.AttachEvent(t.Context(), AttachEventCommand{
		InvestigationID: "case-1", ExpectedVersion: 3, EventID: "event-1",
		From: now.Add(-time.Hour), To: now.Add(time.Hour),
		Actor: ports.Actor{Subject: "demo", Role: "INVESTIGATOR"}, RequestID: "request-1",
	})
	if err != nil {
		t.Fatalf("AttachEvent() error = %v", err)
	}
	if !result.Attached || result.Investigation.LockVersion != 4 || result.Evidence.Reference != "event-1" {
		t.Fatalf("AttachEvent() = %#v", result)
	}
	if len(result.Investigation.RelatedCorrelationIDs) != 1 || result.Investigation.RelatedCorrelationIDs[0] != "SHIPMENT-1" {
		t.Fatalf("related correlations = %#v", result.Investigation.RelatedCorrelationIDs)
	}
	if audit.action != "ATTACH_INVESTIGATION_EVENT" || audit.metadata["event_id"] != "event-1" {
		t.Fatalf("audit = %q %#v", audit.action, audit.metadata)
	}
}

func TestAttachEventRejectsMissingSourceAndStaleVersion(t *testing.T) {
	now := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	repository := &attachmentRepositoryFake{current: domain.InvestigationCase{ID: "case-1", Status: domain.StatusOpen, CorrelationID: "ORDER-1", LockVersion: 4}}
	service := NewEventEvidenceService(repository, eventLookupFake{}, &auditWriterFake{}, attachmentDirectUnitOfWork{})
	_, err := service.AttachEvent(t.Context(), AttachEventCommand{InvestigationID: "case-1", ExpectedVersion: 3, EventID: "event-1", From: now.Add(-time.Hour), To: now})
	var conflict VersionConflictError
	if !errors.As(err, &conflict) || conflict.CurrentVersion != 4 {
		t.Fatalf("stale AttachEvent() error = %#v", err)
	}
	_, err = service.AttachEvent(t.Context(), AttachEventCommand{InvestigationID: "case-1", ExpectedVersion: 4, EventID: "event-1", From: now.Add(-time.Hour), To: now})
	if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("missing AttachEvent() error = %v, want %v", err, ErrEventNotFound)
	}
}

func TestAttachEventRollsBackEvidenceWhenAuditFails(t *testing.T) {
	now := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	initial := domain.InvestigationCase{ID: "case-1", Status: domain.StatusInvestigating, CorrelationID: "ORDER-1", LockVersion: 3}
	repository := &attachmentRepositoryFake{current: initial}
	auditFailure := errors.New("audit unavailable")
	audit := &auditWriterFake{err: auditFailure}
	unit := &attachmentRollbackUnitOfWorkFake{repository: repository}
	service := NewEventEvidenceService(repository, eventLookupFake{events: []forensics.ForensicsEvent{{EventID: "event-1", CorrelationID: "SHIPMENT-1"}}}, audit, unit)
	service.now = func() time.Time { return now }

	_, err := service.AttachEvent(t.Context(), AttachEventCommand{
		InvestigationID: "case-1", ExpectedVersion: 3, EventID: "event-1",
		From: now.Add(-time.Hour), To: now.Add(time.Hour), Actor: ports.Actor{Subject: "demo"},
	})
	if !errors.Is(err, auditFailure) {
		t.Fatalf("AttachEvent() error = %v, want audit failure", err)
	}
	if !reflect.DeepEqual(repository.current, initial) {
		t.Fatalf("case changed despite audit rollback: got %#v want %#v", repository.current, initial)
	}
}

type attachmentRepositoryFake struct {
	current domain.InvestigationCase
}

func (repository *attachmentRepositoryFake) Get(context.Context, string) (domain.InvestigationCase, error) {
	return repository.current, nil
}

func (repository *attachmentRepositoryFake) AppendEvidence(_ context.Context, value domain.InvestigationCase, expectedVersion int64, evidence domain.CaseEvidence) (domain.InvestigationCase, domain.CaseEvidence, bool, error) {
	if expectedVersion != repository.current.LockVersion {
		return domain.InvestigationCase{}, domain.CaseEvidence{}, false, domain.ErrOptimisticConflict
	}
	value.LockVersion++
	repository.current = value
	return value, evidence, true, nil
}

type eventLookupFake struct {
	events []forensics.ForensicsEvent
	err    error
}

func (lookup eventLookupFake) Search(context.Context, forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error) {
	return lookup.events, lookup.err
}

type auditWriterFake struct {
	action   string
	metadata map[string]any
	err      error
}

func (writer *auditWriterFake) RecordAudit(_ context.Context, _ ports.Actor, action, _, _ string, metadata map[string]any) error {
	writer.action = action
	writer.metadata = metadata
	return writer.err
}

type attachmentRollbackUnitOfWorkFake struct {
	repository *attachmentRepositoryFake
}

type attachmentDirectUnitOfWork struct{}

func (attachmentDirectUnitOfWork) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

func (unit *attachmentRollbackUnitOfWorkFake) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	before := unit.repository.current
	if err := operation(ctx); err != nil {
		unit.repository.current = before
		return err
	}
	return nil
}
