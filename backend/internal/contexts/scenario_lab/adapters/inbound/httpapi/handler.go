package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"event-hunter/backend/internal/contexts/scenario_lab/domain"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Service interface {
	Start(context.Context, string) (domain.Run, error)
	Get(context.Context, string) (domain.Run, error)
	List(context.Context, domain.RunFilter) (domain.RunPage, error)
}
type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }
func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/scenarios", handler.catalog)
	mux.HandleFunc("GET /api/v1/scenario-runs", handler.list)
	mux.HandleFunc("POST /api/v1/scenario-runs", handler.start)
	mux.HandleFunc("GET /api/v1/scenario-runs/{runID}", handler.get)
}
func (handler *Handler) catalog(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"items": domain.Catalog()})
}
func (handler *Handler) list(writer http.ResponseWriter, request *http.Request) {
	pageSize := 20
	if raw := request.URL.Query().Get("page_size"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(writer, http.StatusUnprocessableEntity, "INVALID_PAGE_SIZE")
			return
		}
		pageSize = parsed
	}
	filter := domain.RunFilter{ScenarioID: strings.TrimSpace(request.URL.Query().Get("scenario_id")), Status: strings.TrimSpace(request.URL.Query().Get("status")), ExecutionMode: strings.TrimSpace(request.URL.Query().Get("execution_mode")), PageSize: pageSize}
	if filter.ScenarioID != "" {
		if _, err := domain.Scenario(filter.ScenarioID); err != nil {
			writeError(writer, http.StatusUnprocessableEntity, "INVALID_SCENARIO_ID")
			return
		}
	}
	if filter.Status != "" && !slices.Contains([]string{domain.RunAccepted, domain.RunRunning, domain.RunPassed, domain.RunFailed, domain.RunTimedOut}, filter.Status) {
		writeError(writer, http.StatusUnprocessableEntity, "INVALID_STATUS")
		return
	}
	if filter.ExecutionMode != "" && !slices.Contains([]string{domain.LiveServices, domain.LabInjection}, filter.ExecutionMode) {
		writeError(writer, http.StatusUnprocessableEntity, "INVALID_EXECUTION_MODE")
		return
	}
	for name, target := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
		if raw := strings.TrimSpace(request.URL.Query().Get(name)); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
				return
			}
			*target = &parsed
		}
	}
	if filter.From != nil && filter.To != nil && !filter.To.After(*filter.From) {
		writeError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	result, err := handler.service.List(request.Context(), filter)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "SCENARIO_RUNS_UNAVAILABLE")
		return
	}
	writeJSON(writer, http.StatusOK, result)
}
func (handler *Handler) start(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		ScenarioID string `json:"scenario_id"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.ScenarioID == "" {
		writeError(writer, http.StatusUnprocessableEntity, "INVALID_SCENARIO_RUN")
		return
	}
	result, err := handler.service.Start(request.Context(), input.ScenarioID)
	if err != nil {
		if strings.Contains(err.Error(), "unknown scenario") {
			writeError(writer, http.StatusNotFound, "SCENARIO_NOT_FOUND")
			return
		}
		writeError(writer, http.StatusConflict, "SCENARIO_ENGINE_UNAVAILABLE")
		return
	}
	writeJSON(writer, http.StatusAccepted, result)
}
func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	result, err := handler.service.Get(request.Context(), request.PathValue("runID"))
	if errors.Is(err, domain.ErrRunNotFound) {
		writeError(writer, http.StatusNotFound, "SCENARIO_RUN_NOT_FOUND")
		return
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "SCENARIO_RUN_UNAVAILABLE")
		return
	}
	writeJSON(writer, http.StatusOK, result)
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func writeError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]string{"code": code})
}
