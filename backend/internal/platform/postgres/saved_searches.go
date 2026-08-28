package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"event-hunter/backend/internal/contexts/investigation/domain"
)

type SavedSearchRepository struct {
	db *sql.DB
}

func NewSavedSearchRepository(db *sql.DB) *SavedSearchRepository {
	return &SavedSearchRepository{db: db}
}

func (repository *SavedSearchRepository) Create(ctx context.Context, search domain.SavedSearch) (domain.SavedSearch, error) {
	queryState, err := json.Marshal(search.Query)
	if err != nil {
		return domain.SavedSearch{}, fmt.Errorf("encode saved search query: %w", err)
	}
	const statement = `
INSERT INTO saved_searches (owner_subject, name, target, query_state, created_at, updated_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6)
RETURNING id::text, owner_subject, name, target, query_state::text, created_at, updated_at`
	result, err := scanSavedSearch(repository.db.QueryRowContext(ctx, statement,
		search.OwnerSubject, search.Name, search.Target, queryState, search.CreatedAt, search.UpdatedAt,
	))
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return domain.SavedSearch{}, domain.ErrSavedSearchNameConflict
	}
	return result, err
}

func (repository *SavedSearchRepository) ListByOwner(ctx context.Context, ownerSubject string) ([]domain.SavedSearch, error) {
	const statement = `
SELECT id::text, owner_subject, name, target, query_state::text, created_at, updated_at
FROM saved_searches
WHERE owner_subject = $1
ORDER BY updated_at DESC, id DESC
LIMIT 100`
	rows, err := repository.db.QueryContext(ctx, statement, ownerSubject)
	if err != nil {
		return nil, fmt.Errorf("list saved searches: %w", err)
	}
	defer rows.Close()
	result := make([]domain.SavedSearch, 0)
	for rows.Next() {
		item, scanErr := scanSavedSearch(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate saved searches: %w", err)
	}
	return result, nil
}

func (repository *SavedSearchRepository) DeleteByOwner(ctx context.Context, id, ownerSubject string) error {
	result, err := repository.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE id = $1::uuid AND owner_subject = $2`, id, ownerSubject)
	if err != nil {
		return fmt.Errorf("delete saved search: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted saved search count: %w", err)
	}
	if count == 0 {
		return domain.ErrSavedSearchNotFound
	}
	return nil
}

func scanSavedSearch(row rowScanner) (domain.SavedSearch, error) {
	var id, ownerSubject, name, target, queryState string
	var createdAt, updatedAt sql.NullTime
	if err := row.Scan(&id, &ownerSubject, &name, &target, &queryState, &createdAt, &updatedAt); err != nil {
		return domain.SavedSearch{}, fmt.Errorf("scan saved search: %w", err)
	}
	var query domain.SavedSearchQuery
	if err := json.Unmarshal([]byte(queryState), &query); err != nil {
		return domain.SavedSearch{}, fmt.Errorf("decode saved search query: %w", err)
	}
	if !createdAt.Valid || !updatedAt.Valid {
		return domain.SavedSearch{}, fmt.Errorf("saved search timestamps are required")
	}
	result, err := domain.RehydrateSavedSearch(id, ownerSubject, name, domain.SavedSearchTarget(target), query, createdAt.Time, updatedAt.Time)
	if err != nil {
		return domain.SavedSearch{}, fmt.Errorf("rehydrate saved search: %w", err)
	}
	return result, nil
}
