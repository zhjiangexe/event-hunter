package emission

import (
	"event-hunter/backend/internal/contexts/scenario_lab/ports"
	"time"
)

// Builder owns the synthetic, replayable canonical-event fixtures used by
// Scenario Lab. It is an outbound adapter because these envelopes model test
// traffic rather than Scenario Lab domain state.
type Builder struct{}

func (Builder) BuildScenario(scenarioID, correlationID, traceID string, now time.Time) ([]ports.Message, error) {
	values, err := BuildEmissions(scenarioID, correlationID, traceID, now)
	if err != nil {
		return nil, err
	}
	result := make([]ports.Message, 0, len(values))
	for _, value := range values {
		eventID, eventType := "", ""
		var source *ports.AttemptSource
		if value.Envelope != nil {
			eventID = value.Envelope.EventID
			eventType = value.Envelope.EventType
			source = &ports.AttemptSource{EventID: eventID, EventType: eventType, CorrelationID: value.Envelope.CorrelationID, TraceID: value.Envelope.TraceID}
		}
		result = append(result, ports.Message{Topic: value.Topic, Key: value.Key, Value: value.Value, EventID: eventID, EventType: eventType, AttemptSource: source})
	}
	return result, nil
}
func (Builder) BuildAttempts(source ports.AttemptSource, record ports.PublishedRecord, now time.Time) ([]ports.Message, error) {
	envelope := AttemptEnvelope{EventID: source.EventID, EventType: source.EventType, CorrelationID: source.CorrelationID, TraceID: source.TraceID}
	values, err := BuildAttemptEmissions(envelope, PublishedRecord{Partition: record.Partition, Offset: record.Offset}, now)
	if err != nil {
		return nil, err
	}
	result := make([]ports.Message, 0, len(values))
	for _, value := range values {
		result = append(result, ports.Message{Topic: value.Topic, Key: value.Key, Value: value.Value})
	}
	return result, nil
}
