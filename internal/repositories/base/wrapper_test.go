package base

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	trsql "github.com/avito-tech/go-transaction-manager/drivers/sql/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPrepare = errors.New("prepare failed")
	errExec    = errors.New("exec failed")
	errQuery   = errors.New("query failed")
)

type fakeTransactionGetter struct {
	defaultCalled int
	keyCalled     int
	lastKey       trm.CtxKey
}

func (f *fakeTransactionGetter) DefaultTrOrDB(_ context.Context, db trsql.Tr) trsql.Tr {
	f.defaultCalled++

	return db
}

func (f *fakeTransactionGetter) TrOrDB(_ context.Context, key trm.CtxKey, db trsql.Tr) trsql.Tr {
	f.keyCalled++
	f.lastKey = key

	return db
}

func setupDBTxWrapper(t *testing.T) (*DBTxWrapper, sqlmock.Sqlmock, *fakeTransactionGetter, func()) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	getter := &fakeTransactionGetter{}
	wrapper := NewDBTxWrapper(db, getter)

	return wrapper, mock, getter, func() {
		_ = db.Close()
	}
}

func TestNewDBTxWrapper(t *testing.T) {
	t.Parallel()

	// ARRANGE
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	getter := &fakeTransactionGetter{}

	// ACT
	wrapper := NewDBTxWrapper(db, getter)

	// ASSERT
	require.NotNil(t, wrapper)
	assert.Same(t, db, wrapper.db, "db field must reference the provided connection")
	assert.Same(t, getter, wrapper.getter, "getter field must reference the provided transaction getter")
}

func TestDBTxWrapper_PrepareContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		query     string
		wantError string
	}{
		{
			name: "delegates_to_getter_and_prepares_statement",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectPrepare("SELECT id FROM users WHERE id = ?")
			},
			query: "SELECT id FROM users WHERE id = ?",
		},
		{
			name: "propagates_error_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectPrepare("BAD QUERY").WillReturnError(errPrepare)
			},
			query:     "BAD QUERY",
			wantError: "prepare failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			stmt, err := wrapper.PrepareContext(context.Background(), tt.query)

			// ASSERT
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, stmt)
			} else {
				require.NoError(t, err)
				require.NotNil(t, stmt)
				defer func() { _ = stmt.Close() }()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_Prepare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		query     string
		wantError string
	}{
		{
			name: "delegates_to_getter_and_prepares_statement",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectPrepare("SELECT 1")
			},
			query: "SELECT 1",
		},
		{
			name: "propagates_error_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectPrepare("INVALID").WillReturnError(errPrepare)
			},
			query:     "INVALID",
			wantError: "prepare failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			stmt, err := wrapper.Prepare(tt.query)

			// ASSERT
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, stmt)
			} else {
				require.NoError(t, err)
				require.NotNil(t, stmt)
				defer func() { _ = stmt.Close() }()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_ExecContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setupMock        func(sqlmock.Sqlmock)
		query            string
		args             []any
		wantError        string
		wantRowsAffected int64
	}{
		{
			name: "executes_and_returns_result",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectExec("UPDATE users SET name = ? WHERE id = ?").
					WithArgs("alice", 7).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			query:            "UPDATE users SET name = ? WHERE id = ?",
			args:             []any{"alice", 7},
			wantRowsAffected: 1,
		},
		{
			name: "propagates_error_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectExec("DELETE FROM users").WillReturnError(errExec)
			},
			query:     "DELETE FROM users",
			wantError: "exec failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			result, err := wrapper.ExecContext(context.Background(), tt.query, tt.args...)

			// ASSERT
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				rows, rerr := result.RowsAffected()
				require.NoError(t, rerr)
				assert.Equal(t, tt.wantRowsAffected, rows, "rows affected mismatch")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_Exec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setupMock        func(sqlmock.Sqlmock)
		query            string
		args             []any
		wantError        string
		wantRowsAffected int64
	}{
		{
			name: "executes_and_returns_result",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectExec("INSERT INTO t VALUES (?)").
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(13, 1))
			},
			query:            "INSERT INTO t VALUES (?)",
			args:             []any{42},
			wantRowsAffected: 1,
		},
		{
			name: "propagates_error_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectExec("BROKEN").WillReturnError(errExec)
			},
			query:     "BROKEN",
			wantError: "exec failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			result, err := wrapper.Exec(tt.query, tt.args...)

			// ASSERT
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				rows, rerr := result.RowsAffected()
				require.NoError(t, rerr)
				assert.Equal(t, tt.wantRowsAffected, rows, "rows affected mismatch")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_QueryContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupMock   func(sqlmock.Sqlmock)
		query       string
		args        []any
		wantError   string
		wantRowVals []int
	}{
		{
			name: "returns_rows_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2)
				m.ExpectQuery("SELECT id FROM t").WillReturnRows(rows)
			},
			query:       "SELECT id FROM t",
			wantRowVals: []int{1, 2},
		},
		{
			name: "propagates_error_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("BROKEN").WillReturnError(errQuery)
			},
			query:     "BROKEN",
			wantError: "query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			rows, err := wrapper.QueryContext(context.Background(), tt.query, tt.args...)

			// ASSERT
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, rows)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rows)
				got := scanIntColumn(t, rows)
				assert.Equal(t, tt.wantRowVals, got, "scanned rows mismatch")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_Query(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupMock   func(sqlmock.Sqlmock)
		query       string
		args        []any
		wantError   string
		wantRowVals []int
	}{
		{
			name: "returns_rows_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(7)
				m.ExpectQuery("SELECT id FROM t").WillReturnRows(rows)
			},
			query:       "SELECT id FROM t",
			wantRowVals: []int{7},
		},
		{
			name: "propagates_error_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("X").WillReturnError(errQuery)
			},
			query:     "X",
			wantError: "query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			rows, err := wrapper.Query(tt.query, tt.args...)

			// ASSERT
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, rows)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rows)
				got := scanIntColumn(t, rows)
				assert.Equal(t, tt.wantRowVals, got, "scanned rows mismatch")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_QueryRowContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		query     string
		args      []any
		wantValue int
		wantError string
	}{
		{
			name: "returns_row_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(99)
				m.ExpectQuery("SELECT id FROM t WHERE name = ?").
					WithArgs("alice").
					WillReturnRows(rows)
			},
			query:     "SELECT id FROM t WHERE name = ?",
			args:      []any{"alice"},
			wantValue: 99,
		},
		{
			name: "row_scan_returns_no_rows_when_no_match",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"})
				m.ExpectQuery("SELECT id FROM t WHERE name = ?").
					WithArgs("missing").
					WillReturnRows(rows)
			},
			query:     "SELECT id FROM t WHERE name = ?",
			args:      []any{"missing"},
			wantError: sql.ErrNoRows.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			row := wrapper.QueryRowContext(context.Background(), tt.query, tt.args...)

			// ASSERT
			require.NotNil(t, row, "QueryRowContext must never return nil")
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			var got int
			err := row.Scan(&got)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, got, "scanned value mismatch")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDBTxWrapper_QueryRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		query     string
		args      []any
		wantValue int
		wantError string
	}{
		{
			name: "returns_row_from_db",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(123)
				m.ExpectQuery("SELECT id FROM t").WillReturnRows(rows)
			},
			query:     "SELECT id FROM t",
			wantValue: 123,
		},
		{
			name: "row_scan_returns_no_rows_when_empty",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"})
				m.ExpectQuery("SELECT id FROM t").WillReturnRows(rows)
			},
			query:     "SELECT id FROM t",
			wantError: sql.ErrNoRows.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			wrapper, mock, getter, cleanup := setupDBTxWrapper(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			row := wrapper.QueryRow(tt.query, tt.args...)

			// ASSERT
			require.NotNil(t, row, "QueryRow must never return nil")
			assert.Equal(t, 1, getter.defaultCalled, "DefaultTrOrDB must be called exactly once")
			var got int
			err := row.Scan(&got)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, got, "scanned value mismatch")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

type fakeDB struct {
	prepareErr  error
	execErr     error
	queryErr    error
	prepareStmt *sql.Stmt
	execResult  sql.Result
	queryRows   *sql.Rows
	queryRow    *sql.Row

	lastQuery string
	lastArgs  []any
	callCount int
}

func (f *fakeDB) PrepareContext(_ context.Context, query string) (*sql.Stmt, error) {
	f.callCount++
	f.lastQuery = query

	return f.prepareStmt, f.prepareErr
}

func (f *fakeDB) Prepare(query string) (*sql.Stmt, error) {
	f.callCount++
	f.lastQuery = query

	return f.prepareStmt, f.prepareErr
}

func (f *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.callCount++
	f.lastQuery = query
	f.lastArgs = args

	return f.execResult, f.execErr
}

func (f *fakeDB) Exec(query string, args ...any) (sql.Result, error) {
	f.callCount++
	f.lastQuery = query
	f.lastArgs = args

	return f.execResult, f.execErr
}

func (f *fakeDB) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	f.callCount++
	f.lastQuery = query
	f.lastArgs = args

	return f.queryRows, f.queryErr
}

func (f *fakeDB) Query(query string, args ...any) (*sql.Rows, error) {
	f.callCount++
	f.lastQuery = query
	f.lastArgs = args

	return f.queryRows, f.queryErr
}

func (f *fakeDB) QueryRowContext(_ context.Context, query string, args ...any) *sql.Row {
	f.callCount++
	f.lastQuery = query
	f.lastArgs = args

	return f.queryRow
}

func (f *fakeDB) QueryRow(query string, args ...any) *sql.Row {
	f.callCount++
	f.lastQuery = query
	f.lastArgs = args

	return f.queryRow
}

func TestNewDBLogWrapper(t *testing.T) {
	t.Parallel()

	// ARRANGE
	fake := &fakeDB{}

	// ACT
	wrapper := NewDBLogWrapper(fake)

	// ASSERT
	require.NotNil(t, wrapper)
	assert.Same(t, fake, wrapper.db, "wrapper must hold the provided DB implementation")
}

func TestDBLogWrapper_PrepareContext(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name      string
		fake      *fakeDB
		query     string
		wantError string
	}{
		{
			name:  "delegates_to_underlying_db",
			fake:  &fakeDB{},
			query: "SELECT 1",
		},
		{
			name:      "propagates_error_from_underlying_db",
			fake:      &fakeDB{prepareErr: errPrepare},
			query:     "SELECT 1",
			wantError: "prepare failed",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			capture := captureSlog(t)
			wrapper := NewDBLogWrapper(tt.fake)

			// ACT
			stmt, err := wrapper.PrepareContext(context.Background(), tt.query)

			// ASSERT
			assert.Equal(t, 1, tt.fake.callCount, "underlying DB must be called exactly once")
			assert.Equal(t, tt.query, tt.fake.lastQuery, "query must be passed unchanged")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.fake.prepareStmt, stmt, "prepared statement must be passed through")
			assert.Contains(t, capture.String(), "DB PrepareContext", "operation name must be logged")
			assert.Contains(t, capture.String(), tt.query, "query must be logged")
		})
	}
}

func TestDBLogWrapper_Prepare(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name      string
		fake      *fakeDB
		query     string
		wantError string
	}{
		{
			name:  "delegates_to_underlying_db",
			fake:  &fakeDB{},
			query: "SELECT 2",
		},
		{
			name:      "propagates_error_from_underlying_db",
			fake:      &fakeDB{prepareErr: errPrepare},
			query:     "SELECT 2",
			wantError: "prepare failed",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			capture := captureSlog(t)
			wrapper := NewDBLogWrapper(tt.fake)

			// ACT
			stmt, err := wrapper.Prepare(tt.query)

			// ASSERT
			assert.Equal(t, 1, tt.fake.callCount, "underlying DB must be called exactly once")
			assert.Equal(t, tt.query, tt.fake.lastQuery, "query must be passed unchanged")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.fake.prepareStmt, stmt, "prepared statement must be passed through")
			assert.Contains(t, capture.String(), "DB Prepare", "operation name must be logged")
			assert.Contains(t, capture.String(), tt.query, "query must be logged")
		})
	}
}

func TestDBLogWrapper_ExecContext(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name      string
		fake      *fakeDB
		query     string
		args      []any
		wantError string
	}{
		{
			name:  "delegates_to_underlying_db",
			fake:  &fakeDB{execResult: sqlmock.NewResult(0, 5)},
			query: "UPDATE t SET v = ?",
			args:  []any{"x"},
		},
		{
			name:      "propagates_error_from_underlying_db",
			fake:      &fakeDB{execErr: errExec},
			query:     "UPDATE t SET v = ?",
			args:      []any{"x"},
			wantError: "exec failed",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			capture := captureSlog(t)
			wrapper := NewDBLogWrapper(tt.fake)

			// ACT
			result, err := wrapper.ExecContext(context.Background(), tt.query, tt.args...)

			// ASSERT
			assert.Equal(t, 1, tt.fake.callCount, "underlying DB must be called exactly once")
			assert.Equal(t, tt.query, tt.fake.lastQuery, "query must be passed unchanged")
			assert.Equal(t, tt.args, tt.fake.lastArgs, "args must be passed unchanged")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.fake.execResult, result, "result must be passed through")
			}
			assert.Contains(t, capture.String(), "DB ExecContext", "operation name must be logged")
		})
	}
}

func TestDBLogWrapper_Exec(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name      string
		fake      *fakeDB
		query     string
		args      []any
		wantError string
	}{
		{
			name:  "delegates_to_underlying_db",
			fake:  &fakeDB{execResult: sqlmock.NewResult(11, 1)},
			query: "DELETE FROM t",
			args:  []any{1},
		},
		{
			name:      "propagates_error_from_underlying_db",
			fake:      &fakeDB{execErr: errExec},
			query:     "DELETE FROM t",
			args:      []any{1},
			wantError: "exec failed",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			capture := captureSlog(t)
			wrapper := NewDBLogWrapper(tt.fake)

			// ACT
			result, err := wrapper.Exec(tt.query, tt.args...)

			// ASSERT
			assert.Equal(t, 1, tt.fake.callCount, "underlying DB must be called exactly once")
			assert.Equal(t, tt.query, tt.fake.lastQuery, "query must be passed unchanged")
			assert.Equal(t, tt.args, tt.fake.lastArgs, "args must be passed unchanged")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.fake.execResult, result, "result must be passed through")
			}
			assert.Contains(t, capture.String(), "DB Exec", "operation name must be logged")
		})
	}
}

func TestDBLogWrapper_QueryContext(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name      string
		fake      *fakeDB
		query     string
		args      []any
		wantError string
	}{
		{
			name:  "delegates_to_underlying_db",
			fake:  &fakeDB{},
			query: "SELECT * FROM t",
			args:  []any{1, 2},
		},
		{
			name:      "propagates_error_from_underlying_db",
			fake:      &fakeDB{queryErr: errQuery},
			query:     "SELECT * FROM t",
			args:      []any{1, 2},
			wantError: "query failed",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			capture := captureSlog(t)
			wrapper := NewDBLogWrapper(tt.fake)

			// ACT
			rows, err := wrapper.QueryContext(context.Background(), tt.query, tt.args...) //nolint:rowserrcheck // fake returns nil *sql.Rows; calling Err() would dereference nil

			// ASSERT
			assert.Equal(t, 1, tt.fake.callCount, "underlying DB must be called exactly once")
			assert.Equal(t, tt.query, tt.fake.lastQuery, "query must be passed unchanged")
			assert.Equal(t, tt.args, tt.fake.lastArgs, "args must be passed unchanged")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.fake.queryRows, rows, "rows must be passed through")
			assert.Contains(t, capture.String(), "DB QueryContext", "operation name must be logged")
		})
	}
}

func TestDBLogWrapper_Query(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name      string
		fake      *fakeDB
		query     string
		args      []any
		wantError string
	}{
		{
			name:  "delegates_to_underlying_db",
			fake:  &fakeDB{},
			query: "SELECT * FROM t",
			args:  []any{1},
		},
		{
			name:      "propagates_error_from_underlying_db",
			fake:      &fakeDB{queryErr: errQuery},
			query:     "SELECT * FROM t",
			args:      []any{1},
			wantError: "query failed",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			capture := captureSlog(t)
			wrapper := NewDBLogWrapper(tt.fake)

			// ACT
			rows, err := wrapper.Query(tt.query, tt.args...) //nolint:rowserrcheck // fake returns nil *sql.Rows; calling Err() would dereference nil

			// ASSERT
			assert.Equal(t, 1, tt.fake.callCount, "underlying DB must be called exactly once")
			assert.Equal(t, tt.query, tt.fake.lastQuery, "query must be passed unchanged")
			assert.Equal(t, tt.args, tt.fake.lastArgs, "args must be passed unchanged")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.fake.queryRows, rows, "rows must be passed through")
			assert.Contains(t, capture.String(), "DB Query", "operation name must be logged")
		})
	}
}

func TestDBLogWrapper_QueryRowContext(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	// ARRANGE
	fake := &fakeDB{}
	capture := captureSlog(t)
	wrapper := NewDBLogWrapper(fake)
	args := []any{42}

	// ACT
	row := wrapper.QueryRowContext(context.Background(), "SELECT id FROM t WHERE id = ?", args...)

	// ASSERT
	assert.Equal(t, 1, fake.callCount, "underlying DB must be called exactly once")
	assert.Equal(t, "SELECT id FROM t WHERE id = ?", fake.lastQuery, "query must be passed unchanged")
	assert.Equal(t, args, fake.lastArgs, "args must be passed unchanged")
	assert.Equal(t, fake.queryRow, row, "row must be passed through")
	assert.Contains(t, capture.String(), "DB QueryRowContext", "operation name must be logged")
}

func TestDBLogWrapper_QueryRow(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	// ARRANGE
	fake := &fakeDB{}
	capture := captureSlog(t)
	wrapper := NewDBLogWrapper(fake)
	args := []any{42}

	// ACT
	row := wrapper.QueryRow("SELECT id FROM t WHERE id = ?", args...)

	// ASSERT
	assert.Equal(t, 1, fake.callCount, "underlying DB must be called exactly once")
	assert.Equal(t, "SELECT id FROM t WHERE id = ?", fake.lastQuery, "query must be passed unchanged")
	assert.Equal(t, args, fake.lastArgs, "args must be passed unchanged")
	assert.Equal(t, fake.queryRow, row, "row must be passed through")
	assert.Contains(t, capture.String(), "DB QueryRow", "operation name must be logged")
}

func TestFakeTransactionGetter_TrOrDB_Records_Key(t *testing.T) {
	t.Parallel()

	// ARRANGE
	getter := &fakeTransactionGetter{}
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	type ctxKey struct{}
	key := ctxKey{}

	// ACT
	got := getter.TrOrDB(context.Background(), key, db)

	// ASSERT
	assert.Same(t, db, got, "TrOrDB must pass through the provided db")
	assert.Equal(t, 1, getter.keyCalled, "TrOrDB must be invoked exactly once")
	assert.Equal(t, key, getter.lastKey, "TrOrDB must record the supplied key")
}

var (
	_ DB       = (*fakeDB)(nil)
	_ trsql.Tr = (*sql.DB)(nil)
)

func scanIntColumn(t *testing.T, rows *sql.Rows) []int {
	t.Helper()

	defer func() { _ = rows.Close() }()

	got := make([]int, 0)
	for rows.Next() {
		var v int
		require.NoError(t, rows.Scan(&v))
		got = append(got, v)
	}
	require.NoError(t, rows.Err())

	return got
}

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	return buf
}
