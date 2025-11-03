package deleteservertask

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
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultSetupAuth = func() context.Context {
	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User: &domain.User{
			ID:    1,
			Login: "admin",
			Email: "admin@example.com",
		},
	}

	return auth.ContextWithSession(context.Background(), session)
}

var defaultSetupRepos = func(
	taskRepo *inmemory.ServerTaskRepository,
	serverRepo *inmemory.ServerRepository,
	rbacRepo *inmemory.RBACRepository,
) error {
	now := time.Now()

	server := &domain.Server{
		ID:         1,
		UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:  "short1",
		Name:       "Test Server",
		GameID:     "cs",
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		Dir:        "/home/gameap/servers/test1",
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	err := serverRepo.Save(context.Background(), server)
	if err != nil {
		return err
	}

	task := &domain.ServerTask{
		ID:          1,
		Command:     "restart",
		ServerID:    1,
		Repeat:      1,
		ExecuteDate: time.Now().Add(time.Hour),
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	err = taskRepo.Save(context.Background(), task)
	if err != nil {
		return err
	}

	adminRole := &domain.Role{
		Name:  "admin",
		Title: lo.ToPtr("Administrator"),
		Level: lo.ToPtr(uint(100)),
	}
	err = rbacRepo.SaveRole(context.Background(), adminRole)
	if err != nil {
		return err
	}

	assignedRole := &domain.AssignedRole{
		RoleID:     adminRole.ID,
		EntityID:   1,
		EntityType: domain.EntityTypeUser,
	}
	err = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
	if err != nil {
		return err
	}

	ability := &domain.Ability{
		Name:  domain.AbilityNameAdminRolesPermissions,
		Title: lo.ToPtr("Admin Permissions"),
	}
	err = rbacRepo.SaveAbility(context.Background(), ability)
	if err != nil {
		return err
	}

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   lo.ToPtr(uint(1)),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}
	err = rbacRepo.SavePermission(context.Background(), permission)
	if err != nil {
		return err
	}

	return nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		setupAuth  func() context.Context
		setupRepos func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository) error
		wantStatus int
		wantError  string
	}{
		{
			name:       "successful task deletion with admin user",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			wantStatus: http.StatusNoContent,
		},
		{
			name: "unauthenticated request",
			setupRepos: func(
				_ *inmemory.ServerTaskRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) error {
				return nil
			},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
		{
			name: "access denied - no server access",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    2,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				taskRepo *inmemory.ServerTaskRepository,
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) error {
				now := time.Now()

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Name:       "Test Server",
					GameID:     "cs",
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					Dir:        "/home/gameap/servers/test1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				err := serverRepo.Save(context.Background(), server)
				if err != nil {
					return err
				}

				task := &domain.ServerTask{
					ID:          1,
					Command:     "restart",
					ServerID:    1,
					Repeat:      1,
					ExecuteDate: time.Now().Add(time.Hour),
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				err = taskRepo.Save(context.Background(), task)
				if err != nil {
					return err
				}

				return nil
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access to server denied",
		},
		{
			name:       "task not found",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			wantStatus: http.StatusNotFound,
			wantError:  "server task not found",
		},
		{
			name:      "task belongs to different server",
			setupAuth: defaultSetupAuth,
			setupRepos: func(
				taskRepo *inmemory.ServerTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) error {
				now := time.Now()

				// Create server 1
				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Name:       "Test Server 1",
					GameID:     "cs",
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					Dir:        "/home/gameap/servers/test1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				err := serverRepo.Save(context.Background(), server1)
				if err != nil {
					return err
				}

				// Create server 2
				server2 := &domain.Server{
					ID:         2,
					UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:  "short2",
					Name:       "Test Server 2",
					GameID:     "cs",
					ServerIP:   "127.0.0.2",
					ServerPort: 27016,
					Dir:        "/home/gameap/servers/test2",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				err = serverRepo.Save(context.Background(), server2)
				if err != nil {
					return err
				}

				// Create task for server 2
				task := &domain.ServerTask{
					ID:          1,
					Command:     "restart",
					ServerID:    2,
					Repeat:      1,
					ExecuteDate: time.Now().Add(time.Hour),
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				err = taskRepo.Save(context.Background(), task)
				if err != nil {
					return err
				}

				adminRole := &domain.Role{
					Name:  "admin",
					Title: lo.ToPtr("Administrator"),
					Level: lo.ToPtr(uint(100)),
				}
				err = rbacRepo.SaveRole(context.Background(), adminRole)
				if err != nil {
					return err
				}

				assignedRole := &domain.AssignedRole{
					RoleID:     adminRole.ID,
					EntityID:   1,
					EntityType: domain.EntityTypeUser,
				}
				err = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
				if err != nil {
					return err
				}

				ability := &domain.Ability{
					Name:  domain.AbilityNameAdminRolesPermissions,
					Title: lo.ToPtr("Admin Permissions"),
				}
				err = rbacRepo.SaveAbility(context.Background(), ability)
				if err != nil {
					return err
				}

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  false,
				}
				err = rbacRepo.SavePermission(context.Background(), permission)
				if err != nil {
					return err
				}

				return nil
			},
			wantStatus: http.StatusNotFound,
			wantError:  "server task not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup repositories
			serversRepo := inmemory.NewServerRepository()
			serverTasksRepo := inmemory.NewServerTaskRepository(serversRepo)
			rbacRepo := inmemory.NewRBACRepository()

			if tt.setupRepos != nil {
				err := tt.setupRepos(serverTasksRepo, serversRepo, rbacRepo)
				require.NoError(t, err)
			}

			// Create handler
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverTasksRepo, serversRepo, rbacService, responder)

			// Setup auth context
			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			// Create request
			taskID := "1"
			if tt.name == "task not found" {
				taskID = "999"
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/servers/1/tasks/"+taskID, nil)
			req = req.WithContext(ctx)

			// Set URL variables
			req = mux.SetURLVars(req, map[string]string{"server": "1", "id": taskID})

			// Execute request
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Validate response
			if rr.Code != tt.wantStatus {
				t.Logf("Response body: %s", rr.Body.String())
			}
			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantError != "" {
				var errorResponse map[string]any
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				require.NoError(t, err)
				errorMsg, ok := errorResponse["error"].(string)
				if !ok {
					errorMsg, _ = errorResponse["message"].(string)
				}
				assert.Contains(t, errorMsg, tt.wantError)
			}
		})
	}
}
