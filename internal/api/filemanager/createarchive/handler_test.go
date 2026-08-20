package createarchive

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAcceptedOpID = "op-1"

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

type mockArchiveStarter struct {
	startFunc func(ctx context.Context, node *domain.Node, p daemon.CreateArchiveParams) (string, error)
}

func (m *mockArchiveStarter) StartCreate(
	ctx context.Context, node *domain.Node, p daemon.CreateArchiveParams,
) (string, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, node, p)
	}

	return "op-test", nil
}

func newTestServer(dsid uint) *domain.Server {
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
		Dir:           "servers/test1",
		SuUser:        new("gameap"),
		CreatedAt:     &now,
		UpdatedAt:     &now,
		ProcessActive: false,
	}
}

func allowUserFilesAbility(t *testing.T, rbacRepo *inmemory.RBACRepository, userID, serverID uint) {
	t.Helper()

	ability := &domain.Ability{
		Name:       domain.AbilityNameGameServerFiles,
		EntityType: new(domain.EntityTypeServer),
		EntityID:   new(serverID),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(userID),
		EntityType: new(domain.EntityTypeUser),
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

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1)))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)

	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))
}

//nolint:maintidx
func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		requestBody      any
		setupStarter     func() *mockArchiveStarter
		expectedStatus   int
		wantError        string
		validateResponse func(*testing.T, []byte)
	}{
		{
			name: "successful_start_builds_params_and_returns_202",
			requestBody: createArchiveRequest{
				Disk:      "server",
				Path:      "backups",
				Name:      "world.tar.gz",
				Sources:   []string{"backups/maps", "backups/config.cfg"},
				Overwrite: true,
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, node *domain.Node, p daemon.CreateArchiveParams) (string, error) {
						assert.Equal(t, "/srv/gameap", node.WorkPath)
						assert.Equal(t, "/srv/gameap/servers/test1/backups/world.tar.gz", p.ArchivePath)
						assert.Equal(t, "/srv/gameap/servers/test1/backups", p.BasePath)
						assert.Equal(t, []string{
							"/srv/gameap/servers/test1/backups/maps",
							"/srv/gameap/servers/test1/backups/config.cfg",
						}, p.Sources)
						assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ, p.Format,
							"format must be inferred from the .tar.gz extension")
						assert.True(t, p.Overwrite)
						assert.Nil(t, p.CompressionLevel)
						assert.Equal(t, daemon.OwnerOptions{User: "gameap"}, p.Owner)
						assert.Equal(t, uint(1), p.Options.ServerID)
						assert.Equal(t, "user:1", p.Options.Initiator)

						return "op-42", nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response createArchiveResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "op-42", response.OperationID)
			},
		},
		{
			name: "explicit_format_overrides_extension",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Path:    "",
				Name:    "weird.name",
				Format:  "zip",
				Sources: []string{"maps"},
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, _ *domain.Node, p daemon.CreateArchiveParams) (string, error) {
						assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP, p.Format)
						assert.Equal(t, "/srv/gameap/servers/test1/weird.name", p.ArchivePath)
						assert.Equal(t, "/srv/gameap/servers/test1", p.BasePath)

						return testAcceptedOpID, nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "compression_level_is_passed_through",
			requestBody: createArchiveRequest{
				Disk:             "server",
				Name:             "logs.zip",
				Sources:          []string{"logs"},
				CompressionLevel: new(int32(9)),
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, _ *domain.Node, p daemon.CreateArchiveParams) (string, error) {
						require.NotNil(t, p.CompressionLevel)
						assert.Equal(t, int32(9), *p.CompressionLevel)

						return testAcceptedOpID, nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "tgz_extension_resolves_to_tar_gz",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "b.tgz",
				Sources: []string{"maps"},
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, _ *domain.Node, p daemon.CreateArchiveParams) (string, error) {
						assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ, p.Format)

						return testAcceptedOpID, nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "unsupported_disk",
			requestBody: createArchiveRequest{
				Disk:    "local",
				Name:    "a.zip",
				Sources: []string{"x"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name: "name_without_recognizable_extension",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "archive",
				Sources: []string{"x"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "cannot infer archive format",
		},
		{
			name: "extraction_only_format_rejected",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "a.7z",
				Format:  "7z",
				Sources: []string{"x"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "supports extraction only",
		},
		{
			name: "unknown_format_rejected",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "a.zip",
				Format:  "arj",
				Sources: []string{"x"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unknown archive format",
		},
		{
			name: "empty_sources",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "a.zip",
				Sources: []string{},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "sources array is empty",
		},
		{
			name: "single_file_format_with_multiple_sources",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "a.gz",
				Sources: []string{"one.log", "two.log"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "compresses a single file",
		},
		{
			name: "source_outside_base_path",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Path:    "backups",
				Name:    "a.zip",
				Sources: []string{"maps/de_dust2.bsp"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "outside the base path",
		},
		{
			name: "source_equal_to_base_path_rejected",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Path:    "backups",
				Name:    "a.zip",
				Sources: []string{"backups"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "outside the base path",
		},
		{
			name: "root_source_rejected",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "a.zip",
				Sources: []string{"/"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path refers to the server root",
		},
		{
			name: "traversal_in_name_rejected",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "../evil.zip",
				Sources: []string{"x"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "filename contains invalid directory traversal",
		},
		{
			name: "traversal_in_path_rejected",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Path:    "../outside",
				Name:    "a.zip",
				Sources: []string{"x"},
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name: "invalid_compression_level",
			requestBody: createArchiveRequest{
				Disk:             "server",
				Name:             "a.zip",
				Sources:          []string{"x"},
				CompressionLevel: new(int32(10)),
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid compression level",
		},
		{
			name: "archive_not_supported_maps_to_bad_gateway",
			requestBody: createArchiveRequest{
				Disk:    "server",
				Name:    "a.zip",
				Sources: []string{"x"},
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(context.Context, *domain.Node, daemon.CreateArchiveParams) (string, error) {
						return "", daemon.ErrArchiveNotSupported
					},
				}
			},
			expectedStatus: http.StatusBadGateway,
			wantError:      "Bad Gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			handler := NewHandler(serverRepo, nodeRepo, rbacService, tt.setupStarter(), api.NewResponder(), nil)

			setupRepo(t, serverRepo, nodeRepo, rbacRepo)

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/archive", bytes.NewReader(body))
			req = req.WithContext(authenticatedSession(&testUser1))
			req = mux.SetURLVars(req, map[string]string{"server": "1"})
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

func TestHandler_Audit_SuccessfulStartIsRecorded(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	setupRepo(t, serverRepo, nodeRepo, rbacRepo)

	recorder := &auditCapture{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, &mockArchiveStarter{}, api.NewResponder(), recorder)

	body, err := json.Marshal(createArchiveRequest{
		Disk:    "server",
		Name:    "maps.zip",
		Sources: []string{"maps"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/archive", bytes.NewReader(body))
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusAccepted, w.Code, "body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventFileArchiveCreate, events[0].Type)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	assert.Equal(t, audit.CategoryFileOp, events[0].Category)
	assert.Equal(t, "server", events[0].ResourceType)
	assert.Equal(t, "1", events[0].ResourceID)
	assert.Equal(t, "archive.create", events[0].Action)
	assert.Equal(t, testUser1.ID, events[0].ActorID)
}

func TestHandler_Audit_DeniedStartIsNotRecorded(t *testing.T) {
	t.Parallel()

	// ARRANGE: no files ability granted.
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1)))
	serverRepo.AddUserServer(1, 1)
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	recorder := &auditCapture{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, &mockArchiveStarter{}, api.NewResponder(), recorder)

	body, err := json.Marshal(createArchiveRequest{
		Disk:    "server",
		Name:    "maps.zip",
		Sources: []string{"maps"},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/archive", bytes.NewReader(body))
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, recorder.snapshot(), "a denied start must not leave an audit event")
}
