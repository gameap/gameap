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

	data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Role, error) {
		return r.inner.GetRoles(ctx)
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load GetRoles")
	}

	return data, nil
}

// SaveRole saves a role and invalidates cache.
func (r *RBACRepository) SaveRole(ctx context.Context, role *domain.Role) error {
	err := r.inner.SaveRole(ctx, role)
	if err != nil {
		return errors.WithMessage(err, "failed to save role")
	}

	r.invalidateRoleCache(ctx)

	return nil
}

// DeleteRole removes a role and invalidates cache.
func (r *RBACRepository) DeleteRole(ctx context.Context, id uint) error {
	err := r.inner.DeleteRole(ctx, id)
	if err != nil {
		return errors.WithMessage(err, "failed to delete role")
	}

	r.invalidateRoleCache(ctx)
	r.invalidateEntityCache(ctx, id, domain.EntityTypeRole)

	return nil
}

// GetPermissions retrieves permissions for an entity - very frequently called.
func (r *RBACRepository) GetPermissions(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) ([]domain.Permission, error) {
	key := r.keyBuilder.BuildKey("permissions", fmt.Sprintf("%s_%d", entityType, entityID))

	data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Permission, error) {
		return r.inner.GetPermissions(ctx, entityID, entityType)
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load GetPermissions")
	}

	return data, nil
}

// GetRolesForEntity gets roles assigned to an entity - frequently called in auth.
func (r *RBACRepository) GetRolesForEntity(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) ([]domain.RestrictedRole, error) {
	key := r.keyBuilder.BuildKey("roles", fmt.Sprintf("%s_%d", entityType, entityID))

	data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.RestrictedRole, error) {
		return r.inner.GetRolesForEntity(ctx, entityID, entityType)
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load GetRolesForEntity")
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

	r.invalidateEntityCache(ctx, entityID, entityType)

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

	r.invalidateEntityCache(ctx, entityID, entityType)

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

	r.invalidatePermissionCache(ctx, entityID, entityType)

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

	r.invalidatePermissionCache(ctx, entityID, entityType)

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

	r.invalidatePermissionCache(ctx, entityID, entityType)

	return nil
}

func (r *RBACRepository) invalidateRoleCache(ctx context.Context) {
	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("roles", "all"))
	r.wrapper.InvalidatePatternBestEffort(ctx, r.keyBuilder.BuildPattern("role"))
	r.wrapper.InvalidatePatternBestEffort(ctx, r.keyBuilder.BuildPattern("roles"))
}

func (r *RBACRepository) invalidateEntityCache(ctx context.Context, entityID uint, entityType domain.EntityType) {
	key := fmt.Sprintf("%s_%d", entityType, entityID)
	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("roles", key))
	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("permissions", key))
}

func (r *RBACRepository) invalidatePermissionCache(
	ctx context.Context, entityID uint, entityType domain.EntityType,
) {
	key := fmt.Sprintf("%s_%d", entityType, entityID)
	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("permissions", key))
}
