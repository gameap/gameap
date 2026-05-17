package chmod

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errDaemonPermissionDenied = errors.New("permission denied on daemon")
	errDaemonDiskFull         = errors.New("disk full")
)

//nolint:unparam
func allowUserFilesAbility(t *testing.T, rbacRepo *inmemory.RBACRepository, userID, serverID uint) {
	t.Helper()

	ability := &domain.Ability{
		Name:       domain.AbilityNameGameServerFiles,
		EntityType: lo.ToPtr(domain.EntityTypeServer),
		EntityID:   new(serverID),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(userID),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}
	require.NoError(t, rbacRepo.SavePermission(context.Background(), permission))
}

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

var testUser2 = domain.User{
	ID:    2,
	Login: "admin",
	Email: "admin@example.com",
}

var testNode = domain.Node{
	ID:                  1,
	Enabled:             true,
	Name:                "Test Node",
	OS:                  "linux",
	Location:            "Test Location",
	GdaemonHost:         "127.0.0.1",
	GdaemonPort:         31717,
	GdaemonAPIKey:       "test-key",
	WorkPath:            "/srv/gameap",
	GdaemonServerCert:   "test-cert",
	ClientCertificateID: 1,
}

type mockFileService struct {
	chmodFunc func(ctx context.Context, node *domain.Node, path string, perm uint32) error
}

func (m *mockFileService) Chmod(
	ctx context.Context,
	node *domain.Node,
	path string,
	perm uint32,
) error {
	if m.chmodFunc != nil {
		return m.chmodFunc(ctx, node, path, perm)
	}

	return nil
}

//nolint:unparam
func newTestServer(dsid uint, dir string) *domain.Server {
	now := time.Now()

	return &domain.Server{
		ID:            1,
		UID:           uuid.New(),
		UUIDShort:     "short",
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "cs",
		DSID:          dsid,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           dir,
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
}

func authenticatedSession(user *domain.User) context.Context {
	session := &auth.Session{
		Login: user.Login,
		Email: user.Email,
		User:  user,
	}

	return auth.ContextWithSession(context.Background(), session)
}

//nolint:maintidx
func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		serverID         string
		requestBody      any
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupFileService func() *mockFileService
		expectedStatus   int
		wantError        string
		validateResponse func(*testing.T, []byte)
	}{
		{
			name:     "successful_file_chmod_0644",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "config.cfg"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, path string, perm uint32) error {
						assert.Equal(t, "/srv/gameap/servers/test1/config.cfg", path)
						assert.Equal(t, uint32(0o644), perm)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response chmodResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Equal(t, "Permissions changed!", response.Result.Message)
			},
		},
		{
			name:     "successful_directory_chmod_0755",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o755,
				Items: []chmodItem{
					{Path: "scripts"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, path string, perm uint32) error {
						assert.Equal(t, "/srv/gameap/servers/test1/scripts", path)
						assert.Equal(t, uint32(0o755), perm)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "batch_chmod_multiple_items",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o600,
				Items: []chmodItem{
					{Path: "secret1.key"},
					{Path: "secret2.key"},
					{Path: "config/private.cfg"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				callCount := 0
				expectedPaths := []string{
					"/srv/gameap/servers/test1/secret1.key",
					"/srv/gameap/servers/test1/secret2.key",
					"/srv/gameap/servers/test1/config/private.cfg",
				}

				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, path string, perm uint32) error {
						require.Less(t, callCount, len(expectedPaths), "more calls than expected")
						assert.Equal(t, expectedPaths[callCount], path)
						assert.Equal(t, uint32(0o600), perm)
						callCount++

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response chmodResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "successful_chmod_zero_mode",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0,
				Items: []chmodItem{
					{Path: "locked.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, _ string, perm uint32) error {
						assert.Equal(t, uint32(0), perm)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "successful_chmod_max_mode_0777",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o777,
				Items: []chmodItem{
					{Path: "shared.bin"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, _ string, perm uint32) error {
						assert.Equal(t, uint32(0o777), perm)

						return nil
					},
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "invalid_mode_negative",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: -1,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid mode",
		},
		{
			name:     "invalid_mode_above_0o777",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o1000,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid mode",
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "local",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:     "empty_items_array",
			serverID: "1",
			requestBody: chmodRequest{
				Disk:  "server",
				Mode:  0o644,
				Items: []chmodItem{},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "items array is empty",
		},
		{
			name:     "user_not_authenticated",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:     "server_not_found",
			serverID: "999",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "node_not_found",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(999, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:        "invalid_request_body",
			serverID:    "1",
			requestBody: "invalid json",
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request body",
		},
		{
			name:     "path_traversal_in_items",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o777,
				Items: []chmodItem{
					{Path: "../../../etc/passwd"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "daemon_chmod_error",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, _ string, _ uint32) error {
						return errDaemonPermissionDenied
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
		{
			name:     "batch_aborts_on_first_error",
			serverID: "1",
			requestBody: chmodRequest{
				Disk: "server",
				Mode: 0o644,
				Items: []chmodItem{
					{Path: "first.txt"},
					{Path: "second.txt"},
					{Path: "third.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				callCount := 0

				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, _ string, _ uint32) error {
						callCount++
						if callCount == 2 {
							return errDaemonDiskFull
						}

						require.LessOrEqual(t, callCount, 2, "should stop after error on second item")

						return nil
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
		{
			name:     "user_without_files_permission",
			serverID: "1",
			requestBody: map[string]any{
				"disk": "server",
				"mode": 0o644,
				"items": []map[string]any{
					{"path": "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser1)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:     "admin_bypasses_files_permission",
			serverID: "1",
			requestBody: map[string]any{
				"disk": "server",
				"mode": 0o644,
				"items": []map[string]any{
					{"path": "test.txt"},
				},
			},
			setupAuth: func() context.Context {
				return authenticatedSession(&testUser2)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				server := newTestServer(1, "servers/test1")
				require.NoError(t, serverRepo.Save(context.Background(), server))

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))

				adminAbility := &domain.Ability{
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response chmodResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			fileService := tt.setupFileService()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, fileService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, nodeRepo, rbacRepo)
			}

			ctx := tt.setupAuth()

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/"+tt.serverID+"/chmod", bytes.NewReader(body))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.validateResponse != nil {
				tt.validateResponse(t, w.Body.Bytes())
			}
		})
	}
}
