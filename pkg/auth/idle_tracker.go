package auth

import (
	"context"
	"errors"
	"time"

	"github.com/gameap/gameap/internal/cache"
)

const idleKeyPrefix = "auth:idle:"

// IdleTracker is the per-token sliding-TTL state behind the session idle
// timeout (ASVS §3.3.2). The auth middleware records activity after each
// authenticated request; the same middleware refuses the next request
// when no recent activity is on file.
//
// Implementations MUST tolerate a missing entry (no prior activity) as
// "not active" — the middleware decides whether that constitutes a hard
// failure (session is too old) or a first-touch warm-up (session was
// just issued, RecordActivity has not landed yet).
type IdleTracker interface {
	// RecordActivity stamps the token with the current time. TTL is the
	// idle ceiling — the entry self-evicts after that wall-clock window.
	RecordActivity(ctx context.Context, identifier string, ttl time.Duration) error

	// LastActivity returns the age of the most recent recorded activity
	// for this token, or (0, false) when there is none. Callers use the
	// age to decide whether to skip the next RecordActivity (probabilistic
	// refresh) without hitting the cache write path.
	LastActivity(ctx context.Context, identifier string) (age time.Duration, present bool, err error)
}

// NoopIdleTracker is the safe default for code paths that should never
// engage the idle check (PAT auth, short-lived tokens) and for tests
// that do not exercise the feature.
type NoopIdleTracker struct{}

func (NoopIdleTracker) RecordActivity(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

func (NoopIdleTracker) LastActivity(_ context.Context, _ string) (time.Duration, bool, error) {
	return 0, false, nil
}

// CacheIdleTracker stores activity timestamps in the project's cache
// abstraction. Redis-backed deployments get a cluster-wide view; the
// in-memory cache keeps state local to the process (acceptable for
// single-instance deployments).
//
// Each entry stores an int64 unix-nano timestamp keyed by
// "auth:idle:<identifier>" with TTL equal to the idle ceiling. A
// successful Get returns the timestamp; the caller derives the age by
// subtracting from the current time. Missing entries surface as
// (0, false, nil).
type CacheIdleTracker struct {
	cache cache.Cache
	now   func() time.Time
}

// NewCacheIdleTracker wires the tracker. The clock argument lets tests
// drive deterministic age values; pass nil for time.Now.
func NewCacheIdleTracker(c cache.Cache, nowFn func() time.Time) *CacheIdleTracker {
	if nowFn == nil {
		nowFn = time.Now
	}

	return &CacheIdleTracker{cache: c, now: nowFn}
}

func (t *CacheIdleTracker) RecordActivity(ctx context.Context, identifier string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}

	return t.cache.Set(ctx, idleKeyPrefix+identifier, t.now().UnixNano(), cache.WithExpiration(ttl))
}

func (t *CacheIdleTracker) LastActivity(ctx context.Context, identifier string) (time.Duration, bool, error) {
	raw, err := t.cache.Get(ctx, idleKeyPrefix+identifier)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return 0, false, nil
		}

		return 0, false, err
	}

	// cache.Cache returns values as `any` — accept the common int64
	// (Redis backend marshals to int64), but also accept float64 (JSON)
	// because cache implementations may round-trip through JSON.
	var ns int64
	switch v := raw.(type) {
	case int64:
		ns = v
	case float64:
		ns = int64(v)
	default:
		// Stored shape unrecognised — treat as missing so the caller
		// re-records on the next request instead of crashing.
		return 0, false, nil
	}

	age := max(t.now().Sub(time.Unix(0, ns)), 0)

	return age, true, nil
}
