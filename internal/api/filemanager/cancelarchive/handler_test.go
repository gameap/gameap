package cancelarchive

import (
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
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

type mockArchiveCanceler struct {
	cancelFunc  func(ctx context.Context, node *domain.Node, operationID, reason string) error
	snapshots   map[string]daemon.ArchiveOpSnapshot
	cancelCalls int
}

func (m *mockArchiveCanceler) Cancel(
	ctx context.Context, node *domain.Node, operationID, reason string,
) error {
	m.cancelCalls++
	if m.cancelFunc != nil {
		return m.cancelFunc(ctx, node, operationID, reason)
	}

	return nil
}

func (m *mockArchiveCanceler) GetSnapshot(operationID string) (daemon.ArchiveOpSnapshot, bool) {
	snapshot, ok := m.snapshots[operationID]

	return snapshot, ok
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

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1)))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)

	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		operationID    string
		setupCanceler  func() *mockArchiveCanceler
		expectedStatus int
		wantError      string
	}{
		{
			name:        "successful_cancel_returns_202",
			operationID: "op-abc",
			setupCanceler: func() *mockArchiveCanceler {
				return &mockArchiveCanceler{
					cancelFunc: func(_ context.Context, node *domain.Node, operationID, reason string) error {
						assert.Equal(t, uint(1), node.ID)
						assert.Equal(t, "op-abc", operationID)
						assert.Equal(t, "canceled by user", reason)

						return nil
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "empty_operation_id_rejected",
			operationID:    "",
			setupCanceler:  func() *mockArchiveCanceler { return &mockArchiveCanceler{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "operation id is required",
		},
		{
			name:        "daemon_not_connected_maps_to_bad_gateway",
			operationID: "op-abc",
			setupCanceler: func() *mockArchiveCanceler {
				return &mockArchiveCanceler{
					cancelFunc: func(context.Context, *domain.Node, string, string) error {
						return daemon.ErrDaemonNotConnected
					},
				}
			},
			expectedStatus: http.StatusBadGateway,
			wantError:      "Bad Gateway",
		},
		{
			name:        "operation_of_another_server_is_rejected",
			operationID: "op-foreign",
			setupCanceler: func() *mockArchiveCanceler {
				return &mockArchiveCanceler{
					snapshots: map[string]daemon.ArchiveOpSnapshot{
						"op-foreign": {OperationID: "op-foreign", ServerID: 42, NodeID: 1},
					},
				}
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "archive operation not found",
		},
		{
			name:        "operation_of_this_server_passes_the_ownership_check",
			operationID: "op-own",
			setupCanceler: func() *mockArchiveCanceler {
				return &mockArchiveCanceler{
					snapshots: map[string]daemon.ArchiveOpSnapshot{
						"op-own": {OperationID: "op-own", ServerID: 1, NodeID: 1},
					},
				}
			},
			expectedStatus: http.StatusAccepted,
		},
		{
			name:        "operation_unknown_on_this_instance_passes_through",
			operationID: "op-elsewhere",
			setupCanceler: func() *mockArchiveCanceler {
				return &mockArchiveCanceler{}
			},
			expectedStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			canceler := tt.setupCanceler()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, canceler, api.NewResponder(), nil)

			setupRepo(t, serverRepo, nodeRepo, rbacRepo)

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/file-manager/1/archive-operations/"+tt.operationID+"/cancel",
				nil,
			)
			req = req.WithContext(authenticatedSession(&testUser1))
			vars := map[string]string{"server": "1"}
			if tt.operationID != "" {
				vars["operationID"] = tt.operationID
			}
			req = mux.SetURLVars(req, vars)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())

			// 4xx from validation or the ownership check happens before the
			// daemon is contacted; a 502 comes from Cancel itself.
			if tt.expectedStatus == http.StatusBadRequest || tt.expectedStatus == http.StatusNotFound {
				assert.Zero(t, canceler.cancelCalls, "a rejected request must never reach Cancel")
			}

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}
		})
	}
}

func TestHandler_UserWithoutFilesAbility_Returns403(t *testing.T) {
	// ARRANGE: server and node exist, but no files ability is granted.
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(1)))
	serverRepo.AddUserServer(1, 1)
	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))

	canceler := &mockArchiveCanceler{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, canceler, api.NewResponder(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/archive-operations/op-1/cancel", nil)
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1", "operationID": "op-1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Zero(t, canceler.cancelCalls, "an unauthorized request must never reach Cancel")
}

func TestHandler_NodeNotFound_Returns404(t *testing.T) {
	// ARRANGE: the server references a node that does not exist.
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	require.NoError(t, serverRepo.Save(context.Background(), newTestServer(999)))
	serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, rbacRepo, 1, 1)

	canceler := &mockArchiveCanceler{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, canceler, api.NewResponder(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/archive-operations/op-1/cancel", nil)
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1", "operationID": "op-1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	assert.Zero(t, canceler.cancelCalls)
}

func TestHandler_Audit_SuccessfulCancelIsRecorded(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	setupRepo(t, serverRepo, nodeRepo, rbacRepo)

	recorder := &auditCapture{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, &mockArchiveCanceler{}, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodPost, "/api/file-manager/1/archive-operations/op-1/cancel", nil)
	req = req.WithContext(authenticatedSession(&testUser1))
	req = mux.SetURLVars(req, map[string]string{"server": "1", "operationID": "op-1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusAccepted, w.Code, "body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventFileArchiveCancel, events[0].Type)
	assert.Equal(t, "archive.cancel", events[0].Action)
	assert.Equal(t, testUser1.ID, events[0].ActorID)
}
