// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins the timing-oracle equalisation helper: a login against a non-existent
// user must spend approximately the same wall-clock time as a login against
// an existing user with a wrong password, so user-enumeration via response
// timing is not viable.
package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyDummyPassword_ApproximatesRealVerifyTime — OWASP API2:2023 —
// compares the latency of VerifyDummyPassword to a real wrong-password
// VerifyPassword at the same cost. The ratio must stay within an order of
// magnitude; in practice they are both dominated by a single bcrypt round
// at the active cost.
//
// We use the lowest allowed cost so the test is fast in CI (each compare
// is still ~1 ms). The assertion is intentionally loose because GC pauses,
// scheduler noise and warm-up effects can swing the ratio by 2-3× even when
// the two paths execute the same bcrypt operation.
func TestVerifyDummyPassword_ApproximatesRealVerifyTime(t *testing.T) {
	t.Cleanup(func() { _ = SetDefaultBcryptCost(DefaultBcryptCost) })
	require.NoError(t, SetDefaultBcryptCost(MinBcryptCost))

	// Prime the dummy hash so the once.Do cost is not charged to the
	// measured Verify call.
	VerifyDummyPassword("primer")

	realHash, err := HashPassword("real-password")
	require.NoError(t, err)

	// 5 iterations to smooth scheduler noise.
	const iters = 5

	var dummyTotal, realTotal time.Duration

	for range iters {
		t0 := time.Now()
		VerifyDummyPassword("guess")
		dummyTotal += time.Since(t0)

		t1 := time.Now()
		_, _ = VerifyPassword(realHash, "guess")
		realTotal += time.Since(t1)
	}

	ratio := float64(realTotal) / float64(dummyTotal)
	// Allow 0.1× to 10× — wide because CI workers vary wildly. The point
	// is "same order of magnitude", not exact equality.
	assert.GreaterOrEqualf(t, ratio, 0.1, "dummy must not be drastically slower than real (ratio=%.2f)", ratio)
	assert.LessOrEqualf(t, ratio, 10.0, "dummy must not be drastically faster than real (ratio=%.2f)", ratio)
}

// TestVerifyDummyPassword_AlwaysReturnsCleanly — OWASP API2:2023 — the
// helper must never panic, never propagate an error, and must accept any
// candidate string (empty, ASCII, Unicode, oversized). It is a timing
// equaliser, not an authenticator.
func TestVerifyDummyPassword_AlwaysReturnsCleanly(t *testing.T) {
	for _, candidate := range []string{
		"",
		"x",
		"short",
		"a-realistic-looking-password",
		"日本語のパスワード",
		string(make([]byte, 1024)), // oversized; bcrypt would reject normally
	} {
		assert.NotPanicsf(t, func() { VerifyDummyPassword(candidate) },
			"candidate %q caused panic", candidate)
	}
}
