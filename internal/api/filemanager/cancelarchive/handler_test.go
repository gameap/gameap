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
	cancelFunc func(ctx context.Context, node *domain.Node, operationID, reason string) error
}

func (m *mockArchiveCanceler) Cancel(
	ctx context.Context, node *domain.Node, operationID, reason string,
) error {
	if m.cancelFunc != nil {
		return m.cancelFunc(ctx, node, operationID, reason)
	}

	return nil
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			handler := NewHandler(serverRepo, nodeRepo, rbacService, tt.setupCanceler(), api.NewResponder(), nil)

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
