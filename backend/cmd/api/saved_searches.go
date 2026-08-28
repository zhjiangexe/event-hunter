package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	savedsearch "event-hunter/backend/internal/contexts/investigation/application/saved_search"
	"event-hunter/backend/internal/contexts/investigation/domain"
)

var savedSearchIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type savedSearchAPI struct {
	service  *savedsearch.Service
	sessions sessionManager
}

func (api savedSearchAPI) list(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	items, err := api.service.List(request.Context(), principal.Subject)
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "SAVED_SEARCH_LIST_UNAVAILABLE")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"items": items})
}

func (api savedSearchAPI) create(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	var input struct {
		Name   string             `json:"name"`
		Target savedsearch.Target `json:"target"`
		Query  savedsearch.Query  `json:"query"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SAVED_SEARCH")
		return
	}
	created, err := api.service.Create(request.Context(), principal.Subject, input.Name, input.Target, input.Query)
	if errors.Is(err, domain.ErrInvalidSavedSearch) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SAVED_SEARCH")
		return
	}
	if errors.Is(err, domain.ErrSavedSearchNameConflict) {
		writeAPIError(writer, http.StatusConflict, "SAVED_SEARCH_NAME_CONFLICT")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "SAVED_SEARCH_CREATE_FAILED")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(writer).Encode(created)
}

func (api savedSearchAPI) delete(writer http.ResponseWriter, request *http.Request) {
	principal, ok := api.sessions.read(request)
	if !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	id := request.PathValue("id")
	if !savedSearchIDPattern.MatchString(id) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SAVED_SEARCH_ID")
		return
	}
	err := api.service.Delete(request.Context(), id, principal.Subject)
	if errors.Is(err, domain.ErrSavedSearchNotFound) {
		writeAPIError(writer, http.StatusNotFound, "SAVED_SEARCH_NOT_FOUND")
		return
	}
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "SAVED_SEARCH_DELETE_FAILED")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api savedSearchAPI) presets(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.sessions.read(request); !ok {
		writeAPIError(writer, http.StatusUnauthorized, "UNAUTHENTICATED")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"items": api.service.Presets()})
}
