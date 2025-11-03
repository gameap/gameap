package cached

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

// PersonalAccessTokenRepository wraps PersonalAccessTokenRepository with caching.
type PersonalAccessTokenRepository struct {
	inner      repositories.PersonalAccessTokenRepository
	cache      cache.Cache
	wrapper    *Wrapper
	keyBuilder CacheKeyBuilder
}

// NewPersonalAccessTokenRepository creates a new cached PAT repository.
func NewPersonalAccessTokenRepository(
	inner repositories.PersonalAccessTokenRepository, cache cache.Cache, ttl time.Duration,
) *PersonalAccessTokenRepository {
	keyBuilder := NewDefaultKeyBuilder("pat")
	config := CacheConfig{
		TTL:                ttl,
		KeyBuilder:         keyBuilder,
		InvalidateOnSave:   true,
		InvalidateOnDelete: true,
	}

	return &PersonalAccessTokenRepository{
		inner:      inner,
		cache:      cache,
		wrapper:    NewWrapper(cache, config),
		keyBuilder: keyBuilder,
	}
}

// hashToken creates a secure hash of the token for cache key.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}

// Find retrieves tokens with filters - most importantly by token value.
func (r *PersonalAccessTokenRepository) Find(
	ctx context.Context,
	filter *filters.FindPersonalAccessToken,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PersonalAccessToken, error) {
	if filter == nil || len(order) > 0 {
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if pagination != nil && (pagination.Limit > 1 || pagination.Offset > 0) {
		// Do not cache paginated results.
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if filter.FilterCount() > 1 {
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if len(filter.IDs) == 1 {
		key := r.keyBuilder.BuildKey("id", filter.IDs[0])

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find PAT by ID")
		}

		data, err := cache.GetTyped[[]domain.PersonalAccessToken](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find PAT by ID")
		}

		return data, nil
	}

	if len(filter.Tokens) == 1 {
		tokenHash := hashToken(filter.Tokens[0])
		key := r.keyBuilder.BuildKey("token", tokenHash)

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})

		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find PAT by token")
		}

		data, err := cache.GetTyped[[]domain.PersonalAccessToken](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find PAT by token")
		}

		return data, nil
	}

	return r.inner.Find(ctx, filter, order, pagination)
}

// Save creates or updates a token and invalidates cache.
func (r *PersonalAccessTokenRepository) Save(ctx context.Context, token *domain.PersonalAccessToken) error {
	// Store the token value before save (for cache invalidation)
	var tokenValue string
	if token.Token != "" {
		tokenValue = token.Token
	}

	err := r.inner.Save(ctx, token)
	if err != nil {
		return errors.WithMessage(err, "failed to save PAT")
	}

	// Invalidate cache for this token
	if tokenValue != "" {
		tokenHash := hashToken(tokenValue)
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("token", tokenHash)); err != nil {
			return errors.WithMessage(err, "failed to invalidate PAT cache by token after save")
		}
	}

	if err := r.wrapper.InvalidatePattern(ctx, "pat:id:*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate PAT find pattern cache after save")
	}

	if err := r.wrapper.InvalidatePattern(ctx, "pat:find:*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate PAT find pattern cache after save")
	}

	return nil
}

// Delete removes a token and invalidates cache.
func (r *PersonalAccessTokenRepository) Delete(ctx context.Context, id uint) error {
	// Try to get the token first to invalidate its cache
	filter := &filters.FindPersonalAccessToken{IDs: []uint{id}}
	tokens, findErr := r.inner.Find(ctx, filter, nil, nil)

	err := r.inner.Delete(ctx, id)
	if err != nil {
		return errors.WithMessage(err, "failed to delete PAT")
	}

	// Invalidate cache
	if findErr == nil && len(tokens) > 0 && tokens[0].Token != "" {
		tokenHash := hashToken(tokens[0].Token)
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("token", tokenHash)); err != nil {
			return errors.WithMessage(err, "failed to invalidate PAT cache by token after delete")
		}
	}

	key := r.keyBuilder.BuildKey("id", id)

	if err = r.wrapper.Invalidate(ctx, key); err != nil {
		return errors.WithMessage(err, "failed to invalidate PAT cache by ID after delete")
	}

	if err = r.wrapper.InvalidatePattern(ctx, "pat:find:*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate PAT find pattern cache after delete")
	}

	return nil
}

// UpdateLastUsedAt updates the last used timestamp
// Note: We might choose NOT to invalidate cache here to avoid cache thrashing
// since this is updated on every request.
func (r *PersonalAccessTokenRepository) UpdateLastUsedAt(
	ctx context.Context, id uint, lastUsedAt time.Time,
) error {
	err := r.inner.UpdateLastUsedAt(ctx, id, lastUsedAt)
	if err != nil {
		return errors.WithMessage(err, "failed to update PAT last used at")
	}

	key := r.keyBuilder.BuildKey("id", id)

	// Update value in cache if exists
	cachedData, err := cache.GetTyped[[]domain.PersonalAccessToken](ctx, r.cache, key)
	if err == nil && len(cachedData) == 1 {
		cachedData[0].LastUsedAt = &lastUsedAt
		if setErr := r.cache.Set(ctx, key, cachedData, cache.WithExpiration(r.wrapper.config.TTL)); setErr != nil {
			return errors.WithMessage(setErr, "failed to update cached PAT last used at")
		}
	}

	// Optional: Invalidate cache instead of updating to avoid stale data
	// Commented out to avoid cache thrashing on every request
	// _ = r.wrapper.InvalidatePattern(ctx, "pat:*")

	return nil
}
