package fixtures

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type Event struct {
	EventID       string          `json:"eventId"`
	EventType     string          `json:"eventType"`
	EventVersion  uint32          `json:"eventVersion"`
	OccurredAt    string          `json:"occurredAt"`
	Producer      string          `json:"producer"`
	CorrelationID string          `json:"correlationId"`
	CausationID   *string         `json:"causationId"`
	TraceID       *string         `json:"traceId"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   string          `json:"aggregateId"`
	Sequence      uint64          `json:"sequence"`
	Payload       json.RawMessage `json:"payload"`
}

type document struct {
	FixtureID string  `json:"fixtureId"`
	Events    []Event `json:"events"`
	Cases     []struct {
		FixtureID string  `json:"fixtureId"`
		Events    []Event `json:"events"`
	} `json:"cases"`
}

func LoadFile(path string) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	events, err := decode(data, path)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("fixture %s contains no events", path)
	}
	return events, nil
}

func decode(data []byte, path string) ([]Event, error) {
	var fixture document
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	result := append([]Event(nil), fixture.Events...)
	for _, fixtureCase := range fixture.Cases {
		result = append(result, fixtureCase.Events...)
	}
	return result, nil
}

func LoadDirectory(directory string) ([]Event, error) {
	var paths []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk fixtures: %w", err)
	}
	sort.Strings(paths)
	var result []Event
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read fixture %s: %w", path, err)
		}
		events, err := decode(data, path)
		if err != nil {
			return nil, err
		}
		// The repository fixture directory also contains Grafana, quality and
		// processing-attempt fixtures. They are valid contracts but not canonical
		// Domain Event documents, so the event loader deliberately ignores them.
		if len(events) == 0 {
			continue
		}
		result = append(result, events...)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("fixture directory %s contains no JSON events", directory)
	}
	return result, nil
}
