package patterneffectiveness

import (
	"context"
	"time"

	domainpatterns "event-hunter/backend/internal/contexts/investigation/domain/patterns"
)

const defaultWindow = 30 * 24 * time.Hour

type Metric struct {
	PatternID          string     `json:"pattern_id"`
	HitCount           int64      `json:"hit_count"`
	LastHitAt          *time.Time `json:"last_hit_at"`
	InvestigationCount int64      `json:"investigation_count"`
}

type Summary struct {
	GeneratedAt time.Time `json:"generated_at"`
	Window      Window    `json:"window"`
	Items       []Metric  `json:"items"`
}

type Window struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type Reader interface {
	Effectiveness(context.Context, time.Time, time.Time) ([]Metric, error)
}

type Service struct {
	reader Reader
	now    func() time.Time
}

func NewService(reader Reader) *Service {
	return &Service{reader: reader, now: time.Now}
}

func (service *Service) Get(ctx context.Context) (Summary, error) {
	generatedAt := service.now().UTC()
	window := Window{From: generatedAt.Add(-defaultWindow), To: generatedAt}
	stored, err := service.reader.Effectiveness(ctx, window.From, window.To)
	if err != nil {
		return Summary{}, err
	}
	byPattern := make(map[string]Metric, len(stored))
	for _, metric := range stored {
		byPattern[metric.PatternID] = metric
	}
	items := make([]Metric, 0, len(domainpatterns.Registry()))
	for _, definition := range domainpatterns.Registry() {
		if definition.Status != "ACTIVE" {
			continue
		}
		metric := byPattern[definition.ID]
		metric.PatternID = definition.ID
		items = append(items, metric)
	}
	return Summary{GeneratedAt: generatedAt, Window: window, Items: items}, nil
}
