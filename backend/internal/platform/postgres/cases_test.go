package postgres

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

type caseRowStub struct{}

func (caseRowStub) Scan(dest ...any) error {
	*(dest[0].(*string)) = "case-1"
	*(dest[1].(*string)) = "EH-1"
	*(dest[2].(*string)) = "Case"
	*(dest[3].(*domain.Severity)) = domain.SeverityHigh
	*(dest[4].(*domain.CaseStatus)) = domain.StatusOpen
	*(dest[5].(*string)) = "ORDER-1"
	*(dest[6].(*time.Time)) = time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	*(dest[7].(*time.Time)) = time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	*(dest[8].(*domain.IncidentWindowSource)) = domain.IncidentWindowTimelineSearch
	*(dest[10].(*domain.CasePriority)) = domain.PriorityP1
	*(dest[11].(*[]byte)) = []byte(`["payments","vip"]`)
	*(dest[12].(*[]byte)) = []byte(`["PAYMENT-1"]`)
	*(dest[13].(*string)) = "demo:investigator"
	*(dest[19].(*int64)) = 3
	*(dest[20].(*time.Time)) = time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	*(dest[21].(*time.Time)) = time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	return nil
}

func TestScanCaseDecodesPostgresArraysReturnedAsJSON(t *testing.T) {
	result, err := scanCase(caseRowStub{})
	if err != nil {
		t.Fatalf("scanCase() error = %v", err)
	}
	if !reflect.DeepEqual(result.Tags, []string{"payments", "vip"}) || !reflect.DeepEqual(result.RelatedCorrelationIDs, []string{"PAYMENT-1"}) {
		t.Fatalf("scanCase() = %#v", result)
	}
}

type invalidSeverityCaseRowStub struct{ caseRowStub }

func (invalidSeverityCaseRowStub) Scan(dest ...any) error {
	if err := (caseRowStub{}).Scan(dest...); err != nil {
		return err
	}
	*(dest[3].(*domain.Severity)) = domain.Severity("URGENT")
	return nil
}

func TestScanCaseRejectsPersistedStateOutsideDomainInvariant(t *testing.T) {
	_, err := scanCase(invalidSeverityCaseRowStub{})
	if !errors.Is(err, domain.ErrInvalidCaseSeverity) {
		t.Fatalf("scanCase() error = %v, want %v", err, domain.ErrInvalidCaseSeverity)
	}
}
