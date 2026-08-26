package cache

import "context"

type Cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, options ...Option) error
	Delete(ctx context.Context, key string) error
	// Pull atomically returns a key's value and removes it, reporting
	// ErrNotFound when the key is absent or expired. Concurrent callers race for
	// the single value: exactly one receives it, every other gets ErrNotFound.
	// It backs one-time tokens (an SSO ticket) where a plain Get-then-Delete
	// would leave a replay window between the two calls.
	Pull(ctx context.Context, key string) (any, error)
	Clear(ctx context.Context) error
}
