package eventsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"event-hunter/backend/internal/contexts/investigation/application/forensics"
	domainpatterns "event-hunter/backend/internal/contexts/investigation/domain/patterns"
)

const MaxQualifiedCorrelations = 1000

var (
	ErrUnknownPattern          = errors.New("unknown or inactive pattern")
	ErrInvalidSeverity         = errors.New("invalid minimum severity")
	ErrSearchQualifierSource   = errors.New("event search qualifier source unavailable")
	ErrQualifierResultTooLarge = errors.New("event search qualifier result exceeds limit")
)

type EventSearchFilter = forensics.EventSearchFilter
type ForensicsEvent = forensics.ForensicsEvent
type ProcessingSummary = forensics.ProcessingSummary

type ForensicsReadModel interface {
	Search(ctx context.Context, filter forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error)
}

type EventSearchQualifierRepository interface {
	CorrelationsByAlertFingerprint(ctx context.Context, fingerprint string) ([]string, error)
	CorrelationsByMinimumSeverity(ctx context.Context, severity string) ([]string, error)
}

type AdvancedEventSearchFilter struct {
	forensics.EventSearchFilter
	PatternID       string
	AlertID         string
	MinimumSeverity string
}

// EventSearchService owns filters that cross the Pattern Registry, PostgreSQL
// control plane and ClickHouse read model. Raw ClickHouse filters remain in
// ForensicsService so timeline reads do not depend on PostgreSQL.
type EventSearchService struct {
	readModel  ForensicsReadModel
	qualifiers EventSearchQualifierRepository
}

func NewEventSearchService(readModel ForensicsReadModel, qualifiers EventSearchQualifierRepository) *EventSearchService {
	return &EventSearchService{readModel: readModel, qualifiers: qualifiers}
}

func (service *EventSearchService) Search(ctx context.Context, filter AdvancedEventSearchFilter) ([]forensics.ForensicsEvent, error) {
	patternID := strings.TrimSpace(filter.PatternID)
	if patternID != "" {
		definition, ok := domainpatterns.Lookup(patternID)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownPattern, patternID)
		}
		filter.EventTypes = uniqueStrings(append(append(
			append([]string{}, definition.RequiredEventTypes...),
			definition.ExpectedEventTypes...),
			definition.ExclusionEventTypes...))
		if filter.EventType != "" && !containsString(filter.EventTypes, filter.EventType) {
			return []forensics.ForensicsEvent{}, nil
		}
	}

	severity := strings.ToUpper(strings.TrimSpace(filter.MinimumSeverity))
	if severity != "" && !containsString([]string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}, severity) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidSeverity, filter.MinimumSeverity)
	}
	alertID := strings.TrimSpace(filter.AlertID)
	if alertID == "" && severity == "" {
		return service.readModel.Search(ctx, filter.EventSearchFilter)
	}
	if service.qualifiers == nil {
		return nil, ErrSearchQualifierSource
	}

	var qualified []string
	if alertID != "" {
		values, err := service.qualifiers.CorrelationsByAlertFingerprint(ctx, alertID)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSearchQualifierSource, err)
		}
		qualified = uniqueStrings(values)
	}
	if severity != "" {
		values, err := service.qualifiers.CorrelationsByMinimumSeverity(ctx, severity)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSearchQualifierSource, err)
		}
		if alertID == "" {
			qualified = uniqueStrings(values)
		} else {
			qualified = intersectStrings(qualified, values)
		}
	}
	if len(qualified) > MaxQualifiedCorrelations {
		return nil, ErrQualifierResultTooLarge
	}
	if len(qualified) == 0 {
		return []forensics.ForensicsEvent{}, nil
	}
	if filter.CorrelationID != "" {
		if !containsString(qualified, filter.CorrelationID) {
			return []forensics.ForensicsEvent{}, nil
		}
	} else {
		filter.CorrelationIDs = qualified
	}
	return service.readModel.Search(ctx, filter.EventSearchFilter)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intersectStrings(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[strings.TrimSpace(value)] = struct{}{}
	}
	result := make([]string, 0, len(left))
	for _, value := range uniqueStrings(left) {
		if _, exists := rightSet[value]; exists {
			result = append(result, value)
		}
	}
	return result
}
