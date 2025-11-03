package cached

import (
	"context"
	"fmt"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

// RBACRepository wraps RBACRepository with caching.
type RBACRepository struct {
	inner      repositories.RBACRepository
	cache      cache.Cache
	wrapper    *Wrapper
	keyBuilder CacheKeyBuilder
}

// NewRBACRepository creates a new cached RBAC repository.
func NewRBACRepository(
	inner repositories.RBACRepository, cache cache.Cache, ttl time.Duration,
) *RBACRepository {
	keyBuilder := NewDefaultKeyBuilder("rbac")
	config := CacheConfig{
		TTL:                ttl,
		KeyBuilder:         keyBuilder,
		InvalidateOnSave:   true,
		InvalidateOnDelete: true,
	}

	return &RBACRepository{
		inner:      inner,
		cache:      cache,
		wrapper:    NewWrapper(cache, config),
		keyBuilder: keyBuilder,
	}
}

// GetRoles retrieves all roles - frequently called, perfect for caching.
func (r *RBACRepository) GetRoles(ctx context.Context) ([]domain.Role, error) {
	key := r.keyBuilder.BuildKey("roles", "all")

	_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
		return r.inner.GetRoles(ctx)
	})

	if err != nil {
		return nil, errors.WithMessage(err, "failed to get or set cache for GetRoles")
	}

	data, err := cache.GetTyped[[]domain.Role](ctx, r.cache, key)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get typed cached data for GetRoles")
	}

	return data, nil
}

// SaveRole saves a role and invalidates cache.
func (r *RBACRepository) SaveRole(ctx context.Context, role *domain.Role) error {
	err := r.inner.SaveRole(ctx, role)
	if err != nil {
		return errors.WithMessage(err, "failed to save role")
	}

	if err := r.invalidateRoleCache(ctx); err != nil {
		return errors.WithMessage(err, "failed to invalidate role cache after save")
	}

	return nil
}

// GetPermissions retrieves permissions for an entity - very frequently called.
func (r *RBACRepository) GetPermissions(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) ([]domain.Permission, error) {
	key := r.keyBuilder.BuildKey("permissions", fmt.Sprintf("%s_%d", entityType, entityID))

	_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
		return r.inner.GetPermissions(ctx, entityID, entityType)
	})

	if err != nil {
		return nil, errors.WithMessage(err, "failed to get or set cache for GetPermissions")
	}

	data, err := cache.GetTyped[[]domain.Permission](ctx, r.cache, key)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get typed cached data for GetPermissions")
	}

	return data, nil
}

// GetRolesForEntity gets roles assigned to an entity - frequently called in auth.
func (r *RBACRepository) GetRolesForEntity(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) ([]domain.RestrictedRole, error) {
	key := r.keyBuilder.BuildKey("roles", fmt.Sprintf("%s_%d", entityType, entityID))

	_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
		return r.inner.GetRolesForEntity(ctx, entityID, entityType)
	})

	if err != nil {
		return nil, errors.WithMessage(err, "failed to get or set cache for GetRolesForEntity")
	}

	data, err := cache.GetTyped[[]domain.RestrictedRole](ctx, r.cache, key)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get typed cached data for GetRolesForEntity")
	}

	return data, nil
}

// AssignRolesForEntity assigns roles and invalidates cache.
func (r *RBACRepository) AssignRolesForEntity(
	ctx context.Context, entityID uint, entityType domain.EntityType, roles []domain.RestrictedRole,
) error {
	err := r.inner.AssignRolesForEntity(ctx, entityID, entityType, roles)
	if err != nil {
		return errors.WithMessage(err, "failed to assign roles for entity")
	}

	if err := r.invalidateEntityCache(ctx, entityID, entityType); err != nil {
		return errors.WithMessage(err, "failed to invalidate entity cache after assign roles")
	}

	return nil
}

// ClearRolesForEntity clears roles and invalidates cache.
func (r *RBACRepository) ClearRolesForEntity(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) error {
	err := r.inner.ClearRolesForEntity(ctx, entityID, entityType)
	if err != nil {
		return errors.WithMessage(err, "failed to clear roles for entity")
	}

	if err := r.invalidateEntityCache(ctx, entityID, entityType); err != nil {
		return errors.WithMessage(err, "failed to invalidate entity cache after clear roles")
	}

	return nil
}

// Allow grants permission and invalidates cache.
func (r *RBACRepository) Allow(
	ctx context.Context, entityID uint, entityType domain.EntityType, abilities []domain.Ability,
) error {
	err := r.inner.Allow(ctx, entityID, entityType, abilities)
	if err != nil {
		return errors.WithMessage(err, "failed to allow abilities")
	}

	if err := r.invalidatePermissionCache(ctx, entityID, entityType); err != nil {
		return errors.WithMessage(err, "failed to invalidate permission cache after allow")
	}

	return nil
}

// Forbid denies permission and invalidates cache.
func (r *RBACRepository) Forbid(
	ctx context.Context, entityID uint, entityType domain.EntityType, abilities []domain.Ability,
) error {
	err := r.inner.Forbid(ctx, entityID, entityType, abilities)
	if err != nil {
		return errors.WithMessage(err, "failed to forbid abilities")
	}

	if err := r.invalidatePermissionCache(ctx, entityID, entityType); err != nil {
		return errors.WithMessage(err, "failed to invalidate permission cache after forbid")
	}

	return nil
}

// Revoke removes permission and invalidates cache.
func (r *RBACRepository) Revoke(
	ctx context.Context, entityID uint, entityType domain.EntityType, abilities []domain.Ability,
) error {
	err := r.inner.Revoke(ctx, entityID, entityType, abilities)
	if err != nil {
		return errors.WithMessage(err, "failed to revoke abilities")
	}

	if err := r.invalidatePermissionCache(ctx, entityID, entityType); err != nil {
		return errors.WithMessage(err, "failed to invalidate permission cache after revoke")
	}

	return nil
}

func (r *RBACRepository) invalidateRoleCache(ctx context.Context) error {
	if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("roles", "all")); err != nil {
		return err
	}
	if err := r.wrapper.InvalidatePattern(ctx, r.keyBuilder.BuildPattern("role")); err != nil {
		return err
	}
	if err := r.wrapper.InvalidatePattern(ctx, r.keyBuilder.BuildPattern("roles")); err != nil {
		return err
	}

	return nil
}

func (r *RBACRepository) invalidateEntityCache(ctx context.Context, entityID uint, entityType domain.EntityType) error {
	key := fmt.Sprintf("%s_%d", entityType, entityID)
	if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("roles", key)); err != nil {
		return err
	}
	if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("permissions", key)); err != nil {
		return err
	}

	return nil
}

func (r *RBACRepository) invalidatePermissionCache(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) error {
	key := fmt.Sprintf("%s_%d", entityType, entityID)
	if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("permissions", key)); err != nil {
		return err
	}

	return nil
}
