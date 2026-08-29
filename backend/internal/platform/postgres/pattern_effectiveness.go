package postgres

import (
	"context"
	"database/sql"
	"time"

	patterneffectiveness "event-hunter/backend/internal/contexts/investigation/application/compatibility"
)

type PatternEffectivenessReader struct {
	db *sql.DB
}

func NewPatternEffectivenessReader(db *sql.DB) *PatternEffectivenessReader {
	return &PatternEffectivenessReader{db: db}
}

func (reader *PatternEffectivenessReader) Effectiveness(ctx context.Context, from, to time.Time) ([]patterneffectiveness.Metric, error) {
	const query = `
SELECT finding.pattern_id,
       count(*),
       max(finding.created_at),
       count(DISTINCT finding.investigation_case_id),
       count(*) FILTER (WHERE feedback.status = 'CONFIRMED'),
       count(*) FILTER (WHERE feedback.status = 'FALSE_POSITIVE'),
       count(*) FILTER (WHERE feedback.status = 'NEEDS_REVIEW'),
       count(*) FILTER (WHERE feedback.finding_id IS NULL),
       count(feedback.finding_id)
FROM pattern_findings finding
LEFT JOIN pattern_finding_feedback feedback ON feedback.finding_id = finding.id
WHERE finding.created_at >= $1 AND finding.created_at < $2
GROUP BY finding.pattern_id
ORDER BY finding.pattern_id`
	rows, err := reader.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metrics := make([]patterneffectiveness.Metric, 0)
	for rows.Next() {
		var metric patterneffectiveness.Metric
		if err := rows.Scan(
			&metric.PatternID,
			&metric.HitCount,
			&metric.LastHitAt,
			&metric.InvestigationCount,
			&metric.ConfirmedCount,
			&metric.FalsePositiveCount,
			&metric.NeedsReviewCount,
			&metric.UnreviewedCount,
			&metric.ReviewedCount,
		); err != nil {
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
