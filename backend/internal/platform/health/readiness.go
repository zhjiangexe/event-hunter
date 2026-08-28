package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// Probe verifies one capability required before the API can serve trustworthy
// investigation results. Liveness is intentionally separate from readiness.
type Probe interface {
	Name() string
	Check(context.Context) error
}

type probe struct {
	name  string
	check func(context.Context) error
}

func (candidate probe) Name() string                    { return candidate.name }
func (candidate probe) Check(ctx context.Context) error { return candidate.check(ctx) }

func Database(name string, db *sql.DB) Probe {
	return probe{name: name, check: db.PingContext}
}

func HTTPStatus(name, endpoint string, client *http.Client) Probe {
	if client == nil {
		client = http.DefaultClient
	}
	return probe{name: name, check: func(ctx context.Context) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("dependency returned %s", response.Status)
		}
		return nil
	}}
}

func RedpandaConnect(name, endpoint string, client *http.Client) Probe {
	if client == nil {
		client = http.DefaultClient
	}
	return probe{name: name, check: func(ctx context.Context) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("connect returned %s", response.Status)
		}
		var document struct {
			Statuses []struct {
				Path      string `json:"path"`
				Connected bool   `json:"connected"`
			} `json:"statuses"`
		}
		if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
			return fmt.Errorf("decode connect readiness: %w", err)
		}
		if len(document.Statuses) == 0 {
			return errors.New("connect readiness has no component statuses")
		}
		for _, status := range document.Statuses {
			if !status.Connected {
				return fmt.Errorf("connect component %s is disconnected", status.Path)
			}
		}
		return nil
	}}
}

// KafkaConnectConnector verifies the connector and every declared task, not
// merely the Kafka Connect worker process. This is the candidate domain-event
// ingestion capability used by the ClickHouse-first path.
func KafkaConnectConnector(name, endpoint string, client *http.Client) Probe {
	if client == nil {
		client = http.DefaultClient
	}
	return probe{name: name, check: func(ctx context.Context) error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("kafka connect returned %s", response.Status)
		}
		var document struct {
			Connector struct {
				State string `json:"state"`
			} `json:"connector"`
			Tasks []struct {
				ID    int    `json:"id"`
				State string `json:"state"`
			} `json:"tasks"`
		}
		if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
			return fmt.Errorf("decode kafka connect status: %w", err)
		}
		if !strings.EqualFold(document.Connector.State, "RUNNING") {
			return fmt.Errorf("kafka connector state is %s", document.Connector.State)
		}
		if len(document.Tasks) == 0 {
			return errors.New("kafka connector has no tasks")
		}
		for _, task := range document.Tasks {
			if !strings.EqualFold(task.State, "RUNNING") {
				return fmt.Errorf("kafka connector task %d state is %s", task.ID, task.State)
			}
		}
		return nil
	}}
}

func InvalidConfiguration(name, message string) Probe {
	return probe{name: name, check: func(context.Context) error { return errors.New(message) }}
}

func KafkaConsumerGroup(name string, brokers []string, groupID string) Probe {
	return probe{name: name, check: func(ctx context.Context) error {
		client, err := kgo.NewClient(
			kgo.SeedBrokers(brokers...),
			kgo.ClientID("event-hunter-readiness"),
			kgo.RequestTimeoutOverhead(time.Second),
		)
		if err != nil {
			return fmt.Errorf("create Kafka readiness client: %w", err)
		}
		defer client.Close()

		request := kmsg.NewPtrDescribeGroupsRequest()
		request.Groups = []string{groupID}
		response, err := request.RequestWith(ctx, client)
		if err != nil {
			return fmt.Errorf("describe Kafka consumer group: %w", err)
		}
		return validateConsumerGroup(response, groupID)
	}}
}

func validateConsumerGroup(response *kmsg.DescribeGroupsResponse, groupID string) error {
	if response == nil {
		return errors.New("kafka group response is empty")
	}
	for _, group := range response.Groups {
		if group.Group != groupID {
			continue
		}
		if group.ErrorCode != 0 {
			return fmt.Errorf("kafka group %s returned error code %d", groupID, group.ErrorCode)
		}
		if !strings.EqualFold(group.State, "Stable") {
			return fmt.Errorf("kafka group %s state is %s", groupID, group.State)
		}
		if len(group.Members) == 0 {
			return fmt.Errorf("kafka group %s has no active members", groupID)
		}
		return nil
	}
	return fmt.Errorf("kafka group %s was not found", groupID)
}

type Handler struct {
	Probes  []Probe
	Timeout time.Duration
}

func (handler Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	timeout := handler.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()

	checks := make(map[string]string, len(handler.Probes))
	var mutex sync.Mutex
	var waitGroup sync.WaitGroup
	for _, candidate := range handler.Probes {
		candidate := candidate
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			status := "ready"
			if err := candidate.Check(ctx); err != nil {
				status = "not_ready"
				slog.WarnContext(ctx, "readiness probe failed", "probe", candidate.Name(), "error", err)
			}
			mutex.Lock()
			checks[candidate.Name()] = status
			mutex.Unlock()
		}()
	}
	waitGroup.Wait()

	status := "ready"
	statusCode := http.StatusOK
	for _, checkStatus := range checks {
		if checkStatus != "ready" {
			status = "not_ready"
			statusCode = http.StatusServiceUnavailable
			break
		}
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(map[string]any{"status": status, "checks": checks})
}
