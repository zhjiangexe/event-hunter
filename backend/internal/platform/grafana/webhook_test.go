package grafana

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	alertintake "event-hunter/backend/internal/contexts/investigation/application/alerts"
)

func TestWebhookHandlerVerifiesRawBodyAndDelegatesToApplication(t *testing.T) {
	const secret = "test-secret"
	const timestamp = "1787292000"
	body := `{"receiver":"event-hunter","status":"firing","orgId":1,"alerts":[{"status":"firing","labels":{"severity":"HIGH","event_hunter":"investigate"},"annotations":{},"generatorURL":"/alerting/grafana/event-quality-delay/view","fingerprint":"fingerprint-1"}],"groupLabels":{},"commonLabels":{"correlation_id":"ORDER-2001"},"commonAnnotations":{},"externalURL":"http://grafana:3000","version":"1","groupKey":"group-1","truncatedAlerts":0}`
	processor := &alertProcessorFake{results: []alertintake.GrafanaAlertResult{{Fingerprint: "fingerprint-1", Disposition: "CREATED_CASE", ReasonCode: "BUSINESS_ALERT_ELIGIBLE", InvestigationID: "case-1"}}}
	handler := NewWebhookHandler(processor, secret)
	handler.now = func() time.Time { return time.Unix(1787292000, 0) }
	request := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/grafana/alerts", strings.NewReader(body))
	request.Header.Set("X-Grafana-Alerting-Timestamp", timestamp)
	request.Header.Set("X-Grafana-Alerting-Signature", webhookSignature(secret, timestamp, body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"investigation_id":"case-1"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if processor.calls != 1 || processor.batch.OrgID != 1 || processor.batch.Alerts[0].Fingerprint != "fingerprint-1" {
		t.Fatalf("processor batch = %#v", processor.batch)
	}
}

func TestWebhookHandlerRejectsInvalidSignatureBeforeApplication(t *testing.T) {
	processor := &alertProcessorFake{}
	handler := NewWebhookHandler(processor, "test-secret")
	handler.now = func() time.Time { return time.Unix(1787292000, 0) }
	request := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/grafana/alerts", strings.NewReader(`{"version":"1"}`))
	request.Header.Set("X-Grafana-Alerting-Timestamp", "1787292000")
	request.Header.Set("X-Grafana-Alerting-Signature", "invalid")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || processor.calls != 0 {
		t.Fatalf("status = %d, processor calls = %d", response.Code, processor.calls)
	}
}

type alertProcessorFake struct {
	batch   alertintake.GrafanaAlertBatch
	results []alertintake.GrafanaAlertResult
	calls   int
}

func (processor *alertProcessorFake) Process(_ context.Context, batch alertintake.GrafanaAlertBatch) ([]alertintake.GrafanaAlertResult, error) {
	processor.calls++
	processor.batch = batch
	return processor.results, nil
}

func webhookSignature(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + ":" + body))
	return hex.EncodeToString(mac.Sum(nil))
}
