package domain

import (
	"testing"
	"time"
)

func TestEligibleAppliesGraceAndUsesClosedUTCMinute(t *testing.T) {
	now := time.Date(2026, 8, 21, 4, 5, 30, 0, time.FixedZone("UTC+8", 8*60*60))
	window, err := Eligible(now, time.Minute, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom := time.Date(2026, 8, 20, 20, 2, 0, 0, time.UTC)
	wantTo := time.Date(2026, 8, 20, 20, 3, 0, 0, time.UTC)
	if !window.From.Equal(wantFrom) || !window.To.Equal(wantTo) {
		t.Fatalf("Eligible() = %s..%s, want %s..%s", window.From, window.To, wantFrom, wantTo)
	}
}

func TestNewWindowValidation(t *testing.T) {
	start := time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC)
	for name, test := range map[string]struct {
		to      time.Time
		wantErr bool
	}{
		"one minute":    {to: start.Add(time.Minute)},
		"same boundary": {to: start, wantErr: true},
		"reversed":      {to: start.Add(-time.Second), wantErr: true},
		"over 31 days":  {to: start.Add(MaxBackfillDuration + time.Second), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewWindow(start, test.to)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewWindow() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestSplitCreatesPartialFinalWindow(t *testing.T) {
	start := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	windows, err := Split(start, start.Add(150*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 3 {
		t.Fatalf("window count = %d, want 3", len(windows))
	}
	if !windows[2].From.Equal(start.Add(2*time.Minute)) || !windows[2].To.Equal(start.Add(150*time.Second)) {
		t.Fatalf("last window = %s..%s", windows[2].From, windows[2].To)
	}
	if _, err := Split(start, start.Add(time.Minute), 0); err == nil {
		t.Fatal("zero duration was accepted")
	}
}
