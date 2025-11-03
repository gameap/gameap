package rbac

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

type RBAC struct {
	tm    base.TransactionManager
	repo  Repository
	cache *permissionCache
}

func NewRBAC(
	tm base.TransactionManager,
	repo Repository,
	cacheTTL time.Duration,
) *RBAC {
	return &RBAC{
		tm:    tm,
		repo:  repo,
		cache: newPermissionCache(cacheTTL),
	}
}

func (r *RBAC) Close() {
	r.cache.Close()
}

// Can checks if the user has all the specified abilities.
func (r *RBAC) Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error) {
	permissionsByAbilityName, err := r.getAllPermissionsForUser(ctx, userID)
	if err != nil {
		return false, errors.WithMessage(err, "get permissions for user")
	}

	counter := 0

	for _, abilityName := range abilities {
		for _, permissions := range permissionsByAbilityName[abilityName] {
			if permissions.Forbidden {
				return false, nil
			}

			ability := permissions.Ability

			if ability == nil {
				continue
			}

			if ability.EntityType != nil {
				continue
			}

			if ability.EntityType == nil && ability.EntityID == nil {
				counter++

				break
			}
		}
	}

	return counter == len(abilities), nil
}

// CanOneOf checks if the user has at least one of the specified abilities.
func (r *RBAC) CanOneOf(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error) {
	permissionsByAbilityName, err := r.getAllPermissionsForUser(ctx, userID)
	if err != nil {
		return false, errors.WithMessage(err, "get permissions for user")
	}

	for _, abilityName := range abilities {
		for _, permissions := range permissionsByAbilityName[abilityName] {
			if permissions.Forbidden {
				continue
			}

			ability := permissions.Ability

			if ability == nil {
				continue
			}

			if ability.EntityType != nil {
				continue
			}

			if ability.EntityType == nil && ability.EntityID == nil {
				return true, nil
			}
		}
	}

	return false, nil
}

func (r *RBAC) CanForEntity(
	ctx context.Context,
	userID uint,
	entityType domain.EntityType,
	entityID uint,
	abilities []domain.AbilityName,
) (bool, error) {
	permissionsByAbilityName, err := r.getAllPermissionsForUser(ctx, userID)
	if err != nil {
		return false, errors.WithMessage(err, "get permissions for user")
	}

	counter := 0

	for _, abilityName := range abilities {
		for _, permissions := range permissionsByAbilityName[abilityName] {
			if permissions.Forbidden {
				return false, nil
			}

			ability := permissions.Ability

			if ability == nil {
				continue
			}

			// Check if ability is global (no entity restrictions)
			if ability.EntityType == nil && ability.EntityID == nil {
				counter++

				break
			}

			// Check if ability is for the specific entity type (e.g., all servers)
			if ability.EntityType != nil && *ability.EntityType == entityType && ability.EntityID == nil {
				counter++

				break
			}

			// Check if ability is for the specific entity
			if ability.EntityType != nil && *ability.EntityType == entityType &&
				ability.EntityID != nil && *ability.EntityID == entityID {
				counter++

				break
			}
		}
	}

	return counter == len(abilities), nil
}

func (r *RBAC) CanAnyForEntity(
	ctx context.Context,
	userID uint,
	entityType domain.EntityType,
	entityID uint,
	abilities []domain.AbilityName,
) (bool, error) {
	permissionsByAbilityName, err := r.getAllPermissionsForUser(ctx, userID)
	if err != nil {
		return false, errors.WithMessage(err, "get permissions for user")
	}

	for _, abilityName := range abilities {
		for _, permissions := range permissionsByAbilityName[abilityName] {
			ability := permissions.Ability

			if ability == nil {
				continue
			}

			if ability.EntityID == nil {
				// Check if ability is global
				if ability.EntityID == nil && ability.EntityType == nil {
					return !permissions.Forbidden, nil
				}

				// Check if ability is for the specific entity type (e.g., all servers)
				if ability.EntityID == nil && ability.EntityType != nil && *ability.EntityType == entityType {
					return !permissions.Forbidden, nil
				}
			} else { //nolint:gocritic
				if *ability.EntityType == entityType &&
					ability.EntityID != nil && *ability.EntityID == entityID {
					return !permissions.Forbidden, nil
				}
			}
		}
	}

	return false, nil
}

func (r *RBAC) GetRoles(ctx context.Context, userID uint) ([]string, error) {
	roles, err := r.repo.GetRolesForEntity(ctx, userID, domain.EntityTypeUser)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get roles for user")
	}

	roleNames := make([]string, 0, len(roles))

	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
	}

	return roleNames, nil
}

func (r *RBAC) SetRolesToUser(ctx context.Context, userID uint, roleNames []string) error {
	return r.tm.Do(ctx, func(ctx context.Context) error {
		return r.setRolesToUser(ctx, userID, roleNames)
	})
}

func (r *RBAC) setRolesToUser(ctx context.Context, userID uint, roleNames []string) error {
	roles, err := r.repo.GetRoles(ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to get roles")
	}

	rolesByName := make(map[string]domain.Role, len(roles))
	for _, role := range roles {
		rolesByName[role.Name] = role
	}

	restrictedRoles := make([]domain.RestrictedRole, 0, len(roleNames))

	for _, roleName := range roleNames {
		if _, ok := rolesByName[roleName]; !ok {
			return NewErrInvalidRoleName(roleName)
		}

		restrictedRoles = append(
			restrictedRoles,
			domain.NewRestrictedRoleFromRole(rolesByName[roleName]),
		)
	}

	err = r.repo.ClearRolesForEntity(ctx, userID, domain.EntityTypeUser)
	if err != nil {
		return errors.WithMessage(err, "failed to clear roles for user")
	}

	err = r.repo.AssignRolesForEntity(
		ctx,
		userID,
		domain.EntityTypeUser,
		restrictedRoles,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to assign roles to user")
	}

	return nil
}

func (r *RBAC) AllowUserAbilitiesForEntity(
	ctx context.Context,
	userID uint,
	entityID uint,
	entityType domain.EntityType,
	abilityNames []domain.AbilityName,
) error {
	abilities := make([]domain.Ability, 0, len(abilityNames))
	for _, abilityName := range abilityNames {
		abilities = append(
			abilities,
			domain.CreateAbilityForEntity(abilityName, entityID, entityType),
		)
	}

	return r.tm.Do(ctx, func(ctx context.Context) error {
		err := r.repo.Revoke(ctx, userID, domain.EntityTypeUser, abilities)
		if err != nil {
			return errors.WithMessage(err, "failed to revoke abilities for user")
		}

		err = r.repo.Allow(ctx, userID, domain.EntityTypeUser, abilities)
		if err != nil {
			return errors.WithMessage(err, "failed to allow abilities for user")
		}

		r.cache.Delete(cacheKey{
			EntityType: domain.EntityTypeUser,
			EntityID:   userID,
		})

		return nil
	})
}

// RevokeOrForbidUserAbilitiesForEntity revokes the specified abilities for a user.
// If the user still has the abilities via other roles or permissions, it forbids them instead.
// This ensures that the user cannot access the abilities through any means.
func (r *RBAC) RevokeOrForbidUserAbilitiesForEntity(
	ctx context.Context,
	userID uint,
	entityID uint,
	entityType domain.EntityType,
	abilityNames []domain.AbilityName,
) error {
	abilities := make([]domain.Ability, 0, len(abilityNames))
	for _, abilityName := range abilityNames {
		abilities = append(
			abilities,
			domain.CreateAbilityForEntity(abilityName, entityID, entityType),
		)
	}

	return r.tm.Do(ctx, func(ctx context.Context) error {
		err := r.repo.Revoke(ctx, userID, domain.EntityTypeUser, abilities)
		if err != nil {
			return errors.WithMessage(err, "failed to revoke abilities for user")
		}

		r.cache.Delete(cacheKey{
			EntityType: domain.EntityTypeUser,
			EntityID:   userID,
		})

		// Check if user still has abilities via other roles or permissions
		forbidAbilities := make([]domain.Ability, 0, len(abilityNames))

		for _, ability := range abilities {
			can, err := r.CanForEntity(ctx, userID, entityType, entityID, []domain.AbilityName{ability.Name})
			if err != nil {
				return errors.WithMessage(err, "failed to check abilities for user")
			}

			if can {
				forbidAbilities = append(forbidAbilities, ability)
			}
		}

		if len(forbidAbilities) > 0 {
			err = r.repo.Forbid(ctx, userID, domain.EntityTypeUser, forbidAbilities)
			if err != nil {
				return errors.WithMessage(err, "failed to forbid abilities for user")
			}

			r.cache.Delete(cacheKey{
				EntityType: domain.EntityTypeUser,
				EntityID:   userID,
			})
		}

		return nil
	})
}

// getAllPermissionsForUser retrieves all permissions for a user, including those inherited from roles.
// It returns a map where the key is the ability name and the value is a slice of permissions.
//
//nolint:gocognit
func (r *RBAC) getAllPermissionsForUser(
	ctx context.Context,
	userID uint,
) (map[domain.AbilityName][]domain.Permission, error) {
	if permissions, ok := r.cache.Get(cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   userID,
	}); ok {
		return permissions, nil
	}

	userPermissions, err := r.repo.GetPermissions(ctx, userID, domain.EntityTypeUser)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get all permissions for user")
	}

	roles, err := r.repo.GetRolesForEntity(ctx, userID, domain.EntityTypeUser)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get roles for user")
	}

	for _, role := range roles {
		rolePermissions, err := r.repo.GetPermissions(ctx, role.ID, domain.EntityTypeRole)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get permissions for role")
		}

		//nolint:nestif
		if role.RestrictedToType != nil || role.RestrictedToID != nil {
			filteredRolePermissions := make([]domain.Permission, 0, len(rolePermissions))

			if role.RestrictedToType != nil && role.RestrictedToID == nil {
				for _, rolePermission := range rolePermissions {
					if rolePermission.Ability != nil && rolePermission.Ability.EntityType != nil {
						if *rolePermission.Ability.EntityType != *role.RestrictedToType {
							// Skip permission if ability's entity type does not match role's restriction
							continue
						}

						filteredRolePermissions = append(filteredRolePermissions, rolePermission)
					}
				}
			}

			if role.RestrictedToType != nil && role.RestrictedToID != nil {
				for _, rolePermission := range rolePermissions {
					if rolePermission.Ability != nil && rolePermission.Ability.EntityID != nil {
						if *rolePermission.Ability.EntityID != *role.RestrictedToID {
							// Skip permission if ability's entity ID does not match role's restriction
							continue
						}

						filteredRolePermissions = append(filteredRolePermissions, rolePermission)
					}
				}
			}

			userPermissions = append(userPermissions, filteredRolePermissions...)
		} else {
			userPermissions = append(userPermissions, rolePermissions...)
		}
	}

	permissions := make(map[domain.AbilityName][]domain.Permission, len(userPermissions))
	for _, permission := range userPermissions {
		if permission.Ability == nil {
			continue
		}

		permissions[permission.Ability.Name] = append(permissions[permission.Ability.Name], permission)
	}

	r.cache.Set(cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   userID,
	}, permissions)

	return permissions, nil
}
