package scenariolab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

type Observer interface {
	Observe(context.Context, string) (Actual, error)
}

type ClickHouseObserver struct {
	URL, Database, User, Password string
	Client                        *http.Client
}

type observedEvent struct {
	EventID, EventType, AggregateType, AggregateID, TraceID, OccurredAt, IngestedAt string
	Sequence                                                                        uint64
}

func (observer *ClickHouseObserver) Observe(ctx context.Context, correlationID string) (Actual, error) {
	actual := EmptyActual()
	events, err := observer.events(ctx, correlationID)
	if err != nil {
		return actual, err
	}
	counts := map[string]int{}
	lastSequence := map[string]uint64{}
	for _, event := range events {
		if actual.TraceID == nil && event.TraceID != "" {
			traceID := event.TraceID
			actual.TraceID = &traceID
		}
		actual.EventTypes = append(actual.EventTypes, event.EventType)
		counts[event.EventID]++
		key := event.AggregateType + "\x00" + event.AggregateID
		if previous, exists := lastSequence[key]; exists && event.Sequence < previous {
			actual.OutOfOrder = true
		}
		lastSequence[key] = event.Sequence
		occurred, occurredErr := parseClickHouseTime(event.OccurredAt)
		ingested, ingestedErr := parseClickHouseTime(event.IngestedAt)
		if occurredErr == nil && ingestedErr == nil && ingested.After(occurred) {
			delay := ingested.Sub(occurred).Milliseconds()
			if delay > actual.MaxEventDelayMS {
				actual.MaxEventDelayMS = delay
			}
		}
	}
	actual.EventCount = len(events)
	for id, count := range counts {
		if count > 1 {
			actual.DuplicateEventIDs = append(actual.DuplicateEventIDs, id)
		}
	}
	sort.Strings(actual.DuplicateEventIDs)

	actual.ProcessingStatuses, err = observer.stringColumn(ctx, `SELECT processing_status AS value
        FROM canonical_event_processing_attempts WHERE correlation_id={correlation:String}
        ORDER BY observed_at,attempt_id FORMAT JSON`, correlationID)
	if err != nil {
		return actual, err
	}
	actual.IngestionFailureTypes, err = observer.stringColumn(ctx, `SELECT value
        FROM (
            SELECT error_type AS value,failed_at AS occurred_at,source_offset
            FROM event_ingestion_failures WHERE correlation_id={correlation:String}
            UNION ALL
            SELECT error_code AS value,failed_at AS occurred_at,source_offset
            FROM poc_event_admission_failures WHERE correlation_id={correlation:String}
        )
        ORDER BY occurred_at,source_offset FORMAT JSON`, correlationID)
	if err != nil {
		return actual, err
	}
	actual.IngestionFailureCount = len(actual.IngestionFailureTypes)
	return actual, nil
}

func (observer *ClickHouseObserver) events(ctx context.Context, correlationID string) ([]observedEvent, error) {
	data, err := observer.query(ctx, `SELECT event_id,event_type,aggregate_type,aggregate_id,trace_id,sequence,occurred_at,ingested_at
        FROM canonical_forensics_events WHERE correlation_id={correlation:String}
        ORDER BY occurred_at,event_id FORMAT JSON`, correlationID)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			EventID       string `json:"event_id"`
			EventType     string `json:"event_type"`
			AggregateType string `json:"aggregate_type"`
			AggregateID   string `json:"aggregate_id"`
			TraceID       string `json:"trace_id"`
			Sequence      uint64 `json:"sequence"`
			OccurredAt    string `json:"occurred_at"`
			IngestedAt    string `json:"ingested_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode observed events: %w", err)
	}
	events := make([]observedEvent, 0, len(result.Data))
	for _, item := range result.Data {
		events = append(events, observedEvent{
			EventID: item.EventID, EventType: item.EventType, AggregateType: item.AggregateType,
			AggregateID: item.AggregateID, TraceID: item.TraceID, OccurredAt: item.OccurredAt,
			IngestedAt: item.IngestedAt, Sequence: item.Sequence,
		})
	}
	return events, nil
}

func (observer *ClickHouseObserver) stringColumn(ctx context.Context, statement, correlationID string) ([]string, error) {
	data, err := observer.query(ctx, statement, correlationID)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode observed values: %w", err)
	}
	values := make([]string, 0, len(result.Data))
	for _, item := range result.Data {
		values = append(values, item.Value)
	}
	return values, nil
}

func (observer *ClickHouseObserver) query(ctx context.Context, statement, correlationID string) ([]byte, error) {
	endpoint, err := url.Parse(observer.URL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("database", observer.Database)
	query.Set("readonly", "2")
	query.Set("param_correlation", correlationID)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewBufferString(statement))
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(observer.User, observer.Password)
	response, err := observer.client().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("clickhouse status %s: %s", response.Status, data)
	}
	return data, nil
}

func (observer *ClickHouseObserver) client() *http.Client {
	if observer.Client != nil {
		return observer.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func parseClickHouseTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999Z07:00", time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(unix, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unsupported ClickHouse timestamp %q", value)
}
