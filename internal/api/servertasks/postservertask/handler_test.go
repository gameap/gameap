package postservertask

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
	_ *inmemory.ServerTaskRepository,
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
		requestBody      any
		wantStatus       int
		wantError        string
		validateResponse func(t *testing.T, r serverTaskResponse)
	}{
		{
			name:       "successful task creation with admin user",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
				"payload":      "test payload",
			},
			wantStatus: http.StatusCreated,
			validateResponse: func(t *testing.T, r serverTaskResponse) {
				t.Helper()

				assert.Equal(t, "restart", r.Command)
				assert.Equal(t, uint(1), r.ServerID)
				assert.Equal(t, uint8(1), r.Repeat)
				assert.NotNil(t, r.Payload)
				assert.Equal(t, "test payload", *r.Payload)
			},
		},
		{
			name:       "successful task creation with repeat period",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			requestBody: map[string]any{
				"command":       "update",
				"repeat":        5,
				"repeat_period": "1 hour",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusCreated,
			validateResponse: func(t *testing.T, r serverTaskResponse) {
				t.Helper()

				assert.Equal(t, "update", r.Command)
				assert.Equal(t, uint(1), r.ServerID)
				assert.Equal(t, uint8(5), r.Repeat)
				assert.Equal(t, "1 hour", r.RepeatPeriod)
			},
		},
		{
			name: "unauthenticated request",
			setupRepos: func(
				_ *inmemory.ServerTaskRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) error {
				// No setup needed
				return nil
			},
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
		{
			name:       "empty command",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
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
			requestBody: map[string]any{
				"command": "restart",
				"repeat":  1,
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed",
		},
		{
			name:       "repeat_period is required",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
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
			name:       "repeat_period is invalid format",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
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
			name:       "repeat_period with invalid unit",
			setupAuth:  defaultSetupAuth,
			setupRepos: defaultSetupRepos,
			requestBody: map[string]any{
				"command":       "update",
				"repeat":        5,
				"repeat_period": "1 millennium",
				"execute_date":  time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "repeat_period must match format: '<number> <unit>",
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
				_ *inmemory.ServerTaskRepository,
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

				// User has no access to this server
				return nil
			},
			requestBody: map[string]any{
				"command":      "restart",
				"repeat":       1,
				"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusForbidden,
			wantError:  "access to server denied",
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
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/servers/1/tasks", bytes.NewReader(body))
			req = req.WithContext(ctx)

			// Set URL variables
			req = mux.SetURLVars(req, map[string]string{"server": "1"})

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
			} else {
				require.Equal(t, tt.wantStatus, rr.Code)

				var response serverTaskResponse
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				if tt.validateResponse != nil {
					tt.validateResponse(t, response)
				}
			}
		})
	}
}
