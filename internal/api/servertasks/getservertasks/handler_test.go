package getservertasks

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

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepos     func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository)
		queryParams    string
		expectedStatus int
		wantError      string
		expectedTasks  int
	}{
		{
			name: "successful retrieval with admin user",
			setupAuth: func() context.Context {
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
			},
			setupRepos: func(taskRepo *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				executeDate := now.Add(time.Hour)

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
				_ = serverRepo.Save(context.Background(), server)

				task1 := &domain.ServerTask{
					Command:      "restart",
					ServerID:     1,
					Repeat:       0,
					RepeatPeriod: 0,
					Counter:      0,
					ExecuteDate:  executeDate,
					Payload:      lo.ToPtr("task payload"),
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				_ = taskRepo.Save(context.Background(), task1)

				task2 := &domain.ServerTask{
					Command:      "update",
					ServerID:     1,
					Repeat:       1,
					RepeatPeriod: 3600,
					Counter:      5,
					ExecuteDate:  executeDate.Add(2 * time.Hour),
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				_ = taskRepo.Save(context.Background(), task2)

				adminRole := &domain.Role{
					Name:  "admin",
					Title: lo.ToPtr("Administrator"),
					Level: lo.ToPtr(uint(100)),
				}
				_ = rbacRepo.SaveRole(context.Background(), adminRole)

				assignedRole := &domain.AssignedRole{
					RoleID:     adminRole.ID,
					EntityID:   1,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)

				ability := &domain.Ability{
					Name:  domain.AbilityNameAdminRolesPermissions,
					Title: lo.ToPtr("Admin Permissions"),
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  false,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)
			},
			queryParams:    "1",
			expectedStatus: http.StatusOK,
			expectedTasks:  2,
		},
		{
			name: "successful retrieval with regular user having permissions",
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
			setupRepos: func(taskRepo *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				executeDate := now.Add(time.Hour)

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
				_ = serverRepo.Save(context.Background(), server)
				serverRepo.AddUserServer(2, 1)

				task := &domain.ServerTask{
					Command:      "restart",
					ServerID:     1,
					Repeat:       0,
					RepeatPeriod: 0,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				_ = taskRepo.Save(context.Background(), task)

				ability := &domain.Ability{
					Name:       domain.AbilityNameGameServerTasks,
					Title:      lo.ToPtr("Server Tasks"),
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: lo.ToPtr(domain.EntityTypeServer),
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(2)),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  false,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)
			},
			queryParams:    "1",
			expectedStatus: http.StatusOK,
			expectedTasks:  1,
		},
		{
			name:           "user not authenticated",
			setupRepos:     func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository) {},
			queryParams:    "1",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "invalid server ID",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    1,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos:     func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository) {},
			queryParams:    "invalid",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name: "missing server ID parameter",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    1,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos:     func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository) {},
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name: "access denied - user has no access to server",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    3,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(_ *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
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
				_ = serverRepo.Save(context.Background(), server)
			},
			queryParams:    "1",
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name: "empty tasks list for valid server",
			setupAuth: func() context.Context {
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
			},
			setupRepos: func(_ *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
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
				_ = serverRepo.Save(context.Background(), server)

				adminRole := &domain.Role{
					Name:  "admin",
					Title: lo.ToPtr("Administrator"),
					Level: lo.ToPtr(uint(100)),
				}
				_ = rbacRepo.SaveRole(context.Background(), adminRole)

				assignedRole := &domain.AssignedRole{
					RoleID:     adminRole.ID,
					EntityID:   1,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)

				ability := &domain.Ability{
					Name:  domain.AbilityNameAdminRolesPermissions,
					Title: lo.ToPtr("Admin Permissions"),
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  false,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)
			},
			queryParams:    "1",
			expectedStatus: http.StatusOK,
			expectedTasks:  0,
		},
		{
			name: "user_without_tasks_permission",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    5,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(_ *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
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
				_ = serverRepo.Save(context.Background(), server)
				serverRepo.AddUserServer(5, 1)
			},
			queryParams:    "1",
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name: "user_with_forbidden_tasks_permission",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    6,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(_ *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
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
				_ = serverRepo.Save(context.Background(), server)
				serverRepo.AddUserServer(6, 1)

				ability := &domain.Ability{
					Name:       domain.AbilityNameGameServerTasks,
					Title:      lo.ToPtr("Server Tasks"),
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: lo.ToPtr(domain.EntityTypeServer),
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(6)),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  true,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)
			},
			queryParams:    "1",
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name: "user_with_role_based_permissions",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "user",
					Email: "user@example.com",
					User: &domain.User{
						ID:    7,
						Login: "user",
						Email: "user@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(taskRepo *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				executeDate := now.Add(time.Hour)

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
				_ = serverRepo.Save(context.Background(), server)
				serverRepo.AddUserServer(7, 1)

				task := &domain.ServerTask{
					Command:      "restart",
					ServerID:     1,
					Repeat:       0,
					RepeatPeriod: 0,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				_ = taskRepo.Save(context.Background(), task)

				serverRole := &domain.Role{
					Name:  "server_manager",
					Title: lo.ToPtr("Server Manager"),
				}
				_ = rbacRepo.SaveRole(context.Background(), serverRole)

				_ = rbacRepo.AssignRolesForEntity(
					context.Background(),
					7,
					domain.EntityTypeUser,
					[]domain.RestrictedRole{domain.NewRestrictedRoleFromRole(*serverRole)},
				)

				ability := &domain.Ability{
					Name:       domain.AbilityNameGameServerTasks,
					Title:      lo.ToPtr("Server Tasks"),
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: lo.ToPtr(domain.EntityTypeServer),
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(serverRole.ID),
					EntityType: lo.ToPtr(domain.EntityTypeRole),
					Forbidden:  false,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)
			},
			queryParams:    "1",
			expectedStatus: http.StatusOK,
			expectedTasks:  1,
		},
		{
			name: "admin_bypasses_server_permissions",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User: &domain.User{
						ID:    8,
						Login: "admin",
						Email: "admin@example.com",
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(taskRepo *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				executeDate := now.Add(time.Hour)

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
				_ = serverRepo.Save(context.Background(), server)

				task := &domain.ServerTask{
					Command:      "restart",
					ServerID:     1,
					Repeat:       0,
					RepeatPeriod: 0,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				_ = taskRepo.Save(context.Background(), task)

				ability := &domain.Ability{
					Name:  domain.AbilityNameAdminRolesPermissions,
					Title: lo.ToPtr("Admin Permissions"),
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(8)),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  false,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)
			},
			queryParams:    "1",
			expectedStatus: http.StatusOK,
			expectedTasks:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serversRepo := inmemory.NewServerRepository()
			serverTasksRepo := inmemory.NewServerTaskRepository(serversRepo)
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()

			handler := NewHandler(serverTasksRepo, serversRepo, rbacService, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(serverTasksRepo, serversRepo, rbacRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.queryParams+"/tasks", nil)
			req = req.WithContext(ctx)

			if tt.queryParams != "" {
				req = mux.SetURLVars(req, map[string]string{"server": tt.queryParams})
			}

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
				var tasks []serverTaskResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tasks))
				assert.Len(t, tasks, tt.expectedTasks)

				if tt.expectedTasks > 0 {
					task := tasks[0]
					assert.NotZero(t, task.ID)
					assert.NotEmpty(t, task.Command)
					assert.Equal(t, uint(1), task.ServerID)
					assert.NotZero(t, task.ExecuteDate)
				}
			}
		})
	}
}

func TestHandler_ServerTasksResponseConversion(t *testing.T) {
	now := time.Now()
	executeDate := now.Add(time.Hour)
	payload := "test payload"

	tasks := []domain.ServerTask{
		{
			ID:           1,
			Command:      "start",
			ServerID:     1,
			Repeat:       0,
			RepeatPeriod: 2 * time.Hour,
			Counter:      0,
			ExecuteDate:  executeDate,
			Payload:      &payload,
			CreatedAt:    &now,
			UpdatedAt:    &now,
		},
		{
			ID:           2,
			Command:      "stop",
			ServerID:     1,
			Repeat:       1,
			RepeatPeriod: 3600 * time.Second,
			Counter:      10,
			ExecuteDate:  executeDate.Add(2 * time.Hour),
			CreatedAt:    &now,
			UpdatedAt:    &now,
		},
	}

	response := newServerTasksResponseFromServerTasks(tasks)

	require.Len(t, response, 2)

	assert.Equal(t, uint(1), response[0].ID)
	assert.Equal(t, "start", response[0].Command)
	assert.Equal(t, uint(1), response[0].ServerID)
	assert.Equal(t, uint8(0), response[0].Repeat)
	assert.Equal(t, "2 hours", response[0].RepeatPeriod)
	assert.Equal(t, uint(0), response[0].Counter)
	assert.Equal(t, executeDate, response[0].ExecuteDate)
	assert.NotNil(t, response[0].Payload)
	assert.Equal(t, payload, *response[0].Payload)
	assert.Equal(t, &now, response[0].CreatedAt)
	assert.Equal(t, &now, response[0].UpdatedAt)

	assert.Equal(t, uint(2), response[1].ID)
	assert.Equal(t, "stop", response[1].Command)
	assert.Equal(t, uint(1), response[1].ServerID)
	assert.Equal(t, uint8(1), response[1].Repeat)
	assert.Equal(t, "1 hour", response[1].RepeatPeriod)
	assert.Equal(t, uint(10), response[1].Counter)
	assert.Equal(t, executeDate.Add(2*time.Hour), response[1].ExecuteDate)
	assert.Nil(t, response[1].Payload)
	assert.Equal(t, &now, response[1].CreatedAt)
	assert.Equal(t, &now, response[1].UpdatedAt)
}
