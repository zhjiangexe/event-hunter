package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kmsg"
)

func TestRedpandaConnectRequiresEveryComponentConnected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"statuses":[{"path":"input","connected":true},{"path":"output","connected":false}]}`))
	}))
	defer server.Close()

	if err := RedpandaConnect("ingestion", server.URL, server.Client()).Check(t.Context()); err == nil {
		t.Fatal("disconnected output was accepted as ready")
	}
}

func TestKafkaConnectConnectorRequiresRunningTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"connector":{"state":"RUNNING"},"tasks":[{"id":0,"state":"FAILED"}]}`))
	}))
	defer server.Close()

	if err := KafkaConnectConnector("ingestion", server.URL, server.Client()).Check(t.Context()); err == nil {
		t.Fatal("failed Kafka Connect task was accepted as ready")
	}
}

func TestKafkaConnectConnectorAcceptsRunningConnectorAndTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"connector":{"state":"RUNNING"},"tasks":[{"id":0,"state":"RUNNING"}]}`))
	}))
	defer server.Close()

	if err := KafkaConnectConnector("ingestion", server.URL, server.Client()).Check(t.Context()); err != nil {
		t.Fatalf("running Kafka Connect connector rejected: %v", err)
	}
}

func TestValidateConsumerGroupRequiresStableActiveMember(t *testing.T) {
	group := kmsg.NewDescribeGroupsResponseGroup()
	group.Group = "event-hunter-forensics-ingestion-v1"
	group.State = "Stable"
	response := kmsg.NewPtrDescribeGroupsResponse()
	response.Groups = []kmsg.DescribeGroupsResponseGroup{group}
	if err := validateConsumerGroup(response, group.Group); err == nil {
		t.Fatal("group without members was accepted as ready")
	}

	group.Members = []kmsg.DescribeGroupsResponseGroupMember{{MemberID: "consumer-1"}}
	response.Groups = []kmsg.DescribeGroupsResponseGroup{group}
	if err := validateConsumerGroup(response, group.Group); err != nil {
		t.Fatalf("stable group with member rejected: %v", err)
	}
}

func TestHandlerReturnsServiceUnavailableWhenCapabilityIsMissing(t *testing.T) {
	handler := Handler{Timeout: time.Second, Probes: []Probe{
		probe{name: "postgres", check: func(context.Context) error { return nil }},
		probe{name: "ingestion_group", check: func(context.Context) error { return context.DeadlineExceeded }},
	}}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"ingestion_group":"not_ready"`) {
		t.Fatalf("response does not expose failed capability: %s", response.Body.String())
	}
}
