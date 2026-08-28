package patterns

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Definition struct {
	ID                      string          `json:"id"`
	Version                 int             `json:"version"`
	Name                    string          `json:"name"`
	Condition               string          `json:"condition"`
	Severity                string          `json:"severity"`
	Window                  string          `json:"window"`
	RequiredEventTypes      []string        `json:"required_event_types"`
	ExpectedEventTypes      []string        `json:"expected_event_types"`
	ExclusionEventTypes     []string        `json:"exclusion_event_types"`
	EvidenceQueryTemplateID string          `json:"evidence_query_template_id"`
	Status                  string          `json:"status"`
	MutableAtRuntime        bool            `json:"mutable_at_runtime"`
	SourcePath              string          `json:"source_path"`
	Checksum                string          `json:"checksum"`
	FixtureCoverage         FixtureCoverage `json:"fixture_coverage"`
	TriggerEventType        string          `json:"trigger_event_type"`
	ExpectedEventType       string          `json:"expected_event_type"`
	MatchedConditionCodes   []string        `json:"matched_condition_codes"`
	RecommendedNextQuery    string          `json:"recommended_next_query"`
}

type FixtureCoverage struct {
	MatchCount    int `json:"match_count"`
	NonMatchCount int `json:"non_match_count"`
	Total         int `json:"total"`
}

var (
	registryOnce sync.Once
	registry     []Definition
	windowSyntax = regexp.MustCompile(`^PT([1-9][0-9]*)M$`)
)

func Registry() []Definition {
	registryOnce.Do(func() {
		if err := json.Unmarshal([]byte(generatedRegistryJSON), &registry); err != nil {
			panic(fmt.Sprintf("decode generated pattern registry: %v", err))
		}
	})
	result := make([]Definition, len(registry))
	copy(result, registry)
	return result
}

func (definition Definition) MarshalJSON() ([]byte, error) {
	type publicDefinition struct {
		ID                      string          `json:"id"`
		Version                 int             `json:"version"`
		Name                    string          `json:"name"`
		Condition               string          `json:"condition"`
		Severity                string          `json:"severity"`
		Window                  string          `json:"window"`
		RequiredEventTypes      []string        `json:"required_event_types"`
		ExpectedEventTypes      []string        `json:"expected_event_types"`
		ExclusionEventTypes     []string        `json:"exclusion_event_types"`
		EvidenceQueryTemplateID string          `json:"evidence_query_template_id"`
		Status                  string          `json:"status"`
		MutableAtRuntime        bool            `json:"mutable_at_runtime"`
		SourcePath              string          `json:"source_path"`
		Checksum                string          `json:"checksum"`
		FixtureCoverage         FixtureCoverage `json:"fixture_coverage"`
	}
	return json.Marshal(publicDefinition{
		ID: definition.ID, Version: definition.Version, Name: definition.Name, Condition: definition.Condition,
		Severity: definition.Severity, Window: definition.Window, RequiredEventTypes: definition.RequiredEventTypes,
		ExpectedEventTypes: definition.ExpectedEventTypes, ExclusionEventTypes: definition.ExclusionEventTypes,
		EvidenceQueryTemplateID: definition.EvidenceQueryTemplateID, Status: definition.Status, MutableAtRuntime: definition.MutableAtRuntime,
		SourcePath: definition.SourcePath, Checksum: definition.Checksum, FixtureCoverage: definition.FixtureCoverage,
	})
}

func Lookup(id string) (Definition, bool) {
	for _, definition := range Registry() {
		if definition.ID == id && definition.Status == "ACTIVE" {
			return definition, true
		}
	}
	return Definition{}, false
}

func (definition Definition) WindowDuration() (time.Duration, error) {
	parts := windowSyntax.FindStringSubmatch(definition.Window)
	if parts == nil {
		return 0, fmt.Errorf("unsupported pattern window %q", definition.Window)
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("parse pattern window %q: %w", definition.Window, err)
	}
	return time.Duration(minutes) * time.Minute, nil
}
