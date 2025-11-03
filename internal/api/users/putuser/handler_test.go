package putuser

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
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		requestBody    any
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.UserRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectUser     bool
	}{
		{
			name:   "successful user update",
			userID: "1",
			requestBody: updateUserInput{
				Email:    "updated@example.com",
				Name:     stringPtr("Updated User"),
				Password: stringPtr("newpassword123"),
				Roles:    []string{"user"},
				Servers:  []uint{1, 2},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(usersRepo *inmemory.UserRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				name := "Original User"

				user := &domain.User{
					ID:        1,
					Login:     "originaluser",
					Email:     "original@example.com",
					Password:  "$2a$10$test",
					Name:      &name,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))

				userRole := &domain.Role{
					Name: "user",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), userRole))

				assignedRole := &domain.AssignedRole{
					RoleID:     userRole.ID,
					EntityID:   user.ID,
					EntityType: domain.EntityTypeUser,
				}
				require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), assignedRole))
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name:   "successful user update without password change",
			userID: "1",
			requestBody: updateUserInput{
				Email:    "updated@example.com",
				Name:     stringPtr("Updated User"),
				Password: stringPtr(""),
				Roles:    []string{"user"},
				Servers:  []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(usersRepo *inmemory.UserRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				name := "Original User"

				user := &domain.User{
					ID:        1,
					Login:     "originaluser",
					Email:     "original@example.com",
					Password:  "$2a$10$test",
					Name:      &name,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))

				userRole := &domain.Role{
					Name: "user",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), userRole))
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name:   "successful user update without name",
			userID: "2",
			requestBody: updateUserInput{
				Email:   "newuser@example.com",
				Roles:   []string{},
				Servers: []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(usersRepo *inmemory.UserRepository, _ *inmemory.RBACRepository) {
				now := time.Now()

				user := &domain.User{
					ID:        2,
					Login:     "olduser",
					Email:     "olduser@example.com",
					Password:  "$2a$10$test",
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name:   "user not found",
			userID: "999",
			requestBody: updateUserInput{
				Email:   "test@example.com",
				Roles:   []string{},
				Servers: []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "user not found",
			expectUser:     false,
		},
		{
			name:   "user not authenticated",
			userID: "1",
			requestBody: updateUserInput{
				Email:   "test@example.com",
				Roles:   []string{},
				Servers: []uint{},
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectUser:     false,
		},
		{
			name:   "invalid user id",
			userID: "invalid",
			requestBody: updateUserInput{
				Email:   "test@example.com",
				Roles:   []string{},
				Servers: []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid user id",
			expectUser:     false,
		},
		{
			name:   "invalid input - empty email",
			userID: "1",
			requestBody: updateUserInput{
				Email:   "",
				Roles:   []string{},
				Servers: []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "email is required",
			expectUser:     false,
		},
		{
			name:   "invalid input - invalid email",
			userID: "1",
			requestBody: updateUserInput{
				Email:   "invalid-email",
				Roles:   []string{},
				Servers: []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "email is not valid",
			expectUser:     false,
		},
		{
			name:   "invalid input - password too short",
			userID: "1",
			requestBody: updateUserInput{
				Email:    "test@example.com",
				Password: stringPtr("short"),
				Roles:    []string{},
				Servers:  []uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "password must be at least 8 characters",
			expectUser:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usersRepo := inmemory.NewUserRepository()
			userService := services.NewUserService(usersRepo)
			rbacRepo := inmemory.NewRBACRepository()
			serversRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()
			handler := NewHandler(
				userService,
				serversRepo,
				rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
				services.NewNilTransactionManager(),
				responder,
			)

			if tt.setupRepo != nil {
				tt.setupRepo(usersRepo, rbacRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			bodyBytes, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, "/api/users/"+tt.userID, bytes.NewReader(bodyBytes))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.userID})
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

			if tt.expectUser {
				var user userResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
				assert.NotZero(t, user.ID)
				assert.NotEmpty(t, user.Login)
				assert.NotEmpty(t, user.Email)
			}
		})
	}
}

func TestHandler_UpdateUserFields(t *testing.T) {
	usersRepo := inmemory.NewUserRepository()
	userService := services.NewUserService(usersRepo)
	rbacRepo := inmemory.NewRBACRepository()
	serversRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(
		userService,
		serversRepo,
		rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
		services.NewNilTransactionManager(),
		responder,
	)

	now := time.Now()
	originalName := "Original Name"

	user := &domain.User{
		ID:        1,
		Login:     "originallogin",
		Email:     "original@example.com",
		Password:  "$2a$10$originalpassword",
		Name:      &originalName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, usersRepo.Save(context.Background(), user))

	adminRole := &domain.Role{
		Name: "admin",
	}
	require.NoError(t, rbacRepo.SaveRole(context.Background(), adminRole))

	require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), &domain.AssignedRole{
		RoleID:     adminRole.ID,
		EntityID:   user.ID,
		EntityType: domain.EntityTypeUser,
	}))

	newName := "Updated Name"
	newPassword := "newpassword123"
	requestBody := updateUserInput{
		Email:    "updated@example.com",
		Name:     &newName,
		Password: &newPassword,
		Roles:    []string{"admin"},
		Servers:  []uint{},
	}

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/users/1", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var userResp userResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &userResp))

	assert.Equal(t, uint(1), userResp.ID)
	assert.Equal(t, "updated@example.com", userResp.Email)
	require.NotNil(t, userResp.Name)
	assert.Equal(t, "Updated Name", *userResp.Name)
	assert.NotNil(t, userResp.CreatedAt)
	assert.NotNil(t, userResp.UpdatedAt)
	require.NotNil(t, userResp.Roles)
	assert.Contains(t, userResp.Roles, "admin")
}

func TestNewUserResponseFromUser(t *testing.T) {
	now := time.Now()
	name := "Test User"

	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      &name,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	response := newUserResponseFromUser(user, []string{"admin", "user"})

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "testuser", response.Login)
	assert.Equal(t, "test@example.com", response.Email)
	require.NotNil(t, response.Name)
	assert.Equal(t, "Test User", *response.Name)
	assert.Equal(t, &now, response.CreatedAt)
	assert.Equal(t, &now, response.UpdatedAt)
	require.Len(t, response.Roles, 2)
	assert.Contains(t, response.Roles, "admin")
	assert.Contains(t, response.Roles, "user")
}

func TestNewUserResponseFromUser_NoRoles(t *testing.T) {
	now := time.Now()

	user := &domain.User{
		ID:        1,
		Login:     "noroles",
		Email:     "noroles@example.com",
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	response := newUserResponseFromUser(user, []string{})

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "noroles", response.Login)
	assert.Equal(t, "noroles@example.com", response.Email)
	assert.Nil(t, response.Name)
	assert.NotNil(t, response.Roles)
	assert.Empty(t, response.Roles)
}

func stringPtr(s string) *string {
	return &s
}
