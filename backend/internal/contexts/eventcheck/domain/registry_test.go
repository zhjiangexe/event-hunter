package domain

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestLookupModelSourceReturnsOriginalChecksummedYAML(t *testing.T) {
	source, ok := LookupModelSource("order-return", 1)
	if !ok {
		t.Fatal("order-return@1 source not found")
	}
	if source.SourcePath != "contracts/check-models/order-return.yaml" {
		t.Fatalf("source path = %q", source.SourcePath)
	}
	if !strings.Contains(source.YAML, "model_id: order-return") || !strings.HasSuffix(source.YAML, "\n") {
		t.Fatalf("source is not the original YAML: %q", source.YAML)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(source.YAML)))
	if checksum != source.Checksum {
		t.Fatalf("source checksum = %s, want %s", checksum, source.Checksum)
	}
}

func TestLookupModelSourceRejectsUnknownVersion(t *testing.T) {
	if _, ok := LookupModelSource("order-return", 999); ok {
		t.Fatal("unknown source unexpectedly found")
	}
}
