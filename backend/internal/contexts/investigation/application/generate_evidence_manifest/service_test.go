package generateevidencemanifest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

func TestGenerateBuildsVerifiablePartialManifest(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	service := NewService(manifestReaderFake{
		investigation: manifestCase("case-1", now),
		evidence:      []ports.Evidence{{ID: "evidence-1", EvidenceType: "EVENT", Reference: "event-1", CollectedAt: now}},
	})
	service.now = func() time.Time { return now }
	from := now.Add(-time.Hour)
	result, err := service.Generate(t.Context(), Request{InvestigationID: "case-1", From: &from, To: &now})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !result.Partial || len(result.Warnings) != 1 || result.Warnings[0] != "EVIDENCE_CHECKSUM_MISSING:evidence-1" {
		t.Fatalf("result = %#v", result)
	}
	manifest := result.Manifest()
	delete(manifest, "manifest_sha256")
	canonical, _ := json.Marshal(manifest)
	want := fmt.Sprintf("%x", sha256.Sum256(canonical))
	if result.ManifestSHA256 != want {
		t.Fatalf("manifest digest = %q, want %q", result.ManifestSHA256, want)
	}
}

func TestGenerateRestrictsGrafanaAlertOpenAction(t *testing.T) {
	validURL := "https://grafana.example/alerting/grafana/rule_uid/view"
	invalidURL := "https://attacker.example/login"
	orgID := int64(1)
	service := NewService(manifestReaderFake{
		investigation: manifestCase("case-1", time.Now().UTC()),
		evidence: []ports.Evidence{
			{ID: "valid", EvidenceType: "GRAFANA_ALERT", GeneratorURL: &validURL, GrafanaOrgID: &orgID},
			{ID: "invalid", EvidenceType: "GRAFANA_ALERT", GeneratorURL: &invalidURL, GrafanaOrgID: &orgID},
		},
	})
	result, err := service.Generate(t.Context(), Request{InvestigationID: "case-1"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Items[0].OpenAction != "GRAFANA_ALERT" || result.Items[0].SourceLocator == nil {
		t.Fatalf("valid item = %#v", result.Items[0])
	}
	if result.Items[1].OpenAction != "NONE" || result.Items[1].SourceLocator != nil {
		t.Fatalf("invalid item = %#v", result.Items[1])
	}
}

type manifestReaderFake struct {
	investigation domain.InvestigationCase
	evidence      []ports.Evidence
}

func (fake manifestReaderFake) Get(context.Context, string) (domain.InvestigationCase, error) {
	return fake.investigation, nil
}

func (fake manifestReaderFake) Evidence(context.Context, string) ([]ports.Evidence, error) {
	return fake.evidence, nil
}

func manifestCase(id string, now time.Time) domain.InvestigationCase {
	return domain.InvestigationCase{ID: id, IncidentWindow: domain.IncidentWindow{From: now.Add(-time.Hour), To: now, Source: domain.IncidentWindowTimelineSearch}}
}
