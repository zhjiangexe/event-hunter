package postgres

import "testing"

func TestPriorityForSeverity(t *testing.T) {
	tests := map[string]string{
		"CRITICAL": "P0",
		"HIGH":     "P1",
		"MEDIUM":   "P2",
		"LOW":      "P3",
		"":         "P3",
	}

	for severity, want := range tests {
		t.Run(severity, func(t *testing.T) {
			if got := priorityForSeverity(severity); got != want {
				t.Fatalf("priorityForSeverity(%q) = %q, want %q", severity, got, want)
			}
		})
	}
}
