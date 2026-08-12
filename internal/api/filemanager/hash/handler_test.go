package hash

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
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

type mockFileHasher struct {
	hashFunc func(
		ctx context.Context, node *domain.Node, paths []string, algorithm proto.HashAlgorithm,
	) (*proto.HashResult, error)
}

func (m *mockFileHasher) Hash(
	ctx context.Context, node *domain.Node, paths []string, algorithm proto.HashAlgorithm,
) (*proto.HashResult, error) {
	if m.hashFunc != nil {
		return m.hashFunc(ctx, node, paths, algorithm)
	}

	return &proto.HashResult{Algorithm: algorithm}, nil
}

func newTestServer(dsid uint, dir string) *domain.Server {
	now := time.Now()

	return &domain.Server{
		ID:            1,
		UID:           uuid.New(),
		UUIDShort:     "short",
		Enabled:       true,
		Installed:     1,
		Name:          "Test Server",
		GameID:        "cs",
		DSID:          dsid,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           dir,
		CreatedAt:     &now,
		UpdatedAt:     &now,
		ProcessActive: false,
	}
}

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

func authenticatedSession(user *domain.User) context.Context {
	session := &auth.Session{
		Login: user.Login,
		Email: user.Email,
		User:  user,
	}

	return auth.ContextWithSession(context.Background(), session)
}

func setupRepo(
	t *testing.T,
	serverRepo *inmemory.ServerRepository,
	nodeRepo *inmemory.NodeRepository,
	rbacRepo *inmemory.RBACRepository,
) {
	t.Helper()

	server := newTestServer(1, "servers/test1")
	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)

	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))
}

func TestHandler_ServeHTTP(t *testing.T) {
	manyPaths := make([]string, maxHashPaths+1)
	for i := range manyPaths {
		manyPaths[i] = "file.txt"
	}

	tests := []struct {
		name             string
		serverID         string
		requestBody      any
		setupAuth        func() context.Context
		setupRepo        func(*testing.T, *inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupHasher      func() *mockFileHasher
		expectedStatus   int
		wantError        string
		validateResponse func(*testing.T, []byte)
	}{
		{
			name:     "successful_hash_maps_results_back_to_request_paths",
			serverID: "1",
			requestBody: hashRequest{
				Disk:      "server",
				Paths:     []string{"maps/de_dust2.bsp", "missing.cfg"},
				Algorithm: "md5",
			},
			setupAuth: func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo: setupRepo,
			setupHasher: func() *mockFileHasher {
				return &mockFileHasher{
					hashFunc: func(
						_ context.Context, _ *domain.Node, paths []string, algorithm proto.HashAlgorithm,
					) (*proto.HashResult, error) {
						assert.Equal(t, []string{
							"/srv/gameap/servers/test1/maps/de_dust2.bsp",
							"/srv/gameap/servers/test1/missing.cfg",
						}, paths)
						assert.Equal(t, proto.HashAlgorithm_HASH_ALGORITHM_MD5, algorithm)

						return &proto.HashResult{
							Algorithm: algorithm,
							Hashes: []*proto.FileHash{
								{Path: "servers/test1/missing.cfg", Error: "no such file"},
								{Path: "servers/test1/maps/de_dust2.bsp", Hash: "abc123", Size: 42},
							},
						}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response hashResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "md5", response.Algorithm)
				require.Len(t, response.Items, 2)
				assert.Equal(t, "missing.cfg", response.Items[0].Path,
					"daemon-relative paths must map back to request paths")
				assert.Equal(t, "no such file", response.Items[0].Error)
				assert.Equal(t, "maps/de_dust2.bsp", response.Items[1].Path)
				assert.Equal(t, "abc123", response.Items[1].Hash)
				assert.Equal(t, uint64(42), response.Items[1].Size)
			},
		},
		{
			name:     "empty_algorithm_defaults_to_sha256",
			serverID: "1",
			requestBody: hashRequest{
				Disk:  "server",
				Paths: []string{"config.cfg"},
			},
			setupAuth: func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo: setupRepo,
			setupHasher: func() *mockFileHasher {
				return &mockFileHasher{
					hashFunc: func(
						_ context.Context, _ *domain.Node, _ []string, algorithm proto.HashAlgorithm,
					) (*proto.HashResult, error) {
						assert.Equal(t, proto.HashAlgorithm_HASH_ALGORITHM_SHA256, algorithm)

						return &proto.HashResult{Algorithm: algorithm}, nil
					},
				}
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response hashResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "sha256", response.Algorithm)
			},
		},
		{
			name:     "unsupported_disk",
			serverID: "1",
			requestBody: hashRequest{
				Disk:  "local",
				Paths: []string{"a.txt"},
			},
			setupAuth:      func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:      setupRepo,
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:     "empty_paths",
			serverID: "1",
			requestBody: hashRequest{
				Disk:  "server",
				Paths: []string{},
			},
			setupAuth:      func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:      setupRepo,
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "paths array is empty",
		},
		{
			name:     "too_many_paths",
			serverID: "1",
			requestBody: hashRequest{
				Disk:  "server",
				Paths: manyPaths,
			},
			setupAuth:      func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:      setupRepo,
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "too many paths",
		},
		{
			name:     "path_traversal_rejected",
			serverID: "1",
			requestBody: hashRequest{
				Disk:  "server",
				Paths: []string{"../../etc/passwd"},
			},
			setupAuth:      func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:      setupRepo,
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:     "root_path_rejected",
			serverID: "1",
			requestBody: hashRequest{
				Disk:  "server",
				Paths: []string{"/"},
			},
			setupAuth:      func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:      setupRepo,
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path refers to the server root",
		},
		{
			name:     "unknown_algorithm",
			serverID: "1",
			requestBody: hashRequest{
				Disk:      "server",
				Paths:     []string{"a.txt"},
				Algorithm: "sha3",
			},
			setupAuth:      func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:      setupRepo,
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unknown hash algorithm",
		},
		{
			name:        "user_not_authenticated",
			serverID:    "1",
			requestBody: hashRequest{Disk: "server", Paths: []string{"a.txt"}},
			//nolint:gocritic
			setupAuth: func() context.Context { return context.Background() },
			setupRepo: func(*testing.T, *inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository) {
			},
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:        "user_without_files_permission",
			serverID:    "1",
			requestBody: hashRequest{Disk: "server", Paths: []string{"a.txt"}},
			setupAuth:   func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo: func(
				t *testing.T,
				serverRepo *inmemory.ServerRepository,
				nodeRepo *inmemory.NodeRepository,
				_ *inmemory.RBACRepository,
			) {
				t.Helper()
				require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1, "servers/test1")))
				serverRepo.AddUserServer(1, 1)
				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:        "node_not_found",
			serverID:    "1",
			requestBody: hashRequest{Disk: "server", Paths: []string{"a.txt"}},
			setupAuth:   func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo: func(
				t *testing.T,
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.NodeRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				t.Helper()
				require.NoError(t, serverRepo.Save(context.Background(), newTestServer(999, "servers/test1")))
				serverRepo.AddUserServer(1, 1)
				allowUserFilesAbility(t, rbacRepo, 1, 1)
			},
			setupHasher:    func() *mockFileHasher { return &mockFileHasher{} },
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:        "daemon_not_connected_maps_to_bad_gateway",
			serverID:    "1",
			requestBody: hashRequest{Disk: "server", Paths: []string{"a.txt"}},
			setupAuth:   func() context.Context { return authenticatedSession(&testUser1) },
			setupRepo:   setupRepo,
			setupHasher: func() *mockFileHasher {
				return &mockFileHasher{
					hashFunc: func(
						context.Context, *domain.Node, []string, proto.HashAlgorithm,
					) (*proto.HashResult, error) {
						return nil, daemon.ErrDaemonNotConnected
					},
				}
			},
			expectedStatus: http.StatusBadGateway,
			wantError:      "Bad Gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			handler := NewHandler(serverRepo, nodeRepo, rbacService, tt.setupHasher(), api.NewResponder())

			tt.setupRepo(t, serverRepo, nodeRepo, rbacRepo)

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/"+tt.serverID+"/hash", bytes.NewReader(body))
			req = req.WithContext(tt.setupAuth())
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())

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

func TestDaemonRelPath(t *testing.T) {
	tests := []struct {
		name        string
		serverDir   string
		requestPath string
		want        string
	}{
		{name: "plain_join", serverDir: "servers/test1", requestPath: "maps/x.bsp", want: "servers/test1/maps/x.bsp"},
		{name: "leading_slash_trimmed", serverDir: "servers/test1", requestPath: "/cfg/a.cfg", want: "servers/test1/cfg/a.cfg"},
		{name: "empty_server_dir", serverDir: "", requestPath: "a.txt", want: "a.txt"},
		{name: "windows_separators", serverDir: "servers\\test1", requestPath: "cfg\\a.cfg", want: "servers/test1/cfg/a.cfg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, daemonRelPath(tt.serverDir, tt.requestPath))
		})
	}
}
