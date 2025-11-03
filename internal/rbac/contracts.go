package rbac

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type Repository interface {
	GetRoles(context.Context) ([]domain.Role, error)

	GetPermissions(
		ctx context.Context,
		entityID uint,
		entityType domain.EntityType,
	) ([]domain.Permission, error)

	GetRolesForEntity(
		_ context.Context,
		entityID uint,
		entityType domain.EntityType,
	) ([]domain.RestrictedRole, error)

	AssignRolesForEntity(
		ctx context.Context,
		entityID uint,
		entityType domain.EntityType,
		roles []domain.RestrictedRole,
	) error

	ClearRolesForEntity(
		ctx context.Context,
		entityID uint,
		entityType domain.EntityType,
	) error

	Allow(
		ctx context.Context,
		entityID uint,
		entityType domain.EntityType,
		abilities []domain.Ability,
	) error

	Forbid(
		ctx context.Context,
		entityID uint,
		entityType domain.EntityType,
		abilities []domain.Ability,
	) error

	Revoke(
		ctx context.Context,
		entityID uint,
		entityType domain.EntityType,
		abilities []domain.Ability,
	) error
}
