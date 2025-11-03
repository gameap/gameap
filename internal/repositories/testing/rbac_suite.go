package testing

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/rs/xid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RBACRepositorySuite struct {
	suite.Suite

	repo              repositories.RBACRepository
	createRoleFunc    func(ctx context.Context, t *testing.T, name string) domain.Role
	createAbilityFunc func(ctx context.Context, t *testing.T, ability domain.Ability) uint

	fn func(t *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint)
}

type rbacRepoSetupFunc func(t *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint)

func NewRBACRepositorySuite(fn rbacRepoSetupFunc) *RBACRepositorySuite {
	return &RBACRepositorySuite{
		fn: fn,
	}
}

func (s *RBACRepositorySuite) SetupTest() {
	s.repo, s.createRoleFunc, s.createAbilityFunc = s.fn(s.T())
}

func (s *RBACRepositorySuite) TestRBACRepositoryGetRoles() {
	ctx := context.Background()

	s.T().Run("get_all_roles_empty", func(t *testing.T) {
		roles, err := s.repo.GetRoles(ctx)
		require.NoError(t, err)
		assert.Empty(t, roles)
	})

	s.T().Run("get_all_roles_with_data", func(t *testing.T) {
		role1 := s.createRoleFunc(ctx, t, "admin")
		role2 := s.createRoleFunc(ctx, t, "user")
		role3 := s.createRoleFunc(ctx, t, "moderator")

		roles, err := s.repo.GetRoles(ctx)
		require.NoError(t, err)
		assert.Len(t, roles, 3)

		roleNames := make(map[string]bool)
		for _, r := range roles {
			roleNames[r.Name] = true
		}
		assert.True(t, roleNames[role1.Name])
		assert.True(t, roleNames[role2.Name])
		assert.True(t, roleNames[role3.Name])
	})
}

func (s *RBACRepositorySuite) TestRBACRepositorySaveRole() {
	ctx := context.Background()

	s.T().Run("save_new_role", func(t *testing.T) {
		role := &domain.Role{
			Name:  xid.New().String(),
			Title: lo.ToPtr("new role"),
		}
		err := s.repo.SaveRole(ctx, role)

		require.NoError(t, err)
		assert.NotEmpty(t, role.ID)
	})

	s.T().Run("save_role", func(t *testing.T) {
		role := &domain.Role{
			Name:  xid.New().String(),
			Title: lo.ToPtr("new role"),
		}

		err := s.repo.SaveRole(ctx, role)
		require.NoError(t, err)
		assert.NotEmpty(t, role.ID)

		role.Title = lo.ToPtr("new title for role")
		err = s.repo.SaveRole(ctx, role)
		require.NoError(t, err)

		roles, err := s.repo.GetRoles(ctx)
		require.NoError(t, err)

		for _, r := range roles {
			if r.Name == role.Name {
				assert.Equal(t, role.Title, role.Title)

				return
			}
		}

		assert.FailNow(t, "role not found")
	})

	s.T().Run("save_multiple_roles", func(t *testing.T) {
		roleNames := []string{xid.New().String(), xid.New().String(), xid.New().String()}

		for _, roleName := range roleNames {
			role := &domain.Role{
				Name:  roleName,
				Title: lo.ToPtr("role " + roleName),
			}
			err := s.repo.SaveRole(ctx, role)
			require.NoError(t, err)
		}

		roles, err := s.repo.GetRoles(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(roles), len(roleNames))
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryGetRolesForEntity() {
	ctx := context.Background()

	s.T().Run("get_roles_for_entity_no_assignments", func(t *testing.T) {
		entityID := uint(100)
		entityType := domain.EntityTypeUser

		roles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, roles)
	})

	s.T().Run("get_roles_for_entity_with_assignments", func(t *testing.T) {
		role1 := s.createRoleFunc(ctx, t, "role_entity_1")
		role2 := s.createRoleFunc(ctx, t, "role_entity_2")

		entityID := uint(200)
		entityType := domain.EntityTypeUser

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(role1),
			domain.NewRestrictedRoleFromRole(role2),
		})
		require.NoError(t, err)

		roles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, roles, 2)

		roleIDs := []uint{roles[0].ID, roles[1].ID}
		assert.Contains(t, roleIDs, role1.ID)
		assert.Contains(t, roleIDs, role2.ID)
	})

	s.T().Run("get_roles_for_entity_with_restrictions", func(t *testing.T) {
		role := s.createRoleFunc(ctx, t, "role_restricted")

		entityID := uint(300)
		entityType := domain.EntityTypeUser
		restrictedToID := uint(500)
		restrictedToType := domain.EntityTypeServer

		restrictedRole := domain.NewRestrictedRoleFromRole(role)
		restrictedRole.RestrictedToID = &restrictedToID
		restrictedRole.RestrictedToType = &restrictedToType

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, []domain.RestrictedRole{restrictedRole})
		require.NoError(t, err)

		roles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		require.Len(t, roles, 1)
		assert.Equal(t, role.ID, roles[0].ID)
		require.NotNil(t, roles[0].RestrictedToID)
		assert.Equal(t, restrictedToID, *roles[0].RestrictedToID)
		require.NotNil(t, roles[0].RestrictedToType)
		assert.Equal(t, restrictedToType, *roles[0].RestrictedToType)
	})

	s.T().Run("get_roles_only_for_specific_entity", func(t *testing.T) {
		role := s.createRoleFunc(ctx, t, "role_specific")

		entity1ID := uint(400)
		entity2ID := uint(401)
		entityType := domain.EntityTypeUser

		err := s.repo.AssignRolesForEntity(ctx, entity1ID, entityType, []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(role),
		})
		require.NoError(t, err)

		roles, err := s.repo.GetRolesForEntity(ctx, entity2ID, entityType)
		require.NoError(t, err)
		assert.Empty(t, roles)

		roles, err = s.repo.GetRolesForEntity(ctx, entity1ID, entityType)
		require.NoError(t, err)
		assert.Len(t, roles, 1)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryGetPermissions() {
	ctx := context.Background()

	s.T().Run("get_permissions_no_permissions", func(t *testing.T) {
		entityID := uint(1000)
		entityType := domain.EntityTypeUser

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})

	s.T().Run("get_permissions_with_data", func(t *testing.T) {
		entityID := uint(1001)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{
				Name:       domain.AbilityNameGameServerStart,
				Title:      lo.ToPtr("Start Server"),
				EntityID:   nil,
				EntityType: nil,
				OnlyOwned:  false,
			},
			{
				Name:       domain.AbilityNameGameServerStop,
				Title:      lo.ToPtr("Stop Server"),
				EntityID:   nil,
				EntityType: nil,
				OnlyOwned:  false,
			},
		}

		err := s.repo.Allow(ctx, entityID, entityType, []domain.Ability{abilities[0]})
		require.NoError(t, err)

		err = s.repo.Forbid(ctx, entityID, entityType, []domain.Ability{abilities[1]})
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 2)

		abilityMap := make(map[domain.AbilityName]bool)
		for _, perm := range permissions {
			require.NotNil(t, perm.Ability)
			switch perm.Ability.Name {
			case domain.AbilityNameGameServerStart:
				assert.False(t, perm.Forbidden)
			case domain.AbilityNameGameServerStop:
				assert.True(t, perm.Forbidden)
			}
			abilityMap[perm.Ability.Name] = true
		}
		assert.True(t, abilityMap[domain.AbilityNameGameServerStart])
		assert.True(t, abilityMap[domain.AbilityNameGameServerStop])
	})

	s.T().Run("get_permissions_only_for_specific_entity", func(t *testing.T) {
		entity1ID := uint(1100)
		entity2ID := uint(1101)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{
				Name:       domain.AbilityNameGameServerRestart,
				Title:      lo.ToPtr("Restart Server"),
				EntityID:   nil,
				EntityType: nil,
				OnlyOwned:  false,
			},
		}

		err := s.repo.Allow(ctx, entity1ID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entity2ID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)

		permissions, err = s.repo.GetPermissions(ctx, entity1ID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryAllow() {
	ctx := context.Background()

	s.T().Run("allow_single_ability", func(t *testing.T) {
		entityID := uint(2000)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{
				Name:       domain.AbilityNameGameServerStart,
				Title:      lo.ToPtr("Start Server"),
				EntityID:   nil,
				EntityType: nil,
				OnlyOwned:  false,
			},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)
		assert.False(t, permissions[0].Forbidden)
		assert.Equal(t, domain.AbilityNameGameServerStart, permissions[0].Ability.Name)
	})

	s.T().Run("allow_multiple_abilities", func(t *testing.T) {
		entityID := uint(2001)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerStop, Title: lo.ToPtr("Stop"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerRestart, Title: lo.ToPtr("Restart"), OnlyOwned: false},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 3)

		for _, perm := range permissions {
			assert.False(t, perm.Forbidden)
		}
	})

	s.T().Run("allow_ability_with_entity_restriction", func(t *testing.T) {
		entityID := uint(2002)
		entityType := domain.EntityTypeUser
		serverID := uint(5000)
		serverType := domain.EntityTypeServer

		abilities := []domain.Ability{
			{
				Name:       domain.AbilityNameGameServerStart,
				Title:      lo.ToPtr("Start Specific Server"),
				EntityID:   &serverID,
				EntityType: &serverType,
				OnlyOwned:  false,
			},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		require.Len(t, permissions, 1)
		assert.False(t, permissions[0].Forbidden)
		require.NotNil(t, permissions[0].Ability.EntityID)
		assert.Equal(t, serverID, *permissions[0].Ability.EntityID)
		require.NotNil(t, permissions[0].Ability.EntityType)
		assert.Equal(t, serverType, *permissions[0].Ability.EntityType)
	})

	s.T().Run("allow_empty_abilities_list", func(t *testing.T) {
		entityID := uint(2003)
		entityType := domain.EntityTypeUser

		err := s.repo.Allow(ctx, entityID, entityType, []domain.Ability{})
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryForbid() {
	ctx := context.Background()

	s.T().Run("forbid_single_ability", func(t *testing.T) {
		entityID := uint(3000)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{
				Name:       domain.AbilityNameGameServerStop,
				Title:      lo.ToPtr("Stop Server"),
				EntityID:   nil,
				EntityType: nil,
				OnlyOwned:  false,
			},
		}

		err := s.repo.Forbid(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)
		assert.True(t, permissions[0].Forbidden)
		assert.Equal(t, domain.AbilityNameGameServerStop, permissions[0].Ability.Name)
	})

	s.T().Run("forbid_multiple_abilities", func(t *testing.T) {
		entityID := uint(3001)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerStop, Title: lo.ToPtr("Stop"), OnlyOwned: false},
		}

		err := s.repo.Forbid(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 2)

		for _, perm := range permissions {
			assert.True(t, perm.Forbidden)
		}
	})

	s.T().Run("forbid_empty_abilities_list", func(t *testing.T) {
		entityID := uint(3002)
		entityType := domain.EntityTypeUser

		err := s.repo.Forbid(ctx, entityID, entityType, []domain.Ability{})
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryRevoke() {
	ctx := context.Background()

	s.T().Run("revoke_existing_permission", func(t *testing.T) {
		entityID := uint(4000)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerStop, Title: lo.ToPtr("Stop"), OnlyOwned: false},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 2)

		err = s.repo.Revoke(ctx, entityID, entityType, []domain.Ability{abilities[0]})
		require.NoError(t, err)

		permissions, err = s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)
		assert.Equal(t, domain.AbilityNameGameServerStop, permissions[0].Ability.Name)
	})

	s.T().Run("revoke_all_permissions", func(t *testing.T) {
		entityID := uint(4001)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerStop, Title: lo.ToPtr("Stop"), OnlyOwned: false},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		err = s.repo.Revoke(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})

	s.T().Run("revoke_non_existent_permission", func(t *testing.T) {
		entityID := uint(4002)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
		}

		err := s.repo.Revoke(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})

	s.T().Run("revoke_empty_abilities_list", func(t *testing.T) {
		entityID := uint(4003)
		entityType := domain.EntityTypeUser

		err := s.repo.Revoke(ctx, entityID, entityType, []domain.Ability{})
		require.NoError(t, err)
	})

	s.T().Run("revoke_with_entity_restriction", func(t *testing.T) {
		entityID := uint(4004)
		entityType := domain.EntityTypeUser
		serverID := uint(6000)
		serverType := domain.EntityTypeServer

		abilities := []domain.Ability{
			{
				Name:       domain.AbilityNameGameServerStart,
				Title:      lo.ToPtr("Start Specific Server"),
				EntityID:   &serverID,
				EntityType: &serverType,
				OnlyOwned:  false,
			},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)

		err = s.repo.Revoke(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err = s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryAssignRolesForEntity() {
	ctx := context.Background()

	s.T().Run("assign_single_role", func(t *testing.T) {
		role := s.createRoleFunc(ctx, t, "role_assign_1")

		entityID := uint(5000)
		entityType := domain.EntityTypeUser

		roles := []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(role),
		}

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, roles)
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles, 1)
		assert.Equal(t, role.ID, assignedRoles[0].ID)
	})

	s.T().Run("assign_multiple_roles", func(t *testing.T) {
		role1 := s.createRoleFunc(ctx, t, "role_assign_2a")
		role2 := s.createRoleFunc(ctx, t, "role_assign_2b")
		role3 := s.createRoleFunc(ctx, t, "role_assign_2c")

		entityID := uint(5001)
		entityType := domain.EntityTypeUser

		roles := []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(role1),
			domain.NewRestrictedRoleFromRole(role2),
			domain.NewRestrictedRoleFromRole(role3),
		}

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, roles)
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles, 3)

		roleIDs := []uint{assignedRoles[0].ID, assignedRoles[1].ID, assignedRoles[2].ID}
		assert.Contains(t, roleIDs, role1.ID)
		assert.Contains(t, roleIDs, role2.ID)
		assert.Contains(t, roleIDs, role3.ID)
	})

	s.T().Run("assign_role_with_restrictions", func(t *testing.T) {
		role := s.createRoleFunc(ctx, t, "role_assign_3")

		entityID := uint(5002)
		entityType := domain.EntityTypeUser
		restrictedToID := uint(7000)
		restrictedToType := domain.EntityTypeServer

		restrictedRole := domain.NewRestrictedRoleFromRole(role)
		restrictedRole.RestrictedToID = &restrictedToID
		restrictedRole.RestrictedToType = &restrictedToType

		roles := []domain.RestrictedRole{restrictedRole}

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, roles)
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		require.Len(t, assignedRoles, 1)
		require.NotNil(t, assignedRoles[0].RestrictedToID)
		assert.Equal(t, restrictedToID, *assignedRoles[0].RestrictedToID)
		require.NotNil(t, assignedRoles[0].RestrictedToType)
		assert.Equal(t, restrictedToType, *assignedRoles[0].RestrictedToType)
	})

	s.T().Run("assign_empty_roles_list", func(t *testing.T) {
		entityID := uint(5003)
		entityType := domain.EntityTypeUser

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, []domain.RestrictedRole{})
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, assignedRoles)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryClearRolesForEntity() {
	ctx := context.Background()

	s.T().Run("clear_existing_roles", func(t *testing.T) {
		role1 := s.createRoleFunc(ctx, t, "role_clear_1a")
		role2 := s.createRoleFunc(ctx, t, "role_clear_1b")

		entityID := uint(6000)
		entityType := domain.EntityTypeUser

		roles := []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(role1),
			domain.NewRestrictedRoleFromRole(role2),
		}

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, roles)
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles, 2)

		err = s.repo.ClearRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)

		assignedRoles, err = s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, assignedRoles)
	})

	s.T().Run("clear_when_no_roles_assigned", func(t *testing.T) {
		entityID := uint(6001)
		entityType := domain.EntityTypeUser

		err := s.repo.ClearRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, assignedRoles)
	})

	s.T().Run("clear_roles_for_one_entity_does_not_affect_others", func(t *testing.T) {
		role := s.createRoleFunc(ctx, t, "role_clear_2")

		entity1ID := uint(6002)
		entity2ID := uint(6003)
		entityType := domain.EntityTypeUser

		roles := []domain.RestrictedRole{domain.NewRestrictedRoleFromRole(role)}

		err := s.repo.AssignRolesForEntity(ctx, entity1ID, entityType, roles)
		require.NoError(t, err)

		err = s.repo.AssignRolesForEntity(ctx, entity2ID, entityType, roles)
		require.NoError(t, err)

		err = s.repo.ClearRolesForEntity(ctx, entity1ID, entityType)
		require.NoError(t, err)

		assignedRoles1, err := s.repo.GetRolesForEntity(ctx, entity1ID, entityType)
		require.NoError(t, err)
		assert.Empty(t, assignedRoles1)

		assignedRoles2, err := s.repo.GetRolesForEntity(ctx, entity2ID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles2, 1)
	})
}

func (s *RBACRepositorySuite) TestRBACRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_permission_lifecycle", func(t *testing.T) {
		entityID := uint(8000)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerStop, Title: lo.ToPtr("Stop"), OnlyOwned: false},
			{Name: domain.AbilityNameGameServerRestart, Title: lo.ToPtr("Restart"), OnlyOwned: false},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 3)

		err = s.repo.Forbid(ctx, entityID, entityType, []domain.Ability{abilities[1]})
		require.NoError(t, err)

		permissions, err = s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 4)

		forbiddenCount := 0
		for _, perm := range permissions {
			if perm.Forbidden {
				forbiddenCount++
			}
		}
		assert.Equal(t, 1, forbiddenCount)

		err = s.repo.Revoke(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		permissions, err = s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, permissions)
	})

	s.T().Run("full_role_assignment_lifecycle", func(t *testing.T) {
		role1 := s.createRoleFunc(ctx, t, "lifecycle_role_1")
		role2 := s.createRoleFunc(ctx, t, "lifecycle_role_2")
		role3 := s.createRoleFunc(ctx, t, "lifecycle_role_3")

		entityID := uint(8001)
		entityType := domain.EntityTypeUser

		roles := []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(role1),
			domain.NewRestrictedRoleFromRole(role2),
		}

		err := s.repo.AssignRolesForEntity(ctx, entityID, entityType, roles)
		require.NoError(t, err)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles, 2)

		additionalRoles := []domain.RestrictedRole{domain.NewRestrictedRoleFromRole(role3)}
		err = s.repo.AssignRolesForEntity(ctx, entityID, entityType, additionalRoles)
		require.NoError(t, err)

		assignedRoles, err = s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles, 3)

		err = s.repo.ClearRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)

		assignedRoles, err = s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, assignedRoles)
	})

	s.T().Run("permissions_and_roles_together", func(t *testing.T) {
		role := s.createRoleFunc(ctx, t, "combined_role")

		entityID := uint(8002)
		entityType := domain.EntityTypeUser

		abilities := []domain.Ability{
			{Name: domain.AbilityNameGameServerStart, Title: lo.ToPtr("Start"), OnlyOwned: false},
		}

		err := s.repo.Allow(ctx, entityID, entityType, abilities)
		require.NoError(t, err)

		roles := []domain.RestrictedRole{domain.NewRestrictedRoleFromRole(role)}
		err = s.repo.AssignRolesForEntity(ctx, entityID, entityType, roles)
		require.NoError(t, err)

		permissions, err := s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)

		assignedRoles, err := s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, assignedRoles, 1)

		err = s.repo.ClearRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)

		permissions, err = s.repo.GetPermissions(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Len(t, permissions, 1)

		assignedRoles, err = s.repo.GetRolesForEntity(ctx, entityID, entityType)
		require.NoError(t, err)
		assert.Empty(t, assignedRoles)
	})
}
