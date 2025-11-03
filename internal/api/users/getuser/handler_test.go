package getuser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
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
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.UserRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectUser     bool
	}{
		{
			name:   "successful user retrieval",
			userID: "1",
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
				name := "Admin User"

				user := &domain.User{
					ID:        1,
					Login:     "admin",
					Email:     "admin@example.com",
					Name:      &name,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))

				adminRole := &domain.Role{
					Name: "admin",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), adminRole))

				assignedRole := &domain.AssignedRole{
					RoleID:     adminRole.ID,
					EntityID:   user.ID,
					EntityType: domain.EntityTypeUser,
				}
				require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), assignedRole))
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name:   "successful user retrieval with multiple roles",
			userID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "testuser@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(usersRepo *inmemory.UserRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				name := "Test User"

				user := &domain.User{
					ID:        1,
					Login:     "testuser",
					Email:     "testuser@example.com",
					Name:      &name,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))

				adminRole := &domain.Role{
					Name: "admin",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), adminRole))

				userRole := &domain.Role{
					Name: "user",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), userRole))

				require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), &domain.AssignedRole{
					RoleID:     adminRole.ID,
					EntityID:   user.ID,
					EntityType: domain.EntityTypeUser,
				}))

				require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), &domain.AssignedRole{
					RoleID:     userRole.ID,
					EntityID:   user.ID,
					EntityType: domain.EntityTypeUser,
				}))
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
		},
		{
			name:   "user not found",
			userID: "999",
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
			name:           "user not authenticated",
			userID:         "1",
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectUser:     false,
		},
		{
			name:   "invalid user id",
			userID: "invalid",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacRepo := inmemory.NewRBACRepository()
			responder := api.NewResponder()
			usersRepo := inmemory.NewUserRepository()
			usersService := services.NewUserService(usersRepo)

			handler := NewHandler(usersService, rbacRepo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(usersRepo, rbacRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/users/"+tt.userID, nil)
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
				assert.NotNil(t, user.Roles)
			}
		})
	}
}

func TestHandler_UserResponseFields(t *testing.T) {
	usersRepo := inmemory.NewUserRepository()
	rbacRepo := inmemory.NewRBACRepository()
	responder := api.NewResponder()
	handler := NewHandler(usersRepo, rbacRepo, responder)

	now := time.Now()
	name := "New Name"

	user := &domain.User{
		ID:        1,
		Login:     "admin",
		Email:     "admin@yousite.local",
		Name:      &name,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, usersRepo.Save(context.Background(), user))

	adminRole := &domain.Role{
		Name: "admin",
	}
	require.NoError(t, rbacRepo.SaveRole(context.Background(), adminRole))

	userRole := &domain.Role{
		Name: "user",
	}
	require.NoError(t, rbacRepo.SaveRole(context.Background(), userRole))

	require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), &domain.AssignedRole{
		RoleID:     adminRole.ID,
		EntityID:   user.ID,
		EntityType: domain.EntityTypeUser,
	}))

	require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), &domain.AssignedRole{
		RoleID:     userRole.ID,
		EntityID:   user.ID,
		EntityType: domain.EntityTypeUser,
	}))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@yousite.local",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/users/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var userResp userResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &userResp))

	assert.Equal(t, uint(1), userResp.ID)
	assert.Equal(t, "admin", userResp.Login)
	assert.Equal(t, "admin@yousite.local", userResp.Email)
	require.NotNil(t, userResp.Name)
	assert.Equal(t, "New Name", *userResp.Name)
	assert.NotNil(t, userResp.CreatedAt)
	assert.NotNil(t, userResp.UpdatedAt)
	require.NotNil(t, userResp.Roles)
	assert.Len(t, userResp.Roles, 2)
	assert.Contains(t, userResp.Roles, "admin")
	assert.Contains(t, userResp.Roles, "user")
}

func TestHandler_NewHandler(t *testing.T) {
	usersRepo := inmemory.NewUserRepository()
	rbacRepo := inmemory.NewRBACRepository()
	responder := api.NewResponder()

	handler := NewHandler(usersRepo, rbacRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, usersRepo, handler.usersRepo)
	assert.Equal(t, rbacRepo, handler.rbacRepo)
	assert.Equal(t, responder, handler.responder)
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

	adminRole := domain.Role{
		ID:   1,
		Name: "admin",
	}

	userRole := domain.Role{
		ID:   2,
		Name: "user",
	}

	roles := []domain.RestrictedRole{
		domain.NewRestrictedRoleFromRole(adminRole),
		domain.NewRestrictedRoleFromRole(userRole),
	}

	response := newUserResponseFromUser(user, roles)

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

	roles := []domain.RestrictedRole{}

	response := newUserResponseFromUser(user, roles)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "noroles", response.Login)
	assert.Equal(t, "noroles@example.com", response.Email)
	assert.Nil(t, response.Name)
	assert.NotNil(t, response.Roles)
	assert.Empty(t, response.Roles)
}
