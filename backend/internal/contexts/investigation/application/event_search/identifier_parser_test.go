package eventsearch

import "testing"

func TestIdentifyInputUsesExplicitPrefixWithoutGuessing(t *testing.T) {
	result := IdentifyInput(" trace:0123456789abcdef0123456789abcdef ")
	if result.Status != IdentificationIdentified || result.NormalizedInput != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Type != IdentifierTraceID {
		t.Fatalf("unexpected candidates: %#v", result.Candidates)
	}
}

func TestIdentifyInputReturnsCandidatesForOpaqueValue(t *testing.T) {
	result := IdentifyInput("ORDER-2001")
	if result.Status != IdentificationAmbiguous {
		t.Fatalf("expected ambiguous result, got %#v", result)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("expected three opaque candidates, got %#v", result.Candidates)
	}
}

func TestIdentifyInputReturnsTraceAndOpaqueCandidatesForRawTraceID(t *testing.T) {
	result := IdentifyInput("0123456789abcdef0123456789abcdef")
	if result.Status != IdentificationAmbiguous || result.Candidates[0].Type != IdentifierTraceID {
		t.Fatalf("raw trace-shaped values must remain user-confirmed: %#v", result)
	}
}

func TestIdentifyInputRejectsInvalidAndAllZeroTrace(t *testing.T) {
	for _, input := range []string{"", "x", "trace:00000000000000000000000000000000", "contains spaces"} {
		if result := IdentifyInput(input); result.Status != IdentificationInvalid {
			t.Fatalf("expected %q to be invalid, got %#v", input, result)
		}
	}
}
