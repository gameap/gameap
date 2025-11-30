package deleteserver

import (
	"bytes"
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
	"github.com/samber/lo"
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
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

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
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

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
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, daemonTaskRepo, handler.daemonTaskRepo)
	assert.Equal(t, rbacService, handler.rbac)
	assert.Equal(t, responder, handler.responder)
}

func TestHandler_DeleteFiles(t *testing.T) {
	t.Run("delete_files_false_hard_deletes_server", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:         1,
			UUID:       u,
			UUIDShort:  u.String()[0:8],
			Enabled:    true,
			Installed:  1,
			Name:       "Test Server",
			GameID:     "cstrike",
			DSID:       1,
			ServerIP:   "192.168.1.1",
			ServerPort: 27015,
			CreatedAt:  &now,
			UpdatedAt:  &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		body := bytes.NewBufferString(`{"delete_files":false}`)
		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusNoContent, w.Code)

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}, WithDeleted: true}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, servers)

		tasks, err := daemonTaskRepo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("delete_files_true_offline_server_creates_delete_task", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:            1,
			UUID:          u,
			UUIDShort:     u.String()[0:8],
			Enabled:       true,
			Installed:     1,
			Name:          "Test Server",
			GameID:        "cstrike",
			DSID:          1,
			ServerIP:      "192.168.1.1",
			ServerPort:    27015,
			ProcessActive: false,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		body := bytes.NewBufferString(`{"delete_files":true}`)
		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusNoContent, w.Code)

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, servers)

		serversWithDeleted, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}, WithDeleted: true}, nil, nil)
		require.NoError(t, err)
		require.Len(t, serversWithDeleted, 1)
		assert.NotNil(t, serversWithDeleted[0].DeletedAt)

		tasks, err := daemonTaskRepo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.Equal(t, domain.DaemonTaskTypeServerDelete, tasks[0].Task)
		assert.Nil(t, tasks[0].RunAftID)
	})

	t.Run("delete_files_true_online_server_creates_stop_and_delete_tasks", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:               1,
			UUID:             u,
			UUIDShort:        u.String()[0:8],
			Enabled:          true,
			Installed:        1,
			Name:             "Test Server",
			GameID:           "cstrike",
			DSID:             1,
			ServerIP:         "192.168.1.1",
			ServerPort:       27015,
			ProcessActive:    true,
			LastProcessCheck: lo.ToPtr(time.Now()),
			CreatedAt:        &now,
			UpdatedAt:        &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		body := bytes.NewBufferString(`{"delete_files":true}`)
		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusNoContent, w.Code)

		serversWithDeleted, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}, WithDeleted: true}, nil, nil)
		require.NoError(t, err)
		require.Len(t, serversWithDeleted, 1)
		assert.NotNil(t, serversWithDeleted[0].DeletedAt)

		tasks, err := daemonTaskRepo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, tasks, 2)

		var stopTask, deleteTask *domain.DaemonTask
		for i := range tasks {
			if tasks[i].Task == domain.DaemonTaskTypeServerStop {
				stopTask = &tasks[i]
			}
			if tasks[i].Task == domain.DaemonTaskTypeServerDelete {
				deleteTask = &tasks[i]
			}
		}

		require.NotNil(t, stopTask)
		require.NotNil(t, deleteTask)
		assert.Nil(t, stopTask.RunAftID)
		assert.NotNil(t, deleteTask.RunAftID)
		assert.Equal(t, stopTask.ID, *deleteTask.RunAftID)
	})

	t.Run("delete_files_false_returns_conflict_when_server_is_online", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:               1,
			UUID:             u,
			UUIDShort:        u.String()[0:8],
			Enabled:          true,
			Installed:        1,
			Name:             "Test Server",
			GameID:           "cstrike",
			DSID:             1,
			ServerIP:         "192.168.1.1",
			ServerPort:       27015,
			ProcessActive:    true,
			LastProcessCheck: lo.ToPtr(time.Now()),
			CreatedAt:        &now,
			UpdatedAt:        &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		body := bytes.NewBufferString(`{"delete_files":false}`)
		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)

		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "error", response["status"])
		errorMsg, ok := response["error"].(string)
		require.True(t, ok)
		assert.Contains(t, errorMsg, "server is online")

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 1)
	})

	t.Run("delete_files_false_returns_conflict_when_server_has_pending_tasks", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:            1,
			UUID:          u,
			UUIDShort:     u.String()[0:8],
			Enabled:       true,
			Installed:     1,
			Name:          "Test Server",
			GameID:        "cstrike",
			DSID:          1,
			ServerIP:      "192.168.1.1",
			ServerPort:    27015,
			ProcessActive: false,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		pendingTask := &domain.DaemonTask{
			DedicatedServerID: server.DSID,
			ServerID:          lo.ToPtr(server.ID),
			Task:              domain.DaemonTaskTypeServerUpdate,
			Status:            domain.DaemonTaskStatusWaiting,
			CreatedAt:         lo.ToPtr(time.Now()),
			UpdatedAt:         lo.ToPtr(time.Now()),
		}
		require.NoError(t, daemonTaskRepo.Save(context.Background(), pendingTask))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		body := bytes.NewBufferString(`{"delete_files":false}`)
		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)

		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "error", response["status"])
		errorMsg, ok := response["error"].(string)
		require.True(t, ok)
		assert.Contains(t, errorMsg, "server has pending tasks")

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 1)
	})

	t.Run("no_body_returns_conflict_when_server_is_online", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:               1,
			UUID:             u,
			UUIDShort:        u.String()[0:8],
			Enabled:          true,
			Installed:        1,
			Name:             "Test Server",
			GameID:           "cstrike",
			DSID:             1,
			ServerIP:         "192.168.1.1",
			ServerPort:       27015,
			ProcessActive:    true,
			LastProcessCheck: lo.ToPtr(time.Now()),
			CreatedAt:        &now,
			UpdatedAt:        &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", nil)
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)

		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "error", response["status"])
		errorMsg, ok := response["error"].(string)
		require.True(t, ok)
		assert.Contains(t, errorMsg, "server is online")

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 1)
	})

	t.Run("no_body_returns_conflict_when_server_has_working_task", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:            1,
			UUID:          u,
			UUIDShort:     u.String()[0:8],
			Enabled:       true,
			Installed:     1,
			Name:          "Test Server",
			GameID:        "cstrike",
			DSID:          1,
			ServerIP:      "192.168.1.1",
			ServerPort:    27015,
			ProcessActive: false,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		workingTask := &domain.DaemonTask{
			DedicatedServerID: server.DSID,
			ServerID:          lo.ToPtr(server.ID),
			Task:              domain.DaemonTaskTypeServerStart,
			Status:            domain.DaemonTaskStatusWorking,
			CreatedAt:         lo.ToPtr(time.Now()),
			UpdatedAt:         lo.ToPtr(time.Now()),
		}
		require.NoError(t, daemonTaskRepo.Save(context.Background(), workingTask))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", nil)
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusConflict, w.Code)

		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "error", response["status"])
		errorMsg, ok := response["error"].(string)
		require.True(t, ok)
		assert.Contains(t, errorMsg, "server has pending tasks")

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, servers, 1)
	})

	t.Run("online_server_with_delete_files_true_succeeds", func(t *testing.T) {
		serverRepo := inmemory.NewServerRepository()
		daemonTaskRepo := inmemory.NewDaemonTaskRepository()
		rbacRepo := inmemory.NewRBACRepository()
		rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
		responder := api.NewResponder()
		handler := NewHandler(serverRepo, daemonTaskRepo, rbacService, responder)

		now := time.Now()
		u := uuid.New()
		server := &domain.Server{
			ID:               1,
			UUID:             u,
			UUIDShort:        u.String()[0:8],
			Enabled:          true,
			Installed:        1,
			Name:             "Test Server",
			GameID:           "cstrike",
			DSID:             1,
			ServerIP:         "192.168.1.1",
			ServerPort:       27015,
			ProcessActive:    true,
			LastProcessCheck: lo.ToPtr(time.Now()),
			CreatedAt:        &now,
			UpdatedAt:        &now,
		}
		require.NoError(t, serverRepo.Save(context.Background(), server))

		adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
		require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
		require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser1.ID, adminAbility.ID))

		session := &auth.Session{Login: "admin", Email: "admin@example.com", User: &testUser1}
		ctx := auth.ContextWithSession(context.Background(), session)

		body := bytes.NewBufferString(`{"delete_files":true}`)
		req := httptest.NewRequest(http.MethodDelete, "/api/servers/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "1"})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusNoContent, w.Code)

		servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, servers)

		serversWithDeleted, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{1}, WithDeleted: true}, nil, nil)
		require.NoError(t, err)
		require.Len(t, serversWithDeleted, 1)
		assert.NotNil(t, serversWithDeleted[0].DeletedAt)

		tasks, err := daemonTaskRepo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, tasks, 2)
	})
}
