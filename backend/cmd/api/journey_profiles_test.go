package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
)

func TestJourneyProfileCatalogListsCompiledProfiles(t *testing.T) {
	server := newServeMuxWithWebhook(nil, nil, &forensics.ForensicsService{})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/journey-profiles", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, expected := range []string{
		`"id":"order-fulfillment"`,
		`"version":1`,
		`"source_path":"contracts/journeys/order-fulfillment.yaml"`,
		`"checksum":"`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("response does not contain %s: %s", expected, response.Body.String())
		}
	}
}
