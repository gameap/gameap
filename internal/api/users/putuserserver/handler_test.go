package putuserserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/audit"
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

var testAdmin = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

func saveTestServer(t *testing.T, serversRepo *inmemory.ServerRepository) {
	t.Helper()

	server := &domain.Server{
		ID:         1,
		UID:        uuid.New(),
		UUIDShort:  "attach01",
		Name:       "Test Server",
		GameID:     "csgo",
		DSID:       1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
		Dir:        "/servers/attach01",
	}
	require.NoError(t, serversRepo.Save(context.Background(), server))
}

func authenticatedCtx() context.Context {
	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testAdmin,
	}

	return auth.ContextWithSession(context.Background(), session)
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		userID          string
		serverID        string
		setupAuth       func() context.Context
		setupRepo       func(*testing.T, *inmemory.UserRepository, *inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus  int
		wantError       string
		verifyUserID    uint
		wantAttachedIDs []uint
		wantAuditEvent  bool
	}{
		{
			name:      "successful_attach",
			userID:    "2",
			serverID:  "1",
			setupAuth: authenticatedCtx,
			setupRepo: func(t *testing.T, usersRepo *inmemory.UserRepository, serversRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				require.NoError(t, usersRepo.Save(context.Background(), &domain.User{
					ID: 2, Login: "target", Email: "target@example.com",
				}))
				saveTestServer(t, serversRepo)
			},
			expectedStatus:  http.StatusNoContent,
			verifyUserID:    2,
			wantAttachedIDs: []uint{1},
			wantAuditEvent:  true,
		},
		{
			name:      "attach_already_attached_is_idempotent",
			userID:    "2",
			serverID:  "1",
			setupAuth: authenticatedCtx,
			setupRepo: func(t *testing.T, usersRepo *inmemory.UserRepository, serversRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				require.NoError(t, usersRepo.Save(context.Background(), &domain.User{
					ID: 2, Login: "target", Email: "target@example.com",
				}))
				saveTestServer(t, serversRepo)
				serversRepo.AddUserServer(2, 1)
			},
			expectedStatus:  http.StatusNoContent,
			verifyUserID:    2,
			wantAttachedIDs: []uint{1},
			wantAuditEvent:  true,
		},
		{
			name:      "user_not_found",
			userID:    "99",
			serverID:  "1",
			setupAuth: authenticatedCtx,
			setupRepo: func(t *testing.T, _ *inmemory.UserRepository, serversRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				saveTestServer(t, serversRepo)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "user not found",
		},
		{
			name:      "server_not_found",
			userID:    "2",
			serverID:  "99",
			setupAuth: authenticatedCtx,
			setupRepo: func(t *testing.T, usersRepo *inmemory.UserRepository, _ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				require.NoError(t, usersRepo.Save(context.Background(), &domain.User{
					ID: 2, Login: "target", Email: "target@example.com",
				}))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:           "invalid_user_id",
			userID:         "abc",
			serverID:       "1",
			setupAuth:      authenticatedCtx,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid user id",
		},
		{
			name:           "invalid_server_id",
			userID:         "2",
			serverID:       "abc",
			setupAuth:      authenticatedCtx,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:           "user_not_authenticated",
			userID:         "2",
			serverID:       "1",
			setupAuth:      context.Background,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			// OWASP API Top 10:2023 API5 (BFLA): a personal access token must
			// not modify administrators, matching PUT /api/users/{id}.
			name:     "pat_token_cannot_attach_to_admin",
			userID:   "2",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "operator",
					Email: "operator@example.com",
					User:  &domain.User{ID: 3, Login: "operator", Email: "operator@example.com"},
					Token: &domain.PersonalAccessToken{ID: 1},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(t *testing.T, usersRepo *inmemory.UserRepository, serversRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				t.Helper()
				require.NoError(t, usersRepo.Save(context.Background(), &domain.User{
					ID: 2, Login: "rootadmin", Email: "rootadmin@example.com",
				}))
				saveTestServer(t, serversRepo)

				adminAbility := &domain.Ability{
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))

				entityTypeUser := domain.EntityTypeUser
				permission := &domain.Permission{
					AbilityID:  adminAbility.ID,
					EntityID:   new(uint(2)),
					EntityType: &entityTypeUser,
					Forbidden:  false,
				}
				require.NoError(t, rbacRepo.SavePermission(context.Background(), permission))
			},
			expectedStatus: http.StatusForbidden,
			wantError:      "personal access tokens cannot modify administrators",
			verifyUserID:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			usersRepo := inmemory.NewUserRepository()
			serversRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			auditLog := &auditCapture{}
			handler := NewHandler(
				usersRepo,
				serversRepo,
				rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
				api.NewResponder(),
				auditLog,
				nil,
			)

			if tt.setupRepo != nil {
				tt.setupRepo(t, usersRepo, serversRepo, rbacRepo)
			}

			req := httptest.NewRequest(http.MethodPut, "/api/users/"+tt.userID+"/servers/"+tt.serverID, nil)
			req = req.WithContext(tt.setupAuth())
			req = mux.SetURLVars(req, map[string]string{"id": tt.userID, "server": tt.serverID})
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

			if tt.verifyUserID != 0 {
				servers, err := serversRepo.FindUserServers(context.Background(), tt.verifyUserID, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, len(tt.wantAttachedIDs))
				for i, wantID := range tt.wantAttachedIDs {
					assert.Equal(t, wantID, servers[i].ID)
				}
			}

			event, found := findEvent(auditLog.snapshot(), audit.EventUserServerAttach)
			if tt.wantAuditEvent {
				require.True(t, found, "expected user.server.attach audit event")
				assert.Equal(t, tt.userID, event.ResourceID)
			} else {
				assert.False(t, found, "no user.server.attach audit event expected")
			}
		})
	}
}
