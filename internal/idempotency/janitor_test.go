package idempotency_test

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/idempotency"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingDeleter struct {
	calls atomic.Int32
}

func (d *countingDeleter) DeleteExpired(context.Context, time.Time) (int, error) {
	d.calls.Add(1)

	return 0, nil
}

func TestJanitor_Sweep_deletes_expired_outcomes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := inmemory.NewIdempotencyKeyRepository()
	now := time.Now()

	for _, record := range []*domain.IdempotencyKey{
		{UserID: 1, KeyHash: "expired", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)},
		{UserID: 1, KeyHash: "live", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		saved, err := repo.Save(ctx, record)
		require.NoError(t, err)
		require.True(t, saved)
	}

	idempotency.NewJanitor(repo, time.Hour, nil).Sweep(ctx)

	// Looked up from before its expiry, so a hit would mean the row survived.
	expired, err := repo.Find(ctx, 1, "expired", now.Add(-90*time.Minute))
	require.NoError(t, err)
	assert.Nil(t, expired)

	live, err := repo.Find(ctx, 1, "live", now)
	require.NoError(t, err)
	assert.NotNil(t, live)
}

func TestJanitor_Run_sweeps_every_interval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		interval     time.Duration
		wantInterval time.Duration
	}{
		{
			name:         "configured_interval",
			interval:     10 * time.Minute,
			wantInterval: 10 * time.Minute,
		},
		{
			name:         "non_positive_interval_falls_back_to_one_hour",
			interval:     0,
			wantInterval: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				deleter := &countingDeleter{}
				ctx, cancel := context.WithCancel(t.Context())

				go idempotency.NewJanitor(deleter, tt.interval, nil).Run(ctx)

				synctest.Wait()
				assert.Equal(t, int32(1), deleter.calls.Load(), "sweeps once on start")

				time.Sleep(tt.wantInterval - time.Second)
				synctest.Wait()
				assert.Equal(t, int32(1), deleter.calls.Load())

				time.Sleep(time.Second)
				synctest.Wait()
				assert.Equal(t, int32(2), deleter.calls.Load())

				cancel()
				synctest.Wait()
			})
		})
	}
}
