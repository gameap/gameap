package extractarchive

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
	startFunc func(ctx context.Context, node *domain.Node, p daemon.ExtractArchiveParams) (string, error)
}

func (m *mockArchiveStarter) StartExtract(
	ctx context.Context, node *domain.Node, p daemon.ExtractArchiveParams,
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

func TestHandler_ServeHTTP(t *testing.T) {
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
			requestBody: extractArchiveRequest{
				Disk:           "server",
				Path:           "backups/world.tar.gz",
				Destination:    "restored",
				ConflictPolicy: "skip",
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, node *domain.Node, p daemon.ExtractArchiveParams) (string, error) {
						assert.Equal(t, "/srv/gameap", node.WorkPath)
						assert.Equal(t, "/srv/gameap/servers/test1/backups/world.tar.gz", p.ArchivePath)
						assert.Equal(t, "/srv/gameap/servers/test1/restored", p.Destination)
						assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED, p.Format,
							"omitted format lets the daemon auto-detect")
						assert.Equal(t, proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_SKIP, p.ConflictPolicy)
						assert.True(t, p.CreateDestination, "create_destination defaults to true")
						assert.True(t, p.PreservePermissions)
						assert.Equal(t, daemon.OwnerOptions{User: "gameap"}, p.Owner)
						assert.Equal(t, uint(1), p.Options.ServerID)
						assert.Equal(t, "user:1", p.Options.Initiator)

						return "op-77", nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
			validateResponse: func(t *testing.T, body []byte) {
				t.Helper()

				var response extractArchiveResponse
				require.NoError(t, json.Unmarshal(body, &response))
				assert.Equal(t, "op-77", response.OperationID)
			},
		},
		{
			name: "extract_into_server_root_is_allowed",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "mod.zip",
				Destination: "",
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, _ *domain.Node, p daemon.ExtractArchiveParams) (string, error) {
						assert.Equal(t, "/srv/gameap/servers/test1", p.Destination)
						assert.Equal(t, proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_ERROR, p.ConflictPolicy,
							"omitted policy defaults to error")

						return testAcceptedOpID, nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "explicit_create_destination_false_is_honored",
			requestBody: extractArchiveRequest{
				Disk:              "server",
				Path:              "mod.zip",
				Destination:       "mods",
				CreateDestination: new(false),
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, _ *domain.Node, p daemon.ExtractArchiveParams) (string, error) {
						assert.False(t, p.CreateDestination)

						return testAcceptedOpID, nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "explicit_format_is_resolved",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "data.bin",
				Destination: "out",
				Format:      "7z",
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(_ context.Context, _ *domain.Node, p daemon.ExtractArchiveParams) (string, error) {
						assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_7Z, p.Format,
							"extraction-only formats are legal here")

						return testAcceptedOpID, nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "unsupported_disk",
			requestBody: extractArchiveRequest{
				Disk:        "local",
				Path:        "a.zip",
				Destination: "out",
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name: "root_archive_path_rejected",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "/",
				Destination: "out",
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path refers to the server root",
		},
		{
			name: "traversal_in_destination_rejected",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "a.zip",
				Destination: "../outside",
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name: "traversal_in_archive_path_rejected",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "../outside/a.zip",
				Destination: "out",
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name: "unknown_format_rejected",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "a.zip",
				Destination: "out",
				Format:      "arj",
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unknown archive format",
		},
		{
			name: "unknown_conflict_policy_rejected",
			requestBody: extractArchiveRequest{
				Disk:           "server",
				Path:           "a.zip",
				Destination:    "out",
				ConflictPolicy: "merge",
			},
			setupStarter:   func() *mockArchiveStarter { return &mockArchiveStarter{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unknown conflict policy",
		},
		{
			name: "daemon_not_connected_maps_to_bad_gateway",
			requestBody: extractArchiveRequest{
				Disk:        "server",
				Path:        "a.zip",
				Destination: "out",
			},
			setupStarter: func() *mockArchiveStarter {
				return &mockArchiveStarter{
					startFunc: func(context.Context, *domain.Node, daemon.ExtractArchiveParams) (string, error) {
						return "", daemon.ErrDaemonNotConnected
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
			handler := NewHandler(serverRepo, nodeRepo, rbacService, tt.setupStarter(), api.NewResponder(), nil)

			setupRepo(t, serverRepo, nodeRepo, rbacRepo)

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/extract", bytes.NewReader(body))
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
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	setupRepo(t, serverRepo, nodeRepo, rbacRepo)

	recorder := &auditCapture{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, &mockArchiveStarter{}, api.NewResponder(), recorder)

	body, err := json.Marshal(extractArchiveRequest{
		Disk:        "server",
		Path:        "mod.zip",
		Destination: "mods",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/extract", bytes.NewReader(body))
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusAccepted, w.Code, "body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventFileArchiveExtract, events[0].Type)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	assert.Equal(t, audit.CategoryFileOp, events[0].Category)
	assert.Equal(t, "archive.extract", events[0].Action)
	assert.Equal(t, testUser1.ID, events[0].ActorID)
}
