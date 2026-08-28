package main

import (
	"testing"
	"time"
)

func TestEligibleWindowAppliesGraceAndUsesClosedUTCMinute(t *testing.T) {
	now := time.Date(2026, 8, 21, 4, 5, 30, 0, time.FixedZone("UTC+8", 8*60*60))
	from, to := eligibleWindow(now, time.Minute, 2*time.Minute)

	wantFrom := time.Date(2026, 8, 20, 20, 2, 0, 0, time.UTC)
	wantTo := time.Date(2026, 8, 20, 20, 3, 0, 0, time.UTC)
	if !from.Equal(wantFrom) || !to.Equal(wantTo) {
		t.Fatalf("eligibleWindow() = %s..%s, want %s..%s", from, to, wantFrom, wantTo)
	}
}

func TestValidateBackfillWindow(t *testing.T) {
	start := time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC)
	for name, test := range map[string]struct {
		to      time.Time
		wantErr bool
	}{
		"one minute":    {to: start.Add(time.Minute)},
		"same boundary": {to: start, wantErr: true},
		"reversed":      {to: start.Add(-time.Second), wantErr: true},
		"over 31 days":  {to: start.Add(maxBackfillDuration + time.Second), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateBackfillWindow(start, test.to)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateBackfillWindow() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestBackfillWindowsSplitsRangeIntoTumblingMinutes(t *testing.T) {
	start := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	windows, err := backfillWindows(start, start.Add(150*time.Second), time.Minute)
	if err != nil {
		t.Fatalf("backfillWindows() error = %v", err)
	}
	if len(windows) != 3 {
		t.Fatalf("window count = %d, want 3", len(windows))
	}
	if !windows[2][0].Equal(start.Add(2*time.Minute)) || !windows[2][1].Equal(start.Add(150*time.Second)) {
		t.Fatalf("last window = %s..%s", windows[2][0], windows[2][1])
	}
	if _, err := backfillWindows(start, start.Add(time.Minute), 0); err == nil {
		t.Fatal("zero duration was accepted")
	}
}
