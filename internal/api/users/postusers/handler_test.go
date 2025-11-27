package postusers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/samber/lo"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/flexible"
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
		requestBody    any
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.UserRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectUser     bool
	}{
		{
			name: "successful user creation",
			requestBody: createUserInput{
				Login:    "newuser",
				Email:    "newuser@example.com",
				Password: "testpass123",
				Name:     lo.ToPtr("New User"),
				Roles:    []string{"user"},
				Servers:  []flexible.Uint{},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.UserRepository, rbac *inmemory.RBACRepository) {
				require.NoError(t, rbac.SaveRole(
					context.Background(),
					&domain.Role{Name: "user"},
				))
			},
			expectedStatus: http.StatusCreated,
			expectUser:     true,
		},
		{
			name: "successful user creation without name",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "testuser@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			expectedStatus: http.StatusCreated,
			expectUser:     true,
		},
		{
			name: "user not authenticated",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "test@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectUser:     false,
		},
		{
			name: "invalid input - empty login",
			requestBody: createUserInput{
				Login:    "",
				Email:    "test@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "login is required",
			expectUser:     false,
		},
		{
			name: "invalid input - empty email",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			name: "invalid input - invalid email",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "invalid-email",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			name: "invalid input - empty password",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "test@example.com",
				Password: "",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "password is required",
			expectUser:     false,
		},
		{
			name: "invalid input - password too short",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "test@example.com",
				Password: "short",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
		{
			name: "duplicate login",
			requestBody: createUserInput{
				Login:    "existinguser",
				Email:    "new@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
				user := &domain.User{
					ID:       2,
					Login:    "existinguser",
					Email:    "existing@example.com",
					Password: "$2a$10$test",
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusConflict,
			wantError:      "user with this login already exists",
			expectUser:     false,
		},
		{
			name: "duplicate email",
			requestBody: createUserInput{
				Login:    "newuser",
				Email:    "existing@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
				user := &domain.User{
					ID:       2,
					Login:    "existinguser",
					Email:    "existing@example.com",
					Password: "$2a$10$test",
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusConflict,
			wantError:      "user with this email already exists",
			expectUser:     false,
		},
		{
			name: "invalid input - login too long",
			requestBody: createUserInput{
				Login:    string(make([]byte, 256)),
				Email:    "test@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "login must not exceed 255 characters",
			expectUser:     false,
		},
		{
			name: "invalid input - email too long",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    string(make([]byte, 256)),
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "email must not exceed 255 characters",
			expectUser:     false,
		},
		{
			name: "invalid input - name too long",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "test@example.com",
				Password: "password123",
				Name:     lo.ToPtr(string(make([]byte, 256))),
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "name must not exceed 255 characters",
			expectUser:     false,
		},
		{
			name: "invalid input - password too long",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "test@example.com",
				Password: string(make([]byte, 256)),
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "password must not exceed 64 characters",
			expectUser:     false,
		},
		{
			name: "invalid input - login with only spaces",
			requestBody: createUserInput{
				Login:    "   ",
				Email:    "test@example.com",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "login cannot be empty",
			expectUser:     false,
		},
		{
			name: "invalid input - email with only spaces",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "   ",
				Password: "password123",
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "email cannot be empty",
			expectUser:     false,
		},
		{
			name: "invalid input - name with only spaces",
			requestBody: createUserInput{
				Login:    "testuser",
				Email:    "test@example.com",
				Password: "password123",
				Name:     lo.ToPtr("   "),
				Roles:    []string{},
				Servers:  []flexible.Uint{},
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
			wantError:      "name cannot be empty",
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

			req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(bodyBytes))
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
			}

			if tt.expectUser {
				var user userResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
				assert.NotZero(t, user.ID)
				assert.NotEmpty(t, user.Login)
				assert.NotEmpty(t, user.Email)
				assert.NotNil(t, user.CreatedAt)
				assert.NotNil(t, user.UpdatedAt)
			}
		})
	}
}

func TestHandler_CreateUserWithRoles(t *testing.T) {
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

	userRole := &domain.Role{
		Name: "user",
	}
	require.NoError(t, rbacRepo.SaveRole(context.Background(), userRole))

	requestBody := createUserInput{
		Login:    "newuser",
		Email:    "newuser@example.com",
		Password: "password123",
		Name:     lo.ToPtr("New User"),
		Roles:    []string{"user"},
		Servers:  []flexible.Uint{},
	}

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var userResp userResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &userResp))

	assert.NotZero(t, userResp.ID)
	assert.Equal(t, "newuser", userResp.Login)
	assert.Equal(t, "newuser@example.com", userResp.Email)
	require.NotNil(t, userResp.Name)
	assert.Equal(t, "New User", *userResp.Name)
	assert.NotNil(t, userResp.CreatedAt)
	assert.NotNil(t, userResp.UpdatedAt)
	assert.NotNil(t, userResp.Roles)
}

func TestHandler_InvalidJSON(t *testing.T) {
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

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte("invalid json")))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
}

func TestNewUserResponseFromUser(t *testing.T) {
	name := "Test User"

	user := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
		Name:  &name,
	}

	response := newUserResponseFromUser(user, []string{"admin", "user"})

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "testuser", response.Login)
	assert.Equal(t, "test@example.com", response.Email)
	require.NotNil(t, response.Name)
	assert.Equal(t, "Test User", *response.Name)
	require.Len(t, response.Roles, 2)
	assert.Contains(t, response.Roles, "admin")
	assert.Contains(t, response.Roles, "user")
}

func TestNewUserResponseFromUser_NoRoles(t *testing.T) {
	user := &domain.User{
		ID:    1,
		Login: "noroles",
		Email: "noroles@example.com",
	}

	response := newUserResponseFromUser(user, []string{})

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "noroles", response.Login)
	assert.Equal(t, "noroles@example.com", response.Email)
	assert.Nil(t, response.Name)
	assert.NotNil(t, response.Roles)
	assert.Empty(t, response.Roles)
}
