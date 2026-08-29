package application

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
	DefaultSnapshotPageSize = 20
	MaxSnapshotPageSize     = 100
)

var ErrInvalidSnapshotFilter = errors.New("invalid Check Snapshot list filter")

type SnapshotModelSummary struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Kind    string `json:"kind"`
}

// Summary is a read projection for the Saved Results list. The immutable
// Snapshot aggregate remains the source of truth and raw event payloads are
// deliberately absent from this projection.
type SnapshotSummary struct {
	ID                 string               `json:"id"`
	CreatedBy          string               `json:"created_by"`
	CreatedByRole      string               `json:"created_by_role"`
	CreatedAt          time.Time            `json:"created_at"`
	EvaluationRequest  json.RawMessage      `json:"evaluation_request"`
	AsOf               time.Time            `json:"as_of"`
	SourceHealthStatus string               `json:"source_health_status"`
	Model              SnapshotModelSummary `json:"model"`
	CheckStatus        string               `json:"check_status"`
	EventCount         int                  `json:"event_count"`
	FindingCount       int                  `json:"finding_count"`
	LinkedCaseCount    int                  `json:"linked_case_count"`
}

type SnapshotCursor struct {
	CreatedAt time.Time
	ID        string
}

type SnapshotListFilter struct {
	Identifier  string
	CheckStatus string
	PageSize    int
	Cursor      *SnapshotCursor
}

type SnapshotPage struct {
	Items      []SnapshotSummary `json:"items"`
	PageSize   int               `json:"page_size"`
	NextCursor *string           `json:"next_cursor"`
}

type SnapshotListReadModel interface {
	ListCheckSnapshotSummaries(context.Context, SnapshotListFilter) ([]SnapshotSummary, error)
}

type ListSnapshotsHandler struct {
	readModel SnapshotListReadModel
}

func NewListSnapshotsHandler(readModel SnapshotListReadModel) *ListSnapshotsHandler {
	return &ListSnapshotsHandler{readModel: readModel}
}

func (service *ListSnapshotsHandler) List(ctx context.Context, filter SnapshotListFilter) (SnapshotPage, error) {
	filter.Identifier = strings.TrimSpace(filter.Identifier)
	filter.CheckStatus = strings.ToUpper(strings.TrimSpace(filter.CheckStatus))
	if filter.PageSize == 0 {
		filter.PageSize = DefaultSnapshotPageSize
	}
	if err := validateFilter(filter); err != nil {
		return SnapshotPage{}, err
	}
	wantedPageSize := filter.PageSize
	filter.PageSize++ // bounded look-ahead row for keyset pagination
	rows, err := service.readModel.ListCheckSnapshotSummaries(ctx, filter)
	if err != nil {
		return SnapshotPage{}, err
	}
	page := SnapshotPage{Items: rows, PageSize: wantedPageSize}
	if len(rows) <= wantedPageSize {
		return page, nil
	}
	page.Items = rows[:wantedPageSize]
	last := page.Items[len(page.Items)-1]
	next, err := EncodeSnapshotCursor(last.CreatedAt, last.ID)
	if err != nil {
		return SnapshotPage{}, err
	}
	page.NextCursor = &next
	return page, nil
}

func EncodeSnapshotCursor(createdAt time.Time, id string) (string, error) {
	if createdAt.IsZero() {
		return "", fmt.Errorf("%w: cursor timestamp is required", ErrInvalidSnapshotFilter)
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("%w: cursor id is invalid", ErrInvalidSnapshotFilter)
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

func DecodeSnapshotCursor(value string) (*SnapshotCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidSnapshotFilter)
	}
	var document struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidSnapshotFilter)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, document.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor timestamp", ErrInvalidSnapshotFilter)
	}
	if _, err := uuid.Parse(document.ID); err != nil {
		return nil, fmt.Errorf("%w: malformed cursor id", ErrInvalidSnapshotFilter)
	}
	return &SnapshotCursor{CreatedAt: createdAt.UTC(), ID: document.ID}, nil
}

func validateFilter(filter SnapshotListFilter) error {
	if filter.PageSize < 1 || filter.PageSize > MaxSnapshotPageSize {
		return fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidSnapshotFilter, MaxSnapshotPageSize)
	}
	if len([]rune(filter.Identifier)) > 200 {
		return fmt.Errorf("%w: identifier is too long", ErrInvalidSnapshotFilter)
	}
	if filter.CheckStatus != "" && !validCheckStatus(filter.CheckStatus) {
		return fmt.Errorf("%w: unknown check status", ErrInvalidSnapshotFilter)
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
