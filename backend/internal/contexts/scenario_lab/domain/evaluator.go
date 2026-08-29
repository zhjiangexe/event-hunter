package domain

import "slices"

func Evaluate(scenarioID string, expectedEvents []string, actual Actual) []Check {
	checks := []Check{}
	if scenarioID != "S6" {
		checks = append(checks, Check{ID: "event-sequence", Label: "實際事件序列", Expected: expectedEvents, Actual: actual.EventTypes, Passed: slices.Equal(expectedEvents, actual.EventTypes)})
	}
	switch scenarioID {
	case "S2":
		missing := !slices.Contains(actual.EventTypes, "ShipmentCreated")
		checks = append(checks, Check{ID: "shipment-missing", Label: "ShipmentCreated 缺少", Expected: true, Actual: missing, Passed: missing})
	case "S3":
		found := len(actual.DuplicateEventIDs) > 0
		checks = append(checks, Check{ID: "duplicate-event", Label: "偵測重複 event ID", Expected: true, Actual: found, Passed: found})
	case "S4":
		checks = append(checks, Check{ID: "out-of-order", Label: "偵測 Aggregate sequence 亂序", Expected: true, Actual: actual.OutOfOrder, Passed: actual.OutOfOrder})
	case "S5":
		expected := []string{"FAILED", "RETRY_SCHEDULED", "DLQ"}
		checks = append(checks, Check{ID: "processing-dlq", Label: "Processing attempts 到達 DLQ", Expected: expected, Actual: actual.ProcessingStatuses, Passed: slices.Equal(expected, actual.ProcessingStatuses)})
	case "S6":
		passed := actual.EventCount == 0 && slices.Contains(actual.IngestionFailureTypes, "SCHEMA_VIOLATION")
		checks = append(checks, Check{ID: "schema-violation-dlq", Label: "違規事件只進 ingestion failure", Expected: "SCHEMA_VIOLATION and 0 timeline events", Actual: map[string]any{"failure_types": actual.IngestionFailureTypes, "event_count": actual.EventCount}, Passed: passed})
	case "S7":
		checks = append(checks, Check{ID: "event-delay", Label: "最大事件延遲至少十分鐘", Expected: int64(600000), Actual: actual.MaxEventDelayMS, Passed: actual.MaxEventDelayMS >= 600000})
	}
	return checks
}

func ChecksPassed(checks []Check) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}
