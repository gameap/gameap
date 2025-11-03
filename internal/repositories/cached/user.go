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

// UserRepository wraps UserRepository with caching.
type UserRepository struct {
	inner      repositories.UserRepository
	cache      cache.Cache
	wrapper    *Wrapper
	keyBuilder CacheKeyBuilder
}

// NewUserRepository creates a new cached user repository.
func NewUserRepository(
	inner repositories.UserRepository, cache cache.Cache, ttl time.Duration,
) *UserRepository {
	keyBuilder := NewDefaultKeyBuilder("user")
	config := CacheConfig{
		TTL:                ttl,
		KeyBuilder:         keyBuilder,
		InvalidateOnSave:   true,
		InvalidateOnDelete: true,
	}

	return &UserRepository{
		inner:      inner,
		cache:      cache,
		wrapper:    NewWrapper(cache, config),
		keyBuilder: keyBuilder,
	}
}

func (r *UserRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	// Do not cache FindAll results
	return r.inner.FindAll(ctx, order, pagination)
}

// Find retrieves users with filters.
func (r *UserRepository) Find(
	ctx context.Context,
	filter *filters.FindUser,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	if filter == nil || order != nil {
		// Do not cache unfiltered Find results.
		// and results with ordering.
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if pagination != nil && (pagination.Limit > 1 || pagination.Offset > 0) {
		// Do not cache paginated results.
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if filter.FilterCount() <= 0 || filter.FilterCount() > 1 {
		// Do not cache results with multiple filters.
		return r.inner.Find(ctx, filter, order, pagination)
	}

	if len(filter.IDs) == 1 {
		// Special case: if searching by single ID, cache it with dedicated key
		key := r.keyBuilder.BuildKey("id", filter.IDs[0])

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})

		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find user by ID")
		}

		result, err := cache.GetTyped[[]domain.User](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find user by ID")
		}

		return result, nil
	}

	if len(filter.Logins) == 1 {
		// Special case: if searching by single Login, cache it with dedicated key
		key := r.keyBuilder.BuildKey("login", filter.Logins[0])

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find user by Login")
		}

		result, err := cache.GetTyped[[]domain.User](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find user by Login")
		}

		return result, nil
	}

	if len(filter.Emails) == 1 {
		// Special case: if searching by single Email, cache it with dedicated key
		key := r.keyBuilder.BuildKey("email", filter.Emails[0])

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find user by Email")
		}

		result, err := cache.GetTyped[[]domain.User](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find user by Email")
		}

		return result, nil
	}

	// Fallback: do not cache
	return r.inner.Find(ctx, filter, order, pagination)
}

// Save creates or updates a user and invalidates cache.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	// Store user login and email before save (for cache invalidation)
	err := r.inner.Save(ctx, user)
	if err != nil {
		return errors.WithMessage(err, "failed to save user")
	}

	if user.ID != 0 {
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("id", user.ID)); err != nil {
			return errors.WithMessage(err, "failed to invalidate user cache by ID after save")
		}
	}

	// Invalidate cache for this user's login
	if user.Login != "" {
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("login", user.Login)); err != nil {
			return errors.WithMessage(err, "failed to invalidate user cache by login after save")
		}
	}

	// Invalidate cache for this user's email
	if user.Email != "" {
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("email", user.Email)); err != nil {
			return errors.WithMessage(err, "failed to invalidate user cache by email after save")
		}
	}

	if err := r.wrapper.InvalidatePattern(ctx, "user:find*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate user find pattern cache after save")
	}

	return nil
}

// Delete removes a user and invalidates cache.
func (r *UserRepository) Delete(ctx context.Context, id uint) error {
	// Try to get the user first to invalidate its cache
	filter := &filters.FindUser{IDs: []uint{id}}
	users, findErr := r.inner.Find(ctx, filter, nil, nil)

	err := r.inner.Delete(ctx, id)
	if err != nil {
		return errors.WithMessage(err, "failed to delete user")
	}

	key := r.keyBuilder.BuildKey("id", id)
	if err := r.wrapper.Invalidate(ctx, key); err != nil {
		return errors.WithMessage(err, "failed to invalidate user cache by ID after delete")
	}

	if err := r.invalidateUserCache(ctx, findErr, users); err != nil {
		return err
	}

	if err := r.wrapper.InvalidatePattern(ctx, "user:find*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate user find pattern cache after delete")
	}

	return nil
}

func (r *UserRepository) invalidateUserCache(ctx context.Context, findErr error, users []domain.User) error {
	if findErr != nil {
		// Unable to find user for cache invalidation, but this shouldn't fail the delete
		return nil //nolint:nilerr
	}

	if len(users) == 0 {
		return nil
	}

	user := users[0]
	if user.ID != 0 {
		err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("id", user.ID))
		if err != nil {
			return errors.WithMessage(err, "failed to invalidate user cache by ID after delete")
		}
	}
	if user.Login != "" {
		err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("login", user.Login))
		if err != nil {
			return errors.WithMessage(err, "failed to invalidate user cache by login after delete")
		}
	}
	if user.Email != "" {
		err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("email", user.Email))
		if err != nil {
			return errors.WithMessage(err, "failed to invalidate user cache by email after delete")
		}
	}

	return nil
}
