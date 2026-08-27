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

	load := func() ([]domain.User, error) {
		return r.inner.Find(ctx, filter, order, pagination)
	}

	switch {
	case len(filter.IDs) == 1:
		return r.cachedFind(ctx, r.keyBuilder.BuildKey("id", filter.IDs[0]), load)
	case len(filter.Logins) == 1:
		return r.cachedFind(ctx, r.keyBuilder.BuildKey("login", filter.Logins[0]), load)
	case len(filter.Emails) == 1:
		// Hashed verbatim, like the login key above: services.UserService has
		// already lowercased both. Folding case here instead would let a cache
		// hit match a spelling that a cache miss — served by an exact-comparing
		// repository — would not.
		return r.cachedFind(ctx, r.keyBuilder.BuildKey("email", filter.Emails[0]), load)
	default:
		// Fallback: do not cache
		return load()
	}
}

func (r *UserRepository) cachedFind(
	ctx context.Context, key string, load func() ([]domain.User, error),
) ([]domain.User, error) {
	data, err := GetOrLoad(ctx, r.wrapper, key, load)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load cached user find")
	}

	return data, nil
}

// Save creates or updates a user and invalidates cache.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	// The login and email keys are derived from the values being written, so
	// changing either would leave the entry under the previous value behind,
	// still serving the whole row — password hash included — until its TTL runs
	// out. Read what this save overwrites first, so both spellings can be
	// dropped.
	prev := r.overwrittenUsers(ctx, user)

	err := r.inner.Save(ctx, user)
	if err != nil {
		return errors.WithMessage(err, "failed to save user")
	}

	// Write committed; invalidation is best-effort so a cache hiccup does not
	// report the applied write as failed. Stale entries expire with TTL.
	for i := range prev {
		r.invalidateKeys(ctx, &prev[i])
	}

	r.invalidateKeys(ctx, user)

	return nil
}

// overwrittenUsers returns the rows this save is about to replace, whose cached
// login and email keys would otherwise survive the write.
//
// An ID-less save is not always an insert: the MySQL upsert names no conflict
// target, so a login or email already in the table updates that row rather than
// adding one. Both identifiers are looked up because a row can be reached by
// either, and MySQL picks only one of them when they point at different rows.
func (r *UserRepository) overwrittenUsers(ctx context.Context, user *domain.User) []domain.User {
	if user.ID != 0 {
		return r.findUncached(ctx, filters.FindUserByIDs(user.ID))
	}

	var found []domain.User

	if user.Login != "" {
		found = append(found, r.findUncached(ctx, filters.FindUserByLogins(user.Login))...)
	}

	if user.Email != "" {
		found = append(found, r.findUncached(ctx, filters.FindUserByEmails(user.Email))...)
	}

	return found
}

// findUncached reads through to the wrapped repository: invalidation has to be
// driven by what is committed, never by what the cache still believes. A failed
// read yields nothing, leaving the keys to expire with their TTL rather than
// failing a write that has not happened yet.
func (r *UserRepository) findUncached(ctx context.Context, filter *filters.FindUser) []domain.User {
	users, err := r.inner.Find(ctx, filter, nil, nil)
	if err != nil {
		return nil
	}

	return users
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

	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("id", id))
	r.invalidateUserCache(ctx, findErr, users)

	return nil
}

func (r *UserRepository) invalidateUserCache(ctx context.Context, findErr error, users []domain.User) {
	if findErr != nil || len(users) == 0 {
		return
	}

	r.invalidateKeys(ctx, &users[0])
}

// invalidateKeys drops every cache key that resolves to this user.
func (r *UserRepository) invalidateKeys(ctx context.Context, user *domain.User) {
	if user.ID != 0 {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("id", user.ID))
	}

	if user.Login != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("login", user.Login))
	}

	if user.Email != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("email", user.Email))
	}
}
