package orderapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Starter struct {
	URL    string
	Client *http.Client
}

func (starter *Starter) Start(ctx context.Context, runID, profile string) (string, error) {
	body, _ := json.Marshal(map[string]any{"customer_id": "SCENARIO-LAB", "total_amount": 990, "currency": "TWD", "simulation_profile": profile})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(starter.URL, "/")+"/api/v1/orders", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "scenario-lab-"+runID)
	client := starter.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("call live Order API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("live Order API status %s", response.Status)
	}
	var accepted struct {
		CorrelationID string `json:"correlation_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil {
		return "", err
	}
	if accepted.CorrelationID == "" {
		return "", fmt.Errorf("live Order API returned empty correlation_id")
	}
	return accepted.CorrelationID, nil
}
