package event

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Envelope is the canonical event contract shared by the three demo services.
// Payload remains an object so each event schema can validate its own business fields.
type Envelope struct {
	EventID       string         `json:"eventId"`
	EventType     string         `json:"eventType"`
	EventVersion  int            `json:"eventVersion"`
	OccurredAt    time.Time      `json:"occurredAt"`
	Producer      string         `json:"producer"`
	CorrelationID string         `json:"correlationId"`
	CausationID   *string        `json:"causationId"`
	TraceID       *string        `json:"traceId"`
	AggregateType string         `json:"aggregateType"`
	AggregateID   string         `json:"aggregateId"`
	Sequence      uint64         `json:"sequence"`
	Payload       map[string]any `json:"payload"`
}

const (
	ProfileNormal            = "NORMAL"
	ProfilePaymentFailed     = "PAYMENT_FAILED"
	ProfileShipmentDelivered = "SHIPMENT_DELIVERED"
	ProfileReturnRefund      = "RETURN_REFUND"
)

func NormalizeSimulationProfile(value string) (string, error) {
	if value == "" {
		return ProfileNormal, nil
	}
	switch value {
	case ProfileNormal, ProfilePaymentFailed, ProfileShipmentDelivered, ProfileReturnRefund:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported simulation profile %q", value)
	}
}

func Decode(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("decode event envelope: %w", err)
	}
	if envelope.EventType == "" || envelope.EventID == "" || envelope.CorrelationID == "" {
		return Envelope{}, fmt.Errorf("event identity fields are required")
	}
	return envelope, nil
}

func NewEnvelope(eventType, producer, correlationID, aggregateType, aggregateID string, sequence uint64, causationID, traceID *string, payload map[string]any) (Envelope, error) {
	if eventType == "" || producer == "" || correlationID == "" || aggregateType == "" || aggregateID == "" {
		return Envelope{}, fmt.Errorf("event identity fields are required")
	}
	eventID, err := newEventID()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID: eventID, EventType: eventType, EventVersion: 1, OccurredAt: time.Now().UTC(),
		Producer: producer, CorrelationID: correlationID, CausationID: causationID,
		TraceID: traceID, AggregateType: aggregateType, AggregateID: aggregateID,
		Sequence: sequence, Payload: payload,
	}, nil
}

func (envelope Envelope) JSON() ([]byte, error) {
	return json.Marshal(envelope)
}

func newEventID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate event ID: %w", err)
	}
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
