// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins ASVS §3.3.2 — idle session timeout. The CacheIdleTracker stores
// sliding activity timestamps used by the auth middleware to reject a
// session that has been quiet for longer than AUTH_SESSION_IDLE_TIMEOUT.
package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopIdleTracker_AlwaysReportsMissing(t *testing.T) {
	t.Parallel()

	tr := auth.NoopIdleTracker{}

	require.NoError(t, tr.RecordActivity(context.Background(), "id", time.Minute))

	age, present, err := tr.LastActivity(context.Background(), "id")
	require.NoError(t, err)
	assert.False(t, present)
	assert.Zero(t, age)
}

func TestCacheIdleTracker_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	tr := auth.NewCacheIdleTracker(cache.NewInMemory(), clk.Now)

	require.NoError(t, tr.RecordActivity(context.Background(), "session-1", 30*time.Minute))

	clk.Advance(5 * time.Minute)

	age, present, err := tr.LastActivity(context.Background(), "session-1")
	require.NoError(t, err)
	assert.True(t, present)
	assert.Equal(t, 5*time.Minute, age,
		"age must reflect wall-clock distance from RecordActivity to LastActivity")
}

func TestCacheIdleTracker_MissingEntryReturnsFalse(t *testing.T) {
	t.Parallel()

	tr := auth.NewCacheIdleTracker(cache.NewInMemory(), nil)

	age, present, err := tr.LastActivity(context.Background(), "never-recorded")
	require.NoError(t, err)
	assert.False(t, present)
	assert.Zero(t, age)
}

func TestCacheIdleTracker_TTLZeroIsNoop(t *testing.T) {
	t.Parallel()

	tr := auth.NewCacheIdleTracker(cache.NewInMemory(), nil)

	require.NoError(t, tr.RecordActivity(context.Background(), "no-ttl", 0))

	_, present, err := tr.LastActivity(context.Background(), "no-ttl")
	require.NoError(t, err)
	assert.False(t, present, "zero/negative TTL must not persist the entry")
}

func TestCacheIdleTracker_ExpiredEntryReturnsFalse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	c := cache.NewInMemory()
	tr := auth.NewCacheIdleTracker(c, clk.Now)

	require.NoError(t, tr.RecordActivity(context.Background(), "expires", 1*time.Millisecond))

	// Advance well past TTL; the in-memory cache evicts on read.
	time.Sleep(5 * time.Millisecond)
	clk.Advance(time.Second)

	_, present, err := tr.LastActivity(context.Background(), "expires")
	require.NoError(t, err)
	assert.False(t, present, "expired idle entry must report missing so middleware can deny")
}

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time          { return f.now }
func (f *fakeClock) Advance(d time.Duration) { f.now = f.now.Add(d) }
