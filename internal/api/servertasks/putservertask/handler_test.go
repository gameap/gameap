package putservertask

import (
	"bytes"
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

	// Add existing server task
	existingTask := &domain.ServerTask{
		ID:           1,
		Command:      "start",
		ServerID:     1,
		Repeat:       1,
		RepeatPeriod: 0,
		Counter:      0,
		ExecuteDate:  now.Add(time.Hour),
		Payload:      lo.ToPtr("original payload"),
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}
	err = taskRepo.Save(context.Background(), existingTask)
	if err != nil {
		return err
	}

	// Add task for another server to test isolation
	anotherServerTask := &domain.ServerTask{
		ID:           2,
		Command:      "stop",
		ServerID:     2,
		Repeat:       1,
		RepeatPeriod: 0,
		Counter:      0,
		ExecuteDate:  now.Add(2 * time.Hour),
		Payload:      lo.ToPtr("another task"),
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}
	err = taskRepo.Save(context.Background(), anotherServerTask)
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
		name             string
		setupAuth        func() context.Context
		setupRepos       func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository, *inmemory.RBACRepository) error
		taskID           string
		serverID         string
		requestBody      any
		wantStatus       int
		wantError        string
		validateResponse func(t *testing.T, r serverTaskResponse)
	}{
		{
			name:       "successful task update with admin user",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":       "restart",
				"repeat":        3,
				"repeat_period": "2 hours",
				"execute_date":  time.Now().Add(2 * time.Hour).Format(time.RFC3339),
				"payload":       "updated payload",
			},
			wantStatus: http.StatusOK,
			validateResponse: func(t *testing.T, r serverTaskResponse) {
				t.Helper()

				assert.Equal(t, uint(1), r.ID)
				assert.Equal(t, "restart", r.Command)
				assert.Equal(t, uint(1), r.ServerID)
				assert.Equal(t, uint8(3), r.Repeat)
				assert.Equal(t, "2 hours", r.RepeatPeriod)
				assert.NotNil(t, r.Payload)
				assert.Equal(t, "updated payload", *r.Payload)
				assert.Equal(t, uint(0), r.Counter)
			},
		},
		{
			name:       "successful task update - change command only",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "stop",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusOK,
			validateResponse: func(t *testing.T, r serverTaskResponse) {
				t.Helper()

				assert.Equal(t, uint(1), r.ID)
				assert.Equal(t, "stop", r.Command)
				assert.Equal(t, uint(1), r.ServerID)
				assert.Equal(t, uint8(1), r.Repeat)
			},
		},
		{
			name: "unauthenticated request",
			setupRepos: func(
				taskRepo *inmemory.ServerTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) error {
				return defaultSetupRepos(taskRepo, serverRepo, rbacRepo)
			},
			taskID:   "1",
			serverID: "1",
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
		{
			name:       "task not found",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "999",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusNotFound,
			wantError:  "server task not found",
		},
		{
			name:       "task belongs to different server",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "2",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusNotFound,
			wantError:  "server task not found",
		},
		{
			name:       "empty command",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed: command is required",
		},
		{
			name:       "invalid command",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "invalid_command",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed: invalid command, must be one of: start, stop, restart, update, reinstall",
		},
		{
			name:       "invalid repeat value",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":       "update",
				"repeat":        1000,
				"repeat_period": "1 hour",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed: repeat must be between 0 and 255",
		},
		{
			name:       "missing execute_date",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command": "restart",
				"repeat":  1,
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed",
		},
		{
			name:       "repeat_period is required when repeat > 1",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "update",
				"repeat":       200,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed: repeat_period is required when repeat is not 1",
		},
		{
			name:       "repeat_period less than 10 minutes",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":       "update",
				"repeat":        5,
				"repeat_period": "1 minute",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "10 minutes is minimum repeat period",
		},
		{
			name:       "repeat_period is too long",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":       "update",
				"repeat":        5,
				"repeat_period": "1000 days",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "repeat period is too long",
		},
		{
			name:       "invalid repeat_period format",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":       "update",
				"repeat":        5,
				"repeat_period": "invalid format",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed: repeat_period must match format",
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
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access to server denied",
		},
		{
			name:       "invalid server id",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "1",
			serverID:   "invalid",
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid server id",
		},
		{
			name:       "invalid task id",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			taskID:     "invalid",
			serverID:   "1",
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task id",
		},
		{
			name:      "preserves counter on update",
			setupAuth: defaultSetupAuth,
			setupRepos: func(
				taskRepo *inmemory.ServerTaskRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) error {
				err := defaultSetupRepos(taskRepo, serverRepo, rbacRepo)
				if err != nil {
					return err
				}

				// Update task to have a counter value
				tasks, _ := taskRepo.Find(context.Background(), nil, nil, nil)
				for i := range tasks {
					if tasks[i].ID == 1 {
						tasks[i].Counter = 5
						_ = taskRepo.Save(context.Background(), &tasks[i])

						break
					}
				}

				return nil
			},
			taskID:   "1",
			serverID: "1",
			requestBody: map[string]any{
				"command":       "restart",
				"repeat":        10,
				"repeat_period": "1 hour",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusOK,
			validateResponse: func(t *testing.T, r serverTaskResponse) {
				t.Helper()

				assert.Equal(t, uint(5), r.Counter)
				assert.Equal(t, "restart", r.Command)
				assert.Equal(t, uint8(10), r.Repeat)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			taskRepo := inmemory.NewServerTaskRepository(serverRepo)
			rbacRepo := inmemory.NewRBACRepository()

			if tt.setupRepos != nil {
				err := tt.setupRepos(taskRepo, serverRepo, rbacRepo)
				require.NoError(t, err)
			}

			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()

			handler := NewHandler(taskRepo, serverRepo, rbacService, responder)

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, "/api/servers/"+tt.serverID+"/tasks/"+tt.taskID, bytes.NewBuffer(body))

			if tt.setupAuth != nil {
				ctx := tt.setupAuth()
				req = req.WithContext(ctx)
			}

			req = mux.SetURLVars(req, map[string]string{
				"server": tt.serverID,
				"id":     tt.taskID,
			})

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantError != "" {
				var errorResp map[string]any
				err := json.NewDecoder(recorder.Body).Decode(&errorResp)
				require.NoError(t, err)

				errorMsg := ""
				if msg, ok := errorResp["message"].(string); ok {
					errorMsg = msg
				} else if msg, ok := errorResp["error"].(string); ok {
					errorMsg = msg
				}
				assert.Contains(t, errorMsg, tt.wantError)
			} else if recorder.Code != http.StatusOK {
				// Debug: print response when test fails
				t.Logf("Unexpected status code %d. Response body: %s", recorder.Code, recorder.Body.String())
			}

			if tt.validateResponse != nil {
				var response serverTaskResponse
				err := json.NewDecoder(recorder.Body).Decode(&response)
				require.NoError(t, err)

				tt.validateResponse(t, response)
			}
		})
	}
}
