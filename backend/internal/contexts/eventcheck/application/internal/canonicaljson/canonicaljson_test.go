package canonicaljson

import (
	"encoding/json"
	"testing"
)

func TestGoldenEventSetVector(t *testing.T) {
	var document any
	if err := json.Unmarshal([]byte(`{
          "scope_contract_version": 1,
          "included_event_metadata_and_payload_checksums": [{
            "event_id": "EVT-HASH-01",
            "occurred_at": "2026-08-28T00:00:00Z",
            "payload_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
            "ordinal": 0
          }],
          "excluded_event_metadata_and_reasons": [],
          "relationship_edges": [{
            "ordinal": 0,
            "from_event_id": null,
            "to_event_id": "EVT-HASH-01",
            "relation_type": "SEED"
          }]
        }`), &document); err != nil {
		t.Fatal(err)
	}
	hash, err := SHA256(document)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "c8eaafeb7a58c9bc591100654839b3a200ca7d272e88dcf80b2da8d4966c8580" {
		t.Fatalf("hash = %s", hash)
	}
}

func TestGoldenEvaluationVector(t *testing.T) {
	var document any
	if err := json.Unmarshal([]byte(`{
          "evaluation_contract_version": 1,
          "normalized_request": {
            "identifier": {"type": "CORRELATION_ID", "value": "ORDER-HASH-01"},
            "from": "2026-08-28T00:00:00Z",
            "to": "2026-08-28T00:10:00Z",
            "model": {"id": "order-fulfillment", "version": 2}
          },
          "model_ref_and_checksum": {
            "id": "order-fulfillment",
            "version": 2,
            "checksum": "1111111111111111111111111111111111111111111111111111111111111111"
          },
          "event_set_hash": "c8eaafeb7a58c9bc591100654839b3a200ca7d272e88dcf80b2da8d4966c8580",
          "source_health": {"status": "HEALTHY", "truncated": false},
          "deterministic_result": {
            "check_status": "IN_PROGRESS",
            "business_outcome": null,
            "expectations": []
          }
        }`), &document); err != nil {
		t.Fatal(err)
	}
	hash, err := SHA256(document)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "bc8a8e4bc3476060170f2901e6363cbe19da58a4530f4440e524570d4e328cf5" {
		t.Fatalf("hash = %s", hash)
	}
}

func TestCanonicalizesFloatingPointAndNegativeZero(t *testing.T) {
	canonical, err := Marshal(map[string]any{"rate": 0.5, "zero": -0.0})
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != `{"rate":0.5,"zero":0}` {
		t.Fatalf("canonical = %s", canonical)
	}
}
