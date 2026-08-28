package postgres

import (
	"context"
	"database/sql"
)

const qualifierQueryLimit = 1001

type EventSearchQualifierRepository struct {
	db *sql.DB
}

func NewEventSearchQualifierRepository(db *sql.DB) *EventSearchQualifierRepository {
	return &EventSearchQualifierRepository{db: db}
}

// CorrelationsByAlertFingerprint resolves the stable Grafana fingerprint. The
// event search from/to window is applied later in ClickHouse; receipt time is
// intentionally not used because an alert can arrive after the event window.
func (repository *EventSearchQualifierRepository) CorrelationsByAlertFingerprint(ctx context.Context, fingerprint string) ([]string, error) {
	return queryCorrelations(ctx, repository.db, `
SELECT DISTINCT correlation_id
FROM grafana_alert_receipts
WHERE fingerprint = $1 AND correlation_id IS NOT NULL
ORDER BY correlation_id
LIMIT 1001`, fingerprint)
}

func (repository *EventSearchQualifierRepository) CorrelationsByMinimumSeverity(ctx context.Context, severity string) ([]string, error) {
	return queryCorrelations(ctx, repository.db, `
SELECT DISTINCT correlation_id
FROM investigation_cases
WHERE CASE severity
        WHEN 'LOW' THEN 1
        WHEN 'MEDIUM' THEN 2
        WHEN 'HIGH' THEN 3
        WHEN 'CRITICAL' THEN 4
      END >= CASE $1
        WHEN 'LOW' THEN 1
        WHEN 'MEDIUM' THEN 2
        WHEN 'HIGH' THEN 3
        WHEN 'CRITICAL' THEN 4
      END
ORDER BY correlation_id
LIMIT 1001`, severity)
}

type correlationQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func queryCorrelations(ctx context.Context, querier correlationQuerier, query string, value string) ([]string, error) {
	rows, err := querier.QueryContext(ctx, query, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0, qualifierQueryLimit)
	for rows.Next() {
		var correlationID string
		if err := rows.Scan(&correlationID); err != nil {
			return nil, err
		}
		result = append(result, correlationID)
	}
	return result, rows.Err()
}
