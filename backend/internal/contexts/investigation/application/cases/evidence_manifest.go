package cases

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"event-hunter/backend/internal/contexts/investigation/domain"
	"event-hunter/backend/internal/contexts/investigation/ports"
)

type EvidenceManifestReader interface {
	Get(ctx context.Context, id string) (domain.InvestigationCase, error)
	Evidence(ctx context.Context, id string) ([]ports.Evidence, error)
}

type EvidenceManifestRequest struct {
	InvestigationID string
	From            *time.Time
	To              *time.Time
}

type EvidenceManifestItem struct {
	ID            string
	EvidenceType  string
	Reference     string
	Checksum      *string
	CollectedAt   time.Time
	Source        string
	OpenAction    string
	SourceLocator *string
	SourceOrgID   *int64
}

type EvidenceManifestResult struct {
	InvestigationID string
	GeneratedAt     time.Time
	From            time.Time
	To              time.Time
	Items           []EvidenceManifestItem
	Partial         bool
	Warnings        []string
	ManifestSHA256  string
}

type EvidenceManifestService struct {
	reader EvidenceManifestReader
	now    func() time.Time
}

func NewEvidenceManifestService(reader EvidenceManifestReader) *EvidenceManifestService {
	return &EvidenceManifestService{reader: reader, now: time.Now}
}

func (service *EvidenceManifestService) Generate(ctx context.Context, request EvidenceManifestRequest) (EvidenceManifestResult, error) {
	loadedCase, err := service.reader.Get(ctx, request.InvestigationID)
	if err != nil {
		return EvidenceManifestResult{}, err
	}
	evidence, err := service.reader.Evidence(ctx, loadedCase.ID)
	if err != nil {
		return EvidenceManifestResult{}, err
	}
	items := ItemsFromEvidence(evidence)
	from, to, err := ResolveWindow(request.From, request.To, loadedCase.IncidentWindow)
	if err != nil {
		return EvidenceManifestResult{}, err
	}
	warnings := make([]string, 0)
	for _, item := range items {
		if item.Checksum == nil {
			warnings = append(warnings, fmt.Sprintf("EVIDENCE_CHECKSUM_MISSING:%s", item.ID))
		}
	}
	result := EvidenceManifestResult{
		InvestigationID: loadedCase.ID, GeneratedAt: service.now().UTC(),
		From: from, To: to, Items: items,
		Partial: len(warnings) > 0, Warnings: warnings,
	}
	canonical, err := json.Marshal(result.manifestMap(false))
	if err != nil {
		return EvidenceManifestResult{}, fmt.Errorf("encode evidence manifest: %w", err)
	}
	digest := sha256.Sum256(canonical)
	result.ManifestSHA256 = fmt.Sprintf("%x", digest[:])
	return result, nil
}

func (result EvidenceManifestResult) Manifest() map[string]any { return result.manifestMap(true) }

func (result EvidenceManifestResult) manifestMap(includeDigest bool) map[string]any {
	items := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, item.manifestMap())
	}
	manifest := map[string]any{
		"schema_version": 1, "investigation_id": result.InvestigationID, "generated_at": result.GeneratedAt,
		"query_window": map[string]any{"from": result.From, "to": result.To}, "items": items,
		"checksum_algorithm": "SHA-256", "partial": result.Partial, "warnings": result.Warnings,
		"source_status": map[string]string{"postgres": "OK", "clickhouse": "NOT_REQUESTED", "technical_observability": "NOT_REQUESTED"},
	}
	if includeDigest {
		manifest["manifest_sha256"] = result.ManifestSHA256
	}
	return manifest
}

func (item EvidenceManifestItem) Response() map[string]any { return item.manifestMap() }

func (item EvidenceManifestItem) manifestMap() map[string]any {
	result := map[string]any{
		"id": item.ID, "evidence_type": item.EvidenceType, "reference": item.Reference,
		"collected_at": item.CollectedAt, "source": item.Source, "open_action": item.OpenAction,
	}
	if item.Checksum == nil {
		result["checksum"] = nil
	} else {
		result["checksum"] = *item.Checksum
	}
	if item.SourceLocator != nil {
		result["source_locator"] = *item.SourceLocator
	}
	if item.SourceOrgID != nil {
		result["source_org_id"] = *item.SourceOrgID
	}
	return result
}

func ItemsFromEvidence(evidence []ports.Evidence) []EvidenceManifestItem {
	items := make([]EvidenceManifestItem, 0, len(evidence))
	for _, source := range evidence {
		items = append(items, itemFromEvidence(source))
	}
	return items
}

func itemFromEvidence(evidence ports.Evidence) EvidenceManifestItem {
	source, openAction := evidenceSource(evidence.EvidenceType)
	item := EvidenceManifestItem{
		ID: evidence.ID, EvidenceType: evidence.EvidenceType, Reference: evidence.Reference,
		Checksum: evidence.Checksum, CollectedAt: evidence.CollectedAt, Source: source, OpenAction: openAction,
	}
	if evidence.EvidenceType != "GRAFANA_ALERT" {
		return item
	}
	generatorURL := ""
	if evidence.GeneratorURL != nil {
		generatorURL = *evidence.GeneratorURL
	}
	if sourcePath, ok := grafanaAlertSourcePath(generatorURL); ok && evidence.GrafanaOrgID != nil && *evidence.GrafanaOrgID > 0 {
		item.SourceLocator = &sourcePath
		item.SourceOrgID = evidence.GrafanaOrgID
	} else {
		item.OpenAction = "NONE"
	}
	return item
}

func evidenceSource(evidenceType string) (string, string) {
	switch evidenceType {
	case "EVENT":
		return "CLICKHOUSE", "GRAFANA_EVENT"
	case "TRACE":
		return "TEMPO", "GRAFANA_TEMPO"
	case "LOG":
		return "LOKI", "GRAFANA_LOKI"
	case "METRIC", "QUALITY_VIOLATION":
		return "GRAFANA", "GRAFANA_DASHBOARD"
	case "GRAFANA_ALERT":
		return "GRAFANA", "GRAFANA_ALERT"
	case "PATTERN_FINDING":
		return "PATTERN_ENGINE", "PATTERN_LIBRARY"
	case "REPORT":
		return "REPORT_STORE", "NONE"
	default:
		return "UNKNOWN", "NONE"
	}
}

func grafanaAlertSourcePath(value string) (string, bool) {
	parsed, err := url.Parse(strings.ReplaceAll(value, `\/`, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	valid := len(parts) == 3 && parts[0] == "alerting" && validGrafanaPathToken(parts[1]) && (parts[2] == "view" || parts[2] == "edit")
	if len(parts) == 4 {
		valid = parts[0] == "alerting" && parts[1] == "grafana" && validGrafanaPathToken(parts[2]) && (parts[3] == "view" || parts[3] == "edit")
	}
	if !valid {
		return "", false
	}
	return "/" + strings.Join(parts, "/"), true
}

func validGrafanaPathToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}
