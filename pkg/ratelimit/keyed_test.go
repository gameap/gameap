// Tests for the keyed token-bucket limiter.
//
// OWASP API Security Top 10:2023:
//   - API4:2023 Unrestricted Resource Consumption — the limiter bounds the
//     rate per client and its own memory, so a flood cannot grow the table
//     without bound either.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestLimiter(limit Limit, clock *fakeClock, opts ...Option) *KeyedLimiter {
	return NewKeyed(limit, append([]Option{WithClock(clock.Now)}, opts...)...)
}

// TestKeyedLimiter_Allow covers OWASP API4:2023.
func TestKeyedLimiter_Allow(t *testing.T) {
	t.Parallel()

	t.Run("burst_then_refill", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
		limiter := newTestLimiter(Limit{RPS: 1, Burst: 2}, clock)

		// ACT + ASSERT
		for i := range 2 {
			ok, wait := limiter.Allow("client")
			assert.Truef(t, ok, "request %d is within the burst", i+1)
			assert.Equal(t, time.Duration(0), wait)
		}

		ok, wait := limiter.Allow("client")
		require.False(t, ok, "the burst is spent")
		assert.Equal(t, time.Second, wait, "one token per second")

		ok, _ = limiter.Allow("client")
		assert.False(t, ok, "a refused request does not consume the token that was not there")

		clock.Advance(time.Second)

		ok, _ = limiter.Allow("client")
		assert.True(t, ok, "a token has been refilled")

		ok, _ = limiter.Allow("client")
		assert.False(t, ok)
	})

	t.Run("keys_are_independent", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
		limiter := newTestLimiter(Limit{RPS: 1, Burst: 1}, clock)

		// ACT
		okA, _ := limiter.Allow("a")
		okA2, _ := limiter.Allow("a")
		okB, _ := limiter.Allow("b")

		// ASSERT
		assert.True(t, okA)
		assert.False(t, okA2)
		assert.True(t, okB, "another key has its own bucket")
		assert.Equal(t, 2, limiter.Len())
	})

	t.Run("burst_defaults_to_the_rate", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
		limiter := newTestLimiter(Limit{RPS: 2.5}, clock)

		// ACT
		allowed := 0

		for range 5 {
			if ok, _ := limiter.Allow("client"); ok {
				allowed++
			}
		}

		// ASSERT
		assert.Equal(t, 3, allowed, "burst is the rate rounded up")
	})

	t.Run("disabled_limit_allows_everything", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
		limiter := newTestLimiter(Limit{}, clock)

		// ACT + ASSERT
		for range 100 {
			ok, wait := limiter.Allow("client")
			assert.True(t, ok)
			assert.Equal(t, time.Duration(0), wait)
		}

		assert.Equal(t, 0, limiter.Len(), "a disabled limiter tracks nothing")
	})

	t.Run("nil_limiter_allows_everything", func(t *testing.T) {
		t.Parallel()

		var limiter *KeyedLimiter

		ok, wait := limiter.Allow("client")

		assert.True(t, ok)
		assert.Equal(t, time.Duration(0), wait)
		assert.Equal(t, 0, limiter.Len())
	})
}

// TestKeyedLimiter_bounds_its_table covers OWASP API4:2023: the table never
// holds more than maxKeys clients, dropping the least recently seen one.
func TestKeyedLimiter_bounds_its_table(t *testing.T) {
	t.Parallel()

	// ARRANGE
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := newTestLimiter(Limit{RPS: 1, Burst: 1}, clock, WithMaxKeys(2))

	// ACT
	_, _ = limiter.Allow("a")
	_, _ = limiter.Allow("b")
	_, _ = limiter.Allow("a") // a is now the most recently seen
	_, _ = limiter.Allow("c") // evicts b

	// ASSERT
	assert.Equal(t, 2, limiter.Len())

	okA, _ := limiter.Allow("a")
	assert.False(t, okA, "a was kept (seen more recently than b at eviction time) and its bucket is still empty")

	okB, _ := limiter.Allow("b")
	assert.True(t, okB, "b was dropped and starts with a fresh bucket")
	assert.Equal(t, 2, limiter.Len(), "the table never grows past its cap")
}

// TestKeyedLimiter_sweeps_idle_keys covers OWASP API4:2023: clients unseen
// for longer than the idle TTL are dropped on the next call.
func TestKeyedLimiter_sweeps_idle_keys(t *testing.T) {
	t.Parallel()

	// ARRANGE
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := newTestLimiter(Limit{RPS: 1, Burst: 1}, clock, WithIdleTTL(10*time.Second))

	_, _ = limiter.Allow("old")

	clock.Advance(6 * time.Second)

	_, _ = limiter.Allow("recent")

	// ACT
	clock.Advance(5 * time.Second) // old is 11s idle, recent 5s

	_, _ = limiter.Allow("new")

	// ASSERT
	assert.Equal(t, 2, limiter.Len(), "old was swept, recent and new remain")
}
