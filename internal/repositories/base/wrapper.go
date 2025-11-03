package base

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	trsql "github.com/avito-tech/go-transaction-manager/drivers/sql/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

// TransactionGetter provides methods to retrieve either an active transaction from context
// or fall back to the default database connection.
type TransactionGetter interface {
	// DefaultTrOrDB returns an active transaction from the context using the default transaction key,
	// or the provided database connection if no transaction is found.
	DefaultTrOrDB(ctx context.Context, db trsql.Tr) trsql.Tr
	// TrOrDB returns an active transaction from the context using the specified transaction key,
	// or the provided database connection if no transaction is found.
	TrOrDB(ctx context.Context, key trm.CtxKey, db trsql.Tr) trsql.Tr
}

// DBTxWrapper wraps a sql.DB connection and provides transaction-aware database operations.
// It automatically uses an active transaction from the context when available,
// or falls back to the underlying database connection otherwise.
type DBTxWrapper struct {
	db     *sql.DB
	getter TransactionGetter
}

// NewDBTxWrapper creates a new DBTxWrapper instance with the provided database connection
// and transaction getter.
func NewDBTxWrapper(db *sql.DB, getter TransactionGetter) *DBTxWrapper {
	return &DBTxWrapper{
		db:     db,
		getter: getter,
	}
}

// PrepareContext creates a prepared statement for the given query.
// It uses an active transaction from the context if available, otherwise uses the database connection.
func (db *DBTxWrapper) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return db.getter.DefaultTrOrDB(ctx, db.db).PrepareContext(ctx, query)
}

// Prepare creates a prepared statement for the given query using a background context.
// It uses an active transaction if available, otherwise uses the database connection.
func (db *DBTxWrapper) Prepare(query string) (*sql.Stmt, error) {
	return db.getter.DefaultTrOrDB(context.Background(), db.db).Prepare(query)
}

// ExecContext executes a query that doesn't return rows.
// It uses an active transaction from the context if available, otherwise uses the database connection.
func (db *DBTxWrapper) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.getter.DefaultTrOrDB(ctx, db.db).ExecContext(ctx, query, args...)
}

// Exec executes a query that doesn't return rows using a background context.
// It uses an active transaction if available, otherwise uses the database connection.
func (db *DBTxWrapper) Exec(query string, args ...any) (sql.Result, error) {
	return db.getter.DefaultTrOrDB(context.Background(), db.db).Exec(query, args...)
}

// QueryContext executes a query that returns rows.
// It uses an active transaction from the context if available, otherwise uses the database connection.
func (db *DBTxWrapper) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.getter.DefaultTrOrDB(ctx, db.db).QueryContext(ctx, query, args...)
}

// Query executes a query that returns rows using a background context.
// It uses an active transaction if available, otherwise uses the database connection.
func (db *DBTxWrapper) Query(query string, args ...any) (*sql.Rows, error) {
	return db.getter.DefaultTrOrDB(context.Background(), db.db).Query(query, args...)
}

// QueryRowContext executes a query that is expected to return at most one row.
// It uses an active transaction from the context if available, otherwise uses the database connection.
func (db *DBTxWrapper) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.getter.DefaultTrOrDB(ctx, db.db).QueryRowContext(ctx, query, args...)
}

// QueryRow executes a query that is expected to return at most one row using a background context.
// It uses an active transaction if available, otherwise uses the database connection.
func (db *DBTxWrapper) QueryRow(query string, args ...any) *sql.Row {
	return db.getter.DefaultTrOrDB(context.Background(), db.db).QueryRow(query, args...)
}

// DBLogWrapper wraps a DB interface and logs all database operations with timing information.
type DBLogWrapper struct {
	db DB
}

// NewDBLogWrapper creates a new DBLogWrapper instance with the provided DB implementation.
func NewDBLogWrapper(db DB) *DBLogWrapper {
	return &DBLogWrapper{db: db}
}

// PrepareContext logs and delegates to the wrapped DB.
func (w *DBLogWrapper) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := w.db.PrepareContext(ctx, query)
	duration := time.Since(start)

	slog.DebugContext(ctx, "DB PrepareContext",
		slog.String("query", query),
		slog.Duration("duration", duration),
	)

	return stmt, err
}

// Prepare logs and delegates to the wrapped DB.
func (w *DBLogWrapper) Prepare(query string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := w.db.Prepare(query)
	duration := time.Since(start)

	slog.Debug("DB Prepare",
		slog.String("query", query),
		slog.Duration("duration", duration),
	)

	return stmt, err
}

// ExecContext logs and delegates to the wrapped DB.
func (w *DBLogWrapper) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := w.db.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	slog.DebugContext(ctx, "DB ExecContext",
		slog.String("query", query),
		slog.Any("args", args),
		slog.Duration("duration", duration),
	)

	return result, err
}

// Exec logs and delegates to the wrapped DB.
func (w *DBLogWrapper) Exec(query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := w.db.Exec(query, args...)
	duration := time.Since(start)

	slog.Debug("DB Exec",
		slog.String("query", query),
		slog.Any("args", args),
		slog.Duration("duration", duration),
	)

	return result, err
}

// QueryContext logs and delegates to the wrapped DB.
func (w *DBLogWrapper) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := w.db.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	slog.DebugContext(ctx, "DB QueryContext",
		slog.String("query", query),
		slog.Any("args", args),
		slog.Duration("duration", duration),
	)

	return rows, err
}

// Query logs and delegates to the wrapped DB.
func (w *DBLogWrapper) Query(query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := w.db.Query(query, args...)
	duration := time.Since(start)

	slog.Debug("DB Query",
		slog.String("query", query),
		slog.Any("args", args),
		slog.Duration("duration", duration),
	)

	return rows, err
}

// QueryRowContext logs and delegates to the wrapped DB.
func (w *DBLogWrapper) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := w.db.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)

	slog.DebugContext(ctx, "DB QueryRowContext",
		slog.String("query", query),
		slog.Any("args", args),
		slog.Duration("duration", duration),
	)

	return row
}

// QueryRow logs and delegates to the wrapped DB.
func (w *DBLogWrapper) QueryRow(query string, args ...any) *sql.Row {
	start := time.Now()
	row := w.db.QueryRow(query, args...)
	duration := time.Since(start)

	slog.Debug("DB QueryRow",
		slog.String("query", query),
		slog.Any("args", args),
		slog.Duration("duration", duration),
	)

	return row
}
