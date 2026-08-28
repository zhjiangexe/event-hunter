package domain

import (
	"errors"
	"sort"
	"time"
)

type ResolutionStatus string

const (
	ResolutionNoData                      ResolutionStatus = "NO_DATA"
	ResolutionIdentifierSelectionRequired ResolutionStatus = "IDENTIFIER_SELECTION_REQUIRED"
	ResolutionModelSelectionRequired      ResolutionStatus = "MODEL_SELECTION_REQUIRED"
	ResolutionNoApplicableModel           ResolutionStatus = "NO_APPLICABLE_MODEL"
	ResolutionEvaluated                   ResolutionStatus = "EVALUATED"
)

var (
	ErrModelVersionUnavailable = errors.New("Check Model version unavailable")
	ErrSourceUnavailable       = errors.New("canonical event source unavailable")
)

type RequestedModel struct {
	ID      string
	Version int
}

type ExecuteInput struct {
	IdentifierType  string
	IdentifierValue string
	AggregateType   string
	BusinessKeyName string
	RequestedModel  *RequestedModel
	Events          []Event
	AsOf            time.Time
	SourceHealth    SourceHealth
}

type ExecuteResult struct {
	ResolutionStatus     ResolutionStatus
	IdentifierCandidates []IdentifierCandidate
	Candidates           []ModelCandidate
	Model                *RegistryEntry
	Result               *EvaluationResult
}

type Engine struct {
	registry []RegistryEntry
}

func NewEngine(entries []RegistryEntry) Engine {
	return Engine{registry: cloneRegistryEntries(entries)}
}

func (engine Engine) Execute(input ExecuteInput) (ExecuteResult, error) {
	if input.SourceHealth.Status == SourceUnavailable {
		return ExecuteResult{}, ErrSourceUnavailable
	}
	events := append([]Event(nil), input.Events...)
	sortEvents(events)
	if len(events) == 0 {
		return ExecuteResult{ResolutionStatus: ResolutionNoData, Candidates: []ModelCandidate{}}, nil
	}
	identifier := ResolveIdentifier(IdentifierQuery{
		Type: input.IdentifierType, Value: input.IdentifierValue, AggregateType: input.AggregateType,
		BusinessKeyName: input.BusinessKeyName,
	}, engine.registry, events)
	if identifier.SelectionRequired {
		return ExecuteResult{
			ResolutionStatus:     ResolutionIdentifierSelectionRequired,
			IdentifierCandidates: identifier.Candidates, Candidates: []ModelCandidate{},
		}, nil
	}
	seeds := identifier.SeedEvents
	if len(seeds) == 0 {
		return ExecuteResult{ResolutionStatus: ResolutionNoData, Candidates: []ModelCandidate{}}, nil
	}
	candidates := ResolveModelCandidates(engine.registry, seeds, events)
	var selected RegistryEntry
	if input.RequestedModel != nil {
		entry, ok := lookupInRegistry(engine.registry, input.RequestedModel.ID, input.RequestedModel.Version)
		if !ok || entry.Model.Status != ModelStatusActive || entry.Model.Kind != ModelKindFlow {
			return ExecuteResult{}, ErrModelVersionUnavailable
		}
		selected = entry
	} else {
		entry, ok := RecommendedCandidate(candidates)
		if !ok {
			if len(candidates) == 0 {
				return ExecuteResult{ResolutionStatus: ResolutionNoApplicableModel, Candidates: candidates}, nil
			}
			return ExecuteResult{ResolutionStatus: ResolutionModelSelectionRequired, Candidates: candidates}, nil
		}
		selected = entry
	}
	evaluation, err := Evaluate(EvaluateInput{
		PrimaryModel: selected, Registry: engine.registry, Events: events, AsOf: input.AsOf,
		SourceHealth: input.SourceHealth,
	})
	if err != nil {
		return ExecuteResult{}, err
	}
	selectedCopy := selected
	return ExecuteResult{
		ResolutionStatus: ResolutionEvaluated, Candidates: candidates, Model: &selectedCopy, Result: &evaluation,
	}, nil
}

func FindingCodes(findings []Finding) []string {
	codes := make([]string, 0, len(findings))
	seen := make(map[string]bool)
	for _, finding := range findings {
		if !seen[finding.Code] {
			seen[finding.Code] = true
			codes = append(codes, finding.Code)
		}
	}
	sort.Strings(codes)
	return codes
}
