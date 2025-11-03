package getsummary

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testAdminUser = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

var testRegularUser = domain.User{
	ID:    2,
	Login: "user",
	Email: "user@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepos     func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		wantTotal      int
		wantOnline     int
		wantOffline    int
	}{
		{
			name: "admin user sees all servers",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testAdminUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				setupAdminUser(rbacRepo, testAdminUser.ID)

				now := time.Now().UTC()
				oldCheck := now.Add(-150 * time.Second)
				recentCheck := now.Add(-30 * time.Second)

				server1 := &domain.Server{
					ID:               1,
					UUID:             uuid.New(),
					Name:             "Server 1",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    true,
					LastProcessCheck: &recentCheck,
				}
				server2 := &domain.Server{
					ID:               2,
					UUID:             uuid.New(),
					Name:             "Server 2",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27016,
					ProcessActive:    false,
					LastProcessCheck: &recentCheck,
				}
				server3 := &domain.Server{
					ID:               3,
					UUID:             uuid.New(),
					Name:             "Server 3",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27017,
					ProcessActive:    true,
					LastProcessCheck: &oldCheck,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))
				require.NoError(t, serverRepo.Save(context.Background(), server3))
			},
			expectedStatus: http.StatusOK,
			wantTotal:      3,
			wantOnline:     1,
			wantOffline:    2,
		},
		{
			name: "regular user sees only their servers",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User:  &testRegularUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now().UTC()
				recentCheck := now.Add(-30 * time.Second)

				server1 := &domain.Server{
					ID:               1,
					UUID:             uuid.New(),
					Name:             "User Server 1",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    true,
					LastProcessCheck: &recentCheck,
				}
				server2 := &domain.Server{
					ID:               2,
					UUID:             uuid.New(),
					Name:             "User Server 2",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27016,
					ProcessActive:    false,
					LastProcessCheck: &recentCheck,
				}
				server3 := &domain.Server{
					ID:               3,
					UUID:             uuid.New(),
					Name:             "Other User Server",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27017,
					ProcessActive:    true,
					LastProcessCheck: &recentCheck,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))
				require.NoError(t, serverRepo.Save(context.Background(), server3))

				serverRepo.AddUserServer(testRegularUser.ID, 1)
				serverRepo.AddUserServer(testRegularUser.ID, 2)
			},
			expectedStatus: http.StatusOK,
			wantTotal:      2,
			wantOnline:     1,
			wantOffline:    1,
		},
		{
			name:           "user not authenticated",
			setupRepos:     func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "user with no servers",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User:  &testRegularUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos:     func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusOK,
			wantTotal:      0,
			wantOnline:     0,
			wantOffline:    0,
		},
		{
			name: "all servers online",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User:  &testRegularUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now().UTC()
				recentCheck := now.Add(-30 * time.Second)

				server1 := &domain.Server{
					ID:               1,
					UUID:             uuid.New(),
					Name:             "Server 1",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    true,
					LastProcessCheck: &recentCheck,
				}
				server2 := &domain.Server{
					ID:               2,
					UUID:             uuid.New(),
					Name:             "Server 2",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27016,
					ProcessActive:    true,
					LastProcessCheck: &recentCheck,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				serverRepo.AddUserServer(testRegularUser.ID, 1)
				serverRepo.AddUserServer(testRegularUser.ID, 2)
			},
			expectedStatus: http.StatusOK,
			wantTotal:      2,
			wantOnline:     2,
			wantOffline:    0,
		},
		{
			name: "all servers offline",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User:  &testRegularUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now().UTC()
				recentCheck := now.Add(-30 * time.Second)

				server1 := &domain.Server{
					ID:               1,
					UUID:             uuid.New(),
					Name:             "Server 1",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    false,
					LastProcessCheck: &recentCheck,
				}
				server2 := &domain.Server{
					ID:               2,
					UUID:             uuid.New(),
					Name:             "Server 2",
					GameID:           "cs",
					ServerIP:         "127.0.0.1",
					ServerPort:       27016,
					ProcessActive:    false,
					LastProcessCheck: &recentCheck,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				serverRepo.AddUserServer(testRegularUser.ID, 1)
				serverRepo.AddUserServer(testRegularUser.ID, 2)
			},
			expectedStatus: http.StatusOK,
			wantTotal:      2,
			wantOnline:     0,
			wantOffline:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			responder := api.NewResponder()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

			if tt.setupRepos != nil {
				tt.setupRepos(serverRepo, rbacRepo)
			}

			handler := NewHandler(serverRepo, rbacService, responder)

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/servers/summary", nil)
			req = req.WithContext(ctx)
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
			} else if tt.expectedStatus == http.StatusOK {
				var summary summaryResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &summary))
				assert.Equal(t, tt.wantTotal, summary.Total)
				assert.Equal(t, tt.wantOnline, summary.Online)
				assert.Equal(t, tt.wantOffline, summary.Offline)
			}
		})
	}
}

func TestHandler_calculateSummary(t *testing.T) {
	handler := &Handler{}
	now := time.Now().UTC()

	tests := []struct {
		name        string
		servers     []domain.Server
		wantTotal   int
		wantOnline  int
		wantOffline int
	}{
		{
			name:        "empty servers list",
			servers:     []domain.Server{},
			wantTotal:   0,
			wantOnline:  0,
			wantOffline: 0,
		},
		{
			name: "mixed online and offline servers",
			servers: []domain.Server{
				{
					ProcessActive:    true,
					LastProcessCheck: timePtr(now.Add(-30 * time.Second)),
				},
				{
					ProcessActive:    false,
					LastProcessCheck: timePtr(now.Add(-30 * time.Second)),
				},
				{
					ProcessActive:    true,
					LastProcessCheck: timePtr(now.Add(-150 * time.Second)),
				},
			},
			wantTotal:   3,
			wantOnline:  1,
			wantOffline: 2,
		},
		{
			name: "all online",
			servers: []domain.Server{
				{
					ProcessActive:    true,
					LastProcessCheck: timePtr(now.Add(-30 * time.Second)),
				},
				{
					ProcessActive:    true,
					LastProcessCheck: timePtr(now.Add(-60 * time.Second)),
				},
			},
			wantTotal:   2,
			wantOnline:  2,
			wantOffline: 0,
		},
		{
			name: "all offline",
			servers: []domain.Server{
				{
					ProcessActive:    false,
					LastProcessCheck: timePtr(now.Add(-30 * time.Second)),
				},
				{
					ProcessActive:    true,
					LastProcessCheck: nil,
				},
			},
			wantTotal:   2,
			wantOnline:  0,
			wantOffline: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.calculateSummary(tt.servers)
			assert.Equal(t, tt.wantTotal, result.Total)
			assert.Equal(t, tt.wantOnline, result.Online)
			assert.Equal(t, tt.wantOffline, result.Offline)
		})
	}
}

func setupAdminUser(rbacRepo *inmemory.RBACRepository, userID uint) {
	ability := &domain.Ability{
		Name:       domain.AbilityNameAdminRolesPermissions,
		EntityType: nil,
		EntityID:   nil,
	}

	_ = rbacRepo.SaveAbility(context.Background(), ability)
	_ = rbacRepo.Allow(context.Background(), userID, domain.EntityTypeUser, []domain.Ability{*ability})
}

func timePtr(t time.Time) *time.Time {
	return &t
}
