package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	attachchecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/attach_check_snapshot"
	classifycheckfinding "event-hunter/backend/internal/contexts/eventcheck/application/classify_check_finding"
	evaluateeventcheck "event-hunter/backend/internal/contexts/eventcheck/application/evaluate_event_check"
	getchecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/get_check_snapshot"
	listcheckmodels "event-hunter/backend/internal/contexts/eventcheck/application/list_check_models"
	listchecksnapshots "event-hunter/backend/internal/contexts/eventcheck/application/list_check_snapshots"
	savechecksnapshot "event-hunter/backend/internal/contexts/eventcheck/application/save_check_snapshot"
	eventcheckdomain "event-hunter/backend/internal/contexts/eventcheck/domain"
	eventcheckports "event-hunter/backend/internal/contexts/eventcheck/ports"
	investigationdomain "event-hunter/backend/internal/contexts/investigation/domain"
)

type eventCheckAPI struct {
	evaluator   *evaluateeventcheck.Service
	models      *listcheckmodels.Service
	saver       *savechecksnapshot.Service
	getter      *getchecksnapshot.Service
	lister      *listchecksnapshots.Service
	classifier  *classifycheckfinding.Service
	attachments *attachchecksnapshot.Service
	sessions    sessionManager
}

func (api eventCheckAPI) listSnapshots(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	pageSize := listchecksnapshots.DefaultPageSize
	if value := strings.TrimSpace(request.URL.Query().Get("page_size")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_PAGE_SIZE")
			return
		}
		pageSize = parsed
	}
	cursor, err := listchecksnapshots.DecodeCursor(request.URL.Query().Get("cursor"))
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CURSOR")
		return
	}
	page, err := api.lister.List(request.Context(), listchecksnapshots.Filter{
		Identifier: request.URL.Query().Get("identifier"), CheckStatus: request.URL.Query().Get("check_status"),
		PageSize: pageSize, Cursor: cursor,
	})
	if errors.Is(err, listchecksnapshots.ErrInvalidFilter) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CHECK_SNAPSHOT_FILTER")
		return
	}
	if err != nil {
		slog.ErrorContext(request.Context(), "list Check Snapshots", "error", err)
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_SNAPSHOT_LIST_UNAVAILABLE")
		return
	}
	_ = json.NewEncoder(writer).Encode(page)
}

func (api eventCheckAPI) evaluate(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	var input evaluateeventcheck.Request
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_CHECK_REQUEST")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_CHECK_REQUEST")
		return
	}
	result, err := api.evaluator.Evaluate(request.Context(), input)
	switch {
	case errors.Is(err, evaluateeventcheck.ErrInvalidRequest):
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_EVENT_CHECK_REQUEST")
		return
	case errors.Is(err, evaluateeventcheck.ErrModelUnavailable):
		writeAPIError(writer, http.StatusUnprocessableEntity, "MODEL_VERSION_UNAVAILABLE")
		return
	case err != nil:
		slog.ErrorContext(request.Context(), "evaluate Event Check", "error", err)
		writeClickHouseError(writer, err, "EVENT_CHECK_SOURCE_UNAVAILABLE", "EVENT_CHECK_SOURCE_TIMEOUT")
		return
	}
	_ = json.NewEncoder(writer).Encode(result)
}

func (api eventCheckAPI) listModels(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	_ = json.NewEncoder(writer).Encode(api.models.List())
}

func (api eventCheckAPI) getModel(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	version, err := strconv.Atoi(request.PathValue("version"))
	if err != nil || version < 1 {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_MODEL_VERSION")
		return
	}
	result, err := api.models.Get(request.PathValue("modelId"), version)
	if errors.Is(err, listcheckmodels.ErrNotFound) {
		writeAPIError(writer, http.StatusNotFound, "CHECK_MODEL_NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_MODEL_REGISTRY_UNAVAILABLE")
		return
	}
	_ = json.NewEncoder(writer).Encode(result)
}

func (api eventCheckAPI) getModelSource(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	version, err := strconv.Atoi(request.PathValue("version"))
	if err != nil || version < 1 {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_MODEL_VERSION")
		return
	}
	result, err := api.models.GetSource(request.PathValue("modelId"), version)
	if errors.Is(err, listcheckmodels.ErrNotFound) {
		writeAPIError(writer, http.StatusNotFound, "CHECK_MODEL_NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_MODEL_REGISTRY_UNAVAILABLE")
		return
	}
	_ = json.NewEncoder(writer).Encode(result)
}

func (api eventCheckAPI) createSnapshot(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeAPIError(writer, http.StatusPreconditionRequired, "IDEMPOTENCY_KEY_REQUIRED")
		return
	}
	var input savechecksnapshot.Request
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || ensureJSONEOF(decoder) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CHECK_SNAPSHOT_REQUEST")
		return
	}
	result, created, err := api.saver.Save(request.Context(), input,
		eventcheckdomain.SnapshotActor{Subject: principal.Subject, Role: principal.Role}, idempotencyKey, request.Header.Get("X-Request-ID"))
	var changed savechecksnapshot.EvaluationChangedError
	switch {
	case errors.As(err, &changed):
		writeAPIErrorWithData(writer, http.StatusConflict, "EVALUATION_CHANGED", map[string]any{
			"current_event_set_hash": changed.CurrentEventSetHash, "current_evaluation_hash": changed.CurrentEvaluationHash,
		})
		return
	case errors.Is(err, savechecksnapshot.ErrIdempotencyKeyReused):
		writeAPIError(writer, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED")
		return
	case errors.Is(err, evaluateeventcheck.ErrModelUnavailable):
		writeAPIError(writer, http.StatusConflict, "MODEL_VERSION_UNAVAILABLE")
		return
	case errors.Is(err, savechecksnapshot.ErrInvalidSaveRequest), errors.Is(err, eventcheckdomain.ErrInvalidSnapshot):
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CHECK_SNAPSHOT_REQUEST")
		return
	case err != nil:
		slog.ErrorContext(request.Context(), "save Check Snapshot", "error", err)
		writeClickHouseError(writer, err, "CHECK_SNAPSHOT_UNAVAILABLE", "CHECK_SNAPSHOT_TIMEOUT")
		return
	}
	if created {
		writer.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(writer).Encode(result)
}

func (api eventCheckAPI) getSnapshot(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if _, err := uuid.Parse(request.PathValue("snapshotId")); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SNAPSHOT_ID")
		return
	}
	result, err := api.getter.Get(request.Context(), request.PathValue("snapshotId"))
	if errors.Is(err, eventcheckdomain.ErrSnapshotNotFound) {
		writeAPIError(writer, http.StatusNotFound, "CHECK_SNAPSHOT_NOT_FOUND")
		return
	}
	if err != nil {
		slog.ErrorContext(request.Context(), "get Check Snapshot", "error", err, "snapshot_id", request.PathValue("snapshotId"))
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_SNAPSHOT_UNAVAILABLE")
		return
	}
	_ = json.NewEncoder(writer).Encode(result)
}

func (api eventCheckAPI) classifyFinding(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	findingID := request.PathValue("findingId")
	if _, err := uuid.Parse(findingID); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_FINDING_ID")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil || match < 0 {
		writeAPIError(writer, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		Status eventcheckdomain.FindingFeedbackStatus `json:"status"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 8*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || ensureJSONEOF(decoder) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CHECK_FINDING_FEEDBACK")
		return
	}
	result, err := api.classifier.Classify(request.Context(), findingID, input.Status, match,
		eventcheckdomain.SnapshotActor{Subject: principal.Subject, Role: principal.Role}, request.Header.Get("X-Request-ID"))
	switch {
	case errors.Is(err, eventcheckdomain.ErrFindingNotFound):
		writeAPIError(writer, http.StatusNotFound, "CHECK_FINDING_NOT_FOUND")
		return
	case errors.Is(err, eventcheckdomain.ErrFeedbackConflict):
		writeAPIError(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT")
		return
	case errors.Is(err, eventcheckdomain.ErrInvalidFindingFeedback):
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CHECK_FINDING_FEEDBACK")
		return
	case err != nil:
		slog.ErrorContext(request.Context(), "classify Check Finding", "error", err, "finding_id", findingID)
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_FINDING_FEEDBACK_UNAVAILABLE")
		return
	}
	writer.Header().Set("ETag", etag(result.LockVersion))
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"finding_id": result.FindingID, "status": result.Status, "actor_id": result.ActorID,
		"actor_role": result.ActorRole, "updated_at": result.UpdatedAt, "lock_version": result.LockVersion,
	})
}

func (api eventCheckAPI) listInvestigationSnapshots(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	investigationID := request.PathValue("investigationId")
	if _, err := uuid.Parse(investigationID); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_INVESTIGATION_ID")
		return
	}
	links, err := api.attachments.List(request.Context(), investigationID)
	if errors.Is(err, investigationdomain.ErrCaseNotFound) {
		writeAPIError(writer, http.StatusNotFound, "INVESTIGATION_NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_SNAPSHOT_LINKS_UNAVAILABLE")
		return
	}
	result := make([]map[string]any, 0, len(links))
	for _, link := range links {
		result = append(result, investigationSnapshotLinkResponse(link))
	}
	_ = json.NewEncoder(writer).Encode(result)
}

func (api eventCheckAPI) attachInvestigationSnapshot(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	if !canWrite(principal.Role) {
		writeAPIError(writer, http.StatusForbidden, "FORBIDDEN")
		return
	}
	investigationID := request.PathValue("investigationId")
	if _, err := uuid.Parse(investigationID); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_INVESTIGATION_ID")
		return
	}
	match, err := strconv.ParseInt(strings.Trim(request.Header.Get("If-Match"), `"v`), 10, 64)
	if err != nil || match < 0 {
		writeAPIError(writer, http.StatusPreconditionRequired, "IF_MATCH_REQUIRED")
		return
	}
	var input struct {
		SnapshotID string `json:"snapshot_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 8*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || ensureJSONEOF(decoder) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SNAPSHOT_LINK")
		return
	}
	if _, err := uuid.Parse(input.SnapshotID); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SNAPSHOT_LINK")
		return
	}
	link, attached, err := api.attachments.Attach(request.Context(), investigationID, input.SnapshotID, match,
		eventcheckdomain.SnapshotActor{Subject: principal.Subject, Role: principal.Role}, request.Header.Get("X-Request-ID"))
	switch {
	case errors.Is(err, investigationdomain.ErrCaseNotFound):
		writeAPIError(writer, http.StatusNotFound, "INVESTIGATION_NOT_FOUND")
		return
	case errors.Is(err, eventcheckdomain.ErrSnapshotNotFound):
		writeAPIError(writer, http.StatusNotFound, "CHECK_SNAPSHOT_NOT_FOUND")
		return
	case errors.Is(err, investigationdomain.ErrOptimisticConflict):
		writeAPIError(writer, http.StatusConflict, "OPTIMISTIC_LOCK_CONFLICT")
		return
	case errors.Is(err, investigationdomain.ErrInvalidTransition):
		writeAPIError(writer, http.StatusConflict, "INVESTIGATION_CLOSED")
		return
	case err != nil:
		slog.ErrorContext(request.Context(), "attach Check Snapshot", "error", err, "investigation_id", investigationID, "snapshot_id", input.SnapshotID)
		writeAPIError(writer, http.StatusServiceUnavailable, "CHECK_SNAPSHOT_LINK_UNAVAILABLE")
		return
	}
	writer.Header().Set("ETag", etag(link.CaseLockVersion))
	if attached {
		writer.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(writer).Encode(investigationSnapshotLinkResponse(link))
}

func investigationSnapshotLinkResponse(link eventcheckports.InvestigationSnapshotLink) map[string]any {
	return map[string]any{
		"investigation_id": link.InvestigationID, "snapshot_id": link.SnapshotID,
		"linked_by": link.LinkedBy, "linked_by_role": link.LinkedByRole, "linked_at": link.LinkedAt,
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}
