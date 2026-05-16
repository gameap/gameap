package shorttoken

import (
	"context"

	"github.com/gameap/gameap/internal/cache"
)

// tokenCache is the slice of cache.Cache the handler needs to persist a
// freshly minted short-lived token. Keeping it narrow makes the handler
// trivially fakeable in tests.
type tokenCache interface {
	Set(ctx context.Context, key string, value any, options ...cache.Option) error
}
