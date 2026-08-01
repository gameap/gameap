package hostlibrary

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	rbacservice "github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/plugin/sdk/authz"
	"github.com/gameap/gameap/pkg/plugin/sdk/rbac"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rbacTestPluginID = uint64(11)

// stubPermissionChecker replaces the database lookup of the plugin's grants.
type stubPermissionChecker struct {
	allowed bool
	err     error
}

func (c stubPermissionChecker) Has(context.Context, uint64, domain.PluginPermission) (bool, error) {
	return c.allowed, c.err
}

type rbacTestEnv struct {
	service *RBACServiceImpl
	authz   *AuthzServiceImpl
	repo    *inmemory.RBACRepository
}

func newRBACTestEnv(t *testing.T, checker PluginPermissionChecker, pluginID uint64) rbacTestEnv {
	t.Helper()

	repo := inmemory.NewRBACRepository()

	// A non-zero TTL is deliberate: without it the cache is disabled and the
	// invalidation this library performs would go untested.
	manager := rbacservice.NewRBAC(services.NewNilTransactionManager(), repo, time.Hour)
	t.Cleanup(manager.Close)

	return rbacTestEnv{
		service: NewRBACService(pluginID, manager, repo, checker),
		authz:   NewAuthzService(manager),
		repo:    repo,
	}
}

func newAllowedRBACEnv(t *testing.T) rbacTestEnv {
	t.Helper()

	return newRBACTestEnv(t, stubPermissionChecker{allowed: true}, rbacTestPluginID)
}

func TestRBACService_denied_without_the_grant(t *testing.T) {
	ctx := context.Background()
	env := newRBACTestEnv(t, stubPermissionChecker{allowed: false}, rbacTestPluginID)

	t.Run("mutations_are_refused", func(t *testing.T) {
		resp, err := env.service.SaveRole(ctx, &rbac.SaveRoleRequest{
			Role: &rbac.Role{Name: "should-not-exist"},
		})

		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, string(domain.PluginPermissionManageRBAC))

		roles, err := env.repo.GetRoles(ctx)
		require.NoError(t, err)
		assert.Empty(t, roles, "a denied call must not reach the repository")
	})

	t.Run("reads_are_refused", func(t *testing.T) {
		resp, err := env.service.GetRoles(ctx, &rbac.GetRolesRequest{})

		require.NoError(t, err)
		assert.Empty(t, resp.Roles)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, string(domain.PluginPermissionManageRBAC))
	})

	t.Run("ability_writes_are_refused", func(t *testing.T) {
		resp, err := env.service.Allow(ctx, &rbac.AbilitiesRequest{
			EntityType: proto.EntityType_ENTITY_TYPE_USER,
			EntityId:   1,
			Abilities:  []*rbac.Ability{{Name: string(domain.AbilityNameView)}},
		})

		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, string(domain.PluginPermissionManageRBAC))
	})
}

// A plugin loaded without a database record (dry-run, install validation) has
// no grants to read, so the real checker must refuse it.
func TestRBACService_transient_plugin_id_is_denied(t *testing.T) {
	ctx := context.Background()
	checker := NewRepositoryPermissionChecker(inmemory.NewPluginRepository())

	env := newRBACTestEnv(t, checker, 0)

	resp, err := env.service.SaveRole(ctx, &rbac.SaveRoleRequest{Role: &rbac.Role{Name: "nope"}})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, string(domain.PluginPermissionManageRBAC))
}

// A failing permission lookup must deny rather than fall open.
func TestRBACService_permission_check_failure_denies(t *testing.T) {
	ctx := context.Background()
	env := newRBACTestEnv(t, stubPermissionChecker{err: errors.New("db down")}, rbacTestPluginID)

	resp, err := env.service.DeleteRole(ctx, &rbac.DeleteRoleRequest{Id: 1})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "db down")
}

// The flow a role-management plugin actually performs: define a role, give it
// abilities, hand it to a user, and have the panel honour it.
func TestRBACService_create_role_grant_abilities_assign_to_user(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(500)
	const serverID = uint64(9)

	saved, err := env.service.SaveRole(ctx, &rbac.SaveRoleRequest{
		Role: &rbac.Role{Name: "server-operator", Title: new("Server Operator")},
	})
	require.NoError(t, err)
	require.True(t, saved.Success)
	require.NotNil(t, saved.Role)
	require.NotZero(t, saved.Role.Id, "a created role must come back with its id")

	allowResp, err := env.service.Allow(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_ROLE,
		EntityId:   saved.Role.Id,
		Abilities: []*rbac.Ability{{
			Name:       string(domain.AbilityNameGameServerRestart),
			EntityType: new(proto.EntityType_ENTITY_TYPE_SERVER),
			EntityId:   new(serverID),
		}},
	})
	require.NoError(t, err)
	require.True(t, allowResp.Success, allowResp.Error)

	assignResp, err := env.service.AssignRolesForEntity(ctx, &rbac.AssignRolesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Roles:      []*rbac.RestrictedRole{{Role: saved.Role}},
	})
	require.NoError(t, err)
	require.True(t, assignResp.Success, assignResp.Error)

	canResp, err := env.authz.CanForEntity(ctx, &authz.CanForEntityRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})
	require.NoError(t, err)
	assert.Nil(t, canResp.Error)
	assert.True(t, canResp.Allowed, "the ability granted through the new role must be honoured")

	rolesResp, err := env.service.GetRoles(ctx, &rbac.GetRolesRequest{})
	require.NoError(t, err)
	assert.Nil(t, rolesResp.Error)
	require.Len(t, rolesResp.Roles, 1)
	assert.Equal(t, "server-operator", rolesResp.Roles[0].Name)

	permsResp, err := env.service.GetPermissions(ctx, &rbac.EntityRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_ROLE,
		EntityId:   saved.Role.Id,
	})
	require.NoError(t, err)
	assert.Nil(t, permsResp.Error)
	require.Len(t, permsResp.Permissions, 1)
	require.NotNil(t, permsResp.Permissions[0].Ability)
	assert.Equal(t, string(domain.AbilityNameGameServerRestart), permsResp.Permissions[0].Ability.Name)
}

// Writes that bypass the RBAC service must still drop its permission cache,
// otherwise checks keep answering from a stale snapshot.
func TestRBACService_writes_invalidate_the_permission_cache(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(600)

	canReq := &authz.CanRequest{UserId: userID, Abilities: []string{string(domain.AbilityNameView)}}

	// Populates the cache with "denied".
	before, err := env.authz.Can(ctx, canReq)
	require.NoError(t, err)
	require.False(t, before.Allowed)

	allowResp, err := env.service.Allow(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Abilities:  []*rbac.Ability{{Name: string(domain.AbilityNameView)}},
	})
	require.NoError(t, err)
	require.True(t, allowResp.Success, allowResp.Error)

	after, err := env.authz.Can(ctx, canReq)
	require.NoError(t, err)
	assert.True(t, after.Allowed, "the grant must be visible immediately, not after the cache TTL")
}

func TestRBACService_role_changes_invalidate_the_whole_cache(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(700)

	role := &domain.Role{Name: "temporary"}
	require.NoError(t, env.repo.SaveRole(ctx, role))
	require.NoError(t, env.repo.Allow(ctx, role.ID, domain.EntityTypeRole, []domain.Ability{
		{Name: domain.AbilityNameView},
	}))
	require.NoError(t, env.repo.AssignRolesForEntity(ctx, uint(userID), domain.EntityTypeUser,
		[]domain.RestrictedRole{domain.NewRestrictedRoleFromRole(*role)}))

	canReq := &authz.CanRequest{UserId: userID, Abilities: []string{string(domain.AbilityNameView)}}

	before, err := env.authz.Can(ctx, canReq)
	require.NoError(t, err)
	require.True(t, before.Allowed)

	deleteResp, err := env.service.DeleteRole(ctx, &rbac.DeleteRoleRequest{Id: uint64(role.ID)})
	require.NoError(t, err)
	require.True(t, deleteResp.Success, deleteResp.Error)

	after, err := env.authz.Can(ctx, canReq)
	require.NoError(t, err)
	assert.False(t, after.Allowed, "abilities of a deleted role must stop applying at once")
}

func TestRBACService_SetUserRoles(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(800)

	require.NoError(t, env.repo.SaveRole(ctx, &domain.Role{Name: "editor"}))

	resp, err := env.service.SetUserRoles(ctx, &rbac.SetUserRolesRequest{
		UserId:    userID,
		RoleNames: []string{"editor"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)

	rolesResp, err := env.authz.GetUserRoles(ctx, &authz.GetUserRolesRequest{UserId: userID})
	require.NoError(t, err)
	assert.Equal(t, []string{"editor"}, rolesResp.Roles)

	t.Run("unknown_role_is_rejected", func(t *testing.T) {
		resp, err := env.service.SetUserRoles(ctx, &rbac.SetUserRolesRequest{
			UserId:    userID,
			RoleNames: []string{"does-not-exist"},
		})

		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "does-not-exist")
	})
}

func TestRBACService_SaveRole_requires_a_name(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	resp, err := env.service.SaveRole(ctx, &rbac.SaveRoleRequest{Role: &rbac.Role{}})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "name is required")
}

func TestRBACService_ClearRolesForEntity(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(900)

	role := &domain.Role{Name: "clearable"}
	require.NoError(t, env.repo.SaveRole(ctx, role))
	require.NoError(t, env.repo.AssignRolesForEntity(ctx, uint(userID), domain.EntityTypeUser,
		[]domain.RestrictedRole{domain.NewRestrictedRoleFromRole(*role)}))

	resp, err := env.service.ClearRolesForEntity(ctx, &rbac.EntityRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
	})
	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)

	rolesResp, err := env.service.GetRolesForEntity(ctx, &rbac.EntityRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
	})
	require.NoError(t, err)
	assert.Empty(t, rolesResp.Roles)
}

func TestRBACService_unspecified_entity_type_is_rejected(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	resp, err := env.service.Allow(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_UNSPECIFIED,
		EntityId:   1,
		Abilities:  []*rbac.Ability{{Name: string(domain.AbilityNameView)}},
	})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "unknown entity type")
}

func TestRepositoryPermissionChecker(t *testing.T) {
	ctx := context.Background()

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:                 domain.Uint64ID(rbacTestPluginID),
		Name:               "granted",
		AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionManageRBAC},
	}))
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:                 domain.Uint64ID(rbacTestPluginID + 1),
		Name:               "not-granted",
		AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionListenEvents},
	}))

	checker := NewRepositoryPermissionChecker(repo)

	tests := []struct {
		name     string
		pluginID uint64
		want     bool
	}{
		{name: "granted", pluginID: rbacTestPluginID, want: true},
		{name: "other_permission_only", pluginID: rbacTestPluginID + 1, want: false},
		{name: "unknown_plugin", pluginID: 12345, want: false},
		{name: "transient_load", pluginID: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checker.Has(ctx, tt.pluginID, domain.PluginPermissionManageRBAC)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Per-user grants bypass roles entirely, so the direct user-ability path must
// grant and take away access on its own.
func TestRBACService_user_abilities_for_entity_round_trip(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(700)
	const serverID = uint64(21)

	// ARRANGE / ACT — grant the ability directly to the user.
	allowResp, err := env.service.AllowUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})

	// ASSERT
	require.NoError(t, err)
	require.True(t, allowResp.Success, allowResp.Error)

	canResp, err := env.authz.CanForEntity(ctx, &authz.CanForEntityRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})
	require.NoError(t, err)
	assert.True(t, canResp.Allowed, "a directly granted ability must be honoured")

	// ACT — take it away again.
	revokeResp, err := env.service.RevokeOrForbidUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})

	// ASSERT
	require.NoError(t, err)
	require.True(t, revokeResp.Success, revokeResp.Error)

	canResp, err = env.authz.CanForEntity(ctx, &authz.CanForEntityRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})
	require.NoError(t, err)
	assert.False(t, canResp.Allowed, "a revoked ability must stop being honoured")
}

func TestRBACService_user_abilities_rejections(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		env       func(t *testing.T) rbacTestEnv
		call      func(context.Context, *RBACServiceImpl) (*rbac.Result, error)
		wantError string
	}{
		{
			name: "allow_without_the_grant_is_refused",
			env: func(t *testing.T) rbacTestEnv {
				t.Helper()

				return newRBACTestEnv(t, stubPermissionChecker{allowed: false}, rbacTestPluginID)
			},
			call: func(ctx context.Context, s *RBACServiceImpl) (*rbac.Result, error) {
				return s.AllowUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
					UserId:     1,
					EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
					EntityId:   1,
					Abilities:  []string{string(domain.AbilityNameView)},
				})
			},
			wantError: string(domain.PluginPermissionManageRBAC),
		},
		{
			name: "revoke_without_the_grant_is_refused",
			env: func(t *testing.T) rbacTestEnv {
				t.Helper()

				return newRBACTestEnv(t, stubPermissionChecker{allowed: false}, rbacTestPluginID)
			},
			call: func(ctx context.Context, s *RBACServiceImpl) (*rbac.Result, error) {
				return s.RevokeOrForbidUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
					UserId:     1,
					EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
					EntityId:   1,
					Abilities:  []string{string(domain.AbilityNameView)},
				})
			},
			wantError: string(domain.PluginPermissionManageRBAC),
		},
		{
			name: "allow_with_unspecified_entity_type_is_rejected",
			env:  newAllowedRBACEnv,
			call: func(ctx context.Context, s *RBACServiceImpl) (*rbac.Result, error) {
				return s.AllowUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
					UserId:     1,
					EntityType: proto.EntityType_ENTITY_TYPE_UNSPECIFIED,
					EntityId:   1,
					Abilities:  []string{string(domain.AbilityNameView)},
				})
			},
			wantError: "unknown entity type",
		},
		{
			name: "revoke_with_unspecified_entity_type_is_rejected",
			env:  newAllowedRBACEnv,
			call: func(ctx context.Context, s *RBACServiceImpl) (*rbac.Result, error) {
				return s.RevokeOrForbidUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
					UserId:     1,
					EntityType: proto.EntityType_ENTITY_TYPE_UNSPECIFIED,
					EntityId:   1,
					Abilities:  []string{string(domain.AbilityNameView)},
				})
			},
			wantError: "unknown entity type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			env := tt.env(t)

			// ACT
			resp, err := tt.call(ctx, env.service)

			// ASSERT
			require.NoError(t, err)
			assert.False(t, resp.Success)
			require.NotNil(t, resp.Error)
			assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")
		})
	}
}

// Forbid outranks a role grant; Revoke only drops the direct grant.
func TestRBACService_Forbid_overrides_a_role_grant(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(710)
	const serverID = uint64(22)

	// ARRANGE — the user gets the ability through a role.
	saved, err := env.service.SaveRole(ctx, &rbac.SaveRoleRequest{
		Role: &rbac.Role{Name: "restarter"},
	})
	require.NoError(t, err)
	require.True(t, saved.Success, saved.Error)

	allowResp, err := env.service.Allow(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_ROLE,
		EntityId:   saved.Role.Id,
		Abilities: []*rbac.Ability{{
			Name:       string(domain.AbilityNameGameServerRestart),
			EntityType: new(proto.EntityType_ENTITY_TYPE_SERVER),
			EntityId:   new(serverID),
		}},
	})
	require.NoError(t, err)
	require.True(t, allowResp.Success, allowResp.Error)

	assignResp, err := env.service.AssignRolesForEntity(ctx, &rbac.AssignRolesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Roles:      []*rbac.RestrictedRole{{Role: saved.Role}},
	})
	require.NoError(t, err)
	require.True(t, assignResp.Success, assignResp.Error)

	canResp, err := env.authz.CanForEntity(ctx, &authz.CanForEntityRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})
	require.NoError(t, err)
	require.True(t, canResp.Allowed, "the role grant must apply before the forbid")

	// ACT — forbid it for the user specifically.
	forbidResp, err := env.service.Forbid(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Abilities: []*rbac.Ability{{
			Name:       string(domain.AbilityNameGameServerRestart),
			EntityType: new(proto.EntityType_ENTITY_TYPE_SERVER),
			EntityId:   new(serverID),
		}},
	})

	// ASSERT
	require.NoError(t, err)
	require.True(t, forbidResp.Success, forbidResp.Error)

	canResp, err = env.authz.CanForEntity(ctx, &authz.CanForEntityRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})
	require.NoError(t, err)
	assert.False(t, canResp.Allowed, "an explicit forbid must outrank the role grant")
}

func TestRBACService_Revoke_drops_a_direct_grant(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(720)
	const serverID = uint64(23)

	// ARRANGE
	allowResp, err := env.service.Allow(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Abilities: []*rbac.Ability{{
			Name:       string(domain.AbilityNameGameServerRestart),
			EntityType: new(proto.EntityType_ENTITY_TYPE_SERVER),
			EntityId:   new(serverID),
		}},
	})
	require.NoError(t, err)
	require.True(t, allowResp.Success, allowResp.Error)

	// ACT
	revokeResp, err := env.service.Revoke(ctx, &rbac.AbilitiesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Abilities: []*rbac.Ability{{
			Name:       string(domain.AbilityNameGameServerRestart),
			EntityType: new(proto.EntityType_ENTITY_TYPE_SERVER),
			EntityId:   new(serverID),
		}},
	})

	// ASSERT
	require.NoError(t, err)
	require.True(t, revokeResp.Success, revokeResp.Error)

	canResp, err := env.authz.CanForEntity(ctx, &authz.CanForEntityRequest{
		UserId:     userID,
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   serverID,
		Abilities:  []string{string(domain.AbilityNameGameServerRestart)},
	})
	require.NoError(t, err)
	assert.False(t, canResp.Allowed, "a revoked direct grant must stop being honoured")
}

// GetRolesForEntity is the read side of AssignRolesForEntity; the role fields a
// plugin needs must survive the domain → proto conversion.
func TestRBACService_GetRolesForEntity_returns_assigned_roles(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	const userID = uint64(730)

	// ARRANGE
	saved, err := env.service.SaveRole(ctx, &rbac.SaveRoleRequest{
		Role: &rbac.Role{Name: "auditor", Title: new("Auditor")},
	})
	require.NoError(t, err)
	require.True(t, saved.Success, saved.Error)

	assignResp, err := env.service.AssignRolesForEntity(ctx, &rbac.AssignRolesRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
		Roles:      []*rbac.RestrictedRole{{Role: saved.Role}},
	})
	require.NoError(t, err)
	require.True(t, assignResp.Success, assignResp.Error)

	// ACT
	resp, err := env.service.GetRolesForEntity(ctx, &rbac.EntityRequest{
		EntityType: proto.EntityType_ENTITY_TYPE_USER,
		EntityId:   userID,
	})

	// ASSERT
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	require.Len(t, resp.Roles, 1)
	require.NotNil(t, resp.Roles[0].Role)
	assert.Equal(t, "auditor", resp.Roles[0].Role.Name)
	assert.Equal(t, saved.Role.Id, resp.Roles[0].Role.Id, "the role id must survive the round trip")
}

func TestRBACService_entity_reads_reject_unspecified_entity_type(t *testing.T) {
	ctx := context.Background()
	env := newAllowedRBACEnv(t)

	t.Run("get_permissions", func(t *testing.T) {
		// ACT
		resp, err := env.service.GetPermissions(ctx, &rbac.EntityRequest{
			EntityType: proto.EntityType_ENTITY_TYPE_UNSPECIFIED,
			EntityId:   1,
		})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "unknown entity type")
		assert.Empty(t, resp.Permissions)
	})

	t.Run("get_roles_for_entity", func(t *testing.T) {
		// ACT
		resp, err := env.service.GetRolesForEntity(ctx, &rbac.EntityRequest{
			EntityType: proto.EntityType_ENTITY_TYPE_UNSPECIFIED,
			EntityId:   1,
		})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "unknown entity type")
		assert.Empty(t, resp.Roles)
	})
}
