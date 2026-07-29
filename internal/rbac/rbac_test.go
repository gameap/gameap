package rbac

import (
	"context"
	"testing"
	"time"

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
		{ID: 201, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: new(uint(1))},
		{ID: 202, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: new(uint(123))},
	}

	testRoles = map[string]domain.Role{
		"admin": {ID: 1, Name: "admin"},
		"user":  {ID: 2, Name: "user"},
	}

	testPermissions = []domain.Permission{
		{AbilityID: 1, EntityID: new(testRoles["admin"].ID), EntityType: lo.ToPtr(domain.EntityTypeRole)},
		{AbilityID: 2, EntityID: new(testRoles["admin"].ID), EntityType: lo.ToPtr(domain.EntityTypeRole)},
		{AbilityID: 2, EntityID: new(testRoles["user"].ID), EntityType: lo.ToPtr(domain.EntityTypeRole)},

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
			RestrictedToID:   new(uint(123)),
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
						EntityID:   new(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
					{
						AbilityID:  2,
						EntityID:   new(uint(1)),
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
					{ID: 1, Name: domain.AbilityNameCreate, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: new(uint(123))},
					{ID: 2, Name: domain.AbilityNameView},
				},
				permissions: []domain.Permission{
					{
						AbilityID:  1,
						EntityID:   new(uint(1)),
						EntityType: lo.ToPtr(domain.EntityTypeRole),
					},
					{
						AbilityID:  2,
						EntityID:   new(uint(1)),
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
						RestrictedToID:   new(uint(123)),
						RestrictedToType: lo.ToPtr(domain.EntityTypeServer),
					},
				},
				abilities: []domain.Ability{
					{ID: 1, Name: domain.AbilityNameView},
				},
				permissions: []domain.Permission{
					{
						AbilityID:  1,
						EntityID:   new(uint(1)),
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
						EntityID:   new(uint(1)),
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

func TestRBAC_grantsAbilityForEntity(t *testing.T) {
	serverType := domain.EntityTypeServer

	allowAllServers := domain.Permission{
		Ability: &domain.Ability{Name: domain.AbilityNameView, EntityType: new(serverType)},
	}
	forbidServer5 := domain.Permission{
		Forbidden: true,
		Ability: &domain.Ability{
			Name: domain.AbilityNameView, EntityType: new(serverType), EntityID: new(uint(5)),
		},
	}
	allowServer5 := domain.Permission{
		Ability: &domain.Ability{
			Name: domain.AbilityNameView, EntityType: new(serverType), EntityID: new(uint(5)),
		},
	}
	globalAllow := domain.Permission{Ability: &domain.Ability{Name: domain.AbilityNameView}}
	globalForbid := domain.Permission{Forbidden: true, Ability: &domain.Ability{Name: domain.AbilityNameView}}

	tests := []struct {
		name        string
		permissions []domain.Permission
		entityID    uint
		want        bool
	}{
		{
			name:        "entity_type_allow_with_specific_forbid_denies_that_entity",
			permissions: []domain.Permission{allowAllServers, forbidServer5},
			entityID:    5,
			want:        false,
		},
		{
			name:        "forbid_wins_regardless_of_slice_order",
			permissions: []domain.Permission{forbidServer5, allowAllServers},
			entityID:    5,
			want:        false,
		},
		{
			name:        "specific_forbid_does_not_leak_to_sibling_entity",
			permissions: []domain.Permission{allowAllServers, forbidServer5},
			entityID:    7,
			want:        true,
		},
		{
			name:        "specific_forbid_does_not_leak_to_sibling_entity_reversed_order",
			permissions: []domain.Permission{forbidServer5, allowAllServers},
			entityID:    7,
			want:        true,
		},
		{
			name:        "entity_type_allow_grants_matching_entity",
			permissions: []domain.Permission{allowAllServers},
			entityID:    5,
			want:        true,
		},
		{
			name:        "lone_forbid_for_other_entity_does_not_grant",
			permissions: []domain.Permission{forbidServer5},
			entityID:    7,
			want:        false,
		},
		{
			name:        "specific_allow_grants_only_that_entity",
			permissions: []domain.Permission{allowServer5},
			entityID:    5,
			want:        true,
		},
		{
			name:        "specific_allow_does_not_grant_other_entity",
			permissions: []domain.Permission{allowServer5},
			entityID:    9,
			want:        false,
		},
		{
			name:        "global_allow_grants_any_entity",
			permissions: []domain.Permission{globalAllow},
			entityID:    5,
			want:        true,
		},
		{
			name:        "global_forbid_denies_any_entity",
			permissions: []domain.Permission{globalAllow, globalForbid},
			entityID:    5,
			want:        false,
		},
		{
			name:        "no_permissions_denies",
			permissions: nil,
			entityID:    5,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grantsAbilityForEntity(tt.permissions, serverType, tt.entityID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBAC_EntityForbidSemantics_endToEnd(t *testing.T) {
	ctx := context.Background()

	user := domain.User{ID: 40, Login: "scoped"}

	abilities := []domain.Ability{
		{ID: 400, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer)},
		{ID: 401, Name: domain.AbilityNameView, EntityType: lo.ToPtr(domain.EntityTypeServer), EntityID: new(uint(5))},
	}
	permissions := []domain.Permission{
		{AbilityID: 400, EntityID: &user.ID, EntityType: lo.ToPtr(domain.EntityTypeUser)},
		{AbilityID: 401, EntityID: &user.ID, EntityType: lo.ToPtr(domain.EntityTypeUser), Forbidden: true},
	}

	rbacService := prepareRBACService(t, nil, nil, abilities, permissions)

	view := []domain.AbilityName{domain.AbilityNameView}

	forbiddenEntity, err := rbacService.CanForEntity(ctx, user.ID, domain.EntityTypeServer, 5, view)
	require.NoError(t, err)
	assert.False(t, forbiddenEntity, "forbid on server 5 must deny that server")

	siblingEntity, err := rbacService.CanForEntity(ctx, user.ID, domain.EntityTypeServer, 7, view)
	require.NoError(t, err)
	assert.True(t, siblingEntity, "forbid on server 5 must NOT leak to server 7 (M1 over-deny)")

	anyForbidden, err := rbacService.CanAnyForEntity(ctx, user.ID, domain.EntityTypeServer, 5, view)
	require.NoError(t, err)
	assert.False(t, anyForbidden, "CanAnyForEntity must honour the forbid on server 5 (M2 fail-open)")

	anySibling, err := rbacService.CanAnyForEntity(ctx, user.ID, domain.EntityTypeServer, 7, view)
	require.NoError(t, err)
	assert.True(t, anySibling, "CanAnyForEntity must allow server 7")
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

// Role assignments change what a user may do, so the cached permission set
// has to go with them. Every other test here runs with the cache disabled
// (TTL 0), which would hide a missing invalidation.
func TestRBAC_SetRolesToUser_invalidates_the_cached_permissions(t *testing.T) {
	ctx := context.Background()

	repo := inmemory.NewRBACRepository()
	rbacService := NewRBAC(services.NewNilTransactionManager(), repo, time.Hour)
	t.Cleanup(rbacService.Close)

	role := &domain.Role{Name: "viewer"}
	require.NoError(t, repo.SaveRole(ctx, role))
	require.NoError(t, repo.Allow(ctx, role.ID, domain.EntityTypeRole, []domain.Ability{
		{Name: domain.AbilityNameView},
	}))

	can, err := rbacService.Can(ctx, noPermUser.ID, []domain.AbilityName{domain.AbilityNameView})
	require.NoError(t, err)
	require.False(t, can)

	require.NoError(t, rbacService.SetRolesToUser(ctx, noPermUser.ID, []string{"viewer"}))

	can, err = rbacService.Can(ctx, noPermUser.ID, []domain.AbilityName{domain.AbilityNameView})
	require.NoError(t, err)
	assert.True(t, can, "the new role must apply at once, not after the cache TTL")
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
