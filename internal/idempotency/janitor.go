package idempotency

import (
	"context"
	"log/slog"
	"time"
)

const defaultJanitorInterval = time.Hour

// Janitor deletes expired outcomes of the database driver; Redis expires its
// keys on its own. Every panel instance may run one: deleting expired rows
// twice is harmless.
type Janitor struct {
	store    ExpiredDeleter
	interval time.Duration
	now      func() time.Time
	logger   *slog.Logger
}

// NewJanitor returns a Janitor that sweeps every interval; a non-positive
// interval falls back to one hour.
func NewJanitor(store ExpiredDeleter, interval time.Duration, logger *slog.Logger) *Janitor {
	if interval <= 0 {
		interval = defaultJanitorInterval
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Janitor{
		store:    store,
		interval: interval,
		now:      time.Now,
		logger:   logger,
	}
}

// Run sweeps once immediately and then every interval until ctx is done.
func (j *Janitor) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	j.Sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.Sweep(ctx)
		}
	}
}

func (j *Janitor) Sweep(ctx context.Context) {
	deleted, err := j.store.DeleteExpired(ctx, j.now())
	if err != nil {
		j.logger.WarnContext(ctx, "idempotency janitor failed to delete expired keys", slog.String("error", err.Error()))

		return
	}

	if deleted > 0 {
		j.logger.DebugContext(ctx, "idempotency janitor deleted expired keys", slog.Int("deleted", deleted))
	}
}
