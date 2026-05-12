package getservertaskexecutions

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
	"github.com/rs/xid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOutputInline      = "tail of stdout\nline2"
	testOutputStoragePath = "s3://bucket/server-tasks/exec-1.log"
)

type testRepos struct {
	tasks      *inmemory.ServerTaskRepository
	executions *inmemory.ServerTaskExecutionRepository
	servers    *inmemory.ServerRepository
	rbac       *inmemory.RBACRepository
}

func TestHandler_ServeHTTP(t *testing.T) {
	type wantOutput struct {
		expected bool
		check    bool
	}

	tests := []struct {
		name             string
		setupAuth        func() context.Context
		setupRepos       func(repos *testRepos)
		serverParam      string
		taskParam        string
		statusQuery      string
		expectedStatus   int
		wantError        string
		expectedExecs    int
		wantOutputFields wantOutput
	}{
		{
			name: "admin_sees_output_fields",
			setupAuth: func() context.Context {
				return contextWithUser(1, "admin")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
				seedAdminPermission(repos.rbac)
				task := seedTask(t, repos.tasks)
				seedExecutionWithOutput(t, repos.executions, task.ID, 1)
			},
			serverParam:      "1",
			taskParam:        "1",
			expectedStatus:   http.StatusOK,
			expectedExecs:    1,
			wantOutputFields: wantOutput{expected: true, check: true},
		},
		{
			name: "regular_user_with_tasks_ability_does_not_see_output_fields",
			setupAuth: func() context.Context {
				return contextWithUser(2, "user")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
				repos.servers.AddUserServer(2, 1)
				seedServerTasksAbility(repos.rbac, 2, 1)
				task := seedTask(t, repos.tasks)
				seedExecutionWithOutput(t, repos.executions, task.ID, 1)
			},
			serverParam:      "1",
			taskParam:        "1",
			expectedStatus:   http.StatusOK,
			expectedExecs:    1,
			wantOutputFields: wantOutput{expected: false, check: true},
		},
		{
			name:           "user_not_authenticated",
			setupRepos:     func(*testRepos) {},
			serverParam:    "1",
			taskParam:      "1",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "invalid_server_id",
			setupAuth: func() context.Context {
				return contextWithUser(1, "user")
			},
			setupRepos:     func(*testRepos) {},
			serverParam:    "invalid",
			taskParam:      "1",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name: "invalid_task_id",
			setupAuth: func() context.Context {
				return contextWithUser(1, "admin")
			},
			setupRepos:     func(*testRepos) {},
			serverParam:    "1",
			taskParam:      "abc",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid task id",
		},
		{
			name: "server_not_found_for_user",
			setupAuth: func() context.Context {
				return contextWithUser(3, "user")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
			},
			serverParam:    "1",
			taskParam:      "1",
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name: "user_without_tasks_permission",
			setupAuth: func() context.Context {
				return contextWithUser(5, "user")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
				repos.servers.AddUserServer(5, 1)
			},
			serverParam:    "1",
			taskParam:      "1",
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name: "task_not_found",
			setupAuth: func() context.Context {
				return contextWithUser(1, "admin")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
				seedAdminPermission(repos.rbac)
			},
			serverParam:    "1",
			taskParam:      "999",
			expectedStatus: http.StatusNotFound,
			wantError:      "server task not found",
		},
		{
			name: "status_filter_applied",
			setupAuth: func() context.Context {
				return contextWithUser(1, "admin")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
				seedAdminPermission(repos.rbac)
				task := seedTask(t, repos.tasks)
				seedExecutionWithStatus(
					t, repos.executions, task.ID, 1,
					xid.New(), domain.ServerTaskExecutionStatusSuccess,
				)
				seedExecutionWithStatus(
					t, repos.executions, task.ID, 1,
					xid.New(), domain.ServerTaskExecutionStatusFailed,
				)
				seedExecutionWithStatus(
					t, repos.executions, task.ID, 1,
					xid.New(), domain.ServerTaskExecutionStatusFailed,
				)
			},
			serverParam:      "1",
			taskParam:        "1",
			statusQuery:      "failed",
			expectedStatus:   http.StatusOK,
			expectedExecs:    2,
			wantOutputFields: wantOutput{check: false},
		},
		{
			name: "executions_for_soft_deleted_task",
			setupAuth: func() context.Context {
				return contextWithUser(1, "admin")
			},
			setupRepos: func(repos *testRepos) {
				seedServer(repos.servers)
				seedAdminPermission(repos.rbac)
				task := seedTask(t, repos.tasks)
				seedExecutionWithOutput(t, repos.executions, task.ID, 1)
				require.NoError(t, repos.tasks.SoftDelete(context.Background(), task.ID))
			},
			serverParam:      "1",
			taskParam:        "1",
			expectedStatus:   http.StatusOK,
			expectedExecs:    1,
			wantOutputFields: wantOutput{expected: true, check: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos := newTestRepos()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), repos.rbac, 0)
			responder := api.NewResponder()

			handler := NewHandler(
				repos.tasks, repos.executions, repos.servers, rbacService, responder,
			)

			if tt.setupRepos != nil {
				tt.setupRepos(repos)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			url := "/api/servers/" + tt.serverParam + "/tasks/" + tt.taskParam + "/executions"
			if tt.statusQuery != "" {
				url += "?status=" + tt.statusQuery
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(ctx)

			vars := map[string]string{}
			if tt.serverParam != "" {
				vars["server"] = tt.serverParam
			}
			if tt.taskParam != "" {
				vars["id"] = tt.taskParam
			}
			if len(vars) > 0 {
				req = mux.SetURLVars(req, vars)
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

				return
			}

			if tt.expectedStatus != http.StatusOK {
				return
			}

			var body struct {
				Data []map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body.Data, tt.expectedExecs)

			if !tt.wantOutputFields.check {
				return
			}

			for _, exec := range body.Data {
				_, hasInline := exec["output_inline"]
				_, hasStorage := exec["output_storage_path"]

				if tt.wantOutputFields.expected {
					require.True(t, hasInline, "output_inline must be present for admin")
					require.True(t, hasStorage, "output_storage_path must be present for admin")
					assert.Equal(t, testOutputInline, exec["output_inline"])
					assert.Equal(t, testOutputStoragePath, exec["output_storage_path"])
				} else {
					assert.False(t, hasInline, "output_inline must be omitted for non-admin")
					assert.False(t, hasStorage, "output_storage_path must be omitted for non-admin")
				}
			}
		})
	}
}

func TestNewExecutionsResponse_HidesOutputFieldsForNonAdmin(t *testing.T) {
	inline := "stdout chunk"
	storagePath := "s3://bucket/exec.log"
	now := time.Now()

	execID := xid.New()
	execs := []domain.ServerTaskExecution{
		{
			ID:                1,
			ExecutionID:       execID,
			ServerTaskID:      10,
			ServerID:          5,
			NodeID:            2,
			Command:           domain.ServerTaskCommandRestart,
			TaskVersion:       1,
			Status:            domain.ServerTaskExecutionStatusSuccess,
			StartedAt:         now,
			OutputInline:      &inline,
			OutputStoragePath: &storagePath,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}

	t.Run("admin_keeps_output_fields", func(t *testing.T) {
		resp := newExecutionsResponse(execs, true)
		require.Len(t, resp.Data, 1)
		require.NotNil(t, resp.Data[0].OutputInline)
		require.NotNil(t, resp.Data[0].OutputStoragePath)
		assert.Equal(t, inline, *resp.Data[0].OutputInline)
		assert.Equal(t, storagePath, *resp.Data[0].OutputStoragePath)
	})

	t.Run("non_admin_loses_output_fields", func(t *testing.T) {
		resp := newExecutionsResponse(execs, false)
		require.Len(t, resp.Data, 1)
		assert.Nil(t, resp.Data[0].OutputInline)
		assert.Nil(t, resp.Data[0].OutputStoragePath)
		assert.Equal(t, execID, resp.Data[0].ExecutionID)
		assert.Equal(t, "restart", resp.Data[0].Command)
		assert.Equal(t, "success", resp.Data[0].Status)
	})
}

func newTestRepos() *testRepos {
	servers := inmemory.NewServerRepository()

	return &testRepos{
		tasks:      inmemory.NewServerTaskRepository(servers),
		executions: inmemory.NewServerTaskExecutionRepository(),
		servers:    servers,
		rbac:       inmemory.NewRBACRepository(),
	}
}

func contextWithUser(userID uint, login string) context.Context {
	session := &auth.Session{
		Login: login,
		Email: login + "@example.com",
		User: &domain.User{
			ID:    userID,
			Login: login,
			Email: login + "@example.com",
		},
	}

	return auth.ContextWithSession(context.Background(), session)
}

func seedServer(repo *inmemory.ServerRepository) {
	now := time.Now()
	server := &domain.Server{
		ID:         1,
		UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:  "short1",
		Name:       "Test Server",
		GameID:     "cs",
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		Dir:        "/home/gameap/servers/test1",
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	_ = repo.Save(context.Background(), server)
}

func seedTask(t *testing.T, repo *inmemory.ServerTaskRepository) *domain.ServerTask {
	t.Helper()

	now := time.Now()
	task := &domain.ServerTask{
		Command:      domain.ServerTaskCommandRestart,
		ServerID:     1,
		Repeat:       0,
		RepeatPeriod: 0,
		Counter:      0,
		ExecuteDate:  now.Add(time.Hour),
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}
	require.NoError(t, repo.Save(context.Background(), task))

	return task
}

func seedExecutionWithOutput(
	t *testing.T,
	repo *inmemory.ServerTaskExecutionRepository,
	taskID uint,
	serverID uint,
) {
	t.Helper()

	inline := testOutputInline
	storagePath := testOutputStoragePath
	now := time.Now()

	exec := &domain.ServerTaskExecution{
		ExecutionID:       xid.New(),
		ServerTaskID:      taskID,
		ServerID:          serverID,
		NodeID:            1,
		Command:           domain.ServerTaskCommandRestart,
		TaskVersion:       1,
		Status:            domain.ServerTaskExecutionStatusSuccess,
		StartedAt:         now,
		OutputInline:      &inline,
		OutputStoragePath: &storagePath,
	}
	require.NoError(t, repo.Create(context.Background(), exec))
}

func seedExecutionWithStatus(
	t *testing.T,
	repo *inmemory.ServerTaskExecutionRepository,
	taskID uint,
	serverID uint,
	executionID xid.ID,
	status domain.ServerTaskExecutionStatus,
) {
	t.Helper()

	now := time.Now()
	exec := &domain.ServerTaskExecution{
		ExecutionID:  executionID,
		ServerTaskID: taskID,
		ServerID:     serverID,
		NodeID:       1,
		Command:      domain.ServerTaskCommandRestart,
		TaskVersion:  1,
		Status:       status,
		StartedAt:    now,
	}
	require.NoError(t, repo.Create(context.Background(), exec))
}

func seedAdminPermission(repo *inmemory.RBACRepository) {
	ability := &domain.Ability{
		Name:  domain.AbilityNameAdminRolesPermissions,
		Title: new("Admin Permissions"),
	}
	_ = repo.SaveAbility(context.Background(), ability)

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(uint(1)),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}
	_ = repo.SavePermission(context.Background(), permission)
}

func seedServerTasksAbility(repo *inmemory.RBACRepository, userID uint, serverID uint) {
	ability := &domain.Ability{
		Name:       domain.AbilityNameGameServerTasks,
		Title:      new("Server Tasks"),
		EntityID:   new(serverID),
		EntityType: lo.ToPtr(domain.EntityTypeServer),
	}
	_ = repo.SaveAbility(context.Background(), ability)

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(userID),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}
	_ = repo.SavePermission(context.Background(), permission)
}
