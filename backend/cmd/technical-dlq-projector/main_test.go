package main

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestSummarizeRecordKeepsSafeKafkaConnectContextOnly(t *testing.T) {
	partition := make([]byte, 4)
	binary.BigEndian.PutUint32(partition, 3)
	offset := make([]byte, 8)
	binary.BigEndian.PutUint64(offset, 42)
	record := &kgo.Record{
		Topic: "event-hunter.poc-clickhouse-sink.dlq", Partition: 1, Offset: 9,
		Timestamp: time.Date(2026, 8, 27, 2, 3, 4, 0, time.UTC), Value: []byte(`{"cardNumber":"4111111111111111"}`),
		Headers: []kgo.RecordHeader{
			{Key: "__connect.errors.topic", Value: []byte("order.events")},
			{Key: "__connect.errors.partition", Value: partition},
			{Key: "__connect.errors.offset", Value: offset},
			{Key: "__connect.errors.connector.name", Value: []byte("event-hunter-poc-raw-landing")},
			{Key: "__connect.errors.task.id", Value: []byte("0")},
			{Key: "__connect.errors.stage", Value: []byte("VALUE_CONVERTER")},
			{Key: "__connect.errors.exception.class.name", Value: []byte("org.example.BadValue")},
			{Key: "__connect.errors.exception.message", Value: []byte("secret message")},
			{Key: "__connect.errors.exception.stacktrace", Value: []byte("secret stack")},
		},
	}
	failure := summarizeRecord(record)
	if failure.SourceTopic == nil || *failure.SourceTopic != "order.events" || failure.SourcePartition == nil || *failure.SourcePartition != 3 || failure.SourceOffset == nil || *failure.SourceOffset != 42 {
		t.Fatalf("source transport = %#v", failure)
	}
	if failure.FailureStage == nil || *failure.FailureStage != "VALUE_CONVERTER" || failure.ExceptionClass == nil || *failure.ExceptionClass != "org.example.BadValue" {
		t.Fatalf("safe failure classification = %#v", failure)
	}
	if len(failure.FailureID) != 64 || len(failure.PayloadSHA256) != 64 || failure.PayloadSHA256 == string(record.Value) {
		t.Fatalf("hashes = %#v", failure)
	}
}

func TestHeaderNumbersAcceptKafkaConnectTextFallback(t *testing.T) {
	record := &kgo.Record{Headers: []kgo.RecordHeader{
		{Key: "partition", Value: []byte("7")},
		{Key: "offset", Value: []byte("9001")},
	}}
	if value := headerUint32(record, "partition"); value == nil || *value != 7 {
		t.Fatalf("partition = %v", value)
	}
	if value := headerUint64(record, "offset"); value == nil || *value != 9001 {
		t.Fatalf("offset = %v", value)
	}
}
