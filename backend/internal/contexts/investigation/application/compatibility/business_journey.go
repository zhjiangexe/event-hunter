package compatibility

import (
	"context"
	"time"

	forensics "event-hunter/backend/internal/contexts/investigation/application/search"
	"event-hunter/backend/internal/contexts/investigation/domain/journeys"
)

type JourneyEventReader interface {
	Search(context.Context, forensics.EventSearchFilter) ([]forensics.ForensicsEvent, error)
}

type Query struct {
	CorrelationID string
	From          time.Time
	To            time.Time
}

type Status = journeys.Status

const (
	StatusEmpty       = journeys.StatusEmpty
	StatusInProgress  = journeys.StatusInProgress
	StatusCompleted   = journeys.StatusCompleted
	StatusFailed      = journeys.StatusFailed
	StatusCompensated = journeys.StatusCompensated
)

type MilestoneState = journeys.MilestoneState

const (
	MilestoneCompleted     = journeys.MilestoneCompleted
	MilestoneInProgress    = journeys.MilestoneInProgress
	MilestoneFailed        = journeys.MilestoneFailed
	MilestoneCompensated   = journeys.MilestoneCompensated
	MilestoneNotApplicable = journeys.MilestoneNotApplicable
)

type EventReference = journeys.EventReference
type Milestone = journeys.EvaluatedMilestone
type Anomaly = journeys.Anomaly
type Journey = journeys.Evaluation

type JourneyService struct {
	events  JourneyEventReader
	profile journeys.Profile
}

func NewJourneyService(events JourneyEventReader) *JourneyService {
	profile, ok := journeys.Default()
	if !ok {
		panic("business journey requires one active default Journey Profile")
	}
	return NewJourneyServiceWithProfile(events, profile)
}

func NewJourneyServiceWithProfile(events JourneyEventReader, profile journeys.Profile) *JourneyService {
	return &JourneyService{events: events, profile: profile}
}

func (service *JourneyService) Get(ctx context.Context, query Query) (Journey, error) {
	events, err := service.events.Search(ctx, forensics.EventSearchFilter{
		From: query.From, To: query.To, Limit: 1000, CorrelationID: query.CorrelationID,
	})
	if err != nil {
		return Journey{}, err
	}
	observed := make([]journeys.Event, 0, len(events))
	for _, event := range events {
		observed = append(observed, journeys.Event{
			EventID: event.EventID, EventType: event.EventType, OccurredAt: event.OccurredAt,
			Producer: event.Producer, AggregateType: event.AggregateType, AggregateID: event.AggregateID, TraceID: event.TraceID,
		})
	}
	return journeys.Evaluate(query.CorrelationID, query.From, query.To, observed, service.profile), nil
}
