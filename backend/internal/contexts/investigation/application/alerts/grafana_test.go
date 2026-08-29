package alerts

import (
	"context"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

func TestGrafanaAlertServiceCreatesCaseAndEvidenceForEligibleAlert(t *testing.T) {
	transaction := &grafanaAlertTransactionFake{createdID: "case-created", receiptID: "receipt-1"}
	service := NewGrafanaAlertService(&grafanaAlertRepositoryFake{transaction: transaction})

	results, err := service.Process(t.Context(), grafanaAlertBatch("firing"))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(results) != 1 || results[0].Disposition != "CREATED_CASE" || results[0].ReasonCode != "BUSINESS_ALERT_ELIGIBLE" || results[0].InvestigationID != "case-created" {
		t.Fatalf("results = %#v", results)
	}
	if transaction.created.CorrelationID != "ORDER-2001" || transaction.created.Severity != "HIGH" || transaction.created.Title != "Shipment delay" {
		t.Fatalf("created investigation = %#v", transaction.created)
	}
	if transaction.created.WindowSource != string(domain.IncidentWindowGrafanaAlert) || transaction.created.IncidentTo.Sub(transaction.created.IncidentFrom) != 10*time.Minute {
		t.Fatalf("Grafana incident window = %#v", transaction.created)
	}
	if len(transaction.savedReceipt.DedupKey) != 64 || transaction.savedReceipt.Disposition != "CREATED_CASE" {
		t.Fatalf("saved receipt = %#v", transaction.savedReceipt)
	}
	if transaction.evidenceInvestigation != "case-created" || transaction.evidenceReceipt != "receipt-1" || len(transaction.evidenceChecksum) != 64 {
		t.Fatalf("saved evidence = %q %q %q", transaction.evidenceInvestigation, transaction.evidenceReceipt, transaction.evidenceChecksum)
	}
}

func TestGrafanaAlertServiceUsesAlertStartAndConfiguredIncidentWindow(t *testing.T) {
	transaction := &grafanaAlertTransactionFake{createdID: "case-created", receiptID: "receipt-1"}
	service := NewGrafanaAlertService(&grafanaAlertRepositoryFake{transaction: transaction})
	batch := grafanaAlertBatch("firing")
	startsAt := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	batch.Alerts[0].StartsAt = startsAt
	batch.Alerts[0].Labels["incident_window_seconds"] = "1800"

	if _, err := service.Process(t.Context(), batch); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if !transaction.created.IncidentTo.Equal(startsAt) || transaction.created.IncidentTo.Sub(transaction.created.IncidentFrom) != 30*time.Minute {
		t.Fatalf("configured Grafana incident window = %#v", transaction.created)
	}
}

func TestGrafanaAlertServiceLinksExistingCase(t *testing.T) {
	transaction := &grafanaAlertTransactionFake{openFound: true, openID: "case-existing", receiptID: "receipt-2"}
	service := NewGrafanaAlertService(&grafanaAlertRepositoryFake{transaction: transaction})

	results, err := service.Process(t.Context(), grafanaAlertBatch("firing"))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if results[0].Disposition != "LINKED_CASE" || results[0].InvestigationID != "case-existing" {
		t.Fatalf("result = %#v", results[0])
	}
	if transaction.createCalls != 0 || transaction.savedReceipt.InvestigationID != "case-existing" {
		t.Fatalf("unexpected transaction state = %#v", transaction)
	}
}

func TestGrafanaAlertServiceDuplicateDoesNotWriteAgain(t *testing.T) {
	transaction := &grafanaAlertTransactionFake{dedupFound: true, dedupID: "case-existing"}
	service := NewGrafanaAlertService(&grafanaAlertRepositoryFake{transaction: transaction})

	results, err := service.Process(t.Context(), grafanaAlertBatch("firing"))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if results[0].Disposition != "DUPLICATE" || results[0].ReasonCode != "DUPLICATE_NOTIFICATION" || results[0].InvestigationID != "case-existing" {
		t.Fatalf("result = %#v", results[0])
	}
	if transaction.receiptCalls != 0 || transaction.evidenceCalls != 0 || transaction.createCalls != 0 {
		t.Fatalf("duplicate performed writes: %#v", transaction)
	}
}

func TestGrafanaAlertServiceRecordsResolutionWithoutClosingCase(t *testing.T) {
	transaction := &grafanaAlertTransactionFake{firingFound: true, firingID: "case-firing", receiptID: "receipt-resolved"}
	service := NewGrafanaAlertService(&grafanaAlertRepositoryFake{transaction: transaction})

	results, err := service.Process(t.Context(), grafanaAlertBatch("resolved"))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if results[0].Disposition != "RECORDED_RESOLUTION" || results[0].ReasonCode != "RESOLUTION_RECORDED" || results[0].InvestigationID != "case-firing" {
		t.Fatalf("result = %#v", results[0])
	}
	if transaction.createCalls != 0 || transaction.evidenceCalls != 1 {
		t.Fatalf("unexpected transaction state = %#v", transaction)
	}
}

func TestGrafanaAlertServiceRecordsIneligibleAlertWithoutCaseEvidence(t *testing.T) {
	batch := grafanaAlertBatch("firing")
	batch.Alerts[0].Labels["severity"] = "MEDIUM"
	transaction := &grafanaAlertTransactionFake{receiptID: "receipt-ignored"}
	service := NewGrafanaAlertService(&grafanaAlertRepositoryFake{transaction: transaction})

	results, err := service.Process(t.Context(), batch)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if results[0].Disposition != "IGNORED" || results[0].ReasonCode != "NOT_ELIGIBLE" || results[0].InvestigationID != "" {
		t.Fatalf("result = %#v", results[0])
	}
	if transaction.receiptCalls != 1 || transaction.evidenceCalls != 0 || transaction.createCalls != 0 {
		t.Fatalf("unexpected transaction state = %#v", transaction)
	}
}

func grafanaAlertBatch(status string) GrafanaAlertBatch {
	return GrafanaAlertBatch{
		OrgID: 1, Receiver: "event-hunter", GroupKey: "group-1", Status: status,
		CommonLabels: map[string]string{"correlation_id": "ORDER-2001"},
		Alerts: []GrafanaAlert{{
			Fingerprint:  "fingerprint-1",
			Labels:       map[string]string{"severity": "high", "event_hunter": "investigate", "alertname": "Fallback title"},
			Annotations:  map[string]string{"summary": "Shipment delay"},
			GeneratorURL: "/alerting/grafana/event-quality-delay/view",
		}},
	}
}

type grafanaAlertRepositoryFake struct {
	transaction *grafanaAlertTransactionFake
}

func (repository *grafanaAlertRepositoryFake) Transact(ctx context.Context, operation func(GrafanaAlertTransaction) error) error {
	return operation(repository.transaction)
}

type grafanaAlertTransactionFake struct {
	dedupFound            bool
	dedupID               string
	firingFound           bool
	firingID              string
	openFound             bool
	openID                string
	createdID             string
	receiptID             string
	created               GrafanaAlertInvestigation
	savedReceipt          GrafanaAlertReceipt
	evidenceInvestigation string
	evidenceReceipt       string
	evidenceChecksum      string
	createCalls           int
	receiptCalls          int
	evidenceCalls         int
}

func (*grafanaAlertTransactionFake) LockDedup(context.Context, string) error { return nil }
func (transaction *grafanaAlertTransactionFake) InvestigationByDedup(context.Context, string) (string, bool, error) {
	return transaction.dedupID, transaction.dedupFound, nil
}
func (transaction *grafanaAlertTransactionFake) LatestFiringInvestigation(context.Context, string) (string, bool, error) {
	return transaction.firingID, transaction.firingFound, nil
}
func (transaction *grafanaAlertTransactionFake) OpenInvestigationByCorrelation(context.Context, string) (string, bool, error) {
	return transaction.openID, transaction.openFound, nil
}
func (transaction *grafanaAlertTransactionFake) CreateInvestigation(_ context.Context, investigation GrafanaAlertInvestigation) (string, error) {
	transaction.createCalls++
	transaction.created = investigation
	return transaction.createdID, nil
}
func (transaction *grafanaAlertTransactionFake) SaveReceipt(_ context.Context, receipt GrafanaAlertReceipt) (string, error) {
	transaction.receiptCalls++
	transaction.savedReceipt = receipt
	return transaction.receiptID, nil
}
func (transaction *grafanaAlertTransactionFake) SaveEvidence(_ context.Context, investigationID, receiptID, checksum string) error {
	transaction.evidenceCalls++
	transaction.evidenceInvestigation = investigationID
	transaction.evidenceReceipt = receiptID
	transaction.evidenceChecksum = checksum
	return nil
}
