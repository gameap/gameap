package getstatus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name                  string
		serverID              string
		setupAuth             func() context.Context
		setupRepo             func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus        int
		wantError             string
		expectStatus          bool
		expectedProcessActive bool
	}{
		{
			name:     "successful status retrieval - server online",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()
				lastCheck := now.Add(-30 * time.Second)

				server := &domain.Server{
					ID:               1,
					UUID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:        "short1",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					Dir:              "/home/gameap/servers/test1",
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus:        http.StatusOK,
			expectStatus:          true,
			expectedProcessActive: true,
		},
		{
			name:     "successful status retrieval - server offline",
			serverID: "2",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 2",
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
			},
			expectedStatus:        http.StatusOK,
			expectStatus:          true,
			expectedProcessActive: false,
		},
		{
			name:     "successful status retrieval - server process expired",
			serverID: "3",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()
				lastCheck := now.Add(-5 * time.Minute)

				server := &domain.Server{
					ID:               3,
					UUID:             uuid.MustParse("33333333-3333-3333-3333-333333333333"),
					UUIDShort:        "short3",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 3",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27017,
					Dir:              "/home/gameap/servers/test3",
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 3)
			},
			expectedStatus:        http.StatusOK,
			expectStatus:          true,
			expectedProcessActive: false,
		},
		{
			name:     "server not found",
			serverID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectStatus:   false,
		},
		{
			name:     "user not authenticated",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectStatus:   false,
		},
		{
			name:     "invalid server id",
			serverID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectStatus:   false,
		},
		{
			name:     "user does not have access to server",
			serverID: "4",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
					ID:            4,
					UUID:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
					UUIDShort:     "short4",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Other User Server",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27018,
					Dir:           "/home/gameap/servers/test4",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 4)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectStatus:   false,
		},
		{
			name:     "admin can access any server",
			serverID: "5",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				lastCheck := now.Add(-1 * time.Minute)

				server := &domain.Server{
					ID:               5,
					UUID:             uuid.MustParse("55555555-5555-5555-5555-555555555555"),
					UUIDShort:        "short5",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Server 5",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27019,
					Dir:              "/home/gameap/servers/test5",
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 5)

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus:        http.StatusOK,
			expectStatus:          true,
			expectedProcessActive: true,
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

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/status", nil)
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

			if tt.expectStatus {
				var status statusResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
				assert.Equal(t, tt.expectedProcessActive, status.ProcessActive)
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.serverFinder)
	assert.Equal(t, responder, handler.responder)
}

func TestNewStatusResponse(t *testing.T) {
	tests := []struct {
		name                  string
		server                *domain.Server
		expectedProcessActive bool
	}{
		{
			name: "server is online",
			server: &domain.Server{
				ID:            1,
				ProcessActive: true,
				LastProcessCheck: func() *time.Time {
					t := time.Now().Add(-30 * time.Second)

					return &t
				}(),
			},
			expectedProcessActive: true,
		},
		{
			name: "server is offline - process not active",
			server: &domain.Server{
				ID:            2,
				ProcessActive: false,
				LastProcessCheck: func() *time.Time {
					t := time.Now().Add(-30 * time.Second)

					return &t
				}(),
			},
			expectedProcessActive: false,
		},
		{
			name: "server is offline - last check expired",
			server: &domain.Server{
				ID:            3,
				ProcessActive: true,
				LastProcessCheck: func() *time.Time {
					t := time.Now().Add(-5 * time.Minute)

					return &t
				}(),
			},
			expectedProcessActive: false,
		},
		{
			name: "server is offline - no last check",
			server: &domain.Server{
				ID:               4,
				ProcessActive:    true,
				LastProcessCheck: nil,
			},
			expectedProcessActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := newStatusResponse(tt.server)
			assert.Equal(t, tt.expectedProcessActive, response.ProcessActive)
		})
	}
}
