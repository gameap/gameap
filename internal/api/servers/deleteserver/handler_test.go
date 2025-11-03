package deleteserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
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
	Login: "admin",
	Email: "admin@example.com",
}

var testUser2 = domain.User{
	ID:    2,
	Login: "user",
	Email: "user@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		serverID       string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
	}{
		{
			name:     "successful server deletion by admin",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				u := uuid.New()

				server := &domain.Server{
					ID:         1,
					UUID:       u,
					UUIDShort:  u.String()[0:8],
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:     "successful server deletion by user with permission",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()
				u := uuid.New()

				server := &domain.Server{
					ID:         1,
					UUID:       u,
					UUIDShort:  u.String()[0:8],
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser2.ID, server.ID)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:     "server not found",
			serverID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "invalid server id",
			serverID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, rbacService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, rbacRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/servers/"+tt.serverID, nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.serverID})
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

			if tt.expectedStatus == http.StatusNoContent {
				assert.Empty(t, w.Body.String())
			}
		})
	}
}

func TestHandler_ServerActuallyDeleted(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, rbacService, responder)

	now := time.Now()
	u := uuid.New()

	server := &domain.Server{
		ID:         1,
		UUID:       u,
		UUIDShort:  u.String()[0:8],
		Enabled:    true,
		Installed:  1,
		Blocked:    false,
		Name:       "Test Server",
		GameID:     "cstrike",
		DSID:       1,
		GameModID:  1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}

	require.NoError(t, serverRepo.Save(context.Background(), server))

	adminAbility := &domain.Ability{
		ID:   1,
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	servers, err := serverRepo.Find(ctx, nil, nil, &filters.Pagination{
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	assert.Len(t, servers, 0)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, rbacService, handler.rbac)
	assert.Equal(t, responder, handler.responder)
}
