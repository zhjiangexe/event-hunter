package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	patterneffectiveness "event-hunter/backend/internal/contexts/investigation/application/pattern_effectiveness"
)

func TestPatternEffectivenessAPI(t *testing.T) {
	generatedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	api := patternEffectivenessAPI{service: patternEffectivenessServiceFake{summary: patterneffectiveness.Summary{
		GeneratedAt: generatedAt,
		Window:      patterneffectiveness.Window{From: generatedAt.Add(-30 * 24 * time.Hour), To: generatedAt},
		Items:       []patterneffectiveness.Metric{{PatternID: "pattern-1", HitCount: 3, InvestigationCount: 2}},
	}}}
	response := httptest.NewRecorder()
	api.get(response, httptest.NewRequest(http.MethodGet, "/api/v1/patterns/effectiveness", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"hit_count":3`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestPatternEffectivenessAPIPreservesUnavailableState(t *testing.T) {
	api := patternEffectivenessAPI{service: patternEffectivenessServiceFake{err: errors.New("postgres unavailable")}}
	response := httptest.NewRecorder()
	api.get(response, httptest.NewRequest(http.MethodGet, "/api/v1/patterns/effectiveness", nil))

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "PATTERN_EFFECTIVENESS_UNAVAILABLE") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

type patternEffectivenessServiceFake struct {
	summary patterneffectiveness.Summary
	err     error
}

func (service patternEffectivenessServiceFake) Get(context.Context) (patterneffectiveness.Summary, error) {
	return service.summary, service.err
}
