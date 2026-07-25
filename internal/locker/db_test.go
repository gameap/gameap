package locker_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/locker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // SQLite driver
)

func setupDBLocker(t *testing.T) *locker.DBLocker {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	// :memory: gives every pooled connection its own database; a single
	// connection keeps the kv_store table visible to all statements.
	db.SetMaxOpenConns(1)

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	return locker.NewDBLocker(db, locker.DBDialectSQLite)
}

func TestDBLocker_Acquire(t *testing.T) {
	tests := []struct {
		name      string
		ttl       time.Duration
		wantError string
	}{
		{name: "valid_ttl_acquires", ttl: 1 * time.Second},
		{name: "zero_ttl_rejected", ttl: 0, wantError: "ttl must be positive"},
		{name: "negative_ttl_rejected", ttl: -time.Second, wantError: "ttl must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := setupDBLocker(t)
			lock, err := l.Acquire(context.Background(), "key", tt.ttl)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, lock)

			err = lock.Release(context.Background())
			require.NoError(t, err)
		})
	}
}

func TestDBLocker_MutualExclusion(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	first, err := l.Acquire(ctx, "shared", 5*time.Second)
	require.NoError(t, err)

	_, err = l.Acquire(ctx, "shared", 5*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLocked))

	require.NoError(t, first.Release(ctx))

	second, err := l.Acquire(ctx, "shared", 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, second.Release(ctx))
}

func TestDBLocker_TTLExpirySteal(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	_, err := l.Acquire(ctx, "ephemeral", 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(80 * time.Millisecond)

	second, err := l.Acquire(ctx, "ephemeral", 1*time.Second)
	require.NoError(t, err)
	require.NoError(t, second.Release(ctx))
}

func TestDBLocker_Refresh(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "refresh-test", 50*time.Millisecond)
	require.NoError(t, err)

	require.NoError(t, lock.Refresh(ctx, 1*time.Second))

	time.Sleep(80 * time.Millisecond)

	_, err = l.Acquire(ctx, "refresh-test", 1*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLocked))

	require.NoError(t, lock.Release(ctx))
}

func TestDBLocker_RefreshLostAfterSteal(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	first, err := l.Acquire(ctx, "stolen", 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(80 * time.Millisecond)

	second, err := l.Acquire(ctx, "stolen", 5*time.Second)
	require.NoError(t, err)

	err = first.Refresh(ctx, 1*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLockLost))

	require.NoError(t, second.Release(ctx))
}

func TestDBLocker_ReleaseKeepsForeignLock(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	first, err := l.Acquire(ctx, "foreign", 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(80 * time.Millisecond)

	second, err := l.Acquire(ctx, "foreign", 5*time.Second)
	require.NoError(t, err)

	require.NoError(t, first.Release(ctx))

	_, err = l.Acquire(ctx, "foreign", 5*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLocked))

	require.NoError(t, second.Release(ctx))
}

func TestDBLocker_RefreshAfterRelease(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "released", 1*time.Second)
	require.NoError(t, err)

	require.NoError(t, lock.Release(ctx))

	err = lock.Refresh(ctx, 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lock already released")
}

func TestDBLocker_DoubleRelease(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "double-release", 1*time.Second)
	require.NoError(t, err)

	require.NoError(t, lock.Release(ctx))
	require.NoError(t, lock.Release(ctx))
}

func TestDBLocker_IndependentKeys(t *testing.T) {
	l := setupDBLocker(t)
	ctx := context.Background()

	first, err := l.Acquire(ctx, "key-a", 5*time.Second)
	require.NoError(t, err)

	second, err := l.Acquire(ctx, "key-b", 5*time.Second)
	require.NoError(t, err)

	require.NoError(t, first.Release(ctx))
	require.NoError(t, second.Release(ctx))
}
