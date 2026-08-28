package scenariolab

import (
	"fmt"
	"testing"
)

func TestCatalogHasStableS1ThroughS14(t *testing.T) {
	items := Catalog()
	if len(items) != 14 {
		t.Fatalf("catalog length = %d, want 14", len(items))
	}
	for index, item := range items {
		want := fmt.Sprintf("S%d", index+1)
		if item.ID != want {
			t.Fatalf("catalog[%d].ID = %s, want %s", index, item.ID, want)
		}
		live := item.ID == "S1" || item.ID == "S12" || item.ID == "S13" || item.ID == "S14"
		if live && (item.ExecutionMode != LiveServices || item.Synthetic) {
			t.Fatalf("%s must use live services and not be synthetic", item.ID)
		}
		if !live && (item.ExecutionMode != LabInjection || !item.Synthetic) {
			t.Fatalf("%s must use synthetic lab injection", item.ID)
		}
	}
}
