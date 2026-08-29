package links

import (
	"encoding/json"
	"event-hunter/backend/internal/contexts/scenario_lab/domain"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Builder struct {
	EventHunterURL string
	GrafanaURL     string
}

func (builder Builder) Build(correlationID string, traceID *string, now time.Time) domain.Links {
	ui := strings.TrimRight(builder.EventHunterURL, "/")
	grafana := strings.TrimRight(builder.GrafanaURL, "/")
	query := url.Values{"identifier_type": {"CORRELATION_ID"}, "identifier": {correlationID}, "from": {now.Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano)}, "to": {now.Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)}, "tab": {"timeline"}}
	rangeValue := map[string]string{"from": strconv.FormatInt(now.Add(-15*time.Minute).UnixMilli(), 10), "to": strconv.FormatInt(now.Add(5*time.Minute).UnixMilli(), 10)}
	result := domain.Links{Timeline: ui + "/event-check?" + query.Encode(), Grafana: explore(grafana, "clickhouse", "grafana-clickhouse-datasource", map[string]any{"rawSql": "SELECT * FROM canonical_forensics_events WHERE correlation_id = " + sqlLiteral(correlationID) + " ORDER BY occurred_at", "format": 1}, rangeValue), Loki: explore(grafana, "loki", "loki", map[string]any{"expr": `{service_name=~".+"} | correlation_id=` + strconv.Quote(correlationID), "queryType": "range"}, rangeValue)}
	if traceID != nil {
		value := explore(grafana, "tempo", "tempo", map[string]any{"query": *traceID, "queryType": "traceql"}, rangeValue)
		result.Tempo = &value
	}
	return result
}
func explore(baseURL, datasource, datasourceType string, query map[string]any, rangeValue map[string]string) string {
	query["refId"] = "A"
	query["datasource"] = map[string]string{"uid": datasource, "type": datasourceType}
	panes, _ := json.Marshal(map[string]any{"event-hunter": map[string]any{"datasource": datasource, "queries": []any{query}, "range": rangeValue}})
	values := url.Values{"panes": {string(panes)}, "schemaVersion": {"1"}, "orgId": {"1"}}
	return baseURL + "/explore?" + values.Encode()
}
func sqlLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
