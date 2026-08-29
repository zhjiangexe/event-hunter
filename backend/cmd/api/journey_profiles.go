package main

import (
	"encoding/json"
	"net/http"

	journeyprofiles "event-hunter/backend/internal/contexts/investigation/application/compatibility"
)

type journeyProfilesAPI struct {
	service *journeyprofiles.JourneyProfileQueries
}

func (api journeyProfilesAPI) list(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"items": api.service.List()})
}
