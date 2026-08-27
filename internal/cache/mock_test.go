package cache_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errInjectedGet    = errors.New("injected get failure")
	errInjectedSet    = errors.New("injected set failure")
	errInjectedDelete = errors.New("injected delete failure")
	errInjectedClear  = errors.New("injected clear failure")
	errInjectedPull   = errors.New("injected pull failure")
)

// failingCache is a Cache implementation that returns the configured per-method
// error on every call. It also records call counts so callers can assert how
// many times each method was invoked. failingCache is intentionally exposed
// only to tests in this package; consumers in other packages that need to
// inject cache failures should copy this type or define their own.
type failingCache struct {
	getErr    error
	setErr    error
	deleteErr error
	clearErr  error
	pullErr   error

	getCalls    atomic.Int64
	setCalls    atomic.Int64
	deleteCalls atomic.Int64
	clearCalls  atomic.Int64
	pullCalls   atomic.Int64
}

func (f *failingCache) Get(_ context.Context, _ string) (any, error) {
	f.getCalls.Add(1)

	return nil, f.getErr
}

func (f *failingCache) Set(_ context.Context, _ string, _ any, _ ...cache.Option) error {
	f.setCalls.Add(1)

	return f.setErr
}

func (f *failingCache) Delete(_ context.Context, _ string) error {
	f.deleteCalls.Add(1)

	return f.deleteErr
}

func (f *failingCache) Pull(_ context.Context, _ string) (any, error) {
	f.pullCalls.Add(1)

	return nil, f.pullErr
}

func (f *failingCache) Clear(_ context.Context) error {
	f.clearCalls.Add(1)

	return f.clearErr
}

// Compile-time check that failingCache satisfies the Cache interface.
var _ cache.Cache = (*failingCache)(nil)

func TestFailingCache_PropagatesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fake      *failingCache
		invoke    func(ctx context.Context, c cache.Cache) error
		callCount func(*failingCache) int64
		wantError string
	}{
		{
			name: "get_returns_configured_error",
			fake: &failingCache{getErr: errInjectedGet},
			invoke: func(ctx context.Context, c cache.Cache) error {
				_, err := c.Get(ctx, "any_key")

				return err
			},
			callCount: func(f *failingCache) int64 { return f.getCalls.Load() },
			wantError: "injected get failure",
		},
		{
			name: "set_returns_configured_error",
			fake: &failingCache{setErr: errInjectedSet},
			invoke: func(ctx context.Context, c cache.Cache) error {
				return c.Set(ctx, "any_key", "any_value")
			},
			callCount: func(f *failingCache) int64 { return f.setCalls.Load() },
			wantError: "injected set failure",
		},
		{
			name: "set_with_options_returns_configured_error",
			fake: &failingCache{setErr: errInjectedSet},
			invoke: func(ctx context.Context, c cache.Cache) error {
				return c.Set(ctx, "any_key", "any_value", cache.WithExpiration(0))
			},
			callCount: func(f *failingCache) int64 { return f.setCalls.Load() },
			wantError: "injected set failure",
		},
		{
			name: "delete_returns_configured_error",
			fake: &failingCache{deleteErr: errInjectedDelete},
			invoke: func(ctx context.Context, c cache.Cache) error {
				return c.Delete(ctx, "any_key")
			},
			callCount: func(f *failingCache) int64 { return f.deleteCalls.Load() },
			wantError: "injected delete failure",
		},
		{
			name: "pull_returns_configured_error",
			fake: &failingCache{pullErr: errInjectedPull},
			invoke: func(ctx context.Context, c cache.Cache) error {
				_, err := c.Pull(ctx, "any_key")

				return err
			},
			callCount: func(f *failingCache) int64 { return f.pullCalls.Load() },
			wantError: "injected pull failure",
		},
		{
			name: "clear_returns_configured_error",
			fake: &failingCache{clearErr: errInjectedClear},
			invoke: func(ctx context.Context, c cache.Cache) error {
				return c.Clear(ctx)
			},
			callCount: func(f *failingCache) int64 { return f.clearCalls.Load() },
			wantError: "injected clear failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := context.Background()

			// ACT
			err := tt.invoke(ctx, tt.fake)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message must contain injected text")
			assert.Equal(t, int64(1), tt.callCount(tt.fake), "method must be invoked exactly once")
		})
	}
}

func TestFailingCache_GetReturnsNilValueOnError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	fake := &failingCache{getErr: errInjectedGet}

	// ACT
	value, err := fake.Get(context.Background(), "anything")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, value, "Get must return nil value when configured to error")
}

func TestFailingCache_NoErrorWhenUnconfigured(t *testing.T) {
	t.Parallel()

	// ARRANGE
	fake := &failingCache{}
	ctx := context.Background()

	// ACT / ASSERT — every method returns nil error and zero value when
	// no error has been injected. Callers that omit a *Err field get a
	// trivially-successful cache.
	value, err := fake.Get(ctx, "k")
	require.NoError(t, err)
	assert.Nil(t, value, "unconfigured Get must return nil value")

	require.NoError(t, fake.Set(ctx, "k", "v"))
	require.NoError(t, fake.Delete(ctx, "k"))
	require.NoError(t, fake.Clear(ctx))
}

func TestFailingCache_CallCountsAccumulate(t *testing.T) {
	t.Parallel()

	// ARRANGE
	fake := &failingCache{}
	ctx := context.Background()

	// ACT
	for range 3 {
		_, _ = fake.Get(ctx, "k")
	}
	for range 2 {
		_ = fake.Set(ctx, "k", "v")
	}
	_ = fake.Delete(ctx, "k")
	_ = fake.Clear(ctx)

	// ASSERT
	assert.Equal(t, int64(3), fake.getCalls.Load(), "Get must accumulate one count per call")
	assert.Equal(t, int64(2), fake.setCalls.Load(), "Set must accumulate one count per call")
	assert.Equal(t, int64(1), fake.deleteCalls.Load(), "Delete must accumulate one count per call")
	assert.Equal(t, int64(1), fake.clearCalls.Load(), "Clear must accumulate one count per call")
}
