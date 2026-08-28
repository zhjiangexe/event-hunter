package postgres

import (
	"context"
	"database/sql"
	"time"

	patterneffectiveness "event-hunter/backend/internal/contexts/investigation/application/pattern_effectiveness"
)

type PatternEffectivenessReader struct {
	db *sql.DB
}

func NewPatternEffectivenessReader(db *sql.DB) *PatternEffectivenessReader {
	return &PatternEffectivenessReader{db: db}
}

func (reader *PatternEffectivenessReader) Effectiveness(ctx context.Context, from, to time.Time) ([]patterneffectiveness.Metric, error) {
	const query = `
SELECT pattern_id, count(*), max(created_at), count(DISTINCT investigation_case_id)
FROM pattern_findings
WHERE created_at >= $1 AND created_at < $2
GROUP BY pattern_id
ORDER BY pattern_id`
	rows, err := reader.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metrics := make([]patterneffectiveness.Metric, 0)
	for rows.Next() {
		var metric patterneffectiveness.Metric
		if err := rows.Scan(&metric.PatternID, &metric.HitCount, &metric.LastHitAt, &metric.InvestigationCount); err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return metrics, nil
}

var _ patterneffectiveness.Reader = (*PatternEffectivenessReader)(nil)
