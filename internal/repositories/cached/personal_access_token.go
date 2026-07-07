package cached

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
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

		data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.PersonalAccessToken, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load Find PAT by ID")
		}

		return data, nil
	}

	if len(filter.Tokens) == 1 {
		key := r.keyBuilder.BuildKey("token", hashToken(filter.Tokens[0]))

		data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.PersonalAccessToken, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load Find PAT by token")
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

	// Write committed; invalidation is best-effort so a cache hiccup does not
	// report the applied write as failed. Stale entries expire with TTL.
	if tokenValue != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("token", hashToken(tokenValue)))
	}

	r.wrapper.InvalidatePatternBestEffort(ctx, "pat:id:*")
	r.wrapper.InvalidatePatternBestEffort(ctx, "pat:find:*")

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

	// Write committed; invalidation is best-effort so a cache hiccup does not
	// report the applied delete as failed. Stale entries expire with TTL.
	if findErr == nil && len(tokens) > 0 && tokens[0].Token != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("token", hashToken(tokens[0].Token)))
	}

	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("id", id))
	r.wrapper.InvalidatePatternBestEffort(ctx, "pat:find:*")

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
		// Best-effort cache refresh: the DB write already succeeded, so a cache
		// write failure must not fail the request.
		if setErr := r.cache.Set(ctx, key, cachedData, cache.WithExpiration(r.wrapper.config.TTL)); setErr != nil {
			slog.WarnContext(ctx, "failed to update cached PAT last used at", "error", setErr)
		}
	}

	// Optional: Invalidate cache instead of updating to avoid stale data
	// Commented out to avoid cache thrashing on every request
	// _ = r.wrapper.InvalidatePattern(ctx, "pat:*")

	return nil
}
