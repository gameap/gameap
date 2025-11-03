package tree

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	WorkPath:            "/var/gameap",
	GdaemonServerCert:   "test-cert",
	ClientCertificateID: 1,
}

type mockFileService struct {
	readDirFunc func(ctx context.Context, node *domain.Node, directory string) ([]*daemon.FileInfo, error)
}

func (m *mockFileService) ReadDir(
	ctx context.Context,
	node *domain.Node,
	directory string,
) ([]*daemon.FileInfo, error) {
	if m.readDirFunc != nil {
		return m.readDirFunc(ctx, node, directory)
	}

	return []*daemon.FileInfo{}, nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		serverID         string
		disk             string
		path             string
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupFileService func() *mockFileService
		expectedStatus   int
		wantError        string
		validateResponse func(*testing.T, []byte)
	}{
		{
			name:     "successful_tree_retrieval_root_directory",
			serverID: "1",
			disk:     "server",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
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

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, directory string) ([]*daemon.FileInfo, error) {
						if directory == "/home/gameap/servers/test1" {
							return []*daemon.FileInfo{
								{
									Name:         "logs",
									Size:         0,
									TimeModified: 1761384055,
									Type:         daemon.FileTypeDir,
									Perm:         0o755,
								},
								{
									Name:         "config",
									Size:         0,
									TimeModified: 1761384000,
									Type:         daemon.FileTypeDir,
									Perm:         0o755,
								},
								{
									Name:         "eula.txt",
									Size:         10,
									TimeModified: 1759648383,
									Type:         daemon.FileTypeFile,
									Perm:         0o644,
								},
							}, nil
						}

						if directory == "/home/gameap/servers/test1/logs" {
							return []*daemon.FileInfo{
								{
									Name:         "latest.log",
									Size:         1024,
									TimeModified: 1759648400,
									Type:         daemon.FileTypeFile,
									Perm:         0o644,
								},
							}, nil
						}

						if directory == "/home/gameap/servers/test1/config" {
							return []*daemon.FileInfo{
								{
									Name:         "subdir",
									Size:         0,
									TimeModified: 1761384000,
									Type:         daemon.FileTypeDir,
									Perm:         0o755,
								},
							}, nil
						}

						return []*daemon.FileInfo{}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response treeResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				assert.Nil(t, response.Result.Message)
				require.Len(t, response.Directories, 2)

				assert.Equal(t, "logs", response.Directories[0].Path)
				assert.Equal(t, "dir", response.Directories[0].Type)
				assert.Equal(t, uint64(1761384055), response.Directories[0].Timestamp)
				assert.Equal(t, "", response.Directories[0].Dirname)
				assert.Equal(t, "logs", response.Directories[0].Basename)
				assert.False(t, response.Directories[0].Props.HasSubdirectories)

				assert.Equal(t, "config", response.Directories[1].Path)
				assert.Equal(t, "dir", response.Directories[1].Type)
				assert.Equal(t, uint64(1761384000), response.Directories[1].Timestamp)
				assert.Equal(t, "", response.Directories[1].Dirname)
				assert.Equal(t, "config", response.Directories[1].Basename)
				assert.True(t, response.Directories[1].Props.HasSubdirectories)
			},
		},
		{
			name:     "successful_tree_retrieval_with_subdirectory",
			serverID: "1",
			disk:     "server",
			path:     "data",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
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

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, directory string) ([]*daemon.FileInfo, error) {
						if directory == "/home/gameap/servers/test1/data" {
							return []*daemon.FileInfo{
								{
									Name:         "subfolder",
									Size:         0,
									TimeModified: 1761495180,
									Type:         daemon.FileTypeDir,
									Perm:         0o755,
								},
							}, nil
						}

						if directory == "/home/gameap/servers/test1/data/subfolder" {
							return []*daemon.FileInfo{
								{
									Name:         "nested",
									Size:         0,
									TimeModified: 1761495200,
									Type:         daemon.FileTypeDir,
									Perm:         0o755,
								},
							}, nil
						}

						return []*daemon.FileInfo{}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response treeResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				require.Len(t, response.Directories, 1)
				assert.Equal(t, "subfolder", response.Directories[0].Path)
				assert.Equal(t, "subfolder", response.Directories[0].Basename)
				assert.True(t, response.Directories[0].Props.HasSubdirectories)
			},
		},
		{
			name:     "empty_directory_tree",
			serverID: "1",
			disk:     "",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
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

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
						return []*daemon.FileInfo{
							{
								Name:         "file.txt",
								Size:         512,
								TimeModified: 1759648383,
								Type:         daemon.FileTypeFile,
								Perm:         0o644,
							},
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response treeResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
				require.Len(t, response.Directories, 0)
			},
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
			disk:     "local",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
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
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:             "user_not_authenticated",
			serverID:         "1",
			disk:             "",
			path:             "",
			setupAuth:        context.Background,
			setupRepo:        func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {},
			setupFileService: func() *mockFileService { return &mockFileService{} },
			expectedStatus:   http.StatusUnauthorized,
			wantError:        "user not authenticated",
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
			disk:     "",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:     "server_not_found",
			serverID: "999",
			disk:     "",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "user_does_not_have_access_to_server",
			serverID: "2",
			disk:     "",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Other User Server",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "/home/gameap/servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 2)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "admin_can_access_any_server",
			serverID: "2",
			disk:     "",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Server 2",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "/home/gameap/servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 2)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
						return []*daemon.FileInfo{}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response treeResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "success", response.Result.Status)
			},
		},
		{
			name:     "invalid_path_with_directory_traversal",
			serverID: "1",
			disk:     "",
			path:     "../../../etc/passwd",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
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
			name:     "node_not_found",
			serverID: "1",
			disk:     "",
			path:     "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          999,
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
			},
			setupFileService: func() *mockFileService {
				return &mockFileService{}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
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

			baseURL := "/api/file-manager/" + tt.serverID + "/tree"
			query := url.Values{}
			if tt.disk != "" {
				query.Add("disk", tt.disk)
			}
			if tt.path != "" {
				query.Add("path", tt.path)
			}
			fullURL := baseURL
			if len(query) > 0 {
				fullURL += "?" + query.Encode()
			}

			req := httptest.NewRequest(http.MethodGet, fullURL, nil)
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

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid_relative_path",
			path:    "logs/latest.log",
			wantErr: false,
		},
		{
			name:    "valid_single_directory",
			path:    "logs",
			wantErr: false,
		},
		{
			name:    "valid_root",
			path:    ".",
			wantErr: false,
		},
		{
			name:    "invalid_directory_traversal_with_dots",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "invalid_path_with_double_dots",
			path:    "logs/../../etc",
			wantErr: true,
		},
		{
			name:    "invalid_just_double_dots",
			path:    "..",
			wantErr: true,
		},
		{
			name:    "invalid_double_dots_at_start",
			path:    "../logs",
			wantErr: true,
		},
		{
			name:    "invalid_double_dots_in_middle",
			path:    "logs/../../../etc",
			wantErr: true,
		},
		{
			name:    "invalid_hidden_traversal",
			path:    "logs/./../../etc",
			wantErr: true,
		},
		{
			name:    "valid_path_with_dots_in_filename",
			path:    "config/server.properties",
			wantErr: false,
		},
		{
			name:    "valid_empty_path",
			path:    "",
			wantErr: false,
		},
		{
			name:    "valid_nested_path",
			path:    "servers/cs/logs/latest.log",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractDirname(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantDirname string
	}{
		{
			name:        "simple_filename",
			path:        "file.txt",
			wantDirname: "",
		},
		{
			name:        "path_with_directory",
			path:        "logs/file.txt",
			wantDirname: "logs",
		},
		{
			name:        "nested_path",
			path:        "data/logs/file.txt",
			wantDirname: "data/logs",
		},
		{
			name:        "directory_only",
			path:        "logs",
			wantDirname: "",
		},
		{
			name:        "root_path",
			path:        ".",
			wantDirname: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirname := extractDirname(tt.path)
			assert.Equal(t, tt.wantDirname, dirname)
		})
	}
}
