package inmemory

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestRBACRepository(t *testing.T) {
	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(_ *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			repo := NewRBACRepository()

			createRoleFunc := func(_ context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				role := domain.Role{
					ID:    uint(repo.nextRoleID.Add(1)),
					Name:  name,
					Title: new(name + " Title"),
					Level: new(uint(1)),
					Scope: new(1),
				}

				repo.mu.Lock()
				repo.roles[role.ID] = &role
				repo.mu.Unlock()

				return role
			}

			createAbilityFunc := func(_ context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				ability.ID = uint(repo.nextAbilityID.Add(1))

				repo.mu.Lock()
				repo.abilities[ability.ID] = &ability
				repo.mu.Unlock()

				return ability.ID
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}

// The tests below cover the seeding API that exists only on the inmemory
// implementation (test helpers write abilities/permissions/assignments
// directly); they are not part of the shared RBACRepository contract.

func TestRBACRepository_SaveAndDeleteRole(t *testing.T) {
	ctx := context.Background()

	// ARRANGE
	repo := NewRBACRepository()
	role := &domain.Role{Name: "seeded_role", Title: new("Seeded Role")}
	require.NoError(t, repo.SaveRole(ctx, role))
	require.NotZero(t, role.ID)

	roles, err := repo.GetRoles(ctx)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	assert.Equal(t, "seeded_role", roles[0].Name)

	// ACT
	require.NoError(t, repo.DeleteRole(ctx, role.ID))

	// ASSERT
	roles, err = repo.GetRoles(ctx)
	require.NoError(t, err)
	assert.Empty(t, roles, "deleted role must disappear from GetRoles")
}

func TestRBACRepository_SaveAssignedRoleLifecycle(t *testing.T) {
	ctx := context.Background()

	// ARRANGE
	repo := NewRBACRepository()
	role := &domain.Role{Name: "assignable_role", Title: new("Assignable Role")}
	require.NoError(t, repo.SaveRole(ctx, role))

	assigned := &domain.AssignedRole{
		RoleID:     role.ID,
		EntityID:   42,
		EntityType: domain.EntityTypeUser,
	}
	require.NoError(t, repo.SaveAssignedRole(ctx, assigned))
	require.NotZero(t, assigned.ID, "SaveAssignedRole must assign an ID")

	roles, err := repo.GetRolesForEntity(ctx, 42, domain.EntityTypeUser)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	assert.Equal(t, role.ID, roles[0].ID)

	// ACT
	require.NoError(t, repo.DeleteAssignedRole(ctx, assigned.ID))

	// ASSERT
	roles, err = repo.GetRolesForEntity(ctx, 42, domain.EntityTypeUser)
	require.NoError(t, err)
	assert.Empty(t, roles, "deleted assignment must disappear from GetRolesForEntity")
}

func TestRBACRepository_SaveAbilityAndPermission(t *testing.T) {
	ctx := context.Background()

	// ARRANGE
	repo := NewRBACRepository()
	ability := &domain.Ability{
		Name:  domain.AbilityNameGameServerStart,
		Title: new("Start Server"),
	}
	require.NoError(t, repo.SaveAbility(ctx, ability))
	require.NotZero(t, ability.ID, "SaveAbility must assign an ID")

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(uint(7)),
		EntityType: new(domain.EntityTypeUser),
		Forbidden:  true,
	}
	require.NoError(t, repo.SavePermission(ctx, permission))
	require.NotZero(t, permission.ID, "SavePermission must assign an ID")

	// ACT
	permissions, err := repo.GetPermissions(ctx, 7, domain.EntityTypeUser)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, permissions, 1)
	assert.Equal(t, ability.ID, permissions[0].AbilityID)
	assert.True(t, permissions[0].Forbidden, "forbidden flag must be preserved")
	require.NotNil(t, permissions[0].Ability)
	assert.Equal(t, domain.AbilityNameGameServerStart, permissions[0].Ability.Name)
	assert.Equal(t, "Start Server", *permissions[0].Ability.Title, "ability title must be preserved")
}

func TestRBACRepository_AssignAbilityToUser(t *testing.T) {
	ctx := context.Background()

	// ARRANGE
	repo := NewRBACRepository()
	ability := &domain.Ability{
		Name:  domain.AbilityNameGameServerStop,
		Title: new("Stop Server"),
	}
	require.NoError(t, repo.SaveAbility(ctx, ability))

	// ACT
	require.NoError(t, repo.AssignAbilityToUser(ctx, 100, ability.ID))

	// ASSERT
	permissions, err := repo.GetPermissions(ctx, 100, domain.EntityTypeUser)
	require.NoError(t, err)
	require.Len(t, permissions, 1)
	assert.Equal(t, ability.ID, permissions[0].AbilityID)
	assert.False(t, permissions[0].Forbidden, "AssignAbilityToUser grants (allows) the ability")
}
