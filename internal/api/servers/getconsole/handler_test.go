package getconsole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	downloadFunc func(ctx context.Context, node *domain.Node, filePath string) ([]byte, error)
}

func (m *mockFileService) Download(
	ctx context.Context,
	node *domain.Node,
	filePath string,
) ([]byte, error) {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, node, filePath)
	}

	return []byte{}, nil
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
		name                 string
		serverID             string
		setupAuth            func() context.Context
		setupRepo            func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupMockFS          func() *mockFileService
		setupMockDaemon      func() *mockDaemonCommands
		expectedStatus       int
		wantError            string
		expectConsole        bool
		validateConsoleValue func(t *testing.T, console string)
	}{
		{
			name:     "successful console retrieval via file download",
			serverID: "1",
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
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-key",
					ClientCertificateID: 1,
					CreatedAt:           &now,
					UpdatedAt:           &now,
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
							Name:       domain.AbilityNameGameServerConsoleView,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{
					downloadFunc: func(_ context.Context, _ *domain.Node, filePath string) ([]byte, error) {
						assert.Equal(t, "/home/gameap/servers/test1/output.txt", filePath)

						return []byte("Server is starting...\nServer is online\n"), nil
					},
				}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusOK,
			expectConsole:  true,
		},
		{
			name:     "successful console retrieval via script execution",
			serverID: "1",
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
				scriptGetConsole := "screen -S {{screen_name}} -Q select . && screen -S {{screen_name}} -p 0 -X hardcopy /tmp/console.txt && cat /tmp/console.txt"
				node := &domain.Node{
					ID:               1,
					Enabled:          true,
					Name:             "test-node",
					OS:               "linux",
					WorkPath:         "/srv/gameap",
					GdaemonHost:      "172.18.0.5",
					GdaemonPort:      31717,
					GdaemonAPIKey:    "test-key",
					ScriptGetConsole: &scriptGetConsole,
					CreatedAt:        &now,
					UpdatedAt:        &now,
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
							Name:       domain.AbilityNameGameServerConsoleView,
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
					executeCommandFunc: func(
						_ context.Context,
						_ *domain.Node,
						_ string,
						_ ...daemon.CommandServiceOption,
					) (*daemon.CommandResult, error) {
						return &daemon.CommandResult{
							Output: "Console output from script\nServer running\n",
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			expectConsole:  true,
			validateConsoleValue: func(t *testing.T, console string) {
				t.Helper()

				assert.Contains(t, console, "Console output from script")
			},
		},
		{
			name:     "script execution fails",
			serverID: "1",
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
				scriptGetConsole := "cat /tmp/console.txt"
				node := &domain.Node{
					ID:               1,
					Enabled:          true,
					Name:             "test-node",
					OS:               "linux",
					WorkPath:         "/srv/gameap",
					GdaemonHost:      "172.18.0.5",
					GdaemonPort:      31717,
					ScriptGetConsole: &scriptGetConsole,
					CreatedAt:        &now,
					UpdatedAt:        &now,
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
							Name:       domain.AbilityNameGameServerConsoleView,
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
			expectConsole:  false,
		},
		{
			name:     "server not found",
			serverID: "999",
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
			expectConsole:  false,
		},
		{
			name:     "user not authenticated",
			serverID: "1",
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
			expectConsole:  false,
		},
		{
			name:     "invalid server id",
			serverID: "invalid",
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
			expectConsole:  false,
		},
		{
			name:     "user does not have permission to view console",
			serverID: "1",
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
			wantError:      "user does not have permission to view console",
			expectConsole:  false,
		},
		{
			name:     "console output with invalid UTF-8 is sanitized",
			serverID: "1",
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
							Name:       domain.AbilityNameGameServerConsoleView,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{
					downloadFunc: func(_ context.Context, _ *domain.Node, _ string) ([]byte, error) {
						// Create invalid UTF-8 bytes
						invalidUTF8 := []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f, 0xff, 0xfe, 0xfd}

						return invalidUTF8, nil
					},
				}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusOK,
			expectConsole:  true,
			validateConsoleValue: func(t *testing.T, console string) {
				t.Helper()

				// Ensure the output is valid UTF-8
				assert.True(t, strings.Contains(console, "Hello"))
			},
		},
		{
			name:     "console log truncates long output",
			serverID: "1",
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
							Name:       domain.AbilityNameGameServerConsoleView,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{
					downloadFunc: func(_ context.Context, _ *domain.Node, _ string) ([]byte, error) {
						longOutput := strings.Repeat("a", consoleMaxSymbols+1000)

						return []byte(longOutput), nil
					},
				}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusOK,
			expectConsole:  true,
			validateConsoleValue: func(t *testing.T, console string) {
				t.Helper()

				assert.LessOrEqual(t, len(console), consoleMaxSymbols)
				assert.Equal(t, consoleMaxSymbols, len(console))
			},
		},
		{
			name:     "node not found",
			serverID: "1",
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
					DSID:      999, // Non-existent node
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
							Name:       domain.AbilityNameGameServerConsoleView,
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
			expectConsole:  false,
		},
		{
			name:     "file download error",
			serverID: "1",
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
							Name:       domain.AbilityNameGameServerConsoleView,
							EntityID:   lo.ToPtr(uint(1)),
							EntityType: lo.ToPtr(domain.EntityTypeServer),
						},
					},
				))
			},
			setupMockFS: func() *mockFileService {
				return &mockFileService{
					downloadFunc: func(_ context.Context, _ *domain.Node, _ string) ([]byte, error) {
						return nil, errors.New("connection refused")
					},
				}
			},
			setupMockDaemon: func() *mockDaemonCommands {
				return &mockDaemonCommands{}
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectConsole:  false,
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

			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/console", nil)
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

			if tt.expectConsole {
				var resp consoleResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.Console)

				if tt.validateConsoleValue != nil {
					tt.validateConsoleValue(t, resp.Console)
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

func TestHandler_SanitizeUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid utf8",
			input:    "Hello World!",
			expected: "Hello World!",
		},
		{
			name:     "valid utf8 with unicode",
			input:    "Hello 世界!",
			expected: "Hello 世界!",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "string with emojis",
			input:    "Server starting 🚀",
			expected: "Server starting 🚀",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeUTF8(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewConsoleResponse(t *testing.T) {
	consoleOutput := "Server starting...\nServer online\n"
	response := newConsoleResponse(consoleOutput)

	assert.Equal(t, consoleOutput, response.Console)
}
