package postgres

import (
	"context"
	"database/sql"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/overview"
)

type OverviewReader struct {
	db *sql.DB
}

func NewOverviewReader(db *sql.DB) *OverviewReader {
	return &OverviewReader{db: db}
}

func (reader *OverviewReader) Overview(ctx context.Context, from, to time.Time) (overview.ControlPlaneSnapshot, error) {
	const aggregateQuery = `
SELECT
    count(*) FILTER (WHERE status = 'OPEN'),
    count(*) FILTER (WHERE status = 'INVESTIGATING'),
    count(*) FILTER (WHERE status = 'CLOSED'),
    count(*) FILTER (WHERE severity = 'LOW' AND status <> 'CLOSED'),
    count(*) FILTER (WHERE severity = 'MEDIUM' AND status <> 'CLOSED'),
    count(*) FILTER (WHERE severity = 'HIGH' AND status <> 'CLOSED'),
    count(*) FILTER (WHERE severity = 'CRITICAL' AND status <> 'CLOSED'),
    count(*) FILTER (WHERE created_at >= $1 AND created_at < $2),
    count(*) FILTER (WHERE closed_at >= $1 AND closed_at < $2),
    (SELECT count(*) FROM grafana_alert_receipts WHERE received_at >= $1 AND received_at < $2),
    (SELECT count(*) FROM scenario_runs WHERE accepted_at >= $1 AND accepted_at < $2 AND status = 'PASSED'),
    (SELECT count(*) FROM scenario_runs WHERE accepted_at >= $1 AND accepted_at < $2 AND status = 'FAILED'),
    (SELECT count(*) FROM scenario_runs WHERE accepted_at >= $1 AND accepted_at < $2 AND status = 'TIMED_OUT')
FROM investigation_cases`

	var snapshot overview.ControlPlaneSnapshot
	err := reader.db.QueryRowContext(ctx, aggregateQuery, from, to).Scan(
		&snapshot.Cases.Open,
		&snapshot.Cases.Investigating,
		&snapshot.Cases.Closed,
		&snapshot.Severity.Low,
		&snapshot.Severity.Medium,
		&snapshot.Severity.High,
		&snapshot.Severity.Critical,
		&snapshot.Activity.CasesCreated,
		&snapshot.Activity.CasesClosed,
		&snapshot.Activity.GrafanaAlerts,
		&snapshot.Activity.ScenarioPassed,
		&snapshot.Activity.ScenarioFailed,
		&snapshot.Activity.ScenarioTimedOut,
	)
	if err != nil {
		return overview.ControlPlaneSnapshot{}, err
	}

	const patternsQuery = `
SELECT pattern_id, count(*)
FROM pattern_findings
WHERE created_at >= $1 AND created_at < $2
GROUP BY pattern_id
ORDER BY count(*) DESC, pattern_id
LIMIT 5`
	rows, err := reader.db.QueryContext(ctx, patternsQuery, from, to)
	if err != nil {
		return overview.ControlPlaneSnapshot{}, err
	}
	defer rows.Close()
	snapshot.TopPatterns = make([]overview.CountByKey, 0, 5)
	for rows.Next() {
		var item overview.CountByKey
		if err := rows.Scan(&item.Key, &item.Count); err != nil {
			return overview.ControlPlaneSnapshot{}, err
		}
		snapshot.TopPatterns = append(snapshot.TopPatterns, item)
	}
	if err := rows.Err(); err != nil {
		return overview.ControlPlaneSnapshot{}, err
	}
	return snapshot, nil
}

var _ overview.ControlPlaneReader = (*OverviewReader)(nil)
