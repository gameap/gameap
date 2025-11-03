package postconsole

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
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

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

type mockFileService struct {
	uploadFunc func(ctx context.Context, node *domain.Node, filePath string, content []byte, perms os.FileMode) error
}

func (m *mockFileService) Upload(
	ctx context.Context,
	node *domain.Node,
	filePath string,
	content []byte,
	perms os.FileMode,
) error {
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, node, filePath, content, perms)
	}

	return nil
}

type mockDaemonCommands struct {
	executeCommandFunc func(
		ctx context.Context,
		node *domain.Node,
		command string,
		opts ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error)
}

func (m *mockDaemonCommands) ExecuteCommand(
	ctx context.Context,
	node *domain.Node,
	command string,
	opts ...daemon.CommandServiceOption,
) (*daemon.CommandResult, error) {
	if m.executeCommandFunc != nil {
		return m.executeCommandFunc(ctx, node, command, opts...)
	}

	return &daemon.CommandResult{Output: ""}, nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name            string
		serverID        string
		requestBody     any
		setupAuth       func() context.Context
		setupRepo       func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupMockFS     func() *mockFileService
		setupMockDaemon func() *mockDaemonCommands
		expectedStatus  int
		wantError       string
		expectSuccess   bool
		validateMessage func(t *testing.T, message string)
	}{
		{
			name:     "successful command send via script execution",
			serverID: "1",
			requestBody: map[string]string{
				"command": "say Hello World",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				scriptSendCommand := "screen -S server_{id} -p 0 -X stuff \"{command}^M\""
				node := &domain.Node{
					ID:                1,
					Enabled:           true,
					Name:              "test-node",
					OS:                "linux",
					WorkPath:          "/srv/gameap",
					GdaemonHost:       "172.18.0.5",
					GdaemonPort:       31717,
					GdaemonAPIKey:     "test-key",
					ScriptSendCommand: &scriptSendCommand,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{
					executeCommandFunc: func(_ context.Context, _ *domain.Node, command string, _ ...daemon.CommandServiceOption) (*daemon.CommandResult, error) {
						assert.Contains(t, command, "say Hello World")

						return &daemon.CommandResult{
							Output:   "Command executed",
							ExitCode: 0,
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:     "successful command send via file upload",
			serverID: "1",
			requestBody: map[string]string{
				"command": "status",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				node := &domain.Node{
					ID:            1,
					Enabled:       true,
					Name:          "test-node",
					OS:            "linux",
					WorkPath:      "/srv/gameap",
					GdaemonHost:   "172.18.0.5",
					GdaemonPort:   31717,
					GdaemonAPIKey: "test-key",
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{
					uploadFunc: func(_ context.Context, _ *domain.Node, filePath string, content []byte, perms os.FileMode) error {
						assert.Equal(t, "/home/gameap/servers/test1/input.txt", filePath)
						assert.Equal(t, []byte("status"), content)
						assert.Equal(t, os.FileMode(0644), perms)

						return nil
					},
				}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:     "script execution fails",
			serverID: "1",
			requestBody: map[string]string{
				"command": "stop",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				scriptSendCommand := "send-command {command}"
				node := &domain.Node{
					ID:                1,
					Enabled:           true,
					Name:              "test-node",
					OS:                "linux",
					WorkPath:          "/srv/gameap",
					GdaemonHost:       "172.18.0.5",
					GdaemonPort:       31717,
					ScriptSendCommand: &scriptSendCommand,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:        1,
					UUID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short1",
					Enabled:   true,
					Installed: 1,
					Name:      "Test Server 1",
					GameID:    "cs",
					DSID:      1,
					GameModID: 1,
					ServerIP:  "127.0.0.1",
					Dir:       "/home/gameap/servers/test1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{
					executeCommandFunc: func(_ context.Context, _ *domain.Node, _ string, _ ...daemon.CommandServiceOption) (*daemon.CommandResult, error) {
						return nil, errors.New("failed to execute command")
					},
				}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectSuccess:  false,
		},
		{
			name:     "file upload fails",
			serverID: "1",
			requestBody: map[string]string{
				"command": "help",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				node := &domain.Node{
					ID:          1,
					Enabled:     true,
					Name:        "test-node",
					OS:          "linux",
					WorkPath:    "/srv/gameap",
					GdaemonHost: "172.18.0.5",
					GdaemonPort: 31717,
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:        1,
					UUID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short1",
					Enabled:   true,
					Installed: 1,
					Name:      "Test Server 1",
					GameID:    "cs",
					DSID:      1,
					GameModID: 1,
					ServerIP:  "127.0.0.1",
					Dir:       "/home/gameap/servers/test1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{
					uploadFunc: func(_ context.Context, _ *domain.Node, _ string, _ []byte, _ os.FileMode) error {
						return errors.New("connection refused")
					},
				}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectSuccess:  false,
		},
		{
			name:     "server not found",
			serverID: "999",
			requestBody: map[string]string{
				"command": "test",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectSuccess:  false,
		},
		{
			name:     "user not authenticated",
			serverID: "1",
			requestBody: map[string]string{
				"command": "test",
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectSuccess:  false,
		},
		{
			name:     "invalid server id",
			serverID: "invalid",
			requestBody: map[string]string{
				"command": "test",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectSuccess:  false,
		},
		{
			name:     "user does not have permission to send console commands",
			serverID: "1",
			requestBody: map[string]string{
				"command": "test",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				node := &domain.Node{
					ID:          1,
					Enabled:     true,
					Name:        "test-node",
					OS:          "linux",
					WorkPath:    "/srv/gameap",
					GdaemonHost: "172.18.0.5",
					GdaemonPort: 31717,
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:        1,
					UUID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short1",
					Enabled:   true,
					Installed: 1,
					Name:      "Test Server 1",
					GameID:    "cs",
					DSID:      1,
					GameModID: 1,
					ServerIP:  "127.0.0.1",
					Dir:       "/home/gameap/servers/test1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have permission to send console commands",
			expectSuccess:  false,
		},
		{
			name:     "empty command",
			serverID: "1",
			requestBody: map[string]string{
				"command": "",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				node := &domain.Node{
					ID:          1,
					Enabled:     true,
					Name:        "test-node",
					OS:          "linux",
					WorkPath:    "/srv/gameap",
					GdaemonHost: "172.18.0.5",
					GdaemonPort: 31717,
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:        1,
					UUID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short1",
					Enabled:   true,
					Installed: 1,
					Name:      "Test Server 1",
					GameID:    "cs",
					DSID:      1,
					GameModID: 1,
					ServerIP:  "127.0.0.1",
					Dir:       "/home/gameap/servers/test1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "command is required",
			expectSuccess:  false,
		},
		{
			name:        "invalid JSON body",
			serverID:    "1",
			requestBody: "invalid json",
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				node := &domain.Node{
					ID:          1,
					Enabled:     true,
					Name:        "test-node",
					OS:          "linux",
					WorkPath:    "/srv/gameap",
					GdaemonHost: "172.18.0.5",
					GdaemonPort: 31717,
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				require.NoError(t, nodeRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:        1,
					UUID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short1",
					Enabled:   true,
					Installed: 1,
					Name:      "Test Server 1",
					GameID:    "cs",
					DSID:      1,
					GameModID: 1,
					ServerIP:  "127.0.0.1",
					Dir:       "/home/gameap/servers/test1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "failed to parse request body",
			expectSuccess:  false,
		},
		{
			name:     "node not found",
			serverID: "1",
			requestBody: map[string]string{
				"command": "test",
			},
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				})
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				server := &domain.Server{
					ID:        1,
					UUID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short1",
					Enabled:   true,
					Installed: 1,
					Name:      "Test Server 1",
					GameID:    "cs",
					DSID:      999,
					GameModID: 1,
					ServerIP:  "127.0.0.1",
					Dir:       "/home/gameap/servers/test1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{
						{
							Name:       domain.AbilityNameGameServerConsoleSend,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			mockFS := tt.setupMockFS()
			mockDaemon := tt.setupMockDaemon()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, mockDaemon, mockFS, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, nodeRepo, rbacRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			var body []byte
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				var err error
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/servers/"+tt.serverID+"/console", bytes.NewReader(body))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			req.Header.Set("Content-Type", "application/json")
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

			if tt.expectSuccess {
				var resp consoleResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, "success", resp.Message)

				if tt.validateMessage != nil {
					tt.validateMessage(t, resp.Message)
				}
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	mockFS := &mockFileService{}
	mockDaemon := &mockDaemonCommands{}
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, nodeRepo, rbacService, mockDaemon, mockFS, responder)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.serverFinder)
	assert.Equal(t, nodeRepo, handler.nodeRepo)
	assert.Equal(t, rbacService, handler.rbac)
	assert.Equal(t, mockDaemon, handler.daemonCommands)
	assert.Equal(t, mockFS, handler.fileService)
	assert.Equal(t, responder, handler.responder)
}

func TestNewConsoleResponse(t *testing.T) {
	response := newConsoleResponse()

	assert.Equal(t, "success", response.Message)
}

func TestConsoleInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     consoleInput
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid command",
			input: consoleInput{
				Command: "say Hello",
			},
			wantError: false,
		},
		{
			name: "empty command",
			input: consoleInput{
				Command: "",
			},
			wantError: true,
			errorMsg:  "command is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.validate()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
