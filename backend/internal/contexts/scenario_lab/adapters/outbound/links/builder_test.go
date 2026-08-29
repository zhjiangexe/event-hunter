package links

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildCreatesRunnableExploreLinks(t *testing.T) {
	now := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	traceID := "0123456789abcdef0123456789abcdef"
	result := Builder{EventHunterURL: "http://localhost:28334", GrafanaURL: "http://localhost:28332"}.Build("ORDER-'quoted", &traceID, now)
	eventCheck, err := url.Parse(result.Timeline)
	if err != nil {
		t.Fatal(err)
	}
	if eventCheck.Path != "/event-check" || eventCheck.Query().Get("identifier") != "ORDER-'quoted" {
		t.Fatalf("timeline = %s", result.Timeline)
	}
	for name, raw := range map[string]string{"grafana": result.Grafana, "loki": result.Loki, "tempo": *result.Tempo} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Query().Get("schemaVersion") != "1" || parsed.Query().Get("panes") == "" {
			t.Fatalf("%s link = %s, err %v", name, raw, err)
		}
	}
	if !strings.Contains(result.Grafana, url.QueryEscape("ORDER-''quoted")) {
		t.Fatalf("SQL literal not escaped: %s", result.Grafana)
	}
}
