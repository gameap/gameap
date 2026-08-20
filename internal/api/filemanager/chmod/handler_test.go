package chmod

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
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

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

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
	t.Parallel()

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
			t.Parallel()

			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			fileService := tt.setupFileService()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, fileService, responder, nil)

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

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — a permission change (chmod) that
//     succeeds without being recorded is a detective-control gap (OWASP ASVS
//     §7.2.1). The event must be scoped to the exact server and attributed to
//     the acting principal.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestHandler_Audit_SuccessfulChmodIsRecorded covers OWASP API8:2023. A
// successful chmod must emit exactly one file.chmod event with outcome
// success, category file_op, the server as the scoped resource, and the
// acting user attributed as the actor.
func TestHandler_Audit_SuccessfulChmodIsRecorded(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1, "servers/test1")))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	recorder := &auditCapture{}
	handler := NewHandler(
		serverRepo, nodeRepo, rbacService, &mockFileService{}, api.NewResponder(), recorder,
	)

	body, err := json.Marshal(chmodRequest{
		Disk:  "server",
		Mode:  0o644,
		Items: []chmodItem{{Path: "config.cfg"}},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/chmod", bytes.NewReader(body))
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "chmod must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventFileChmod),
		"exactly one file-chmod event must be emitted per successful chmod")

	ev, ok := findEvent(events, audit.EventFileChmod)
	require.True(t, ok, "a successful chmod must leave a file.chmod audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryFileOp, ev.Category)
	assert.Equal(t, "server", ev.ResourceType, "a file op is scoped to its server")
	assert.Equal(t, "1", ev.ResourceID, "the targeted server id must be recorded")
	assert.Equal(t, "chmod", ev.Action)
	assert.Equal(t, testUser1.ID, ev.ActorID, "the acting user must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)
}

// TestHandler_Audit_DeniedChmodDoesNotEmitFileChmod covers OWASP API8:2023.
// A chmod refused by the per-server ability check must NOT emit a file.chmod
// success event.
func TestHandler_Audit_DeniedChmodDoesNotEmitFileChmod(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1, "servers/test1")))
	// No files ability granted for this user/server.
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	recorder := &auditCapture{}
	handler := NewHandler(
		serverRepo, nodeRepo, rbacService, &mockFileService{}, api.NewResponder(), recorder,
	)

	body, err := json.Marshal(chmodRequest{
		Disk:  "server",
		Mode:  0o644,
		Items: []chmodItem{{Path: "config.cfg"}},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/chmod", bytes.NewReader(body))
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.NotEqual(t, http.StatusOK, w.Code,
		"a user without the files ability must be denied; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventFileChmod),
		"a refused chmod must not be recorded as a successful file.chmod")
}
