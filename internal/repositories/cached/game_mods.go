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

// NewGameModRepository creates a new cached game mod repository.
func NewGameModRepository(
	inner repositories.GameModRepository, cache cache.Cache, ttl time.Duration,
) *GameModRepository {
	keyBuilder := NewDefaultKeyBuilder("gamemods")
	config := CacheConfig{
		TTL:                ttl,
		KeyBuilder:         keyBuilder,
		InvalidateOnSave:   true,
		InvalidateOnDelete: true,
	}

	return &GameModRepository{
		inner:      inner,
		cache:      cache,
		wrapper:    NewWrapper(cache, config),
		keyBuilder: keyBuilder,
	}
}

// FindAll retrieves all game mods with pagination.
func (r *GameModRepository) FindAll(
	ctx context.Context, order []filters.Sorting, pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	if len(order) > 1 {
		return r.inner.FindAll(ctx, order, pagination)
	}

	if len(order) == 1 && order[0].Field != "name" {
		return r.inner.FindAll(ctx, order, pagination)
	}

	if pagination != nil && (pagination.Limit > 1 || pagination.Offset > 0) {
		// Do not cache paginated results.
		return r.inner.FindAll(ctx, order, pagination)
	}

	key := r.keyBuilder.BuildKey("all", order, pagination)

	_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
		return r.inner.FindAll(ctx, order, pagination)
	})

	if err != nil {
		return nil, errors.WithMessage(err, "failed to get or set cache for FindAll game mods")
	}

	data, err := cache.GetTyped[[]domain.GameMod](ctx, r.cache, key)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get typed cached data for FindAll game mods")
	}

	return data, nil
}

// Find retrieves game mods with specific filters.
func (r *GameModRepository) Find(
	ctx context.Context,
	filter *filters.FindGameMod,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	if filter == nil || filter.FilterCount() > 1 {
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if len(order) == 1 && order[0].Field != "name" {
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if pagination != nil && (pagination.Limit > 1 || pagination.Offset > 0) {
		// Do not cache paginated results.
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if len(filter.GameCodes) > 0 {
		key := r.keyBuilder.BuildKey("gamecodes", filter.GameCodes, filter, order, pagination)

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})

		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find game mods by game codes")
		}

		data, err := cache.GetTyped[[]domain.GameMod](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find game mods by game codes")
		}

		return data, nil
	}

	return r.inner.Find(ctx, filter, order, pagination)
}

// Save creates or updates a game mod and invalidates cache.
func (r *GameModRepository) Save(ctx context.Context, gameMod *domain.GameMod) error {
	err := r.inner.Save(ctx, gameMod)
	if err != nil {
		return err
	}

	if err := r.invalidateAllGameModCache(ctx); err != nil {
		return errors.WithMessage(err, "failed to invalidate game mod cache after save")
	}

	return nil
}

// Delete removes a game mod and invalidates cache.
func (r *GameModRepository) Delete(ctx context.Context, id uint) error {
	err := r.inner.Delete(ctx, id)
	if err != nil {
		return err
	}

	if err := r.invalidateAllGameModCache(ctx); err != nil {
		return errors.WithMessage(err, "failed to invalidate game mod cache after delete")
	}

	return nil
}

func (r *GameModRepository) invalidateAllGameModCache(ctx context.Context) error {
	return r.wrapper.InvalidatePattern(ctx, "gamemods:*")
}
