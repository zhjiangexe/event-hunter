package journeys

import (
	"strings"
	"testing"
)

func TestGeneratedRegistryHasOneActiveDefault(t *testing.T) {
	profiles := Registry()
	defaults := 0
	for _, profile := range profiles {
		if profile.Default && profile.Status == "active" {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("active default profiles = %d, want 1", defaults)
	}
	profile, ok := Default()
	if !ok || profile.ID != "order-fulfillment" || len(profile.Milestones) != 5 {
		t.Fatalf("unexpected default profile: %#v, ok=%v", profile, ok)
	}
	if profile.SourcePath != "contracts/journeys/order-fulfillment.yaml" || len(profile.Checksum) != 64 {
		t.Fatalf("unexpected profile provenance: path=%q checksum=%q", profile.SourcePath, profile.Checksum)
	}
	if strings.Trim(profile.Checksum, "0123456789abcdef") != "" {
		t.Fatalf("checksum is not lowercase SHA-256: %q", profile.Checksum)
	}
}

func TestRegistryReturnsDefensiveCopy(t *testing.T) {
	first := Registry()
	first[0].Title = "mutated"
	first[0].Milestones[0].Label = "mutated"

	second := Registry()
	if second[0].Title == "mutated" || second[0].Milestones[0].Label == "mutated" {
		t.Fatalf("registry leaked mutable generated state: %#v", second[0])
	}
}
