package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBackendUnavailable = errors.New("backend unavailable")

func TestTokenIdentifier(t *testing.T) {
	t.Parallel()

	emptySum := sha256.Sum256(nil)
	emptyHex := hex.EncodeToString(emptySum[:])

	tests := []struct {
		name    string
		input   string
		want    string
		wantLen int
	}{
		{
			name:    "stable_for_same_input",
			input:   "raw-bearer-token",
			wantLen: 64,
		},
		{
			name:    "differs_for_different_inputs",
			input:   "another-bearer-token",
			wantLen: 64,
		},
		{
			name:    "empty_string_hashes_to_known_value",
			input:   "",
			want:    emptyHex,
			wantLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			input := tt.input

			// ACT
			first := TokenIdentifier(input)
			second := TokenIdentifier(input)

			// ASSERT
			assert.Equal(t, first, second, "identifier must be stable for the same input")
			require.Len(t, first, tt.wantLen, "identifier must be 64 hex chars (sha256)")
			if tt.want != "" {
				assert.Equal(t, tt.want, first, "identifier must equal sha256 hex digest")
			}
		})
	}

	t.Run("differs_for_different_inputs", func(t *testing.T) {
		t.Parallel()

		// ARRANGE / ACT
		left := TokenIdentifier("token-a")
		right := TokenIdentifier("token-b")

		// ASSERT
		assert.NotEqual(t, left, right, "different inputs must produce different identifiers")
	})
}

func TestNoopRevocation_Revoke_NeverErrors(t *testing.T) {
	t.Parallel()

	// ARRANGE
	rev := NoopRevocation{}

	// ACT
	err := rev.Revoke(context.Background(), "any-id", time.Hour)

	// ASSERT
	require.NoError(t, err)
}

func TestNoopRevocation_IsRevoked_AlwaysFalse(t *testing.T) {
	t.Parallel()

	// ARRANGE
	rev := NoopRevocation{}
	_ = rev.Revoke(context.Background(), "any-id", time.Hour)

	// ACT
	revoked, err := rev.IsRevoked(context.Background(), "any-id")

	// ASSERT
	require.NoError(t, err)
	assert.False(t, revoked, "Noop must never report a token as revoked")
}

// recordingCache wraps the in-memory cache and records the keys passed to Set,
// optionally short-circuiting Get with a sentinel error to test propagation.
type recordingCache struct {
	mu        sync.Mutex
	inner     cache.Cache
	setKeys   []string
	getKeys   []string
	getErr    error
	getCalled int
}

func newRecordingCache() *recordingCache {
	return &recordingCache{inner: cache.NewInMemory()}
}

func (r *recordingCache) Get(ctx context.Context, key string) (any, error) {
	r.mu.Lock()
	r.getKeys = append(r.getKeys, key)
	r.getCalled++
	err := r.getErr
	r.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return r.inner.Get(ctx, key)
}

func (r *recordingCache) Set(ctx context.Context, key string, value any, options ...cache.Option) error {
	r.mu.Lock()
	r.setKeys = append(r.setKeys, key)
	r.mu.Unlock()

	return r.inner.Set(ctx, key, value, options...)
}

func (r *recordingCache) Delete(ctx context.Context, key string) error {
	return r.inner.Delete(ctx, key)
}

func (r *recordingCache) Clear(ctx context.Context) error {
	return r.inner.Clear(ctx)
}

func (r *recordingCache) snapshotSetKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.setKeys))
	copy(out, r.setKeys)

	return out
}

func TestCacheRevocation(t *testing.T) {
	t.Parallel()

	t.Run("revoke_then_is_revoked_returns_true", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		rev := NewCacheRevocation(cache.NewInMemory())

		// ACT
		require.NoError(t, rev.Revoke(ctx, "id-1", time.Hour))
		revoked, err := rev.IsRevoked(ctx, "id-1")

		// ASSERT
		require.NoError(t, err)
		assert.True(t, revoked, "after Revoke the same identifier must read as revoked")
	})

	t.Run("is_revoked_returns_false_for_unknown_identifier", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		rev := NewCacheRevocation(cache.NewInMemory())

		// ACT
		revoked, err := rev.IsRevoked(ctx, "never-revoked")

		// ASSERT
		require.NoError(t, err)
		assert.False(t, revoked, "unknown identifiers must read as not revoked")
	})

	t.Run("revoke_with_zero_ttl_is_noop", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		recorder := newRecordingCache()
		rev := NewCacheRevocation(recorder)

		// ACT
		require.NoError(t, rev.Revoke(ctx, "id-zero", 0))
		revoked, err := rev.IsRevoked(ctx, "id-zero")

		// ASSERT
		require.NoError(t, err)
		assert.False(t, revoked, "zero TTL must not persist a revocation")
		assert.Empty(t, recorder.snapshotSetKeys(), "zero TTL must not call Set on the underlying cache")
	})

	t.Run("revoke_with_negative_ttl_is_noop", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		recorder := newRecordingCache()
		rev := NewCacheRevocation(recorder)

		// ACT
		require.NoError(t, rev.Revoke(ctx, "id-neg", -5*time.Minute))
		revoked, err := rev.IsRevoked(ctx, "id-neg")

		// ASSERT
		require.NoError(t, err)
		assert.False(t, revoked, "negative TTL must not persist a revocation")
		assert.Empty(t, recorder.snapshotSetKeys(), "negative TTL must not call Set on the underlying cache")
	})

	t.Run("cache_get_returns_unrelated_error_propagates", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		recorder := newRecordingCache()
		recorder.getErr = errBackendUnavailable
		rev := NewCacheRevocation(recorder)

		// ACT
		revoked, err := rev.IsRevoked(ctx, "id-err")

		// ASSERT
		require.Error(t, err)
		assert.ErrorIs(t, err, errBackendUnavailable, "non-NotFound errors from the cache must be surfaced")
		assert.False(t, revoked, "on backend error the function must report revoked=false")
	})

	t.Run("key_uses_revocation_prefix", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		recorder := newRecordingCache()
		rev := NewCacheRevocation(recorder)

		// ACT
		require.NoError(t, rev.Revoke(ctx, "abc123", time.Minute))

		// ASSERT
		keys := recorder.snapshotSetKeys()
		require.Len(t, keys, 1, "Revoke must call Set exactly once for a positive TTL")
		assert.Equal(t, "auth:revoked:abc123", keys[0], "key must use the documented prefix")
	})

	t.Run("parallel_revoke_and_is_revoked", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		rev := NewCacheRevocation(cache.NewInMemory())

		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines * 2)

		// ACT
		for i := range goroutines {
			id := "tok-" + strconv.Itoa(i)

			go func() {
				defer wg.Done()
				_ = rev.Revoke(ctx, id, time.Minute)
			}()

			go func() {
				defer wg.Done()
				_, _ = rev.IsRevoked(ctx, id)
			}()
		}

		wg.Wait()

		// ASSERT
		// Smoke test for -race; correctness is asserted by sequential subtests above.
		// Verify the final state for one of the identifiers to ensure the writes landed.
		revoked, err := rev.IsRevoked(ctx, "tok-0")
		require.NoError(t, err)
		assert.True(t, revoked, "after parallel Revoke calls the identifier must read as revoked")
	})
}
