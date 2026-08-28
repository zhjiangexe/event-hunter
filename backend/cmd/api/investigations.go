package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	caselifecycle "event-hunter/backend/internal/contexts/investigation/application/case_lifecycle"
	evidenceattachment "event-hunter/backend/internal/contexts/investigation/application/evidence_attachment"
	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	patternanalysis "event-hunter/backend/internal/contexts/investigation/application/pattern_analysis"
	patternfeedback "event-hunter/backend/internal/contexts/investigation/application/pattern_feedback"
	"event-hunter/backend/internal/contexts/investigation/domain"
	domainpatterns "event-hunter/backend/internal/contexts/investigation/domain/patterns"
)

const defaultInvestigationQueryWindow = 72 * time.Hour

type investigationAPI struct {
	commands    caselifecycle.Commands
	queries     caselifecycle.Queries
	patterns    *patternanalysis.PatternService
	feedback    *patternfeedback.Service
	attachments *evidenceattachment.Service
	forensics   *forensics.ForensicsService
	sessions    sessionManager
}

func (api investigationAPI) updateFindingFeedback(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil || match < 0 {
		writeAPIError(writer, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_PATTERN_FEEDBACK")
		return
	}
	updated, err := api.feedback.Reclassify(request.Context(), patternfeedback.Command{
		InvestigationID: request.PathValue("id"), FindingID: request.PathValue("findingId"), ExpectedVersion: match,
		Status: domain.PatternFeedbackStatus(input.Status), Actor: actorFromPrincipal(principal), RequestID: request.Header.Get("X-Request-ID"),
	})
	if errors.Is(err, domain.ErrPatternFindingNotFound) {
		writeAPIError(writer, http.StatusNotFound, "PATTERN_FINDING_NOT_FOUND")
		return
	}
	if errors.Is(err, domain.ErrPatternFeedbackConflict) {
		writeAPIError(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT")
		return
	}
	if errors.Is(err, domain.ErrInvalidPatternFeedback) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_PATTERN_FEEDBACK")
		return
	}
	if err != nil {
		slog.ErrorContext(request.Context(), "classify pattern finding", "error", err, "investigation_id", request.PathValue("id"), "finding_id", request.PathValue("findingId"))
		writeAPIError(writer, http.StatusServiceUnavailable, "PATTERN_FEEDBACK_UNAVAILABLE")
		return
	}
	writer.Header().Set("ETag", etag(updated.LockVersion))
	_ = json.NewEncoder(writer).Encode(patternFeedbackResponse(updated))
}

func (api investigationAPI) list(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	pageSize := 100
	if value := request.URL.Query().Get("page_size"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 200 {
			writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_PAGE_SIZE")
			return
		}
		pageSize = parsed
	}
	sortBy := strings.TrimSpace(request.URL.Query().Get("sort_by"))
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortBy != "created_at" && sortBy != "updated_at" {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SORT")
		return
	}
	sortOrder := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("sort_order")))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SORT")
		return
	}
	filter := caselifecycle.CaseFilter{
		Query:  strings.TrimSpace(request.URL.Query().Get("query")),
		Status: strings.TrimSpace(request.URL.Query().Get("status")), Severity: strings.TrimSpace(request.URL.Query().Get("severity")),
		Assignee: strings.TrimSpace(request.URL.Query().Get("assignee")), Priority: strings.TrimSpace(request.URL.Query().Get("priority")),
		Tag: strings.TrimSpace(request.URL.Query().Get("tag")), CorrelationID: strings.TrimSpace(request.URL.Query().Get("correlation_id")), SortBy: sortBy, SortOrder: sortOrder, PageSize: pageSize,
	}
	if len([]rune(filter.Query)) > 100 {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_QUERY")
		return
	}
	if cursor := strings.TrimSpace(request.URL.Query().Get("cursor")); cursor != "" {
		cursorValue, err := decodeInvestigationCursor(cursor)
		if err != nil || cursorValue.SortBy != sortBy || cursorValue.SortOrder != sortOrder {
			writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CURSOR")
			return
		}
		filter.BeforeTime = &cursorValue.Time
		filter.BeforeID = cursorValue.ID
	}
	page, err := api.queries.List(request.Context(), filter)
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "INVESTIGATION_LIST_UNAVAILABLE")
		return
	}
	items := make([]investigationResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, investigationResponseFromCase(item))
	}
	var nextCursor *string
	if page.HasMore && len(items) > 0 {
		last := items[len(items)-1]
		sortTime := last.CreatedAt
		if sortBy == "updated_at" {
			sortTime = last.UpdatedAt
		}
		encoded := encodeInvestigationCursor(investigationCursor{SortBy: sortBy, SortOrder: sortOrder, Time: sortTime, ID: last.ID})
		nextCursor = &encoded
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{"items": items, "next_cursor": nextCursor})
}

type investigationCursor struct {
	SortBy    string
	SortOrder string
	Time      time.Time
	ID        string
}

func encodeInvestigationCursor(value investigationCursor) string {
	raw := strings.Join([]string{"v2", value.SortBy, value.SortOrder, value.Time.UTC().Format(time.RFC3339Nano), value.ID}, "|")
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeInvestigationCursor(value string) (investigationCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return investigationCursor{}, err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 || parts[0] != "v2" || parts[4] == "" ||
		(parts[1] != "created_at" && parts[1] != "updated_at") ||
		(parts[2] != "asc" && parts[2] != "desc") {
		return investigationCursor{}, fmt.Errorf("invalid investigation cursor")
	}
	cursorTime, err := time.Parse(time.RFC3339Nano, parts[3])
	if err != nil {
		return investigationCursor{}, err
	}
	return investigationCursor{SortBy: parts[1], SortOrder: parts[2], Time: cursorTime, ID: parts[4]}, nil
}

func (api investigationAPI) create(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, 401, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, 403, "FORBIDDEN")
		return
	}
	var input struct {
		Title         string `json:"title"`
		Severity      string `json:"severity"`
		CorrelationID string `json:"correlation_id"`
		IncidentFrom  string `json:"incident_from"`
		IncidentTo    string `json:"incident_to"`
	}
	if json.NewDecoder(request.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.CorrelationID) == "" || !validSeverity(input.Severity) {
		writeAPIError(writer, 422, "INVALID_INVESTIGATION")
		return
	}
	incidentWindow, err := incidentWindowForManualCase(input.IncidentFrom, input.IncidentTo, time.Now().UTC())
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_INCIDENT_WINDOW")
		return
	}
	created, err := api.commands.Create(request.Context(), input.Title, domain.Severity(input.Severity), input.CorrelationID, incidentWindow, actorFromPrincipal(principal), request.Header.Get("X-Request-ID"))
	if err != nil {
		slog.ErrorContext(request.Context(), "create investigation", "error", err, "correlation_id", input.CorrelationID)
		writeAPIError(writer, 500, "INVESTIGATION_CREATE_FAILED")
		return
	}
	result := investigationResponseFromCase(created)
	writer.Header().Set("ETag", etag(result.LockVersion))
	writer.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(writer).Encode(result)
}

func (api investigationAPI) get(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, 401, "UNAUTHENTICATED")
		return
	}
	details, err := api.queries.GetDetails(request.Context(), request.PathValue("id"))
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, 404, "NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, 500, "INVESTIGATION_READ_FAILED")
		return
	}
	result := investigationResponseFromCase(details.Case)
	result.PatternFindings = patternFindingResponses(details.Findings)
	result.Evidence = evidenceResponses(details.Evidence)
	result.CollaborationNotes = caseNoteResponses(details.Notes)
	writer.Header().Set("ETag", etag(result.LockVersion))
	_ = json.NewEncoder(writer).Encode(result)
}

func (api investigationAPI) patch(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, 401, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, 403, "FORBIDDEN")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil {
		writeAPIError(writer, 428, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		Title                 *string   `json:"title"`
		Status                *string   `json:"status"`
		Severity              *string   `json:"severity"`
		Assignee              *string   `json:"assignee"`
		Priority              *string   `json:"priority"`
		Tags                  *[]string `json:"tags"`
		RelatedCorrelationIDs *[]string `json:"related_correlation_ids"`
		RootCause             *string   `json:"root_cause"`
		ResolutionSummary     *string   `json:"resolution_summary"`
		FixedVersion          *string   `json:"fixed_version"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeAPIError(writer, 422, "INVALID_UPDATE")
		return
	}
	patch := caselifecycle.CasePatch{Title: input.Title, Assignee: input.Assignee, Tags: input.Tags, RelatedCorrelationIDs: input.RelatedCorrelationIDs, RootCause: input.RootCause, ResolutionSummary: input.ResolutionSummary, FixedVersion: input.FixedVersion}
	if input.Status != nil {
		status := domain.CaseStatus(*input.Status)
		patch.Status = &status
	}
	if input.Severity != nil {
		severity := domain.Severity(*input.Severity)
		patch.Severity = &severity
	}
	if input.Priority != nil {
		priority := domain.CasePriority(*input.Priority)
		patch.Priority = &priority
	}
	updated, err := api.commands.Update(request.Context(), request.PathValue("id"), match, patch, actorFromPrincipal(principal), request.Header.Get("X-Request-ID"))
	var conflict caselifecycle.VersionConflictError
	if errors.As(err, &conflict) {
		writeAPIErrorWithData(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT", map[string]any{"current_lock_version": conflict.CurrentVersion})
		return
	}
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if errors.Is(err, caselifecycle.ErrInvalidTransition) {
		writeAPIError(writer, http.StatusConflict, "INVALID_STATE_TRANSITION")
		return
	}
	if errors.Is(err, caselifecycle.ErrCloseRequired) {
		writeAPIError(writer, http.StatusConflict, "CLOSE_OPERATION_REQUIRED")
		return
	}
	if errors.Is(err, caselifecycle.ErrResolutionFields) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "RESOLUTION_FIELDS_REQUIRED")
		return
	}
	if errors.Is(err, domain.ErrInvalidOwner) || errors.Is(err, domain.ErrInvalidPriority) || errors.Is(err, domain.ErrInvalidTags) || errors.Is(err, domain.ErrInvalidRelatedIDs) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_COLLABORATION_METADATA")
		return
	}
	if err != nil {
		writeAPIError(writer, 500, "INVESTIGATION_UPDATE_FAILED")
		return
	}
	result := investigationResponseFromCase(updated)
	writer.Header().Set("ETag", etag(result.LockVersion))
	_ = json.NewEncoder(writer).Encode(result)
}

func (api investigationAPI) addNote(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil {
		writeAPIError(writer, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		Body string `json:"body"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CASE_NOTE")
		return
	}
	updated, note, err := api.commands.AddNote(request.Context(), request.PathValue("id"), match, input.Body, actorFromPrincipal(principal), request.Header.Get("X-Request-ID"))
	var conflict caselifecycle.VersionConflictError
	if errors.As(err, &conflict) {
		writeAPIErrorWithData(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT", map[string]any{"current_lock_version": conflict.CurrentVersion})
		return
	}
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if errors.Is(err, domain.ErrInvalidCaseNote) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CASE_NOTE")
		return
	}
	if errors.Is(err, domain.ErrInvalidTransition) {
		writeAPIError(writer, http.StatusConflict, "CASE_NOT_MUTABLE")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "CASE_NOTE_APPEND_FAILED")
		return
	}
	result := investigationResponseFromCase(updated)
	writer.Header().Set("ETag", etag(result.LockVersion))
	writer.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(writer).Encode(map[string]any{"investigation": result, "note": caseNoteResponseFromDomain(note)})
}

func (api investigationAPI) attachEvent(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil {
		writeAPIError(writer, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		EventID string `json:"event_id"`
		From    string `json:"from"`
		To      string `json:"to"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_ATTACHMENT")
		return
	}
	from, fromErr := time.Parse(time.RFC3339, input.From)
	to, toErr := time.Parse(time.RFC3339, input.To)
	if fromErr != nil || toErr != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_ATTACHMENT")
		return
	}
	result, err := api.attachments.AttachEvent(request.Context(), evidenceattachment.AttachEventCommand{
		InvestigationID: request.PathValue("id"), ExpectedVersion: match,
		EventID: input.EventID, From: from, To: to,
		Actor: actorFromPrincipal(principal), RequestID: request.Header.Get("X-Request-ID"),
	})
	var conflict evidenceattachment.VersionConflictError
	if errors.As(err, &conflict) {
		writeAPIErrorWithData(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT", map[string]any{"current_lock_version": conflict.CurrentVersion})
		return
	}
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if errors.Is(err, evidenceattachment.ErrEventNotFound) {
		writeAPIError(writer, http.StatusNotFound, "EVENT_NOT_FOUND")
		return
	}
	if errors.Is(err, evidenceattachment.ErrInvalidAttachment) || errors.Is(err, domain.ErrInvalidEvidence) || errors.Is(err, domain.ErrInvalidRelatedIDs) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_ATTACHMENT")
		return
	}
	if errors.Is(err, domain.ErrInvalidTransition) {
		writeAPIError(writer, http.StatusConflict, "CASE_NOT_MUTABLE")
		return
	}
	var sourceErr evidenceattachment.SourceError
	if errors.As(err, &sourceErr) {
		writeClickHouseError(writer, sourceErr.Err, "EVENT_SOURCE_UNAVAILABLE", "EVENT_SOURCE_TIMEOUT")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "EVENT_ATTACHMENT_FAILED")
		return
	}
	investigation := investigationResponseFromCase(result.Investigation)
	evidence := map[string]any{
		"id": result.Evidence.ID, "evidence_type": result.Evidence.EvidenceType,
		"reference": result.Evidence.Reference, "checksum": result.Evidence.Checksum,
		"collected_at": result.Evidence.CollectedAt, "source": "CLICKHOUSE", "open_action": "GRAFANA_EVENT",
	}
	writer.Header().Set("ETag", etag(investigation.LockVersion))
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"investigation": investigation, "evidence": evidence, "attached": result.Attached})
}

func (api investigationAPI) close(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil {
		writeAPIError(writer, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		RootCause         string  `json:"root_cause"`
		ResolutionSummary string  `json:"resolution_summary"`
		FixedVersion      *string `json:"fixed_version"`
	}
	if json.NewDecoder(request.Body).Decode(&input) != nil || strings.TrimSpace(input.RootCause) == "" || strings.TrimSpace(input.ResolutionSummary) == "" {
		writeAPIError(writer, http.StatusUnprocessableEntity, "RESOLUTION_FIELDS_REQUIRED")
		return
	}
	closed, err := api.commands.Close(request.Context(), request.PathValue("id"), match, input.RootCause, input.ResolutionSummary, input.FixedVersion, actorFromPrincipal(principal), request.Header.Get("X-Request-ID"))
	var conflict caselifecycle.VersionConflictError
	if errors.As(err, &conflict) {
		writeAPIErrorWithData(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT", map[string]any{"current_lock_version": conflict.CurrentVersion})
		return
	}
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "INVESTIGATION_CLOSE_FAILED")
		return
	}
	result := investigationResponseFromCase(closed)
	writer.Header().Set("ETag", etag(result.LockVersion))
	_ = json.NewEncoder(writer).Encode(result)
}

func validTransition(from, to string) bool {
	return domain.CanTransition(domain.CaseStatus(from), domain.CaseStatus(to))
}

func patternsHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(domainpatterns.Registry())
}

func (api investigationAPI) analyze(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, 401, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, 403, "FORBIDDEN")
		return
	}
	var input struct {
		PatternIDs    []string `json:"pattern_ids"`
		ExecutionMode string   `json:"execution_mode"`
	}
	_ = json.NewDecoder(request.Body).Decode(&input)
	if input.ExecutionMode == "TEMPORAL" {
		writeAPIError(writer, 409, "TEMPORAL_DISABLED")
		return
	}
	result, err := api.patterns.Analyze(request.Context(), request.PathValue("id"), input.PatternIDs, actorFromPrincipal(principal), request.Header.Get("X-Request-ID"))
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, 404, "NOT_FOUND")
		return
	}
	if errors.Is(err, patternanalysis.ErrUnknownPattern) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "UNKNOWN_PATTERN")
		return
	}
	if errors.Is(err, domain.ErrInvalidTransition) {
		writeAPIError(writer, http.StatusConflict, "INVALID_TRANSITION")
		return
	}
	var windowErr patternanalysis.AnalysisWindowError
	if errors.As(err, &windowErr) {
		writeAPIErrorWithData(writer, http.StatusUnprocessableEntity, "ANALYSIS_WINDOW_EXCEEDS_LIMIT", map[string]any{
			"event_window_from": windowErr.FirstOccurredAt,
			"event_window_to":   windowErr.LastOccurredAt,
			"max_window":        "P7D",
		})
		return
	}
	var sourceErr patternanalysis.PatternSourceError
	if errors.As(err, &sourceErr) {
		writeClickHouseError(writer, err, "PATTERN_SOURCE_UNAVAILABLE", "PATTERN_SOURCE_TIMEOUT")
		return
	}
	var persistenceErr patternanalysis.PatternPersistenceError
	if errors.As(err, &persistenceErr) {
		writeAPIError(writer, http.StatusServiceUnavailable, "PATTERN_PERSISTENCE_UNAVAILABLE")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "PATTERN_SOURCE_UNAVAILABLE")
		return
	}
	responseFindings := make([]map[string]any, 0, len(result.Findings))
	for _, finding := range result.Findings {
		responseFindings = append(responseFindings, patternFindingResponse(finding))
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"investigation_id": request.PathValue("id"), "execution_mode": "SYNC",
		"analyzed_at": result.AnalyzedAt, "analysis_status": result.AnalysisStatus,
		"executed_pattern_ids": result.ExecutedPatternIDs,
		"effective_window":     result.EffectiveWindow, "findings": responseFindings,
	})
}

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
	details, err := api.queries.GetSummaryDetails(request.Context(), request.PathValue("id"))
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "SUMMARY_UNAVAILABLE")
		return
	}
	caseData := investigationResponseFromCase(details.Case)
	from, to, err := queryWindow(request, details.Case.IncidentWindow)
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	timeline, err := loadTimeline(request.Context(), api.forensics, caseData.CorrelationID, from, to, queryLimit(request), includePayload)
	generatedAt := time.Now().UTC()
	partial := false
	warnings := []string{}
	clickhouseStatus := "OK"
	var clickhouseLastSuccessAt *time.Time
	if err != nil {
		partial = true
		clickhouseStatus, warnings = clickHouseSummaryFailure(err)
		timeline = unavailableTimeline(caseData.CorrelationID, from, to)
	} else {
		clickhouseLastSuccessAt = &generatedAt
	}
	findings := patternFindingResponses(details.Findings)
	evidence := evidenceResponses(details.Evidence)
	auditEntries := auditEntryResponses(details.Audit)
	caseData.PatternFindings = findings
	caseData.Evidence = evidence
	caseData.CollaborationNotes = caseNoteResponses(details.Notes)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"investigation_id": caseData.ID, "generated_at": generatedAt,
		"query_window": map[string]any{"from": from, "to": to}, "partial": partial, "warnings": warnings,
		"event_retention_boundary": generatedAt.Add(-90 * 24 * time.Hour),
		"source_status":            map[string]string{"postgres": "OK", "clickhouse": clickhouseStatus, "technical_observability": "NOT_REQUESTED"},
		"source_last_success_at":   map[string]any{"postgres": generatedAt, "clickhouse": clickhouseLastSuccessAt, "technical_observability": nil},
		"case":                     caseData, "timeline": timeline, "quality": map[string]any{}, "pattern_findings": findings,
		"technical_observations": map[string]any{}, "evidence_references": evidence, "audit_entries": auditEntries,
	})
}

func (api investigationAPI) evidenceBundle(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	loadedCase, err := api.queries.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, domain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "EVIDENCE_UNAVAILABLE")
		return
	}
	caseData := investigationResponseFromCase(loadedCase)
	from, to, err := queryWindow(request, loadedCase.IncidentWindow)
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	items, err := api.loadEvidence(request.Context(), caseData.ID)
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "EVIDENCE_UNAVAILABLE")
		return
	}
	partial, warnings := evidenceManifestState(items)
	manifest := map[string]any{
		"schema_version": 1, "investigation_id": caseData.ID, "generated_at": time.Now().UTC(),
		"query_window": map[string]any{"from": from, "to": to}, "items": items,
		"checksum_algorithm": "SHA-256", "partial": partial, "warnings": warnings,
		"source_status": map[string]string{"postgres": "OK", "clickhouse": "NOT_REQUESTED", "technical_observability": "NOT_REQUESTED"},
	}
	canonical, _ := json.Marshal(manifest)
	digest := sha256.Sum256(canonical)
	manifest["manifest_sha256"] = fmt.Sprintf("%x", digest[:])
	_ = json.NewEncoder(writer).Encode(manifest)
}

func auditEntryResponses(entries []caselifecycle.AuditEntry) []auditEntry {
	result := make([]auditEntry, 0, len(entries))
	for _, item := range entries {
		result = append(result, auditEntry{ID: item.ID, ActorID: item.ActorID, ActorRole: item.ActorRole, Action: item.Action, RequestID: item.RequestID, TraceID: item.TraceID, Metadata: item.Metadata, CreatedAt: item.CreatedAt})
	}
	return result
}

func patternFindingResponses(findings []caselifecycle.PatternFinding) []map[string]any {
	result := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		result = append(result, patternFindingResponse(finding))
	}
	return result
}

func (api investigationAPI) loadEvidence(ctx context.Context, id string) ([]map[string]any, error) {
	evidence, err := api.queries.Evidence(ctx, id)
	if err != nil {
		return nil, err
	}
	return evidenceResponses(evidence), nil
}

func evidenceResponses(evidence []caselifecycle.Evidence) []map[string]any {
	result := make([]map[string]any, 0, len(evidence))
	for _, evidenceItem := range evidence {
		source, openAction := evidenceSource(evidenceItem.EvidenceType)
		item := map[string]any{"id": evidenceItem.ID, "evidence_type": evidenceItem.EvidenceType, "reference": evidenceItem.Reference, "collected_at": evidenceItem.CollectedAt, "source": source, "open_action": openAction}
		if evidenceItem.EvidenceType == "GRAFANA_ALERT" {
			generatorURL := ""
			if evidenceItem.GeneratorURL != nil {
				generatorURL = *evidenceItem.GeneratorURL
			}
			if sourcePath, ok := grafanaAlertSourcePath(generatorURL); ok && evidenceItem.GrafanaOrgID != nil && *evidenceItem.GrafanaOrgID > 0 {
				item["source_locator"] = sourcePath
				item["source_org_id"] = *evidenceItem.GrafanaOrgID
			} else {
				item["open_action"] = "NONE"
			}
		}
		if evidenceItem.Checksum != nil {
			item["checksum"] = *evidenceItem.Checksum
		} else {
			item["checksum"] = nil
		}
		result = append(result, item)
	}
	return result
}

func caseNoteResponses(notes []domain.CaseNote) []caseNoteResponse {
	result := make([]caseNoteResponse, 0, len(notes))
	for _, note := range notes {
		result = append(result, caseNoteResponseFromDomain(note))
	}
	return result
}

func grafanaAlertSourcePath(value string) (string, bool) {
	// Karate standalone serializes request strings with escaped slashes; normalize
	// only that JSON-equivalent form before applying the strict path allowlist.
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

func evidenceManifestState(items []map[string]any) (bool, []string) {
	warnings := make([]string, 0)
	for _, item := range items {
		if item["checksum"] == nil {
			warnings = append(warnings, fmt.Sprintf("EVIDENCE_CHECKSUM_MISSING:%v", item["id"]))
		}
	}
	return len(warnings) > 0, warnings
}

func queryWindow(request *http.Request, fallback ...domain.IncidentWindow) (time.Time, time.Time, error) {
	fromValue := request.URL.Query().Get("from")
	toValue := request.URL.Query().Get("to")
	if (fromValue == "") != (toValue == "") {
		return time.Time{}, time.Time{}, fmt.Errorf("from and to must be supplied together")
	}
	if fromValue == "" {
		if len(fallback) > 0 {
			window, err := domain.NewIncidentWindow(fallback[0].From, fallback[0].To, fallback[0].Source)
			if err != nil {
				return time.Time{}, time.Time{}, err
			}
			return window.From, window.To, nil
		}
		to := time.Now().UTC()
		return to.Add(-defaultInvestigationQueryWindow), to, nil
	}
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !to.After(from) || to.Sub(from) > 7*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid query window")
	}
	return from.UTC(), to.UTC(), nil
}

func incidentWindowForManualCase(fromValue, toValue string, now time.Time) (domain.IncidentWindow, error) {
	if (fromValue == "") != (toValue == "") {
		return domain.IncidentWindow{}, domain.ErrInvalidIncidentWindow
	}
	if fromValue == "" {
		return domain.NewIncidentWindow(now.Add(-defaultInvestigationQueryWindow), now, domain.IncidentWindowManualDefault)
	}
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return domain.IncidentWindow{}, domain.ErrInvalidIncidentWindow
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return domain.IncidentWindow{}, domain.ErrInvalidIncidentWindow
	}
	return domain.NewIncidentWindow(from, to, domain.IncidentWindowTimelineSearch)
}

func queryLimit(request *http.Request) int {
	value, err := strconv.Atoi(request.URL.Query().Get("limit"))
	if err != nil || value == 0 {
		return 1000
	}
	if value < 1 {
		return 1
	}
	if value > 10000 {
		return 10000
	}
	return value
}

func loadTimeline(ctx context.Context, service *forensics.ForensicsService, correlationID string, from, to time.Time, limit int, includePayload bool) (map[string]any, error) {
	values, err := service.Search(ctx, forensics.EventSearchFilter{From: from, To: to, Limit: limit, CorrelationID: correlationID, IncludePayload: includePayload})
	if err != nil {
		return nil, err
	}
	events, err := timelineEventsFromForensics(values, includePayload)
	if err != nil {
		return nil, err
	}
	summaries, err := processingSummaries(ctx, service, events)
	if err != nil {
		return nil, err
	}
	for index := range events {
		if summary, ok := summaries[events[index].EventID]; ok {
			events[index].ProcessingSummary = summary
		}
	}
	return map[string]any{"correlation_id": correlationID, "from": from, "to": to, "event_count": len(events), "truncated": len(events) == limit, "events": events}, nil
}

func clickHouseSummaryFailure(err error) (string, []string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT", []string{"CLICKHOUSE_TIMEOUT"}
	}
	return "UNAVAILABLE", []string{"CLICKHOUSE_UNAVAILABLE"}
}

func unavailableTimeline(correlationID string, from, to time.Time) map[string]any {
	return map[string]any{
		"correlation_id": correlationID,
		"from":           from,
		"to":             to,
		"event_count":    0,
		"truncated":      false,
		"events":         []timelineEvent{},
	}
}

type investigationResponse struct {
	ID                    string             `json:"id"`
	CaseNo                string             `json:"case_no"`
	Title                 string             `json:"title"`
	Severity              string             `json:"severity"`
	Status                string             `json:"status"`
	AllowedTransitions    []string           `json:"allowed_transitions"`
	CorrelationID         string             `json:"correlation_id"`
	IncidentFrom          time.Time          `json:"incident_from"`
	IncidentTo            time.Time          `json:"incident_to"`
	IncidentWindowSource  string             `json:"incident_window_source"`
	Assignee              *string            `json:"assignee,omitempty"`
	Priority              string             `json:"priority"`
	Tags                  []string           `json:"tags"`
	RelatedCorrelationIDs []string           `json:"related_correlation_ids"`
	LastUpdatedBy         string             `json:"last_updated_by"`
	SLAStatus             string             `json:"sla_status"`
	SLADueAt              time.Time          `json:"sla_due_at"`
	RootCause             *string            `json:"root_cause,omitempty"`
	ResolutionSummary     *string            `json:"resolution_summary,omitempty"`
	FixedVersion          *string            `json:"fixed_version,omitempty"`
	Notes                 *string            `json:"notes,omitempty"`
	LockVersion           int64              `json:"lock_version"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
	ClosedAt              *time.Time         `json:"closed_at,omitempty"`
	PatternFindings       []map[string]any   `json:"pattern_findings"`
	Evidence              []map[string]any   `json:"evidence"`
	CollaborationNotes    []caseNoteResponse `json:"collaboration_notes"`
}

type caseNoteResponse struct {
	ID         string    `json:"id"`
	Body       string    `json:"body"`
	AuthorID   string    `json:"author_id"`
	AuthorRole string    `json:"author_role"`
	CreatedAt  time.Time `json:"created_at"`
}

type auditEntry struct {
	ID        string         `json:"id"`
	ActorID   string         `json:"actor_id"`
	ActorRole string         `json:"actor_role"`
	Action    string         `json:"action"`
	RequestID string         `json:"request_id"`
	TraceID   *string        `json:"trace_id"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

func investigationResponseFromCase(value domain.InvestigationCase) investigationResponse {
	slaDueAt, slaStatus := value.SLA(time.Now().UTC())
	allowedTransitions := make([]string, 0, len(value.AllowedTransitions()))
	for _, status := range value.AllowedTransitions() {
		allowedTransitions = append(allowedTransitions, string(status))
	}
	return investigationResponse{
		ID: value.ID, CaseNo: value.CaseNo, Title: value.Title, Severity: string(value.Severity), Status: string(value.Status), AllowedTransitions: allowedTransitions,
		CorrelationID: value.CorrelationID, IncidentFrom: value.IncidentWindow.From, IncidentTo: value.IncidentWindow.To,
		IncidentWindowSource: string(value.IncidentWindow.Source), Assignee: value.Assignee, Priority: string(value.Priority), Tags: nonNilStrings(value.Tags),
		RelatedCorrelationIDs: nonNilStrings(value.RelatedCorrelationIDs), LastUpdatedBy: value.LastUpdatedBy, SLAStatus: string(slaStatus), SLADueAt: slaDueAt,
		RootCause: value.RootCause, ResolutionSummary: value.ResolutionSummary,
		FixedVersion: value.FixedVersion, Notes: value.Notes, LockVersion: value.LockVersion, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		ClosedAt: value.ClosedAt, PatternFindings: make([]map[string]any, 0), Evidence: make([]map[string]any, 0), CollaborationNotes: make([]caseNoteResponse, 0),
	}
}

func caseNoteResponseFromDomain(value domain.CaseNote) caseNoteResponse {
	return caseNoteResponse{ID: value.ID, Body: value.Body, AuthorID: value.AuthorID, AuthorRole: value.AuthorRole, CreatedAt: value.CreatedAt}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func actorFromPrincipal(value principal) caselifecycle.Actor {
	return caselifecycle.Actor{Subject: value.Subject, Role: value.Role}
}

func patternFindingResponse(value patternanalysis.PatternFinding) map[string]any {
	result := map[string]any{
		"pattern_id": value.PatternID, "pattern_version": value.PatternVersion, "severity": value.Severity,
		"matched_conditions": value.MatchedConditions, "evidence_references": value.EvidenceReferences,
		"recommended_next_query": value.RecommendedNextQuery, "query_template_id": value.QueryTemplateID,
	}
	if value.ID != "" {
		result["finding_id"] = value.ID
		result["feedback"] = map[string]any{
			"finding_id": value.ID, "status": value.FeedbackStatus, "actor_id": value.FeedbackActorID, "actor_role": value.FeedbackActorRole,
			"updated_at": value.FeedbackUpdatedAt, "lock_version": value.FeedbackLockVersion,
		}
	}
	return result
}

func patternFeedbackResponse(value domain.PatternFindingFeedback) map[string]any {
	return map[string]any{
		"finding_id": value.FindingID, "status": value.Status, "actor_id": value.ActorID, "actor_role": value.ActorRole,
		"updated_at": value.UpdatedAt, "lock_version": value.LockVersion,
	}
}

func etag(version int64) string { return `"v` + strconv.FormatInt(version, 10) + `"` }
func validSeverity(value string) bool {
	return value == "LOW" || value == "MEDIUM" || value == "HIGH" || value == "CRITICAL"
}
func writeAPIError(writer http.ResponseWriter, status int, code string) {
	writeAPIErrorWithData(writer, status, code, nil)
}
func writeAPIErrorWithData(writer http.ResponseWriter, status int, code string, data map[string]any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	body := map[string]any{"code": code}
	for k, v := range data {
		body[k] = v
	}
	_ = json.NewEncoder(writer).Encode(body)
}
