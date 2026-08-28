package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	businessjourney "event-hunter/backend/internal/contexts/investigation/application/business_journey"
)

type businessJourneyAPI struct {
	service *businessjourney.Service
}

func (api businessJourneyAPI) get(writer http.ResponseWriter, request *http.Request) {
	correlationID := strings.TrimSpace(request.PathValue("correlationID"))
	if correlationID == "" {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_CORRELATION_ID")
		return
	}
	from, err := time.Parse(time.RFC3339, request.URL.Query().Get("from"))
	if err != nil {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	to, err := time.Parse(time.RFC3339, request.URL.Query().Get("to"))
	if err != nil || !to.After(from) {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
		return
	}
	if to.Sub(from) > 7*24*time.Hour {
		writeTimelineError(writer, http.StatusUnprocessableEntity, "QUERY_WINDOW_TOO_LARGE")
		return
	}
	journey, err := api.service.Get(request.Context(), businessjourney.Query{
		CorrelationID: correlationID,
		From:          from.UTC(),
		To:            to.UTC(),
	})
	if err != nil {
		writeClickHouseError(writer, err, "BUSINESS_JOURNEY_UNAVAILABLE", "BUSINESS_JOURNEY_TIMEOUT")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(journey)
}
