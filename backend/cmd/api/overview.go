package main

import (
	"encoding/json"
	"net/http"

	overview "event-hunter/backend/internal/contexts/investigation/application/operations"
)

type overviewAPI struct {
	service *overview.Service
}

func (api overviewAPI) get(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(api.service.Get(request.Context()))
}

func (api overviewAPI) sourceHealth(writer http.ResponseWriter, request *http.Request) {
	summary := api.service.Get(request.Context())
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"generated_at": summary.GeneratedAt,
		"partial":      summary.Partial,
		"warnings":     summary.Warnings,
		"sources":      summary.Sources,
	})
}
