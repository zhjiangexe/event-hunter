package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Header struct {
	Key   string
	Value []byte
}

type DLQRecord struct {
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
	Payload   []byte
	Headers   []Header
}

type TechnicalFailure struct {
	FailureID       string
	DLQTopic        string
	DLQPartition    uint32
	DLQOffset       uint64
	SourceTopic     *string
	SourcePartition *uint32
	SourceOffset    *uint64
	ConnectorName   *string
	ConnectorTask   *uint32
	FailureStage    *string
	ExceptionClass  *string
	PayloadSHA256   string
	ObservedAt      time.Time
}

func Summarize(record DLQRecord, now time.Time) TechnicalFailure {
	payloadHash := sha256.Sum256(record.Payload)
	failureHash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", record.Topic, record.Partition, record.Offset)))
	observedAt := record.Timestamp.UTC()
	if observedAt.IsZero() {
		observedAt = now.UTC()
	}
	return TechnicalFailure{
		FailureID:       hex.EncodeToString(failureHash[:]),
		DLQTopic:        record.Topic,
		DLQPartition:    uint32(record.Partition),
		DLQOffset:       uint64(record.Offset),
		SourceTopic:     headerString(record.Headers, "__connect.errors.topic", 249),
		SourcePartition: headerUint32(record.Headers, "__connect.errors.partition"),
		SourceOffset:    headerUint64(record.Headers, "__connect.errors.offset"),
		ConnectorName:   headerString(record.Headers, "__connect.errors.connector.name", 255),
		ConnectorTask:   headerUint32(record.Headers, "__connect.errors.task.id"),
		FailureStage:    headerString(record.Headers, "__connect.errors.stage", 255),
		ExceptionClass:  headerString(record.Headers, "__connect.errors.exception.class.name", 512),
		PayloadSHA256:   hex.EncodeToString(payloadHash[:]),
		ObservedAt:      observedAt,
	}
}

func headerString(headers []Header, key string, maxLength int) *string {
	value := headerValue(headers, key)
	if len(value) == 0 {
		return nil
	}
	text := strings.TrimSpace(string(value))
	if text == "" {
		return nil
	}
	if len(text) > maxLength {
		text = text[:maxLength]
	}
	return &text
}

func headerUint32(headers []Header, key string) *uint32 {
	value := headerValue(headers, key)
	if len(value) == 4 {
		parsed := binary.BigEndian.Uint32(value)
		return &parsed
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 32)
	if err != nil {
		return nil
	}
	result := uint32(parsed)
	return &result
}

func headerUint64(headers []Header, key string) *uint64 {
	value := headerValue(headers, key)
	if len(value) == 8 {
		parsed := binary.BigEndian.Uint64(value)
		return &parsed
	}
	parsed, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func headerValue(headers []Header, key string) []byte {
	for index := len(headers) - 1; index >= 0; index-- {
		if headers[index].Key == key {
			return headers[index].Value
		}
	}
	return nil
}
