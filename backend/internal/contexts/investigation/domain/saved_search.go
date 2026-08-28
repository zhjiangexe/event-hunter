package domain

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidSavedSearch      = errors.New("invalid saved search")
	ErrSavedSearchNotFound     = errors.New("saved search not found")
	ErrSavedSearchNameConflict = errors.New("saved search name already exists")
)

type SavedSearchTarget string
type SavedSearchTimeMode string

const (
	SavedSearchTimeline   SavedSearchTarget   = "TIMELINE"
	SavedSearchJourney    SavedSearchTarget   = "JOURNEY"
	SavedSearchEventCheck SavedSearchTarget   = "EVENT_CHECK"
	SavedSearchAbsolute   SavedSearchTimeMode = "ABSOLUTE"
	SavedSearchRelative   SavedSearchTimeMode = "RELATIVE"
)

type SavedSearchQuery struct {
	TimeMode                  SavedSearchTimeMode `json:"time_mode"`
	RelativeWindowSeconds     *uint32             `json:"relative_window_seconds,omitempty"`
	From                      time.Time           `json:"from"`
	To                        time.Time           `json:"to"`
	CorrelationID             string              `json:"correlation_id,omitempty"`
	EventType                 string              `json:"event_type,omitempty"`
	AggregateID               string              `json:"aggregate_id,omitempty"`
	TraceID                   string              `json:"trace_id,omitempty"`
	EventID                   string              `json:"event_id,omitempty"`
	Producer                  string              `json:"producer,omitempty"`
	EventVersion              *uint32             `json:"event_version,omitempty"`
	CausationID               string              `json:"causation_id,omitempty"`
	KafkaTopic                string              `json:"kafka_topic,omitempty"`
	KafkaPartition            *uint32             `json:"kafka_partition,omitempty"`
	KafkaOffset               *uint64             `json:"kafka_offset,omitempty"`
	PatternID                 string              `json:"pattern_id,omitempty"`
	AlertID                   string              `json:"alert_id,omitempty"`
	Severity                  string              `json:"severity,omitempty"`
	IncludeProcessingAttempts bool                `json:"include_processing_attempts,omitempty"`
	IdentifierType            string              `json:"identifier_type,omitempty"`
	IdentifierValue           string              `json:"identifier_value,omitempty"`
	AggregateType             string              `json:"aggregate_type,omitempty"`
	BusinessKeyName           string              `json:"business_key_name,omitempty"`
	ModelID                   string              `json:"model_id,omitempty"`
	ModelVersion              *uint32             `json:"model_version,omitempty"`
	WorkspaceTab              string              `json:"workspace_tab,omitempty"`
}

type SavedSearch struct {
	ID           string            `json:"id"`
	OwnerSubject string            `json:"owner_subject"`
	Name         string            `json:"name"`
	Target       SavedSearchTarget `json:"target"`
	Query        SavedSearchQuery  `json:"query"`
	OpenURL      string            `json:"open_url"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func NewSavedSearch(ownerSubject, name string, target SavedSearchTarget, query SavedSearchQuery, now time.Time) (SavedSearch, error) {
	ownerSubject = strings.TrimSpace(ownerSubject)
	name = strings.TrimSpace(name)
	if ownerSubject == "" || len(ownerSubject) > 200 || name == "" || len(name) > 80 {
		return SavedSearch{}, ErrInvalidSavedSearch
	}
	query = normalizeSavedSearchQuery(query)
	if err := validateSavedSearchQuery(target, query); err != nil {
		return SavedSearch{}, err
	}
	now = now.UTC()
	result := SavedSearch{OwnerSubject: ownerSubject, Name: name, Target: target, Query: query, CreatedAt: now, UpdatedAt: now}
	result.OpenURL = result.BuildOpenURLAt(now)
	return result, nil
}

func RehydrateSavedSearch(id, ownerSubject, name string, target SavedSearchTarget, query SavedSearchQuery, createdAt, updatedAt time.Time) (SavedSearch, error) {
	result, err := NewSavedSearch(ownerSubject, name, target, query, updatedAt)
	if err != nil || strings.TrimSpace(id) == "" {
		return SavedSearch{}, ErrInvalidSavedSearch
	}
	result.ID = id
	result.CreatedAt = createdAt.UTC()
	result.UpdatedAt = updatedAt.UTC()
	return result, nil
}

func (saved SavedSearch) BuildOpenURL() string {
	return saved.BuildOpenURLAt(saved.UpdatedAt)
}

func (saved SavedSearch) BuildOpenURLAt(reference time.Time) string {
	values := url.Values{}
	from, to := saved.Query.From, saved.Query.To
	if saved.Query.TimeMode == SavedSearchRelative && saved.Query.RelativeWindowSeconds != nil {
		to = reference.UTC()
		from = to.Add(-time.Duration(*saved.Query.RelativeWindowSeconds) * time.Second)
	}
	values.Set("from", from.UTC().Format(time.RFC3339Nano))
	values.Set("to", to.UTC().Format(time.RFC3339Nano))
	add := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			values.Set(key, value)
		}
	}
	if saved.Target == SavedSearchEventCheck {
		add("identifier_type", saved.Query.IdentifierType)
		add("identifier", saved.Query.IdentifierValue)
		add("aggregate_type", saved.Query.AggregateType)
		add("business_key_name", saved.Query.BusinessKeyName)
		add("model_id", saved.Query.ModelID)
		if saved.Query.ModelVersion != nil {
			values.Set("model_version", strconv.FormatUint(uint64(*saved.Query.ModelVersion), 10))
		}
		add("tab", saved.Query.WorkspaceTab)
		return "/event-check?" + values.Encode()
	}
	add("correlation_id", saved.Query.CorrelationID)
	add("event_type", saved.Query.EventType)
	add("aggregate_id", saved.Query.AggregateID)
	add("trace_id", saved.Query.TraceID)
	add("event_id", saved.Query.EventID)
	add("producer", saved.Query.Producer)
	add("causation_id", saved.Query.CausationID)
	add("kafka_topic", saved.Query.KafkaTopic)
	add("pattern_id", saved.Query.PatternID)
	add("alert_id", saved.Query.AlertID)
	add("severity", saved.Query.Severity)
	if saved.Query.EventVersion != nil {
		values.Set("event_version", strconv.FormatUint(uint64(*saved.Query.EventVersion), 10))
	}
	if saved.Query.KafkaPartition != nil {
		values.Set("kafka_partition", strconv.FormatUint(uint64(*saved.Query.KafkaPartition), 10))
	}
	if saved.Query.KafkaOffset != nil {
		values.Set("kafka_offset", strconv.FormatUint(*saved.Query.KafkaOffset, 10))
	}
	if saved.Query.IncludeProcessingAttempts {
		values.Set("include_processing_attempts", "true")
	}
	path := "/timeline"
	if saved.Target == SavedSearchJourney {
		path = "/journey"
	}
	return path + "?" + values.Encode()
}

func (saved SavedSearch) RefreshOpenURL(reference time.Time) SavedSearch {
	saved.OpenURL = saved.BuildOpenURLAt(reference)
	return saved
}

func normalizeSavedSearchQuery(query SavedSearchQuery) SavedSearchQuery {
	if query.TimeMode == "" {
		query.TimeMode = SavedSearchAbsolute
	}
	if query.WorkspaceTab == "" && strings.TrimSpace(query.IdentifierValue) != "" {
		query.WorkspaceTab = "summary"
	}
	return query
}

func validateSavedSearchQuery(target SavedSearchTarget, query SavedSearchQuery) error {
	if target != SavedSearchTimeline && target != SavedSearchJourney && target != SavedSearchEventCheck {
		return ErrInvalidSavedSearch
	}
	if query.From.IsZero() || query.To.IsZero() || !query.To.After(query.From) || query.To.Sub(query.From) > 7*24*time.Hour {
		return ErrInvalidSavedSearch
	}
	if query.TimeMode != SavedSearchAbsolute && query.TimeMode != SavedSearchRelative {
		return ErrInvalidSavedSearch
	}
	if query.TimeMode == SavedSearchRelative {
		if query.RelativeWindowSeconds == nil || *query.RelativeWindowSeconds < 60 || *query.RelativeWindowSeconds > 7*24*60*60 {
			return ErrInvalidSavedSearch
		}
	} else if query.RelativeWindowSeconds != nil {
		return ErrInvalidSavedSearch
	}
	for _, value := range []string{
		query.CorrelationID, query.EventType, query.AggregateID, query.TraceID, query.EventID,
		query.Producer, query.CausationID, query.KafkaTopic, query.PatternID, query.AlertID,
		query.IdentifierValue, query.AggregateType, query.BusinessKeyName, query.ModelID,
	} {
		if len(strings.TrimSpace(value)) > 200 {
			return ErrInvalidSavedSearch
		}
	}
	if query.Severity != "" && query.Severity != "LOW" && query.Severity != "MEDIUM" && query.Severity != "HIGH" && query.Severity != "CRITICAL" {
		return ErrInvalidSavedSearch
	}
	if query.EventVersion != nil && *query.EventVersion < 1 {
		return ErrInvalidSavedSearch
	}
	if target == SavedSearchEventCheck {
		if !validEventCheckIdentifierType(query.IdentifierType) || strings.TrimSpace(query.IdentifierValue) == "" {
			return ErrInvalidSavedSearch
		}
		if query.IdentifierType != "BUSINESS_KEY" && strings.TrimSpace(query.BusinessKeyName) != "" {
			return ErrInvalidSavedSearch
		}
		if (strings.TrimSpace(query.ModelID) == "") != (query.ModelVersion == nil) {
			return ErrInvalidSavedSearch
		}
		if query.ModelVersion != nil && *query.ModelVersion < 1 {
			return ErrInvalidSavedSearch
		}
		if !validEventCheckWorkspaceTab(query.WorkspaceTab) {
			return ErrInvalidSavedSearch
		}
		return nil
	}
	if target == SavedSearchJourney {
		if strings.TrimSpace(query.CorrelationID) == "" {
			return ErrInvalidSavedSearch
		}
		return nil
	}
	if !savedSearchQueryHasFilter(query) {
		return ErrInvalidSavedSearch
	}
	return nil
}

func validEventCheckIdentifierType(value string) bool {
	switch value {
	case "AUTO", "CORRELATION_ID", "EVENT_ID", "TRACE_ID", "AGGREGATE_ID", "BUSINESS_KEY":
		return true
	default:
		return false
	}
}

func validEventCheckWorkspaceTab(value string) bool {
	switch value {
	case "summary", "timeline", "flow", "findings", "cases":
		return true
	default:
		return false
	}
}

func savedSearchQueryHasFilter(query SavedSearchQuery) bool {
	for _, value := range []string{
		query.CorrelationID, query.EventType, query.AggregateID, query.TraceID, query.EventID,
		query.Producer, query.CausationID, query.KafkaTopic, query.PatternID, query.AlertID, query.Severity,
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return query.EventVersion != nil || query.KafkaPartition != nil || query.KafkaOffset != nil
}
