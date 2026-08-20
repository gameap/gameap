package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gameap/gameap/internal/cache"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPostgresEnsureCreate = errors.New("create table refused")
	errPostgresEnsureCheck  = errors.New("information_schema lookup refused")
	errPostgresGetQuery     = errors.New("query row refused")
	errPostgresSetExec      = errors.New("upsert refused")
	errPostgresDeleteExec   = errors.New("delete refused")
	errPostgresClearExec    = errors.New("clear refused")
)

// expectPostgresEnsureTableExists configures the mock so the constructor's
// ensureTable returns "table exists" via SELECT EXISTS.
func expectPostgresEnsureTableExists(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("kv_store").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
}

// newPostgresWithMock constructs a PostgreSQL cache backed by sqlmock; the
// constructor's ensureTable check is satisfied by a stubbed "table exists"
// row so subsequent expectations reflect only the operation under test.
func newPostgresWithMock(t *testing.T) (*cache.PostgreSQL, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	expectPostgresEnsureTableExists(mock)

	return cache.NewPostgreSQL(db), mock, func() {
		_ = db.Close()
	}
}

func TestNewPostgreSQL_EnsureTable_CreatesWhenMissing(t *testing.T) {
	t.Parallel()
	// ARRANGE
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("kv_store").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("CREATE UNLOGGED TABLE IF NOT EXISTS kv_store").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// ACT
	c := cache.NewPostgreSQL(db)

	// ASSERT
	require.NotNil(t, c, "constructor must return a non-nil cache when CREATE TABLE succeeds")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewPostgreSQL_EnsureTable_PanicsWhenCheckErrors(t *testing.T) {
	t.Parallel()
	// ARRANGE
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("kv_store").
		WillReturnError(errPostgresEnsureCheck)

	// ACT / ASSERT
	assert.Panics(
		t,
		func() { _ = cache.NewPostgreSQL(db) },
		"constructor must panic when SELECT EXISTS errors",
	)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewPostgreSQL_EnsureTable_PanicsWhenCreateErrors(t *testing.T) {
	t.Parallel()
	// ARRANGE
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("kv_store").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("CREATE UNLOGGED TABLE IF NOT EXISTS kv_store").
		WillReturnError(errPostgresEnsureCreate)

	// ACT / ASSERT
	assert.Panics(
		t,
		func() { _ = cache.NewPostgreSQL(db) },
		"constructor must panic when CREATE TABLE errors",
	)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQL_Get_ErrorPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupMock func(sqlmock.Sqlmock)
		wantError string
		wantErrIs error
	}{
		{
			name: "returns_not_found_when_no_rows",
			setupMock: func(m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"value", "expires_at"})
				m.ExpectQuery("SELECT value, expires_at FROM kv_store").
					WithArgs("cache:missing").
					WillReturnRows(rows)
			},
			wantErrIs: cache.ErrNotFound,
		},
		{
			name: "wraps_query_error_with_context",
			setupMock: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT value, expires_at FROM kv_store").
					WithArgs("cache:missing").
					WillReturnError(errPostgresGetQuery)
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
			t.Parallel()
			// ARRANGE
			c, mock, cleanup := newPostgresWithMock(t)
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

func TestPostgreSQL_Get_ReturnsNotFoundWhenExpiredWithoutInlineDelete(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c, mock, cleanup := newPostgresWithMock(t)
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

func TestPostgreSQL_Set_ErrorPaths(t *testing.T) {
	t.Parallel()
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
					WillReturnError(errPostgresSetExec)
			},
			wantError: "failed to set cache value: upsert refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			c, mock, cleanup := newPostgresWithMock(t)
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

func TestPostgreSQL_Delete_WrapsExecError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs("cache:to_delete").
		WillReturnError(errPostgresDeleteExec)

	// ACT
	err := c.Delete(context.Background(), "to_delete")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete cache value: delete refused", "error must wrap exec failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQL_Clear_WrapsExecError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs("cache:%").
		WillReturnError(errPostgresClearExec)

	// ACT
	err := c.Clear(context.Background())

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clear cache: clear refused", "error must wrap exec failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQL_CleanupExpired_WrapsExecError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(errPostgresClearExec)

	// ACT
	err := c.CleanupExpired(context.Background())

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to cleanup expired cache entries:", "error must wrap exec failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQL_CleanupExpired_Succeeds(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c, mock, cleanup := newPostgresWithMock(t)
	defer cleanup()
	mock.ExpectExec("DELETE FROM kv_store").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 5))

	// ACT
	err := c.CleanupExpired(context.Background())

	// ASSERT
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
