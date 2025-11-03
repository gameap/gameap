package rbac

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	adminUser     = domain.User{ID: 1, Login: "admin"}
	regularUser   = domain.User{ID: 2, Login: "user"}
	forbiddenUser = domain.User{ID: 3, Login: "forbidden"}
	globalUser    = domain.User{ID: 4, Login: "global"}
	noPermUser    = domain.User{ID: 5, Login: "noperm"}

	testAbilities = []domain.Ability{
		{ID: 1, Name: domain.AbilityNameAdminRolesPermissions},
		{ID: 2, Name: domain.AbilityNameView},
		{ID: 3, Name: domain.AbilityNameEdit},

		// Entity-specific abilities
		{ID: 101, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer)},
		{ID: 201, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: lo.ToPtr(uint(1))},
		{ID: 202, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: lo.ToPtr(uint(123))},
	}

	testRoles = map[string]domain.Role{
		"admin": {ID: 1, Name: "admin"},
		"user":  {ID: 2, Name: "user"},
	}

	testPermissions = []domain.Permission{
		{AbilityID: 1, EntityID: lo.ToPtr(testRoles["admin"].ID), EntityType: lo.ToPtr(domain.EntityTypeRole)},
		{AbilityID: 2, EntityID: lo.ToPtr(testRoles["admin"].ID), EntityType: lo.ToPtr(domain.EntityTypeRole)},
		{AbilityID: 2, EntityID: lo.ToPtr(testRoles["user"].ID), EntityType: lo.ToPtr(domain.EntityTypeRole)},

		{AbilityID: 2, EntityID: &forbiddenUser.ID, EntityType: lo.ToPtr(domain.EntityTypeUser), Forbidden: true},
		{AbilityID: 3, EntityID: &globalUser.ID, EntityType: lo.ToPtr(domain.EntityTypeUser)},

		// Entity-specific permissions
		{AbilityID: 101, EntityID: &globalUser.ID, EntityType: lo.ToPtr(domain.EntityTypeUser)},
		{AbilityID: 201, EntityID: &forbiddenUser.ID, EntityType: lo.ToPtr(domain.EntityTypeUser), Forbidden: true},
		{AbilityID: 202, EntityID: &regularUser.ID, EntityType: lo.ToPtr(domain.EntityTypeUser)},
	}

	testAssignedRoles = []domain.AssignedRole{
		{
			ID:         1,
			EntityID:   adminUser.ID,
			EntityType: domain.EntityTypeUser,
			RoleID:     testRoles["admin"].ID,
		},
		{
			ID:               2,
			EntityID:         regularUser.ID,
			EntityType:       domain.EntityTypeUser,
			RoleID:           testRoles["user"].ID,
			RestrictedToID:   lo.ToPtr(uint(123)),
			RestrictedToType: lo.ToPtr(domain.EntityTypeServer)},
	}
)

func setupRBAC(t *testing.T) *RBAC {
	t.Helper()

	return prepareRBACService(
		t,
		lo.MapToSlice(testRoles, func(_ string, role domain.Role) domain.Role { return role }),
		testAssignedRoles,
		testAbilities,
		testPermissions,
	)
}

func prepareRBACService(
	t *testing.T,
	roles []domain.Role,
	assignedRoles []domain.AssignedRole,
	abilities []domain.Ability,
	permissions []domain.Permission,
) *RBAC {
	t.Helper()

	repo := inmemory.NewRBACRepository()
	rbacService := NewRBAC(services.NewNilTransactionManager(), repo, 0)
	ctx := context.Background()

	for _, ability := range abilities {
		require.NoError(t, repo.SaveAbility(ctx, &ability))
	}

	// Create roles from predefined slice
	for _, role := range roles {
		require.NoError(t, repo.SaveRole(ctx, &role))
	}

	for _, permission := range permissions {
		permissionCopy := permission
		require.NoError(t, repo.SavePermission(ctx, &permissionCopy))
	}

	for _, assignedRole := range assignedRoles {
		require.NoError(t, repo.SaveAssignedRole(ctx, &assignedRole))
	}

	return rbacService
}

func TestRBAC_Can(t *testing.T) {
	ctx := context.Background()
	rbacService := setupRBAC(t)

	tests := []struct {
		name      string
		user      domain.User
		abilities []domain.AbilityName
		want      bool
	}{
		{
			name:      "Admin with admin roles & permissions",
			user:      adminUser,
			abilities: []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
			want:      true,
		},
		{
			name:      "Regular user without admin roles & permissions",
			user:      regularUser,
			abilities: []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
			want:      false,
		},
		{
			name:      "User with forbidden permission",
			user:      forbiddenUser,
			abilities: []domain.AbilityName{domain.AbilityNameView},
			want:      false,
		},
		{
			name:      "User with global allowed permission",
			user:      globalUser,
			abilities: []domain.AbilityName{domain.AbilityNameEdit},
			want:      true,
		},
		{
			name:      "User without global permission",
			user:      noPermUser,
			abilities: []domain.AbilityName{domain.AbilityNameEdit},
			want:      false, // No explicit permission
		},
		{
			name:      "Multiple abilities - admin has one",
			user:      adminUser,
			abilities: []domain.AbilityName{domain.AbilityNameAdminRolesPermissions, domain.AbilityNameView},
			want:      true,
		},
		{
			name:      "Multiple abilities - forbidden user",
			user:      forbiddenUser,
			abilities: []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			want:      false, // Forbidden view should block
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacService.Can(ctx, tt.user.ID, tt.abilities)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBAC_Can_RoleBased(t *testing.T) {
	type state struct {
		roles         []domain.Role
		assignedRoles []domain.AssignedRole
		abilities     []domain.Ability
		permissions   []domain.Permission
	}

	tests := []struct {
		name             string
		state            state
		user             domain.User
		abilitiesToCheck []domain.AbilityName
		want             bool
	}{
		{
			// This test ensures that duplicate permissions do not cause incorrect behavior.
			name: "double permissions",
			state: state{
				roles: []domain.Role{
					{ID: 1, Name: "user"},
				},
				assignedRoles: []domain.AssignedRole{
					{
						ID:         1,
						EntityID:   regularUser.ID,
						EntityType: domain.EntityTypeUser,
						RoleID:     1,
					},
				},
				abilities: []domain.Ability{
					{ID: 1, Name: domain.AbilityNameCreate},
					{ID: 2, Name: domain.AbilityNameView},
				},
				permissions: []domain.Permission{
					{
						AbilityID:  2,
						EntityID:   lo.ToPtr(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
					{
						AbilityID:  2,
						EntityID:   lo.ToPtr(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
				},
			},
			user:             regularUser,
			abilitiesToCheck: []domain.AbilityName{domain.AbilityNameCreate, domain.AbilityNameView},
			want:             false,
		},
		{
			// This test ensures that entity-specific permissions are correctly evaluated.
			// domain.AbilityNameCreate should not be granted because it's only for a specific entity
			name: "entity specific permission",
			state: state{
				roles: []domain.Role{{ID: 1, Name: "user"}},
				assignedRoles: []domain.AssignedRole{
					{
						ID:         1,
						EntityID:   regularUser.ID,
						EntityType: domain.EntityTypeUser,
						RoleID:     1,
					},
				},
				abilities: []domain.Ability{
					{ID: 1, Name: domain.AbilityNameCreate, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: lo.ToPtr(uint(123))},
					{ID: 2, Name: domain.AbilityNameView},
				},
				permissions: []domain.Permission{
					{
						AbilityID:  1,
						EntityID:   lo.ToPtr(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
					{
						AbilityID:  2,
						EntityID:   lo.ToPtr(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
				},
			},
			user:             regularUser,
			abilitiesToCheck: []domain.AbilityName{domain.AbilityNameCreate, domain.AbilityNameView},
			want:             false,
		},
		{
			// This test ensures that permissions restricted by role restrictions are not incorrectly granted.
			name: "restricted to entity role",
			state: state{
				roles: []domain.Role{{ID: 1, Name: "user"}},
				assignedRoles: []domain.AssignedRole{
					{
						ID:               1,
						EntityID:         regularUser.ID,
						EntityType:       domain.EntityTypeUser,
						RoleID:           1,
						RestrictedToID:   lo.ToPtr(uint(123)),
						RestrictedToType: lo.ToPtr(domain.EntityTypeServer),
					},
				},
				abilities: []domain.Ability{
					{ID: 1, Name: domain.AbilityNameView},
				},
				permissions: []domain.Permission{
					{
						AbilityID:  1,
						EntityID:   lo.ToPtr(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
				},
			},
			user:             regularUser,
			abilitiesToCheck: []domain.AbilityName{domain.AbilityNameView},
			want:             false,
		},
		{
			// This case without role restrictions should allow the permission to be granted.
			// domain.AbilityNameView should be granted because there are no restrictions on the role
			name: "without restriction to entity role",
			state: state{
				roles: []domain.Role{{ID: 1, Name: "user"}},
				assignedRoles: []domain.AssignedRole{
					{
						ID:         1,
						EntityID:   regularUser.ID,
						EntityType: domain.EntityTypeUser,
						RoleID:     1,
					},
				},
				abilities: []domain.Ability{
					{ID: 1, Name: domain.AbilityNameView},
				},
				permissions: []domain.Permission{
					{
						AbilityID:  1,
						EntityID:   lo.ToPtr(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
				},
			},
			user:             regularUser,
			abilitiesToCheck: []domain.AbilityName{domain.AbilityNameView},
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rbacService := prepareRBACService(
				t,
				tt.state.roles,
				tt.state.assignedRoles,
				tt.state.abilities,
				tt.state.permissions,
			)

			got, err := rbacService.Can(ctx, tt.user.ID, tt.abilitiesToCheck)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBAC_CanOneOf(t *testing.T) {
	ctx := context.Background()
	rbacService := setupRBAC(t)

	tests := []struct {
		name      string
		user      domain.User
		abilities []domain.AbilityName
		want      bool
	}{
		{
			name:      "Admin with admin roles & permissions",
			user:      adminUser,
			abilities: []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
			want:      true,
		},
		{
			name:      "Regular user without admin roles & permissions",
			user:      regularUser,
			abilities: []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
			want:      false, // No explicit permission granted
		},
		{
			name:      "User with forbidden permission",
			user:      forbiddenUser,
			abilities: []domain.AbilityName{domain.AbilityNameView},
			want:      false,
		},
		{
			name:      "User with global allowed permission",
			user:      globalUser,
			abilities: []domain.AbilityName{domain.AbilityNameEdit},
			want:      true,
		},
		{
			name:      "User without global permission",
			user:      noPermUser,
			abilities: []domain.AbilityName{domain.AbilityNameEdit},
			want:      false, // No explicit permission
		},
		{
			name:      "Multiple abilities - admin has one of them",
			user:      adminUser,
			abilities: []domain.AbilityName{domain.AbilityNameAdminRolesPermissions, domain.AbilityNameView},
			want:      true, // Has admin permission
		},
		{
			name:      "Multiple abilities - global user has one of them",
			user:      globalUser,
			abilities: []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			want:      true, // Has edit permission
		},
		{
			name:      "Multiple abilities - forbidden user has none",
			user:      forbiddenUser,
			abilities: []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			want:      false, // View is forbidden, edit not granted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacService.CanOneOf(ctx, tt.user.ID, tt.abilities)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBAC_CanForEntity(t *testing.T) {
	ctx := context.Background()
	rbacService := setupRBAC(t)

	tests := []struct {
		name       string
		user       domain.User
		entityType domain.EntityType
		entityID   uint
		abilities  []domain.AbilityName
		want       bool
	}{
		{
			name:       "Admin with global permission for any entity",
			user:       adminUser,
			entityType: domain.EntityTypeServer,
			entityID:   1,
			abilities:  []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
			want:       true, // Global admin permission
		},
		{
			name:       "Global user with entity-type permission for all servers",
			user:       globalUser,
			entityType: domain.EntityTypeServer,
			entityID:   100,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       true, // Has entity-type permission for all servers (ID: 101)
		},
		{
			name:       "Regular user with entity-specific permission for server 123",
			user:       regularUser,
			entityType: domain.EntityTypeServer,
			entityID:   123,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       true, // Has specific permission for server 123 (ID: 202)
		},
		{
			name:       "Regular user with entity-specific view permission for server 123",
			user:       regularUser,
			entityType: domain.EntityTypeServer,
			entityID:   123,
			abilities:  []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			want:       false, // Has no edit permission for server 123 (ID: 202)
		},
		{
			name:       "Regular user without permission for different server",
			user:       regularUser,
			entityType: domain.EntityTypeServer,
			entityID:   456,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       false, // No permission for server 456
		},
		{
			name:       "Forbidden user with entity-specific forbidden permission",
			user:       forbiddenUser,
			entityType: domain.EntityTypeServer,
			entityID:   1,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       false, // Has entity-specific forbidden permission (ID: 201)
		},
		{
			name:       "User without any entity permission",
			user:       noPermUser,
			entityType: domain.EntityTypeServer,
			entityID:   1,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       false, // No permissions at all
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacService.CanForEntity(ctx, tt.user.ID, tt.entityType, tt.entityID, tt.abilities)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBAC_CanAnyForEntity(t *testing.T) {
	ctx := context.Background()
	rbacService := setupRBAC(t)

	tests := []struct {
		name       string
		user       domain.User
		entityType domain.EntityType
		entityID   uint
		abilities  []domain.AbilityName
		want       bool
	}{
		{
			name:       "Admin with global permission for any entity",
			user:       adminUser,
			entityType: domain.EntityTypeServer,
			entityID:   1,
			abilities:  []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
			want:       true, // Global admin permission
		},
		{
			name:       "Global user with entity-type permission for all servers",
			user:       globalUser,
			entityType: domain.EntityTypeServer,
			entityID:   100,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       true, // Has entity-type permission for all servers (ID: 101)
		},
		{
			name:       "Regular user with entity-specific permission for server 123",
			user:       regularUser,
			entityType: domain.EntityTypeServer,
			entityID:   123,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       true, // Has specific permission for server 123 (ID: 202)
		},
		{
			name:       "Regular user with entity-specific view permission for server 123",
			user:       regularUser,
			entityType: domain.EntityTypeServer,
			entityID:   123,
			abilities:  []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			want:       true, // Has view permission for server 123 (ID: 202)
		},
		{
			name:       "Regular user without permission for different server",
			user:       regularUser,
			entityType: domain.EntityTypeServer,
			entityID:   456,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       false, // No permission for server 456
		},
		{
			name:       "Forbidden user with entity-specific forbidden permission",
			user:       forbiddenUser,
			entityType: domain.EntityTypeServer,
			entityID:   1,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       false, // Has entity-specific forbidden permission (ID: 201)
		},
		{
			name:       "User without any entity permission",
			user:       noPermUser,
			entityType: domain.EntityTypeServer,
			entityID:   1,
			abilities:  []domain.AbilityName{domain.AbilityNameView},
			want:       false, // No permissions at all
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rbacService.CanAnyForEntity(ctx, tt.user.ID, tt.entityType, tt.entityID, tt.abilities)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBAC_GetRoles(t *testing.T) {
	ctx := context.Background()
	rbacService := setupRBAC(t)

	tests := []struct {
		name      string
		user      domain.User
		wantRoles []string
	}{
		{
			name:      "admin_user_with_admin_role",
			user:      adminUser,
			wantRoles: []string{"admin"},
		},
		{
			name:      "regular_user_with_user_role",
			user:      regularUser,
			wantRoles: []string{"user"},
		},
		{
			name:      "user_without_roles",
			user:      noPermUser,
			wantRoles: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles, err := rbacService.GetRoles(ctx, tt.user.ID)

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantRoles, roles)
		})
	}
}

func TestRBAC_SetRolesToUser(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		userID      uint
		roleNames   []string
		wantErr     bool
		wantErrType error
		verifyRoles []string
	}{
		{
			name:        "set_single_role_to_user",
			userID:      noPermUser.ID,
			roleNames:   []string{"admin"},
			wantErr:     false,
			verifyRoles: []string{"admin"},
		},
		{
			name:        "set_multiple_roles_to_user",
			userID:      noPermUser.ID,
			roleNames:   []string{"admin", "user"},
			wantErr:     false,
			verifyRoles: []string{"admin", "user"},
		},
		{
			name:        "replace_existing_role",
			userID:      adminUser.ID,
			roleNames:   []string{"user"},
			wantErr:     false,
			verifyRoles: []string{"user"},
		},
		{
			name:        "clear_all_roles_with_empty_list",
			userID:      adminUser.ID,
			roleNames:   []string{},
			wantErr:     false,
			verifyRoles: []string{},
		},
		{
			name:        "set_invalid_role_name",
			userID:      noPermUser.ID,
			roleNames:   []string{"invalid_role"},
			wantErr:     true,
			wantErrType: InvalidRoleNameError(""),
		},
		{
			name:        "set_mix_of_valid_and_invalid_roles",
			userID:      noPermUser.ID,
			roleNames:   []string{"admin", "invalid_role"},
			wantErr:     true,
			wantErrType: InvalidRoleNameError(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService := setupRBAC(t)
			err := rbacService.SetRolesToUser(ctx, tt.userID, tt.roleNames)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.IsType(t, tt.wantErrType, err)
				}
			} else {
				require.NoError(t, err)

				roles, err := rbacService.GetRoles(ctx, tt.userID)
				require.NoError(t, err)
				assert.ElementsMatch(t, tt.verifyRoles, roles)
			}
		})
	}
}

func TestRBAC_AllowUserAbilitiesForEntity(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		userID       uint
		entityID     uint
		entityType   domain.EntityType
		abilityNames []domain.AbilityName
		verifyCan    bool
	}{
		{
			name:         "allow_single_ability_for_entity",
			userID:       noPermUser.ID,
			entityID:     123,
			entityType:   domain.EntityTypeServer,
			abilityNames: []domain.AbilityName{domain.AbilityNameView},
			verifyCan:    true,
		},
		{
			name:         "allow_multiple_abilities_for_entity",
			userID:       noPermUser.ID,
			entityID:     456,
			entityType:   domain.EntityTypeServer,
			abilityNames: []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			verifyCan:    true,
		},
		{
			name:         "allow_ability_for_user_with_existing_permission",
			userID:       regularUser.ID,
			entityID:     123,
			entityType:   domain.EntityTypeServer,
			abilityNames: []domain.AbilityName{domain.AbilityNameView},
			verifyCan:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService := setupRBAC(t)
			err := rbacService.AllowUserAbilitiesForEntity(
				ctx,
				tt.userID,
				tt.entityID,
				tt.entityType,
				tt.abilityNames,
			)

			require.NoError(t, err)

			can, err := rbacService.CanForEntity(
				ctx,
				tt.userID,
				tt.entityType,
				tt.entityID,
				tt.abilityNames,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.verifyCan, can)
		})
	}
}

func TestRBAC_RevokeOrForbidUserAbilitiesForEntity(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		setupFunc    func(t *testing.T) *RBAC
		userID       uint
		entityID     uint
		entityType   domain.EntityType
		abilityNames []domain.AbilityName
		verifyCanNot bool
	}{
		{
			name: "revoke_direct_permission",
			setupFunc: func(t *testing.T) *RBAC {
				t.Helper()
				rbacService := setupRBAC(t)
				err := rbacService.AllowUserAbilitiesForEntity(
					ctx,
					noPermUser.ID,
					999,
					domain.EntityTypeServer,
					[]domain.AbilityName{domain.AbilityNameView},
				)
				require.NoError(t, err)

				return rbacService
			},
			userID:       noPermUser.ID,
			entityID:     999,
			entityType:   domain.EntityTypeServer,
			abilityNames: []domain.AbilityName{domain.AbilityNameView},
			verifyCanNot: true,
		},
		{
			name: "forbid_ability_inherited_from_role",
			setupFunc: func(t *testing.T) *RBAC {
				t.Helper()

				return setupRBAC(t)
			},
			userID:       regularUser.ID,
			entityID:     123,
			entityType:   domain.EntityTypeServer,
			abilityNames: []domain.AbilityName{domain.AbilityNameView},
			verifyCanNot: true,
		},
		{
			name: "revoke_multiple_abilities",
			setupFunc: func(t *testing.T) *RBAC {
				t.Helper()
				rbacService := setupRBAC(t)
				err := rbacService.AllowUserAbilitiesForEntity(
					ctx,
					noPermUser.ID,
					888,
					domain.EntityTypeServer,
					[]domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
				)
				require.NoError(t, err)

				return rbacService
			},
			userID:       noPermUser.ID,
			entityID:     888,
			entityType:   domain.EntityTypeServer,
			abilityNames: []domain.AbilityName{domain.AbilityNameView, domain.AbilityNameEdit},
			verifyCanNot: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService := tt.setupFunc(t)

			err := rbacService.RevokeOrForbidUserAbilitiesForEntity(
				ctx,
				tt.userID,
				tt.entityID,
				tt.entityType,
				tt.abilityNames,
			)

			require.NoError(t, err)

			can, err := rbacService.CanForEntity(
				ctx,
				tt.userID,
				tt.entityType,
				tt.entityID,
				tt.abilityNames,
			)
			require.NoError(t, err)
			assert.Equal(t, !tt.verifyCanNot, can)
		})
	}
}
