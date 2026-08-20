package base_test

import (
	"context"
	"net/http"
	"testing"

	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupServerRepo(t *testing.T) *inmemory.ServerRepository {
	t.Helper()

	return inmemory.NewServerRepository()
}

func saveServer(t *testing.T, repo *inmemory.ServerRepository, server *domain.Server) {
	t.Helper()

	err := repo.Save(context.Background(), server)
	require.NoError(t, err)
}

func newServer(id uint, name string) *domain.Server {
	return &domain.Server{
		ID:         id,
		UID:        uuid.New(),
		Enabled:    true,
		Name:       name,
		GameID:     "cs",
		DSID:       1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
	}
}

type errServerRepo struct {
	repositories.ServerRepository

	findErr error
}

func (r *errServerRepo) Find(
	_ context.Context,
	_ *filters.FindServer,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Server, error) {
	return nil, r.findErr
}

type errRBAC struct {
	canErr error
}

func (r *errRBAC) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, r.canErr
}

func (r *errRBAC) CanOneOf(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, nil
}

func (r *errRBAC) CanForEntity(
	_ context.Context,
	_ uint,
	_ domain.EntityType,
	_ uint,
	_ []domain.AbilityName,
) (bool, error) {
	return false, nil
}

func (r *errRBAC) GetRoles(_ context.Context, _ uint) ([]string, error) {
	return nil, nil
}

func (r *errRBAC) SetRolesToUser(_ context.Context, _ uint, _ []string) error {
	return nil
}

func (r *errRBAC) AllowUserAbilitiesForEntity(
	_ context.Context,
	_ uint,
	_ uint,
	_ domain.EntityType,
	_ []domain.AbilityName,
) error {
	return nil
}

func (r *errRBAC) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context,
	_ uint,
	_ uint,
	_ domain.EntityType,
	_ []domain.AbilityName,
) error {
	return nil
}

func TestServerFinder_FindUserServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		user           *domain.User
		serverID       uint
		setup          func(t *testing.T, repo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository)
		wantServerID   uint
		wantServerName string
		wantError      string
		wantStatusCode int
	}{
		{
			name:     "admin_user_finds_any_server",
			user:     &domain.User{ID: 1},
			serverID: 100,
			setup: func(t *testing.T, repo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				t.Helper()
				saveServer(t, repo, newServer(100, "admin-server"))
				adminRole := createAdminRole(t, rbacRepo)
				assignRoleToUser(t, rbacRepo, 1, adminRole)
			},
			wantServerID:   100,
			wantServerName: "admin-server",
		},
		{
			name:     "admin_user_finds_server_not_owned_by_them",
			user:     &domain.User{ID: 7},
			serverID: 200,
			setup: func(t *testing.T, repo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				t.Helper()
				saveServer(t, repo, newServer(200, "other-user-server"))
				repo.AddUserServer(99, 200)
				adminRole := createAdminRole(t, rbacRepo)
				assignRoleToUser(t, rbacRepo, 7, adminRole)
			},
			wantServerID:   200,
			wantServerName: "other-user-server",
		},
		{
			name:     "non_admin_user_finds_their_own_server",
			user:     &domain.User{ID: 42},
			serverID: 300,
			setup: func(t *testing.T, repo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				saveServer(t, repo, newServer(300, "user-owned"))
				repo.AddUserServer(42, 300)
			},
			wantServerID:   300,
			wantServerName: "user-owned",
		},
		{
			name:     "non_admin_user_cannot_access_server_they_do_not_own",
			user:     &domain.User{ID: 42},
			serverID: 400,
			setup: func(t *testing.T, repo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				saveServer(t, repo, newServer(400, "foreign-server"))
				repo.AddUserServer(99, 400)
			},
			wantError:      "server not found",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:     "server_does_not_exist",
			user:     &domain.User{ID: 42},
			serverID: 9999,
			setup: func(t *testing.T, _ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
			},
			wantError:      "server not found",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:     "admin_user_cannot_find_missing_server",
			user:     &domain.User{ID: 1},
			serverID: 5000,
			setup: func(t *testing.T, _ *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				t.Helper()
				adminRole := createAdminRole(t, rbacRepo)
				assignRoleToUser(t, rbacRepo, 1, adminRole)
			},
			wantError:      "server not found",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:     "soft_deleted_server_is_not_returned",
			user:     &domain.User{ID: 1},
			serverID: 600,
			setup: func(t *testing.T, repo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				t.Helper()
				saveServer(t, repo, newServer(600, "deleted"))
				err := repo.SoftDelete(context.Background(), 600)
				require.NoError(t, err)
				adminRole := createAdminRole(t, rbacRepo)
				assignRoleToUser(t, rbacRepo, 1, adminRole)
			},
			wantError:      "server not found",
			wantStatusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rbacService, rbacRepo := setupRBAC(t)
			defer rbacService.Close()

			serverRepo := setupServerRepo(t)

			if tt.setup != nil {
				tt.setup(t, serverRepo, rbacRepo)
			}

			finder := serversbase.NewServerFinder(serverRepo, rbacService)
			server, err := finder.FindUserServer(context.Background(), tt.user, tt.serverID)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, server, "server must be nil when error is returned")

				if tt.wantStatusCode != 0 {
					type httpStatusError interface {
						HTTPStatus() int
					}
					httpErr, ok := err.(httpStatusError)
					require.True(t, ok, "error should expose HTTPStatus")
					assert.Equal(t, tt.wantStatusCode, httpErr.HTTPStatus(), "HTTP status code mismatch")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, server, "server must be returned on success")
			assert.Equal(t, tt.wantServerID, server.ID, "server ID must match the requested one")
			assert.Equal(t, tt.wantServerName, server.Name, "server name must be persisted unchanged")
		})
	}
}

func TestServerFinder_FindUserServer_RepoError(t *testing.T) {
	t.Parallel()
	rbacService, _ := setupRBAC(t)
	defer rbacService.Close()

	repo := &errServerRepo{findErr: errors.New("db boom")}
	finder := serversbase.NewServerFinder(repo, rbacService)

	server, err := finder.FindUserServer(context.Background(), &domain.User{ID: 7}, 1)

	require.Error(t, err)
	assert.Nil(t, server, "server must be nil when repo errored")
	assert.Contains(t, err.Error(), "db boom", "underlying repo error must be wrapped, not swallowed")
	assert.Contains(t, err.Error(), "failed to find server", "wrapper must mention the failed operation")
}

func TestServerFinder_FindUserServer_RBACError(t *testing.T) {
	t.Parallel()
	repo := setupServerRepo(t)
	finder := serversbase.NewServerFinder(repo, &errRBAC{canErr: errors.New("rbac boom")})

	server, err := finder.FindUserServer(context.Background(), &domain.User{ID: 1}, 1)

	require.Error(t, err)
	assert.Nil(t, server, "server must be nil when RBAC errored")
	assert.Contains(t, err.Error(), "rbac boom", "underlying RBAC error must be wrapped, not swallowed")
	assert.Contains(t, err.Error(), "failed to check admin permissions", "wrapper must mention admin check failure")
}
