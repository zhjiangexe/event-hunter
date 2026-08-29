package main

import (
	"encoding/json"
	"errors"
	"net/http"

	cases "event-hunter/backend/internal/contexts/investigation/application/cases"
	"event-hunter/backend/internal/contexts/investigation/domain"
)

func (api investigationAPI) summary(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canRead(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	includePayload := request.URL.Query().Get("include_payload") == "true"
	if includePayload && !canReadSensitivePayload(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	from, to, err := optionalQueryWindow(request)
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	limit := queryLimit(request)
	result, err := api.summaries.Get(request.Context(), cases.SummaryRequest{
		InvestigationID: request.PathValue("id"), From: from, To: to,
		Limit: limit, IncludePayload: includePayload,
	})
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if errors.Is(err, cases.ErrInvalidWindow) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "SUMMARY_UNAVAILABLE")
		return
	}
	caseData := investigationResponseFromCase(result.Details.Case)
	timeline := unavailableTimeline(caseData.CorrelationID, result.From, result.To)
	if !result.Partial {
		events, mappingErr := timelineEventsFromForensics(result.Events, includePayload)
		if mappingErr != nil {
			result.Partial = true
			result.ClickHouseStatus = "UNAVAILABLE"
			result.ClickHouseLastSuccessAt = nil
			result.Warnings = []string{"CLICKHOUSE_UNAVAILABLE"}
		} else {
			for index := range events {
				if summary, exists := result.ProcessingSummaries[events[index].EventID]; exists {
					events[index].ProcessingSummary = map[string]any{"attempt_count": summary.AttemptCount, "final_status": summary.FinalStatus, "consumer_groups": summary.ConsumerGroups, "retry_reasons": []string{}, "last_attempt_at": summary.LastAttemptAt}
				}
			}
			timeline = map[string]any{"correlation_id": caseData.CorrelationID, "from": result.From, "to": result.To, "event_count": len(events), "truncated": len(events) == limit, "events": events}
		}
	}
	findings := patternFindingResponses(result.Details.Findings)
	evidence := evidenceResponses(result.Details.Evidence)
	auditEntries := auditEntryResponses(result.Details.Audit)
	caseData.PatternFindings = findings
	caseData.Evidence = evidence
	caseData.CollaborationNotes = caseNoteResponses(result.Details.Notes)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"investigation_id": caseData.ID, "generated_at": result.GeneratedAt,
		"query_window": map[string]any{"from": result.From, "to": result.To}, "partial": result.Partial, "warnings": result.Warnings,
		"event_retention_boundary": result.EventRetentionBoundary,
		"source_status":            map[string]string{"postgres": result.PostgresStatus, "clickhouse": result.ClickHouseStatus, "technical_observability": "NOT_REQUESTED"},
		"source_last_success_at":   map[string]any{"postgres": result.PostgresLastSuccessAt, "clickhouse": result.ClickHouseLastSuccessAt, "technical_observability": nil},
		"case":                     caseData, "timeline": timeline, "quality": map[string]any{}, "pattern_findings": findings,
		"technical_observations": map[string]any{}, "evidence_references": evidence, "audit_entries": auditEntries,
	})
}

func (api investigationAPI) evidenceBundle(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	from, to, err := optionalQueryWindow(request)
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	result, err := api.manifests.Generate(request.Context(), cases.EvidenceManifestRequest{InvestigationID: request.PathValue("id"), From: from, To: to})
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if errors.Is(err, cases.ErrInvalidWindow) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "EVIDENCE_UNAVAILABLE")
		return
	}
	_ = json.NewEncoder(writer).Encode(result.Manifest())
}
