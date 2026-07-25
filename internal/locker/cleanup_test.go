package locker

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // SQLite driver
)

func TestInMemoryLocker_AcquireReclaimsExpiredEntries(t *testing.T) {
	l := NewInMemoryLocker()
	ctx := context.Background()

	_, err := l.Acquire(ctx, "short-lived", 10*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	_, err = l.Acquire(ctx, "fresh", time.Minute)
	require.NoError(t, err)

	l.mu.Lock()
	defer l.mu.Unlock()
	require.Len(t, l.locks, 1, "expired entries must be swept on acquire")
	_, ok := l.locks["fresh"]
	assert.True(t, ok)
}

func TestDBLocker_CleanupExpiredRemovesOnlyExpiredLockRows(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	l := NewDBLocker(db, DBDialectSQLite)
	ctx := context.Background()

	_, err = l.Acquire(ctx, "live", time.Minute)
	require.NoError(t, err)

	_, err = l.Acquire(ctx, "stale", 10*time.Millisecond)
	require.NoError(t, err)

	// A foreign kv_store row (cache-style prefix) must never be touched.
	_, err = db.ExecContext(ctx,
		"INSERT INTO kv_store (key, value, expires_at) VALUES (?, ?, ?)",
		"cache:entry", []byte("payload"), time.Now().Add(-time.Hour).UnixMilli())
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	require.NoError(t, l.cleanupExpired(ctx, time.Now()))

	var keys []string
	rows, err := db.QueryContext(ctx, "SELECT key FROM kv_store ORDER BY key")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		require.NoError(t, rows.Scan(&key))
		keys = append(keys, key)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"cache:entry", dbLockKeyPrefix + "live"}, keys)
}

func TestDBLocker_AcquireTriggersCleanup(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	l := NewDBLocker(db, DBDialectSQLite)
	ctx := context.Background()

	_, err = l.Acquire(ctx, "stale", 10*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	// Reset the throttle so this acquire sweeps immediately.
	l.lastCleanupNano.Store(0)

	_, err = l.Acquire(ctx, "fresh", time.Minute)
	require.NoError(t, err)

	var count int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM kv_store WHERE key = ?", dbLockKeyPrefix+"stale").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "acquire must opportunistically reclaim expired lock rows")
}
