package cache

import "context"

type Cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, options ...Option) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}
