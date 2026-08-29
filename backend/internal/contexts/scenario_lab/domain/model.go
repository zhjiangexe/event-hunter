package domain

import (
	"errors"
	"time"
)

const (
	LiveServices = "LIVE_SERVICES"
	LabInjection = "LAB_INJECTION"

	RunAccepted = "ACCEPTED"
	RunRunning  = "RUNNING"
	RunPassed   = "PASSED"
	RunFailed   = "FAILED"
	RunTimedOut = "TIMED_OUT"
)

var ErrRunNotFound = errors.New("scenario run not found")

type ScenarioDefinition struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Title              string   `json:"title"`
	Category           string   `json:"category"`
	Description        string   `json:"description"`
	ExecutionMode      string   `json:"execution_mode"`
	Synthetic          bool     `json:"synthetic"`
	ExpectedEventTypes []string `json:"expected_event_types"`
	ExpectedResults    []string `json:"expected_results"`
}

type Actual struct {
	TraceID               *string  `json:"trace_id"`
	EventCount            int      `json:"event_count"`
	EventTypes            []string `json:"event_types"`
	DuplicateEventIDs     []string `json:"duplicate_event_ids"`
	OutOfOrder            bool     `json:"out_of_order"`
	ProcessingStatuses    []string `json:"processing_statuses"`
	IngestionFailureCount int      `json:"ingestion_failure_count"`
	IngestionFailureTypes []string `json:"ingestion_failure_types"`
	MaxEventDelayMS       int64    `json:"max_event_delay_ms"`
}

func EmptyActual() Actual {
	return Actual{EventTypes: []string{}, DuplicateEventIDs: []string{}, ProcessingStatuses: []string{}, IngestionFailureTypes: []string{}}
}

type Check struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Passed   bool   `json:"passed"`
}
type Links struct {
	Timeline string  `json:"timeline"`
	Grafana  string  `json:"grafana"`
	Tempo    *string `json:"tempo"`
	Loki     string  `json:"loki"`
}

type Run struct {
	RunID              string             `json:"run_id"`
	Scenario           ScenarioDefinition `json:"scenario"`
	CorrelationID      string             `json:"correlation_id"`
	TraceID            *string            `json:"trace_id"`
	Status             string             `json:"status"`
	ExecutionMode      string             `json:"execution_mode"`
	Synthetic          bool               `json:"synthetic"`
	ExpectedEventTypes []string           `json:"expected_event_types"`
	Actual             Actual             `json:"actual"`
	Checks             []Check            `json:"checks"`
	Links              Links              `json:"links"`
	Error              *string            `json:"error"`
	AcceptedAt         time.Time          `json:"accepted_at"`
	StartedAt          *time.Time         `json:"started_at"`
	CompletedAt        *time.Time         `json:"completed_at"`
	DurationMS         *int64             `json:"duration_ms"`
	CurrentStep        string             `json:"current_step"`
}

type RunPage struct {
	Items []Run `json:"items"`
}
type RunFilter struct {
	ScenarioID    string
	Status        string
	ExecutionMode string
	From          *time.Time
	To            *time.Time
	PageSize      int
}

type RunRecord struct {
	RunID, ScenarioID, ScenarioName, ExecutionMode, CorrelationID, Status string
	Synthetic                                                             bool
	TraceID, Error                                                        *string
	ExpectedEventTypes                                                    []string
	Actual                                                                Actual
	Checks                                                                []Check
	AcceptedAt                                                            time.Time
	StartedAt, CompletedAt                                                *time.Time
}

func CurrentStep(status string) string {
	switch status {
	case RunAccepted:
		return "等待執行"
	case RunRunning:
		return "等待事件管線結果"
	case RunPassed:
		return "驗收通過"
	case RunTimedOut:
		return "等待結果逾時"
	case RunFailed:
		return "執行失敗"
	default:
		return "狀態未知"
	}
}
