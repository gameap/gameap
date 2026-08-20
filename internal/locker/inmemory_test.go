package locker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/locker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryLocker_Acquire(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			l := locker.NewInMemoryLocker()
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

func TestInMemoryLocker_MutualExclusion(t *testing.T) {
	t.Parallel()
	l := locker.NewInMemoryLocker()
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

func TestInMemoryLocker_TTLExpiry(t *testing.T) {
	t.Parallel()
	l := locker.NewInMemoryLocker()
	ctx := context.Background()

	_, err := l.Acquire(ctx, "ephemeral", 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(80 * time.Millisecond)

	second, err := l.Acquire(ctx, "ephemeral", 1*time.Second)
	require.NoError(t, err)
	require.NoError(t, second.Release(ctx))
}

func TestInMemoryLocker_Refresh(t *testing.T) {
	t.Parallel()
	l := locker.NewInMemoryLocker()
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "refresh-test", 50*time.Millisecond)
	require.NoError(t, err)

	require.NoError(t, lock.Refresh(ctx, 1*time.Second))

	time.Sleep(80 * time.Millisecond)

	_, err = l.Acquire(ctx, "refresh-test", 1*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLocked))

	require.NoError(t, lock.Release(ctx))
}

func TestInMemoryLocker_RefreshLostAfterExpiry(t *testing.T) {
	t.Parallel()
	l := locker.NewInMemoryLocker()
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "expired", 50*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(80 * time.Millisecond)

	err = lock.Refresh(ctx, 1*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLockLost),
		"an expired lock must not be resurrected by refresh")
}

func TestInMemoryLocker_RefreshAfterRelease(t *testing.T) {
	t.Parallel()
	l := locker.NewInMemoryLocker()
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "released", 1*time.Second)
	require.NoError(t, err)

	require.NoError(t, lock.Release(ctx))

	err = lock.Refresh(ctx, 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lock already released")
}

func TestInMemoryLocker_DoubleRelease(t *testing.T) {
	t.Parallel()
	l := locker.NewInMemoryLocker()
	ctx := context.Background()

	lock, err := l.Acquire(ctx, "double-release", 1*time.Second)
	require.NoError(t, err)

	require.NoError(t, lock.Release(ctx))
	require.NoError(t, lock.Release(ctx))
}
