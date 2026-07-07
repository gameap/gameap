package cached

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

// GameRepository wraps GameRepository with caching.
type GameRepository struct {
	inner      repositories.GameRepository
	cache      cache.Cache
	wrapper    *Wrapper
	keyBuilder CacheKeyBuilder
}

// NewGameRepository creates a new cached game repository.
func NewGameRepository(
	inner repositories.GameRepository, cache cache.Cache, ttl time.Duration,
) *GameRepository {
	keyBuilder := NewDefaultKeyBuilder("games")
	config := CacheConfig{
		TTL:                ttl,
		KeyBuilder:         keyBuilder,
		InvalidateOnSave:   true,
		InvalidateOnDelete: true,
	}

	return &GameRepository{
		inner:      inner,
		cache:      cache,
		wrapper:    NewWrapper(cache, config),
		keyBuilder: keyBuilder,
	}
}

// FindAll retrieves all games with pagination - frequently called.
func (r *GameRepository) FindAll(
	ctx context.Context, order []filters.Sorting, pagination *filters.Pagination,
) ([]domain.Game, error) {
	key := r.keyBuilder.BuildKey("all", order, pagination)

	data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Game, error) {
		return r.inner.FindAll(ctx, order, pagination)
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load FindAll games")
	}

	return data, nil
}

// Find retrieves games with specific filters.
func (r *GameRepository) Find(
	ctx context.Context,
	filter *filters.FindGame,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
	if filter == nil || len(order) > 0 {
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if pagination != nil && (pagination.Limit > 1 || pagination.Offset > 0) {
		// Do not cache paginated results.
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if filter.FilterCount() > 1 {
		// Complex filter - skip caching
		return r.inner.Find(ctx, filter, order, pagination)
	}

	// Special case: if searching by codes (common), use specific cache key
	if len(filter.Codes) > 0 {
		key := r.keyBuilder.BuildKey("codes", filter.Codes)

		data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Game, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load Find games by codes")
		}

		return data, nil
	}

	// Fallback: no caching
	return r.inner.Find(ctx, filter, order, pagination)
}

// Save creates or updates a game and invalidates cache.
func (r *GameRepository) Save(ctx context.Context, game *domain.Game) error {
	err := r.inner.Save(ctx, game)
	if err != nil {
		return err
	}

	// Write committed; invalidation is best-effort so a cache hiccup does not
	// report the applied write as failed. Stale entries expire with TTL.
	r.invalidateAllGameCache(ctx)

	return nil
}

// Delete removes a game and invalidates cache.
func (r *GameRepository) Delete(ctx context.Context, code string) error {
	err := r.inner.Delete(ctx, code)
	if err != nil {
		return err
	}

	r.invalidateAllGameCache(ctx)

	return nil
}

func (r *GameRepository) invalidateAllGameCache(ctx context.Context) {
	r.wrapper.InvalidatePatternBestEffort(ctx, "games:*")
}

// GameModRepository wraps GameModRepository with caching.
type GameModRepository struct {
	inner      repositories.GameModRepository
	cache      cache.Cache
	wrapper    *Wrapper
	keyBuilder CacheKeyBuilder
}
