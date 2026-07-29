package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	rbacservice "github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/plugin/sdk/authz"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	authzTestUserID   = uint(42)
	authzTestServerID = uint(7)
)

// newAuthzService wires the real RBAC service over an in-memory repository so
// the tests exercise the same permission semantics the panel uses.
func newAuthzService(t *testing.T, setup func(context.Context, *inmemory.RBACRepository)) *AuthzServiceImpl {
	t.Helper()

	repo := inmemory.NewRBACRepository()
	if setup != nil {
		setup(context.Background(), repo)
	}

	service := rbacservice.NewRBAC(services.NewNilTransactionManager(), repo, 0)
	t.Cleanup(service.Close)

	return NewAuthzService(service)
}

func TestAuthzService_Can(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(context.Context, *inmemory.RBACRepository)
		abilities []string
		want      bool
	}{
		{
			name:      "no_permissions_denies",
			abilities: []string{string(domain.AbilityNameView)},
			want:      false,
		},
		{
			name: "granted_ability_allows",
			setup: func(ctx context.Context, r *inmemory.RBACRepository) {
				_ = r.Allow(ctx, authzTestUserID, domain.EntityTypeUser, []domain.Ability{
					{Name: domain.AbilityNameView},
				})
			},
			abilities: []string{string(domain.AbilityNameView)},
			want:      true,
		},
		{
			name: "all_abilities_required",
			setup: func(ctx context.Context, r *inmemory.RBACRepository) {
				_ = r.Allow(ctx, authzTestUserID, domain.EntityTypeUser, []domain.Ability{
					{Name: domain.AbilityNameView},
				})
			},
			abilities: []string{string(domain.AbilityNameView), string(domain.AbilityNameEdit)},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newAuthzService(t, tt.setup)

			resp, err := service.Can(context.Background(), &authz.CanRequest{
				UserId:    uint64(authzTestUserID),
				Abilities: tt.abilities,
			})

			require.NoError(t, err)
			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.want, resp.Allowed)
		})
	}
}

func TestAuthzService_CanOneOf(t *testing.T) {
	service := newAuthzService(t, func(ctx context.Context, r *inmemory.RBACRepository) {
		_ = r.Allow(ctx, authzTestUserID, domain.EntityTypeUser, []domain.Ability{
			{Name: domain.AbilityNameView},
		})
	})

	resp, err := service.CanOneOf(context.Background(), &authz.CanRequest{
		UserId:    uint64(authzTestUserID),
		Abilities: []string{string(domain.AbilityNameEdit), string(domain.AbilityNameView)},
	})

	require.NoError(t, err)
	assert.True(t, resp.Allowed)
}

func TestAuthzService_CanForEntity(t *testing.T) {
	tests := []struct {
		name     string
		serverID uint64
		want     bool
	}{
		{name: "granted_server_allows", serverID: uint64(authzTestServerID), want: true},
		{name: "other_server_denies", serverID: 999, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newAuthzService(t, func(ctx context.Context, r *inmemory.RBACRepository) {
				_ = r.Allow(ctx, authzTestUserID, domain.EntityTypeUser, []domain.Ability{
					domain.CreateAbilityForEntity(
						domain.AbilityNameGameServerStart, authzTestServerID, domain.EntityTypeServer,
					),
				})
			})

			resp, err := service.CanForEntity(context.Background(), &authz.CanForEntityRequest{
				UserId:     uint64(authzTestUserID),
				EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
				EntityId:   tt.serverID,
				Abilities:  []string{string(domain.AbilityNameGameServerStart)},
			})

			require.NoError(t, err)
			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.want, resp.Allowed)
		})
	}
}

func TestAuthzService_CanAnyForEntity(t *testing.T) {
	service := newAuthzService(t, func(ctx context.Context, r *inmemory.RBACRepository) {
		_ = r.Allow(ctx, authzTestUserID, domain.EntityTypeUser, []domain.Ability{
			domain.CreateAbilityForEntity(
				domain.AbilityNameGameServerStop, authzTestServerID, domain.EntityTypeServer,
			),
		})
	})

	resp, err := service.CanAnyForEntity(context.Background(), &authz.CanForEntityRequest{
		UserId:     uint64(authzTestUserID),
		EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
		EntityId:   uint64(authzTestServerID),
		Abilities: []string{
			string(domain.AbilityNameGameServerStart),
			string(domain.AbilityNameGameServerStop),
		},
	})

	require.NoError(t, err)
	assert.True(t, resp.Allowed)
}

func TestAuthzService_CanForEntity_unspecified_entity_type_is_rejected(t *testing.T) {
	service := newAuthzService(t, nil)

	resp, err := service.CanForEntity(context.Background(), &authz.CanForEntityRequest{
		UserId:     uint64(authzTestUserID),
		EntityType: proto.EntityType_ENTITY_TYPE_UNSPECIFIED,
		EntityId:   1,
		Abilities:  []string{string(domain.AbilityNameView)},
	})

	require.NoError(t, err)
	assert.False(t, resp.Allowed)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "unknown entity type")
}

func TestAuthzService_GetUserRoles(t *testing.T) {
	service := newAuthzService(t, func(ctx context.Context, r *inmemory.RBACRepository) {
		role := &domain.Role{Name: "moderator"}
		_ = r.SaveRole(ctx, role)
		_ = r.AssignRolesForEntity(ctx, authzTestUserID, domain.EntityTypeUser, []domain.RestrictedRole{
			domain.NewRestrictedRoleFromRole(*role),
		})
	})

	resp, err := service.GetUserRoles(context.Background(), &authz.GetUserRolesRequest{
		UserId: uint64(authzTestUserID),
	})

	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.Equal(t, []string{"moderator"}, resp.Roles)
}

// failingRBACChecker stands in for an RBAC service whose storage is down.
type failingRBACChecker struct {
	err error
}

func (c failingRBACChecker) Can(context.Context, uint, []domain.AbilityName) (bool, error) {
	return false, c.err
}

func (c failingRBACChecker) CanOneOf(context.Context, uint, []domain.AbilityName) (bool, error) {
	return false, c.err
}

func (c failingRBACChecker) CanForEntity(
	context.Context, uint, domain.EntityType, uint, []domain.AbilityName,
) (bool, error) {
	return false, c.err
}

func (c failingRBACChecker) CanAnyForEntity(
	context.Context, uint, domain.EntityType, uint, []domain.AbilityName,
) (bool, error) {
	return false, c.err
}

func (c failingRBACChecker) GetRoles(context.Context, uint) ([]string, error) {
	return nil, c.err
}

// A failed check must surface as an error, never as a plain denial: the
// generated host stub panics on a returned error, and a plugin reading only
// Allowed would mistake an outage for a deliberate "no".
func TestAuthzService_check_failure_is_reported_in_the_error_field(t *testing.T) {
	service := NewAuthzService(failingRBACChecker{err: errors.New("database is down")})

	resp, err := service.Can(context.Background(), &authz.CanRequest{
		UserId:    1,
		Abilities: []string{string(domain.AbilityNameView)},
	})

	require.NoError(t, err)
	assert.False(t, resp.Allowed)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "database is down")

	rolesResp, err := service.GetUserRoles(context.Background(), &authz.GetUserRolesRequest{UserId: 1})

	require.NoError(t, err)
	assert.Empty(t, rolesResp.Roles)
	require.NotNil(t, rolesResp.Error)
	assert.Contains(t, *rolesResp.Error, "database is down")
}
