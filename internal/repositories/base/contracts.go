package base

import (
	"context"
	"database/sql"
)

type Scanner interface {
	Scan(dest ...any) error
}

type DB interface {
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	Prepare(query string) (*sql.Stmt, error)

	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Exec(query string, args ...any) (sql.Result, error)

	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	Query(query string, args ...any) (*sql.Rows, error)

	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryRow(query string, args ...any) *sql.Row
}

type TransactionManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) (err error)
}
