// Package locker provides distributed locks shared between panel instances.
// Backends: Redis (SET NX), any SQL database via the kv_store table, and a
// process-local in-memory fallback for single-instance deployments.
package locker

import (
	"context"
	"time"
)

type Locker interface {
	// Acquire takes the lock for the given key, or returns ErrLocked when it
	// is already held. The lock auto-expires after ttl unless refreshed.
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error)
}

type Lock interface {
	Release(ctx context.Context) error
	Refresh(ctx context.Context, ttl time.Duration) error
}
