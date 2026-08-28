package alertintake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

type GrafanaAlertBatch struct {
	OrgID        int64
	Receiver     string
	GroupKey     string
	Status       string
	CommonLabels map[string]string
	Alerts       []GrafanaAlert
}

type GrafanaAlert struct {
	Labels       map[string]string
	Annotations  map[string]string
	GeneratorURL string
	Fingerprint  string
	DashboardURL string
	PanelURL     string
	StartsAt     time.Time
}

type GrafanaAlertResult struct {
	Fingerprint     string
	Disposition     string
	ReasonCode      string
	InvestigationID string
}

type GrafanaAlertReceipt struct {
	DedupKey        string
	OrgID           int64
	Receiver        string
	GroupKey        string
	Fingerprint     string
	AlertStatus     string
	CorrelationID   string
	Severity        string
	Labels          map[string]string
	Annotations     map[string]string
	GeneratorURL    string
	DashboardURL    string
	PanelURL        string
	InvestigationID string
	Disposition     string
}

type GrafanaAlertInvestigation struct {
	CaseNo        string
	Title         string
	Severity      string
	CorrelationID string
	IncidentFrom  time.Time
	IncidentTo    time.Time
	WindowSource  string
}

type GrafanaAlertTransaction interface {
	LockDedup(ctx context.Context, dedupKey string) error
	InvestigationByDedup(ctx context.Context, dedupKey string) (string, bool, error)
	LatestFiringInvestigation(ctx context.Context, fingerprint string) (string, bool, error)
	OpenInvestigationByCorrelation(ctx context.Context, correlationID string) (string, bool, error)
	CreateInvestigation(ctx context.Context, investigation GrafanaAlertInvestigation) (string, error)
	SaveReceipt(ctx context.Context, receipt GrafanaAlertReceipt) (string, error)
	SaveEvidence(ctx context.Context, investigationID, receiptID, checksum string) error
}

type GrafanaAlertRepository interface {
	Transact(ctx context.Context, operation func(GrafanaAlertTransaction) error) error
}

type GrafanaAlertService struct {
	repository GrafanaAlertRepository
	now        func() time.Time
}

func NewGrafanaAlertService(repository GrafanaAlertRepository) *GrafanaAlertService {
	return &GrafanaAlertService{repository: repository, now: time.Now}
}

func (service *GrafanaAlertService) Process(ctx context.Context, batch GrafanaAlertBatch) ([]GrafanaAlertResult, error) {
	results := make([]GrafanaAlertResult, 0, len(batch.Alerts))
	for _, alert := range batch.Alerts {
		result, err := service.processAlert(ctx, batch, alert)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (service *GrafanaAlertService) processAlert(ctx context.Context, batch GrafanaAlertBatch, alert GrafanaAlert) (GrafanaAlertResult, error) {
	correlationID := firstNonEmpty(alert.Labels["correlation_id"], batch.CommonLabels["correlation_id"])
	severity := strings.ToUpper(firstNonEmpty(alert.Labels["severity"], batch.CommonLabels["severity"]))
	dedupKey := hashValue(fmt.Sprintf("%d|%s|%s|%s", batch.OrgID, batch.GroupKey, alert.Fingerprint, batch.Status))
	result := GrafanaAlertResult{Fingerprint: alert.Fingerprint}

	err := service.repository.Transact(ctx, func(transaction GrafanaAlertTransaction) error {
		if err := transaction.LockDedup(ctx, dedupKey); err != nil {
			return err
		}
		if investigationID, found, err := transaction.InvestigationByDedup(ctx, dedupKey); err != nil {
			return err
		} else if found {
			result.Disposition = "DUPLICATE"
			result.ReasonCode = "DUPLICATE_NOTIFICATION"
			result.InvestigationID = investigationID
			return nil
		}

		disposition, reasonCode, investigationID := "IGNORED", "NOT_ELIGIBLE", ""
		if batch.Status == "resolved" {
			var err error
			investigationID, _, err = transaction.LatestFiringInvestigation(ctx, alert.Fingerprint)
			if err != nil {
				return err
			}
			disposition, reasonCode = "RECORDED_RESOLUTION", "RESOLUTION_RECORDED"
		} else if eligibleGrafanaAlert(alert, severity, correlationID) {
			var found bool
			var err error
			investigationID, found, err = transaction.OpenInvestigationByCorrelation(ctx, correlationID)
			if err != nil {
				return err
			}
			if found {
				disposition = "LINKED_CASE"
			} else {
				incidentWindow, err := grafanaIncidentWindow(alert, batch.CommonLabels, service.now().UTC())
				if err != nil {
					return err
				}
				investigationID, err = transaction.CreateInvestigation(ctx, GrafanaAlertInvestigation{
					CaseNo:   "GRAFANA-" + strings.ToUpper(dedupKey[:42]),
					Title:    firstNonEmpty(alert.Annotations["summary"], alert.Labels["alertname"], "Grafana business alert"),
					Severity: severity, CorrelationID: correlationID,
					IncidentFrom: incidentWindow.From, IncidentTo: incidentWindow.To,
					WindowSource: string(incidentWindow.Source),
				})
				if err != nil {
					return err
				}
				disposition = "CREATED_CASE"
			}
			reasonCode = "BUSINESS_ALERT_ELIGIBLE"
		}

		receiptID, err := transaction.SaveReceipt(ctx, GrafanaAlertReceipt{
			DedupKey: dedupKey, OrgID: batch.OrgID, Receiver: batch.Receiver, GroupKey: batch.GroupKey,
			Fingerprint: alert.Fingerprint, AlertStatus: batch.Status, CorrelationID: correlationID,
			Severity: severity, Labels: alert.Labels, Annotations: alert.Annotations,
			GeneratorURL: alert.GeneratorURL, DashboardURL: alert.DashboardURL, PanelURL: alert.PanelURL,
			InvestigationID: investigationID, Disposition: disposition,
		})
		if err != nil {
			return err
		}
		if investigationID != "" {
			if err := transaction.SaveEvidence(ctx, investigationID, receiptID, hashValue("GRAFANA_ALERT:"+dedupKey)); err != nil {
				return err
			}
		}
		result.Disposition = disposition
		result.ReasonCode = reasonCode
		result.InvestigationID = investigationID
		return nil
	})
	return result, err
}

func grafanaIncidentWindow(alert GrafanaAlert, commonLabels map[string]string, now time.Time) (domain.IncidentWindow, error) {
	anchor := alert.StartsAt.UTC()
	if anchor.IsZero() {
		anchor = now.UTC()
	}
	seconds := int64(600)
	if raw := firstNonEmpty(alert.Labels["incident_window_seconds"], commonLabels["incident_window_seconds"]); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > int64(domain.MaximumIncidentWindow/time.Second) {
			return domain.IncidentWindow{}, domain.ErrInvalidIncidentWindow
		}
		seconds = parsed
	}
	return domain.NewIncidentWindow(anchor.Add(-time.Duration(seconds)*time.Second), anchor, domain.IncidentWindowGrafanaAlert)
}

func eligibleGrafanaAlert(alert GrafanaAlert, severity, correlationID string) bool {
	return (severity == "HIGH" || severity == "CRITICAL") && alert.Labels["event_hunter"] == "investigate" && correlationID != ""
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
