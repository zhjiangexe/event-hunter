package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	ingestionissues "event-hunter/backend/internal/contexts/investigation/application/search"
)

type ingestionIssueSearcher interface {
	Search(context.Context, ingestionissues.Filter) (ingestionissues.Page, error)
}

type ingestionIssuesAPI struct {
	service ingestionIssueSearcher
	now     func() time.Time
}

func (api ingestionIssuesAPI) list(writer http.ResponseWriter, request *http.Request) {
	now := api.now
	if now == nil {
		now = time.Now
	}
	query := request.URL.Query()
	fromValue := strings.TrimSpace(query.Get("from"))
	toValue := strings.TrimSpace(query.Get("to"))
	var from, to time.Time
	var err error
	if fromValue == "" && toValue == "" {
		to = now().UTC()
		from = to.Add(-72 * time.Hour)
	} else {
		from, err = time.Parse(time.RFC3339Nano, fromValue)
		if err != nil {
			writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
			return
		}
		to, err = time.Parse(time.RFC3339Nano, toValue)
		if err != nil {
			writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_TIME_WINDOW")
			return
		}
	}
	pageSize := ingestionissues.DefaultPageSize
	if value := strings.TrimSpace(query.Get("page_size")); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil {
			writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_PAGE_SIZE")
			return
		}
	}
	kind, err := ingestionissues.ParseKind(query.Get("kind"))
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_INGESTION_ISSUE_KIND")
		return
	}
	cursor, err := ingestionissues.DecodeCursor(query.Get("cursor"))
	if err != nil {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_CURSOR")
		return
	}
	page, err := api.service.Search(request.Context(), ingestionissues.Filter{
		From: from.UTC(), To: to.UTC(), Kind: kind,
		ErrorCode: query.Get("error_code"), SourceTopic: query.Get("source_topic"), CorrelationID: query.Get("correlation_id"),
		PageSize: pageSize, Cursor: cursor,
	})
	if errors.Is(err, ingestionissues.ErrInvalidFilter) {
		writeAPIError(writer, http.StatusUnprocessableEntity, "INVALID_INGESTION_ISSUE_FILTER")
		return
	}
	if err != nil {
		writeClickHouseError(writer, err, "INGESTION_ISSUES_UNAVAILABLE", "INGESTION_ISSUES_TIMEOUT")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(page)
}
