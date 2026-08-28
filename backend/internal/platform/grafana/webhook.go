package grafana

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/alert_intake"
)

const maxWebhookBody = 1 << 20

type WebhookHandler struct {
	processor AlertProcessor
	secret    []byte
	now       func() time.Time
}

type AlertProcessor interface {
	Process(ctx context.Context, batch alertintake.GrafanaAlertBatch) ([]alertintake.GrafanaAlertResult, error)
}

func NewWebhookHandler(processor AlertProcessor, secret string) *WebhookHandler {
	return &WebhookHandler{processor: processor, secret: []byte(secret), now: time.Now}
}

type webhookPayload struct {
	Receiver        string            `json:"receiver"`
	Status          string            `json:"status"`
	OrgID           int64             `json:"orgId"`
	Alerts          []alert           `json:"alerts"`
	GroupLabels     map[string]string `json:"groupLabels"`
	CommonLabels    map[string]string `json:"commonLabels"`
	CommonAnnots    map[string]string `json:"commonAnnotations"`
	ExternalURL     string            `json:"externalURL"`
	Version         string            `json:"version"`
	GroupKey        string            `json:"groupKey"`
	TruncatedAlerts int               `json:"truncatedAlerts"`
}

type alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
	DashboardURL string            `json:"dashboardURL"`
	PanelURL     string            `json:"panelURL"`
	StartsAt     time.Time         `json:"startsAt"`
}

type responseItem struct {
	Fingerprint   string `json:"fingerprint"`
	Disposition   string `json:"disposition"`
	ReasonCode    string `json:"reason_code"`
	Investigation string `json:"investigation_id,omitempty"`
}

func (handler *WebhookHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxWebhookBody+1))
	if err != nil || len(body) > maxWebhookBody {
		writeError(writer, http.StatusBadRequest, "INVALID_WEBHOOK_BODY")
		return
	}
	if err := handler.verifySignature(request, body); err != nil {
		writeError(writer, http.StatusUnauthorized, err.Error())
		return
	}
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_WEBHOOK_PAYLOAD")
		return
	}
	if payload.Version != "1" || payload.OrgID < 1 || payload.GroupKey == "" || len(payload.Alerts) == 0 {
		writeError(writer, http.StatusBadRequest, "INVALID_WEBHOOK_PAYLOAD")
		return
	}
	if handler.processor == nil {
		writeError(writer, http.StatusServiceUnavailable, "CONTROL_PLANE_UNAVAILABLE")
		return
	}

	results, err := handler.processor.Process(request.Context(), toApplicationBatch(payload))
	if err != nil {
		slog.ErrorContext(request.Context(), "process Grafana alert webhook", "error", err, "org_id", payload.OrgID, "group_key", payload.GroupKey)
		writeError(writer, http.StatusInternalServerError, "WEBHOOK_PROCESSING_FAILED")
		return
	}
	items := make([]responseItem, 0, len(results))
	for _, result := range results {
		items = append(items, responseItem{Fingerprint: result.Fingerprint, Disposition: result.Disposition, ReasonCode: result.ReasonCode, Investigation: result.InvestigationID})
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(writer).Encode(map[string]any{"received_alert_count": len(items), "items": items})
}

func (handler *WebhookHandler) verifySignature(request *http.Request, body []byte) error {
	timestamp := request.Header.Get("X-Grafana-Alerting-Timestamp")
	signature := strings.ToLower(request.Header.Get("X-Grafana-Alerting-Signature"))
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp == "" || signature == "" {
		return errors.New("INVALID_WEBHOOK_SIGNATURE")
	}
	if difference := handler.now().Unix() - seconds; difference > 300 || difference < -300 {
		return errors.New("WEBHOOK_TIMESTAMP_OUT_OF_RANGE")
	}
	mac := hmac.New(sha256.New, handler.secret)
	_, _ = mac.Write([]byte(timestamp + ":"))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	provided, err := hex.DecodeString(signature)
	if err != nil || !hmac.Equal(provided, mustDecodeHex(expected)) {
		return errors.New("INVALID_WEBHOOK_SIGNATURE")
	}
	return nil
}

func toApplicationBatch(payload webhookPayload) alertintake.GrafanaAlertBatch {
	alerts := make([]alertintake.GrafanaAlert, 0, len(payload.Alerts))
	for _, alert := range payload.Alerts {
		alerts = append(alerts, alertintake.GrafanaAlert{
			Labels: alert.Labels, Annotations: alert.Annotations, GeneratorURL: alert.GeneratorURL,
			Fingerprint: alert.Fingerprint, DashboardURL: alert.DashboardURL, PanelURL: alert.PanelURL, StartsAt: alert.StartsAt,
		})
	}
	return alertintake.GrafanaAlertBatch{
		OrgID: payload.OrgID, Receiver: payload.Receiver, GroupKey: payload.GroupKey,
		Status: payload.Status, CommonLabels: payload.CommonLabels, Alerts: alerts,
	}
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"code": code})
}

func mustDecodeHex(value string) []byte { decoded, _ := hex.DecodeString(value); return decoded }
