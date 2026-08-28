package main

import (
	"context"
	"encoding/json"
	"net/http"

	patterneffectiveness "event-hunter/backend/internal/contexts/investigation/application/pattern_effectiveness"
)

type patternEffectivenessService interface {
	Get(context.Context) (patterneffectiveness.Summary, error)
}

type patternEffectivenessAPI struct {
	service patternEffectivenessService
}

func (api patternEffectivenessAPI) get(writer http.ResponseWriter, request *http.Request) {
	result, err := api.service.Get(request.Context())
	if err != nil {
		writeAPIError(writer, http.StatusServiceUnavailable, "PATTERN_EFFECTIVENESS_UNAVAILABLE")
		return
	}
	_ = json.NewEncoder(writer).Encode(result)
}
