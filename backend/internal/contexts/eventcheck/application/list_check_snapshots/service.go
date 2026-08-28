package list_check_snapshots

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

var ErrInvalidFilter = errors.New("invalid Check Snapshot list filter")

type Model struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Kind    string `json:"kind"`
}

// Summary is a read projection for the Saved Results list. The immutable
// Snapshot aggregate remains the source of truth and raw event payloads are
// deliberately absent from this projection.
type Summary struct {
	ID                 string          `json:"id"`
	CreatedBy          string          `json:"created_by"`
	CreatedByRole      string          `json:"created_by_role"`
	CreatedAt          time.Time       `json:"created_at"`
	EvaluationRequest  json.RawMessage `json:"evaluation_request"`
	AsOf               time.Time       `json:"as_of"`
	SourceHealthStatus string          `json:"source_health_status"`
	Model              Model           `json:"model"`
	CheckStatus        string          `json:"check_status"`
	EventCount         int             `json:"event_count"`
	FindingCount       int             `json:"finding_count"`
	LinkedCaseCount    int             `json:"linked_case_count"`
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type Filter struct {
	Identifier  string
	CheckStatus string
	PageSize    int
	Cursor      *Cursor
}

type Page struct {
	Items      []Summary `json:"items"`
	PageSize   int       `json:"page_size"`
	NextCursor *string   `json:"next_cursor"`
}

type ReadModel interface {
	ListCheckSnapshotSummaries(context.Context, Filter) ([]Summary, error)
}

type Service struct {
	readModel ReadModel
}

func NewService(readModel ReadModel) *Service {
	return &Service{readModel: readModel}
}

func (service *Service) List(ctx context.Context, filter Filter) (Page, error) {
	filter.Identifier = strings.TrimSpace(filter.Identifier)
	filter.CheckStatus = strings.ToUpper(strings.TrimSpace(filter.CheckStatus))
	if filter.PageSize == 0 {
		filter.PageSize = DefaultPageSize
	}
	if err := validateFilter(filter); err != nil {
		return Page{}, err
	}
	wantedPageSize := filter.PageSize
	filter.PageSize++ // bounded look-ahead row for keyset pagination
	rows, err := service.readModel.ListCheckSnapshotSummaries(ctx, filter)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: rows, PageSize: wantedPageSize}
	if len(rows) <= wantedPageSize {
		return page, nil
	}
	page.Items = rows[:wantedPageSize]
	last := page.Items[len(page.Items)-1]
	next, err := EncodeCursor(last.CreatedAt, last.ID)
	if err != nil {
		return Page{}, err
	}
	page.NextCursor = &next
	return page, nil
}

func EncodeCursor(createdAt time.Time, id string) (string, error) {
	if createdAt.IsZero() {
		return "", fmt.Errorf("%w: cursor timestamp is required", ErrInvalidFilter)
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("%w: cursor id is invalid", ErrInvalidFilter)
	}
	payload, err := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}{CreatedAt: createdAt.UTC().Format(time.RFC3339Nano), ID: id})
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
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidFilter)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, document.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor timestamp", ErrInvalidFilter)
	}
	if _, err := uuid.Parse(document.ID); err != nil {
		return nil, fmt.Errorf("%w: malformed cursor id", ErrInvalidFilter)
	}
	return &Cursor{CreatedAt: createdAt.UTC(), ID: document.ID}, nil
}

func validateFilter(filter Filter) error {
	if filter.PageSize < 1 || filter.PageSize > MaxPageSize {
		return fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidFilter, MaxPageSize)
	}
	if len([]rune(filter.Identifier)) > 200 {
		return fmt.Errorf("%w: identifier is too long", ErrInvalidFilter)
	}
	if filter.CheckStatus != "" && !validCheckStatus(filter.CheckStatus) {
		return fmt.Errorf("%w: unknown check status", ErrInvalidFilter)
	}
	return nil
}

func validCheckStatus(value string) bool {
	switch value {
	case "NO_DATA", "IN_PROGRESS", "CONFORMANT", "DEVIATED", "INCONCLUSIVE", "AMBIGUOUS":
		return true
	default:
		return false
	}
}
