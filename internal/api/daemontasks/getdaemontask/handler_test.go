package getdaemontask

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func allowUserAbilityForServer(
	t *testing.T,
	repo *inmemory.RBACRepository,
	userID uint,
	serverID uint,
	abilityName domain.AbilityName,
) {
	t.Helper()

	ability := domain.CreateAbilityForEntity(abilityName, serverID, domain.EntityTypeServer)
	require.NoError(t, repo.SaveAbility(context.Background(), &ability))

	require.NoError(t, repo.Allow(
		context.Background(),
		userID,
		domain.EntityTypeUser,
		[]domain.Ability{ability},
	))
}

func setupAdminUser(t *testing.T, rbacRepo *inmemory.RBACRepository, userID uint) {
	t.Helper()

	adminAbility := &domain.Ability{
		ID:   100,
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), userID, adminAbility.ID))
}

func createTestServer(t *testing.T, serverRepo *inmemory.ServerRepository, serverID uint) {
	t.Helper()

	now := time.Now()
	server := &domain.Server{
		ID:         serverID,
		UID:        uuid.New(),
		UUIDShort:  "short1",
		Enabled:    true,
		Installed:  1,
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
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		taskID     string
		setupAuth  func() context.Context
		setupRepo  func(*inmemory.DaemonTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository)
		wantStatus int
		wantError  string
	}{
		{
			name:   "admin_can_access_task_with_server_id",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				setupAdminUser(t, rbacRepo, testUser2.ID)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "admin_can_access_task_without_server_id",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				_ *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          nil,
					Task:              domain.DaemonTaskTypeCmdExec,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				setupAdminUser(t, rbacRepo, testUser2.ID)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "admin_can_access_delete_task_type",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerDelete,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				setupAdminUser(t, rbacRepo, testUser2.ID)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "user_can_access_start_task_for_own_server",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerStart)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "user_can_access_stop_task_for_own_server",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerStop)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "user_can_access_restart_task_for_own_server",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerRestart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerRestart)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "user_can_access_update_task_for_own_server",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerUpdate,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerUpdate)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "user_can_access_install_task_for_own_server",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerInstall,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerUpdate)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "user_cannot_access_task_without_server_id",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          nil,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access denied: task has no associated server",
		},
		{
			name:   "user_cannot_access_unmapped_task_type_delete",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerDelete,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access denied: task type not allowed for regular users",
		},
		{
			name:   "user_cannot_access_unmapped_task_type_move",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerMove,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access denied: task type not allowed for regular users",
		},
		{
			name:   "user_cannot_access_unmapped_task_type_cmdexec",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeCmdExec,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access denied: task type not allowed for regular users",
		},
		{
			name:   "user_cannot_access_task_for_other_users_server",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser2.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerStart)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access denied: no access to the server",
		},
		{
			name:   "user_without_start_ability_cannot_access_start_task",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:   "user_without_stop_ability_cannot_access_stop_task",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:   "user_without_restart_ability_cannot_access_restart_task",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerRestart,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:   "user_without_update_ability_cannot_access_update_task",
			taskID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				taskRepo *inmemory.DaemonTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				serverID := uint(1)
				_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerUpdate,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				createTestServer(t, serverRepo, serverID)
				serverRepo.AddUserServer(testUser1.ID, serverID)
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, serverID, domain.AbilityNameGameServerCommon)
			},
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:   "task_not_found",
			taskID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				_ *inmemory.DaemonTaskRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			wantStatus: http.StatusNotFound,
			wantError:  "daemon task not found",
		},
		{
			name:   "invalid_task_id",
			taskID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				_ *inmemory.DaemonTaskRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task id",
		},
		{
			name:   "not_authenticated",
			taskID: "1",
			setupRepo: func(
				_ *inmemory.DaemonTaskRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			taskRepo := inmemory.NewDaemonTaskRepository()
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()

			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()

			if tt.setupRepo != nil {
				tt.setupRepo(taskRepo, serverRepo, rbacRepo)
			}

			handler := NewHandler(taskRepo, serverRepo, rbacService, responder, true)

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/gdaemon_tasks/"+tt.taskID+"/output", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.taskID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

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

func TestHandler_ServeHTTP_ResponseContent(t *testing.T) {
	t.Parallel()

	taskRepo := inmemory.NewDaemonTaskRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	setupAdminUser(t, rbacRepo, testUser2.ID)

	createdAt := time.Date(2025, 9, 25, 18, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2025, 9, 25, 18, 30, 19, 0, time.UTC)
	serverID := uint(2)
	output := "Installation completed successfully\nServer started on port 27015"

	_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 2,
		ServerID:          &serverID,
		Task:              domain.DaemonTaskTypeServerInstall,
		CreatedAt:         &createdAt,
		UpdatedAt:         &updatedAt,
		Output:            &output,
		Status:            domain.DaemonTaskStatusSuccess,
	})

	handler := NewHandler(taskRepo, serverRepo, rbacService, responder, true)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser2,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/gdaemon_tasks/1/output", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response daemonTaskOutputResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, uint(2), response.DedicatedServerID)
	assert.Equal(t, new(uint(2)), response.ServerID)
	assert.Equal(t, domain.DaemonTaskTypeServerInstall, response.Task)
	assert.Equal(t, domain.DaemonTaskStatusSuccess, response.Status)
	assert.Equal(t, new(output), response.Output)
}

func TestHandler_ServeHTTP_LargeOutput(t *testing.T) {
	t.Parallel()

	taskRepo := inmemory.NewDaemonTaskRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	setupAdminUser(t, rbacRepo, testUser2.ID)

	var largeOutputSb strings.Builder
	for i := range 1000 {
		largeOutputSb.WriteString("Line " + string(rune('0'+i%10)) + ": This is a log line with some content\n")
	}
	largeOutput := largeOutputSb.String()

	_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerUpdate,
		Output:            &largeOutput,
		Status:            domain.DaemonTaskStatusSuccess,
	})

	handler := NewHandler(taskRepo, serverRepo, rbacService, responder, true)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser2,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/gdaemon_tasks/1/output", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response daemonTaskOutputResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, uint(1), response.ID)
	require.NotNil(t, response.Output)
	assert.Equal(t, largeOutput, *response.Output)
}

func TestHandler_WithOutput_False(t *testing.T) {
	t.Parallel()

	taskRepo := inmemory.NewDaemonTaskRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	setupAdminUser(t, rbacRepo, testUser2.ID)

	serverID := uint(1)
	output := "Some output"

	_ = taskRepo.Save(context.Background(), &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 1,
		ServerID:          &serverID,
		Task:              domain.DaemonTaskTypeServerStart,
		Output:            &output,
		Status:            domain.DaemonTaskStatusSuccess,
	})

	handler := NewHandler(taskRepo, serverRepo, rbacService, responder, false)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser2,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/gdaemon_tasks/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response daemonTaskOutputResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, uint(1), response.ID)
	assert.Nil(t, response.Output)
}
