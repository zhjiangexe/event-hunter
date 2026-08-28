package patterneffectiveness

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceReturnsRegisteredPatternsWithBoundedMetrics(t *testing.T) {
	lastHit := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	reader := &readerFake{metrics: []Metric{{
		PatternID: "payment-completed-without-shipment", HitCount: 7, LastHitAt: &lastHit, InvestigationCount: 4,
		ConfirmedCount: 2, FalsePositiveCount: 1, NeedsReviewCount: 1, UnreviewedCount: 3, ReviewedCount: 4,
	}}}
	service := NewService(reader)
	service.now = func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }

	result, err := service.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].HitCount != 7 || result.Items[0].InvestigationCount != 4 {
		t.Fatalf("items = %#v", result.Items)
	}
	if result.Items[0].LastHitAt == nil || !result.Items[0].LastHitAt.Equal(lastHit) {
		t.Fatalf("last hit = %v", result.Items[0].LastHitAt)
	}
	if result.Items[0].FalsePositiveRate == nil || *result.Items[0].FalsePositiveRate != 0.25 {
		t.Fatalf("false positive rate = %v", result.Items[0].FalsePositiveRate)
	}
	if reader.from != result.GeneratedAt.Add(-30*24*time.Hour) || reader.to != result.GeneratedAt {
		t.Fatalf("reader window = [%v, %v), result = %#v", reader.from, reader.to, result)
	}
}

func TestServiceDoesNotTurnReaderFailureIntoZeroMetrics(t *testing.T) {
	want := errors.New("postgres unavailable")
	service := NewService(&readerFake{err: want})

	_, err := service.Get(t.Context())
	if !errors.Is(err, want) {
		t.Fatalf("Get() error = %v, want %v", err, want)
	}
}

type readerFake struct {
	metrics []Metric
	err     error
	from    time.Time
	to      time.Time
}

func (reader *readerFake) Effectiveness(_ context.Context, from, to time.Time) ([]Metric, error) {
	reader.from, reader.to = from, to
	return reader.metrics, reader.err
}
