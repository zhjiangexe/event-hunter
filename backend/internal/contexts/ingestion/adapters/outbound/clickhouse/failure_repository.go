package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"event-hunter/backend/internal/contexts/ingestion/domain"
)

type Config struct {
	URL      string
	Database string
	User     string
	Password string
}

type FailureRepository struct {
	config Config
	client *http.Client
}

type failureRow struct {
	FailureID       string  `json:"failure_id"`
	DLQTopic        string  `json:"dlq_topic"`
	DLQPartition    uint32  `json:"dlq_partition"`
	DLQOffset       uint64  `json:"dlq_offset"`
	SourceTopic     *string `json:"source_topic"`
	SourcePartition *uint32 `json:"source_partition"`
	SourceOffset    *uint64 `json:"source_offset"`
	ConnectorName   *string `json:"connector_name"`
	ConnectorTask   *uint32 `json:"connector_task"`
	FailureStage    *string `json:"failure_stage"`
	ExceptionClass  *string `json:"exception_class"`
	PayloadSHA256   string  `json:"payload_sha256"`
	ObservedAt      string  `json:"observed_at"`
}

func NewFailureRepository(config Config, client *http.Client) *FailureRepository {
	if client == nil {
		panic("ClickHouse HTTP client is required")
	}
	return &FailureRepository{config: config, client: client}
}

func (repository *FailureRepository) Insert(ctx context.Context, failure domain.TechnicalFailure) error {
	body, err := json.Marshal(failureRow{
		FailureID: failure.FailureID, DLQTopic: failure.DLQTopic, DLQPartition: failure.DLQPartition,
		DLQOffset: failure.DLQOffset, SourceTopic: failure.SourceTopic, SourcePartition: failure.SourcePartition,
		SourceOffset: failure.SourceOffset, ConnectorName: failure.ConnectorName, ConnectorTask: failure.ConnectorTask,
		FailureStage: failure.FailureStage, ExceptionClass: failure.ExceptionClass, PayloadSHA256: failure.PayloadSHA256,
		ObservedAt: failure.ObservedAt.UTC().Format("2006-01-02 15:04:05.000"),
	})
	if err != nil {
		return err
	}
	statement := `INSERT INTO ingestion_technical_failures
(failure_id,dlq_topic,dlq_partition,dlq_offset,source_topic,source_partition,source_offset,connector_name,connector_task,failure_stage,exception_class,payload_sha256,observed_at)
FORMAT JSONEachRow
`
	endpoint, err := repository.endpoint("")
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, io.MultiReader(strings.NewReader(statement), bytes.NewReader(body), strings.NewReader("\n")))
	if err != nil {
		return err
	}
	request.SetBasicAuth(repository.config.User, repository.config.Password)
	response, err := repository.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("ClickHouse insert returned %s", response.Status)
	}
	return nil
}

func (repository *FailureRepository) Ping(ctx context.Context) error {
	endpoint, err := repository.endpoint("/ping")
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(repository.config.User, repository.config.Password)
	response, err := repository.client.Do(request)
	if err != nil {
		return fmt.Errorf("ClickHouse: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("ClickHouse returned %s", response.Status)
	}
	return nil
}

func (repository *FailureRepository) endpoint(path string) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(repository.config.URL, "/") + path)
	if err != nil {
		return "", fmt.Errorf("parse ClickHouse URL: %w", err)
	}
	if path == "" {
		query := endpoint.Query()
		query.Set("database", repository.config.Database)
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String(), nil
}
