package main

import (
	"encoding/json"
	"net/http"

	eventsearch "event-hunter/backend/internal/contexts/investigation/application/search"
)

func identifySmartSearchInput(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Input string `json:"input"`
	}
	if json.NewDecoder(request.Body).Decode(&input) != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_SMART_SEARCH_REQUEST")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(eventsearch.IdentifyInput(input.Input))
}
