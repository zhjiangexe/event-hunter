package postgres

import (
	"context"
	"database/sql"
)

type transactionContextKey struct{}

type databaseExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func executorFromContext(ctx context.Context, db *sql.DB) databaseExecutor {
	if transaction, ok := ctx.Value(transactionContextKey{}).(*sql.Tx); ok {
		return transaction
	}
	return db
}

func withinTransaction(ctx context.Context, db *sql.DB, operation func(context.Context) error) error {
	if _, ok := ctx.Value(transactionContextKey{}).(*sql.Tx); ok {
		return operation(ctx)
	}
	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	transactionContext := context.WithValue(ctx, transactionContextKey{}, transaction)
	if err := operation(transactionContext); err != nil {
		return err
	}
	return transaction.Commit()
}

type UnitOfWork struct {
	db *sql.DB
}

func NewUnitOfWork(db *sql.DB) *UnitOfWork {
	return &UnitOfWork{db: db}
}

func (unit *UnitOfWork) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return withinTransaction(ctx, unit.db, operation)
}
