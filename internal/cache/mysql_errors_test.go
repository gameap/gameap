package cache_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gameap/gameap/internal/cache"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errMySQLEnsureCreate = errors.New("create table refused")
	errMySQLEnsureCheck  = errors.New("show tables refused")
	errMySQLGetQuery     = errors.New("query row refused")
	errMySQLSetExec      = errors.New("upsert refused")
	errMySQLDeleteExec   = errors.New("delete refused")
	errMySQLClearExec    = errors.New("clear refused")
)

// expectMySQLEnsureTableExists configures the mock so that the constructor's
// ensureTable call succeeds via the "table already exists" branch.
func expectMySQLEnsureTableExists(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SHOW TABLES LIKE 'kv_store'").
		WillReturnRows(sqlmock.NewRows([]string{"Tables_in_db"}).AddRow("kv_store"))
}

// newMySQLWithMock returns a constructed MySQL cache backed by a fresh
// sqlmock connection. The constructor's ensureTable call is satisfied with
// a stubbed "table already exists" response so that subsequent expectations
// reflect only the operation under test.
func newMySQLWithMock(t *testing.T) (*cache.MySQL, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	expectMySQLEnsureTableExists(mock)

	return cache.NewMySQL(db), mock, func() {
		_ = db.Close()
	}
}

func TestNewMySQL_EnsureTable_CreatesWhenMissing(t *testing.T) {
	// ARRANGE
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SHOW TABLES LIKE 'kv_store'").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS kv_store").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// ACT
	c := cache.NewMySQL(db)

	// ASSERT
	require.NotNil(t, c, "constructor must return a non-nil cache when CREATE TABLE succeeds")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewMySQL_EnsureTable_PanicsWhenCheckErrors(t *testing.T) {
	// ARRANGE
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SHOW TABLES LIKE 'kv_store'").
		WillReturnError(errMySQLEnsureCheck)

	// ACT / ASSERT
	assert.PanicsWithError(
		t,
		"failed to ensure kv_store table: failed to check if table exists: show tables refused \n"+
			"You can manually create the table by executing the following SQL statement:\n "+
			"\nCREATE TABLE IF NOT EXISTS kv_store (\n"+
			"  `key` VARCHAR(128) PRIMARY KEY,\n"+
			"  `value` MEDIUMBLOB NOT NULL,\n"+
			"  `expires_at` TIMESTAMP NULL DEFAULT NULL,\n"+
			"  INDEX idx_expires (`expires_at`)\n"+
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4\n",
		func() { _ = cache.NewMySQL(db) },
		"constructor must panic when SHOW TABLES errors with a non-ErrNoRows error",
	)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewMySQL_EnsureTable_PanicsWhenCreateErrors(t *testing.T) {
	// ARRANGE
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SHOW TABLES LIKE 'kv_store'").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS kv_store").
		WillReturnError(errMySQLEnsureCreate)

	// ACT / ASSERT
	assert.Panics(
		t,
		func() { _ = cache.NewMySQL(db) },
		"constructor must panic when CREATE TABLE errors",
	)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQL_Get_ErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		wantError string
		wantErrIs error
	}{
		{
			name: "returns_not_found_when_no_rows",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT value, expires_at FROM kv_store").
					WithArgs("cache:missing").
					WillReturnError(sql.ErrNoRows)
			},
			wantErrIs: cache.ErrNotFound,
		},
		{
			name: "wraps_query_error_with_context",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT value, expires_at FROM kv_store").
					WithArgs("cache:missing").
					WillReturnError(errMySQLGetQuery)
			},
			wantError: "failed to query row: query row refused",
		},
		{
			name: "wraps_unmarshal_error_when_value_is_invalid_json",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"value", "expires_at"}).
					AddRow("not-json", nil)
				m.ExpectQuery("SELECT value, expires_at FROM kv_store").
					WithArgs("cache:missing").
					WillReturnRows(rows)
			},
			wantError: "failed to unmarshal value:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			c, mock, cleanup := newMySQLWithMock(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			value, err := c.Get(context.Background(), "missing")

			// ASSERT
			require.Error(t, err)
			assert.Nil(t, value, "Get must return nil value on error")
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs, "error must match expected sentinel")
			}
			if tt.wantError != "" {
				assert.Contains(t, err.Error(), tt.wantError, "error message must contain wrapped context")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMySQL_Get_ReturnsNotFoundWhenExpiredWithoutInlineDelete(t *testing.T) {
	// ARRANGE
	c, mock, cleanup := newMySQLWithMock(t)
	defer cleanup()

	expiredAt := time.Now().Add(-1 * time.Hour)
	rows := sqlmock.NewRows([]string{"value", "expires_at"}).
		AddRow(`"stale"`, expiredAt)
	mock.ExpectQuery("SELECT value, expires_at FROM kv_store").
		WithArgs("cache:expired_key").
		WillReturnRows(rows)
	// No DELETE is expected: Get must not write on the read path (CleanupExpired
	// reclaims expired rows in the background).

	// ACT
	value, err := c.Get(context.Background(), "expired_key")

	// ASSERT
	assert.ErrorIs(t, err, cache.ErrNotFound, "expired entry must surface ErrNotFound")
	assert.Nil(t, value, "expired entry must return nil value")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQL_Set_ErrorPaths(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     any
		setupMock func(sqlmock.Sqlmock)
		wantError string
	}{
		{
			name:  "returns_marshal_error_when_value_is_unsupported_type",
			key:   "ch_key",
			value: make(chan int),
			setupMock: func(_ sqlmock.Sqlmock) {
				// Marshal fails before any DB call is made.
			},
			wantError: "failed to marshal value:",
		},
		{
			name:  "wraps_exec_error_with_context",
			key:   "ok_key",
			value: "ok_value",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectExec("INSERT INTO kv_store").
					WithArgs("cache:ok_key", `"ok_value"`, sqlmock.AnyArg()).
					WillReturnError(errMySQLSetExec)
			},
			wantError: "failed to set cache value: upsert refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			c, mock, cleanup := newMySQLWithMock(t)
			defer cleanup()
			tt.setupMock(mock)

			// ACT
			err := c.Set(context.Background(), tt.key, tt.value)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message must contain wrapped context")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMySQL_Delete_WrapsExecError(t *testing.T) {
	// ARRANGE
	c, mock, cleanup := newMySQLWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs("cache:to_delete").
		WillReturnError(errMySQLDeleteExec)

	// ACT
	err := c.Delete(context.Background(), "to_delete")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete cache value: delete refused", "error must wrap exec failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQL_Clear_WrapsExecError(t *testing.T) {
	// ARRANGE
	c, mock, cleanup := newMySQLWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs("cache:%").
		WillReturnError(errMySQLClearExec)

	// ACT
	err := c.Clear(context.Background())

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clear cache: clear refused", "error must wrap exec failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQL_CleanupExpired_WrapsExecError(t *testing.T) {
	// ARRANGE
	c, mock, cleanup := newMySQLWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(errMySQLClearExec)

	// ACT
	err := c.CleanupExpired(context.Background())

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to cleanup expired cache entries:", "error must wrap exec failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQL_CleanupExpired_Succeeds(t *testing.T) {
	// ARRANGE
	c, mock, cleanup := newMySQLWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 3))

	// ACT
	err := c.CleanupExpired(context.Background())

	// ASSERT
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
