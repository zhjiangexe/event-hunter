package journeys

import (
	"encoding/json"
	"fmt"
	"sync"
)

type StateRule struct {
	State               string   `json:"state"`
	WhenAnyEventTypes   []string `json:"when_any_event_types"`
	UnlessAnyEventTypes []string `json:"unless_any_event_types"`
}

type Milestone struct {
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	ExpectedEventTypes []string    `json:"expected_event_types"`
	StateRules         []StateRule `json:"state_rules"`
}

type AnomalyRule struct {
	Code                  string   `json:"code"`
	Severity              string   `json:"severity"`
	Message               string   `json:"message"`
	TriggerEventTypes     []string `json:"trigger_event_types"`
	RequiredAnyEventTypes []string `json:"required_any_event_types"`
	EvidenceEventTypes    []string `json:"evidence_event_types"`
	GracePeriodSeconds    int      `json:"grace_period_seconds"`
}

type DataQuality struct {
	DetectDuplicateEventIDs bool `json:"detect_duplicate_event_ids"`
}

type Profile struct {
	ContractVersion   int           `json:"contract_version"`
	ID                string        `json:"id"`
	Version           int           `json:"version"`
	Status            string        `json:"status"`
	Default           bool          `json:"default"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	SourcePath        string        `json:"source_path"`
	Checksum          string        `json:"checksum"`
	Milestones        []Milestone   `json:"milestones"`
	JourneyStateRules []StateRule   `json:"journey_state_rules"`
	AnomalyRules      []AnomalyRule `json:"anomaly_rules"`
	DataQuality       DataQuality   `json:"data_quality"`
}

var (
	registryOnce sync.Once
	registry     []Profile
)

func Registry() []Profile {
	registryOnce.Do(func() {
		if err := json.Unmarshal([]byte(generatedRegistryJSON), &registry); err != nil {
			panic(fmt.Sprintf("decode generated journey profile registry: %v", err))
		}
	})
	return cloneProfiles(registry)
}

func Default() (Profile, bool) {
	for _, profile := range Registry() {
		if profile.Default && profile.Status == "active" {
			return profile, true
		}
	}
	return Profile{}, false
}

func cloneProfiles(profiles []Profile) []Profile {
	encoded, err := json.Marshal(profiles)
	if err != nil {
		panic(fmt.Sprintf("clone generated journey profile registry: %v", err))
	}
	var cloned []Profile
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		panic(fmt.Sprintf("decode cloned journey profile registry: %v", err))
	}
	return cloned
}

func Lookup(id string) (Profile, bool) {
	for _, profile := range Registry() {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}
