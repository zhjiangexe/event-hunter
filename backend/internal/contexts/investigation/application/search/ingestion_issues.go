package search

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	MaxWindow       = 7 * 24 * time.Hour
)

type Kind string

const (
	KindContractValidation  Kind = "CONTRACT_VALIDATION"
	KindAdmissionQuarantine Kind = "ADMISSION_QUARANTINE"
	KindTechnicalDLQ        Kind = "TECHNICAL_DLQ"
)

var ErrInvalidFilter = errors.New("invalid ingestion issue filter")

type Issue struct {
	ID               string  `json:"id"`
	Kind             Kind    `json:"kind"`
	OccurredAt       string  `json:"occurred_at"`
	Pipeline         string  `json:"pipeline"`
	ErrorCode        string  `json:"error_code"`
	EventID          *string `json:"event_id"`
	EventType        *string `json:"event_type"`
	CorrelationID    *string `json:"correlation_id"`
	SourceTopic      *string `json:"source_topic"`
	SourcePartition  *uint32 `json:"source_partition"`
	SourceOffset     *uint64 `json:"source_offset"`
	DLQTopic         *string `json:"dlq_topic"`
	DLQPartition     *uint32 `json:"dlq_partition"`
	DLQOffset        *uint64 `json:"dlq_offset"`
	PayloadSHA256    string  `json:"payload_sha256"`
	AdmissionProfile *string `json:"admission_profile"`
	ConnectorName    *string `json:"connector_name"`
	ConnectorTask    *uint32 `json:"connector_task"`
	FailureStage     *string `json:"failure_stage"`
	ExceptionClass   *string `json:"exception_class"`
}

type Cursor struct {
	OccurredAt time.Time
	IssueID    string
}

type Filter struct {
	From          time.Time
	To            time.Time
	Kind          Kind
	ErrorCode     string
	SourceTopic   string
	CorrelationID string
	PageSize      int
	Cursor        *Cursor
}

type Page struct {
	Items      []Issue `json:"items"`
	PageSize   int     `json:"page_size"`
	NextCursor *string `json:"next_cursor"`
}

type IngestionIssueReadModel interface {
	SearchIngestionIssues(context.Context, Filter) ([]Issue, error)
}

type IngestionIssueService struct {
	readModel IngestionIssueReadModel
}

func NewIngestionIssueService(readModel IngestionIssueReadModel) *IngestionIssueService {
	return &IngestionIssueService{readModel: readModel}
}

func (service *IngestionIssueService) Search(ctx context.Context, filter Filter) (Page, error) {
	filter.ErrorCode = strings.TrimSpace(filter.ErrorCode)
	filter.SourceTopic = strings.TrimSpace(filter.SourceTopic)
	filter.CorrelationID = strings.TrimSpace(filter.CorrelationID)
	if filter.PageSize == 0 {
		filter.PageSize = DefaultPageSize
	}
	if err := validateFilter(filter); err != nil {
		return Page{}, err
	}
	rows, err := service.readModel.SearchIngestionIssues(ctx, filter)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: rows, PageSize: filter.PageSize}
	if len(rows) <= filter.PageSize {
		return page, nil
	}
	page.Items = rows[:filter.PageSize]
	last := page.Items[len(page.Items)-1]
	next, err := EncodeCursor(last.OccurredAt, last.ID)
	if err != nil {
		return Page{}, err
	}
	page.NextCursor = &next
	return page, nil
}

func ParseKind(value string) (Kind, error) {
	kind := Kind(strings.ToUpper(strings.TrimSpace(value)))
	if kind == "" {
		return "", nil
	}
	if !kind.Valid() {
		return "", fmt.Errorf("%w: unknown kind", ErrInvalidFilter)
	}
	return kind, nil
}

func (kind Kind) Valid() bool {
	return kind == KindContractValidation || kind == KindAdmissionQuarantine || kind == KindTechnicalDLQ
}

func EncodeCursor(occurredAtValue, issueID string) (string, error) {
	occurredAt, err := time.Parse(time.RFC3339Nano, occurredAtValue)
	if err != nil || strings.TrimSpace(issueID) == "" {
		return "", fmt.Errorf("encode ingestion issue cursor: %w", ErrInvalidFilter)
	}
	payload, err := json.Marshal(struct {
		OccurredAt string `json:"occurred_at"`
		IssueID    string `json:"issue_id"`
	}{OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), IssueID: issueID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeCursor(value string) (*Cursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidFilter)
	}
	var document struct {
		OccurredAt string `json:"occurred_at"`
		IssueID    string `json:"issue_id"`
	}
	if err := json.Unmarshal(payload, &document); err != nil || strings.TrimSpace(document.IssueID) == "" {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidFilter)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, document.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor timestamp", ErrInvalidFilter)
	}
	return &Cursor{OccurredAt: occurredAt.UTC(), IssueID: document.IssueID}, nil
}

func validateFilter(filter Filter) error {
	if filter.From.IsZero() || filter.To.IsZero() || !filter.To.After(filter.From) {
		return fmt.Errorf("%w: positive time window required", ErrInvalidFilter)
	}
	if filter.To.Sub(filter.From) > MaxWindow {
		return fmt.Errorf("%w: time window exceeds seven days", ErrInvalidFilter)
	}
	if filter.PageSize < 1 || filter.PageSize > MaxPageSize {
		return fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidFilter, MaxPageSize)
	}
	if filter.Kind != "" && !filter.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind", ErrInvalidFilter)
	}
	return nil
}
