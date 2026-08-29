package compatibility

import "testing"

func TestListReturnsCompiledJourneyProfiles(t *testing.T) {
	profiles := NewJourneyProfileQueries().List()
	if len(profiles) == 0 {
		t.Fatal("expected at least one compiled journey profile")
	}
	if profiles[0].ID != "order-fulfillment" || !profiles[0].Default || profiles[0].Status != "active" {
		t.Fatalf("unexpected first profile: %#v", profiles[0])
	}
}
