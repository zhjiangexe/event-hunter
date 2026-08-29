package search

import (
	"regexp"
	"strings"
)

type IdentificationStatus string

const (
	IdentificationIdentified IdentificationStatus = "IDENTIFIED"
	IdentificationAmbiguous  IdentificationStatus = "AMBIGUOUS"
	IdentificationInvalid    IdentificationStatus = "INVALID"
)

type IdentifierType string

const (
	IdentifierEventID          IdentifierType = "EVENT_ID"
	IdentifierTraceID          IdentifierType = "TRACE_ID"
	IdentifierCorrelationID    IdentifierType = "CORRELATION_ID"
	IdentifierAggregateID      IdentifierType = "AGGREGATE_ID"
	IdentifierAlertFingerprint IdentifierType = "ALERT_FINGERPRINT"
)

type IdentifierCandidate struct {
	Type           IdentifierType `json:"identifier_type"`
	QueryParameter string         `json:"query_parameter"`
	Certainty      string         `json:"certainty"`
	Reason         string         `json:"reason"`
}

type IdentificationResult struct {
	Input           string                `json:"input"`
	NormalizedInput string                `json:"normalized_input"`
	Status          IdentificationStatus  `json:"status"`
	Candidates      []IdentifierCandidate `json:"candidates"`
	Message         string                `json:"message"`
}

var opaqueIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{2,199}$`)
var traceIdentifierPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var fingerprintPattern = regexp.MustCompile(`^[0-9a-fA-F]{16,64}$`)

var explicitIdentifierTypes = map[string]IdentifierType{
	"event":          IdentifierEventID,
	"event_id":       IdentifierEventID,
	"trace":          IdentifierTraceID,
	"trace_id":       IdentifierTraceID,
	"correlation":    IdentifierCorrelationID,
	"correlation_id": IdentifierCorrelationID,
	"aggregate":      IdentifierAggregateID,
	"aggregate_id":   IdentifierAggregateID,
	"alert":          IdentifierAlertFingerprint,
	"fingerprint":    IdentifierAlertFingerprint,
}

func IdentifyInput(input string) IdentificationResult {
	normalized := strings.TrimSpace(input)
	result := IdentificationResult{Input: input, NormalizedInput: normalized, Candidates: []IdentifierCandidate{}}
	if normalized == "" {
		result.Status = IdentificationInvalid
		result.Message = "EMPTY_INPUT"
		return result
	}
	if len(normalized) < 3 {
		result.Status = IdentificationInvalid
		result.Message = "INPUT_TOO_SHORT"
		return result
	}
	if len(normalized) > 220 {
		result.Status = IdentificationInvalid
		result.Message = "INPUT_TOO_LONG"
		return result
	}

	if prefix, value, found := strings.Cut(normalized, ":"); found {
		if identifierType, known := explicitIdentifierTypes[strings.ToLower(strings.TrimSpace(prefix))]; known {
			value = strings.TrimSpace(value)
			if !validExplicitIdentifier(identifierType, value) {
				result.Status = IdentificationInvalid
				result.Message = "INVALID_EXPLICIT_IDENTIFIER"
				return result
			}
			result.NormalizedInput = value
			result.Status = IdentificationIdentified
			result.Candidates = []IdentifierCandidate{candidate(identifierType, "EXACT", "EXPLICIT_PREFIX")}
			result.Message = "IDENTIFIER_CONFIRMED"
			return result
		}
	}

	if !opaqueIdentifierPattern.MatchString(normalized) {
		result.Status = IdentificationInvalid
		result.Message = "UNSUPPORTED_IDENTIFIER_FORMAT"
		return result
	}
	if traceIdentifierPattern.MatchString(normalized) && normalized != strings.Repeat("0", 32) {
		result.Candidates = append(result.Candidates, candidate(IdentifierTraceID, "STRONG", "W3C_TRACE_ID_FORMAT"))
	}
	if fingerprintPattern.MatchString(normalized) {
		result.Candidates = append(result.Candidates, candidate(IdentifierAlertFingerprint, "CANDIDATE", "GRAFANA_FINGERPRINT_FORMAT"))
	}
	result.Candidates = append(result.Candidates,
		candidate(IdentifierCorrelationID, "CANDIDATE", "OPAQUE_IDENTIFIER_FORMAT"),
		candidate(IdentifierAggregateID, "CANDIDATE", "OPAQUE_IDENTIFIER_FORMAT"),
		candidate(IdentifierEventID, "CANDIDATE", "OPAQUE_IDENTIFIER_FORMAT"),
	)
	result.Status = IdentificationAmbiguous
	result.Message = "SELECT_IDENTIFIER_TYPE"
	return result
}

func validExplicitIdentifier(identifierType IdentifierType, value string) bool {
	if identifierType == IdentifierTraceID {
		return traceIdentifierPattern.MatchString(value) && value != strings.Repeat("0", 32)
	}
	return opaqueIdentifierPattern.MatchString(value)
}

func candidate(identifierType IdentifierType, certainty, reason string) IdentifierCandidate {
	parameter := map[IdentifierType]string{
		IdentifierEventID:          "event_id",
		IdentifierTraceID:          "trace_id",
		IdentifierCorrelationID:    "correlation_id",
		IdentifierAggregateID:      "aggregate_id",
		IdentifierAlertFingerprint: "alert_id",
	}[identifierType]
	return IdentifierCandidate{Type: identifierType, QueryParameter: parameter, Certainty: certainty, Reason: reason}
}
