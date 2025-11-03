package cached

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/cache"
)

// CacheKeyBuilder defines how to build cache keys for different operations.
type CacheKeyBuilder interface {
	BuildKey(operation string, params ...any) string
	BuildPattern(operation string) string
}

// DefaultKeyBuilder implements a simple cache key builder.
type DefaultKeyBuilder struct {
	prefix string
}

func NewDefaultKeyBuilder(prefix string) *DefaultKeyBuilder {
	return &DefaultKeyBuilder{prefix: prefix}
}

func (b *DefaultKeyBuilder) BuildKey(operation string, params ...any) string {
	sb := &strings.Builder{}
	sb.Grow(64)

	if len(params) == 0 {
		sb.WriteString(b.prefix)
		sb.WriteByte(':')
		sb.WriteString(operation)

		return sb.String()
	}

	if operation == "id" && len(params) == 1 {
		p, ok := params[0].(uint)

		if ok {
			// Special case for "id" operation with single parameter
			sb.WriteString(b.prefix)
			sb.WriteByte(':')
			sb.WriteString(operation)
			sb.WriteByte(':')
			sb.WriteString(strconv.FormatUint(uint64(p), 10))

			return sb.String()
		}
	}

	// Create a hash of parameters for complex objects
	paramsHash := hashParams(params...)

	sb.WriteString(b.prefix)
	sb.WriteByte(':')
	sb.WriteString(operation)
	sb.WriteByte(':')
	sb.WriteString(paramsHash)

	return sb.String()
}

func (b *DefaultKeyBuilder) BuildPattern(operation string) string {
	sb := &strings.Builder{}
	sb.Grow(64)

	sb.WriteString(b.prefix)
	sb.WriteByte(':')
	sb.WriteString(operation)
	sb.WriteString(":*")

	return sb.String()
}

// hashParams creates a deterministic hash of parameters.
func hashParams(params ...any) string {
	data, err := json.Marshal(params)
	if err != nil {
		// Fallback to simple string representation
		return fmt.Sprintf("%v", params)
	}
	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:])
}

// CacheConfig contains configuration for cached repository.
type CacheConfig struct {
	TTL                time.Duration
	KeyBuilder         CacheKeyBuilder
	InvalidateOnSave   bool
	InvalidateOnDelete bool
}

// Wrapper wraps any repository operation with caching.
type Wrapper struct {
	cache  cache.Cache
	config CacheConfig
}

func NewWrapper(cache cache.Cache, config CacheConfig) *Wrapper {
	return &Wrapper{
		cache:  cache,
		config: config,
	}
}

// GetOrSet tries to get value from cache, if not found calls the loader function.
func (w *Wrapper) GetOrSet(
	ctx context.Context, key string, loader func() (any, error),
) (any, error) {
	// Try to get from cache
	cached, err := w.cache.Get(ctx, key)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Cache miss or error - load from source
	// Note: We intentionally ignore cache errors and continue with loader

	// Load from source
	result, err := loader()
	if err != nil {
		return nil, err
	}

	if err := w.cache.Set(ctx, key, result, cache.WithExpiration(w.config.TTL)); err != nil {
		slog.ErrorContext(ctx, "failed to set cache", "error", err, "key", key)
	}

	return result, nil
}

// InvalidatePattern removes all cache entries matching a pattern.
func (w *Wrapper) InvalidatePattern(ctx context.Context, pattern string) error {
	if redisCache, ok := w.cache.(*cache.Redis); ok {
		return redisCache.DeletePattern(ctx, pattern)
	}
	// For non-Redis caches, we might not support pattern deletion
	// In that case, clear the entire cache (nuclear option)
	slog.WarnContext(ctx, "clearing entire cache due to pattern invalidation on non-Redis cache", "pattern", pattern)

	return w.cache.Clear(ctx)
}

// Invalidate removes specific cache entries.
func (w *Wrapper) Invalidate(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := w.cache.Delete(ctx, key); err != nil && !errors.Is(err, cache.ErrNotFound) {
			return err
		}
	}

	return nil
}

// CacheStats tracks cache hit/miss statistics.
// Uses atomic operations to ensure thread-safety.
type CacheStats struct {
	Hits   atomic.Int64
	Misses atomic.Int64
}

// GetWithStats is like GetOrSet but also tracks statistics.
func (w *Wrapper) GetWithStats(
	ctx context.Context, key string, loader func() (any, error), stats *CacheStats,
) (any, error) {
	cached, err := w.cache.Get(ctx, key)
	if err == nil && cached != nil {
		if stats != nil {
			stats.Hits.Add(1)
		}

		return cached, nil
	}

	if stats != nil {
		stats.Misses.Add(1)
	}

	// Load and cache
	return w.GetOrSet(ctx, key, loader)
}
