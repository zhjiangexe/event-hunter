package scenariolab

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrRunNotFound = errors.New("scenario run not found")

type Repository interface {
	Create(context.Context, RunRecord) error
	Get(context.Context, string) (RunRecord, error)
	List(context.Context, RunFilter) ([]RunRecord, error)
	MarkRunning(context.Context, string, time.Time) error
	Complete(context.Context, string, string, Actual, []Check, *string, time.Time) error
}

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (repository *PostgresRepository) Create(ctx context.Context, run RunRecord) error {
	expected, _ := json.Marshal(run.ExpectedEventTypes)
	actual, _ := json.Marshal(run.Actual)
	checks, _ := json.Marshal(run.Checks)
	_, err := repository.db.ExecContext(ctx, `INSERT INTO scenario_runs
        (id,scenario_id,scenario_name,execution_mode,synthetic,correlation_id,trace_id,status,expected_event_types,actual,checks,error_message,accepted_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11::jsonb,$12,$13)`,
		run.RunID, run.ScenarioID, run.ScenarioName, run.ExecutionMode, run.Synthetic, run.CorrelationID,
		run.TraceID, run.Status, expected, actual, checks, run.Error, run.AcceptedAt)
	if err != nil {
		return fmt.Errorf("insert scenario run: %w", err)
	}
	return nil
}

func (repository *PostgresRepository) Get(ctx context.Context, id string) (RunRecord, error) {
	run, err := scanRun(repository.db.QueryRowContext(ctx, `SELECT id,scenario_id,scenario_name,execution_mode,synthetic,
        correlation_id,trace_id,status,expected_event_types,actual,checks,error_message,accepted_at,started_at,completed_at
		FROM scenario_runs WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return RunRecord{}, ErrRunNotFound
	}
	return run, nil
}

func (repository *PostgresRepository) List(ctx context.Context, filter RunFilter) ([]RunRecord, error) {
	query := `SELECT id,scenario_id,scenario_name,execution_mode,synthetic,
        correlation_id,trace_id,status,expected_event_types,actual,checks,error_message,accepted_at,started_at,completed_at
        FROM scenario_runs WHERE 1=1`
	args := make([]any, 0, 5)
	for _, candidate := range []struct {
		column string
		value  string
	}{
		{column: "scenario_id", value: filter.ScenarioID},
		{column: "status", value: filter.Status},
		{column: "execution_mode", value: filter.ExecutionMode},
	} {
		if candidate.value == "" {
			continue
		}
		args = append(args, candidate.value)
		query += fmt.Sprintf(" AND %s=$%d", candidate.column, len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		query += fmt.Sprintf(" AND accepted_at >= $%d", len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		query += fmt.Sprintf(" AND accepted_at < $%d", len(args))
	}
	query += fmt.Sprintf(" ORDER BY accepted_at DESC,id DESC LIMIT %d", filter.PageSize)
	rows, err := repository.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list scenario runs: %w", err)
	}
	defer rows.Close()
	runs := make([]RunRecord, 0, filter.PageSize)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

type runScanner interface {
	Scan(...any) error
}

func scanRun(scanner runScanner) (RunRecord, error) {
	var run RunRecord
	var expected, actual, checks []byte
	err := scanner.Scan(
		&run.RunID, &run.ScenarioID, &run.ScenarioName, &run.ExecutionMode, &run.Synthetic,
		&run.CorrelationID, &run.TraceID, &run.Status, &expected, &actual, &checks, &run.Error,
		&run.AcceptedAt, &run.StartedAt, &run.CompletedAt,
	)
	if err != nil {
		return RunRecord{}, fmt.Errorf("read scenario run: %w", err)
	}
	if err := json.Unmarshal(expected, &run.ExpectedEventTypes); err != nil {
		return RunRecord{}, fmt.Errorf("decode expected events: %w", err)
	}
	if err := json.Unmarshal(actual, &run.Actual); err != nil {
		return RunRecord{}, fmt.Errorf("decode actual: %w", err)
	}
	if err := json.Unmarshal(checks, &run.Checks); err != nil {
		return RunRecord{}, fmt.Errorf("decode checks: %w", err)
	}
	return run, nil
}

func (repository *PostgresRepository) MarkRunning(ctx context.Context, id string, started time.Time) error {
	_, err := repository.db.ExecContext(ctx, "UPDATE scenario_runs SET status='RUNNING',started_at=$2 WHERE id=$1", id, started)
	return err
}

func (repository *PostgresRepository) Complete(ctx context.Context, id, status string, actual Actual, checks []Check, message *string, completed time.Time) error {
	actualJSON, _ := json.Marshal(actual)
	checksJSON, _ := json.Marshal(checks)
	_, err := repository.db.ExecContext(ctx, `UPDATE scenario_runs SET status=$2,actual=$3::jsonb,checks=$4::jsonb,
		error_message=$5,completed_at=$6,trace_id=COALESCE($7,trace_id) WHERE id=$1`,
		id, status, actualJSON, checksJSON, message, completed, actual.TraceID)
	return err
}
