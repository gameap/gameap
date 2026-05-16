package putuser

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

var testUser1 = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

const originalUserName = "Original User"

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
				Name:     new("Updated User"),
				Password: new("newpassword123"),
				Roles:    []string{"user"},
				Servers:  []flexible.Uint{1, 2},
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
				name := originalUserName

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
				Name:     new("Updated User"),
				Password: new(""),
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
			setupRepo: func(usersRepo *inmemory.UserRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				name := originalUserName

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
				Servers: []flexible.Uint{},
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
				Servers: []flexible.Uint{},
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
				Servers: []flexible.Uint{},
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
				Servers: []flexible.Uint{},
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
				Servers: []flexible.Uint{},
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
				Servers: []flexible.Uint{},
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
				Password: new("short"),
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
			name:        "flexible_uint_parsing_with_string_server_ids",
			userID:      "1",
			requestBody: `{"email": "test@example.com", "roles": [], "servers": ["1", "2", "3"]}`,
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
					ID:        1,
					Login:     "testuser",
					Email:     "original@example.com",
					Password:  "$2a$10$test",
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, usersRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusOK,
			expectUser:     true,
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
				nil,
			)

			if tt.setupRepo != nil {
				tt.setupRepo(usersRepo, rbacRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			var bodyBytes []byte
			var err error
			if strBody, ok := tt.requestBody.(string); ok {
				bodyBytes = []byte(strBody)
			} else {
				bodyBytes, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

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
		nil,
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

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — an administrative user mutation
//     (and especially a role assignment, a privilege change) that succeeds
//     without being recorded is a detective-control gap (OWASP ASVS §7.2.1).
//     The events must be scoped to the target user and attributed to the
//     acting principal.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

func setupPutUserAudit(t *testing.T) (*Handler, *auditCapture) {
	t.Helper()

	usersRepo := inmemory.NewUserRepository()
	userService := services.NewUserService(usersRepo)
	rbacRepo := inmemory.NewRBACRepository()
	serversRepo := inmemory.NewServerRepository()
	recorder := &auditCapture{}
	handler := NewHandler(
		userService,
		serversRepo,
		rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
		services.NewNilTransactionManager(),
		api.NewResponder(),
		recorder,
	)

	now := time.Now()
	name := originalUserName
	require.NoError(t, usersRepo.Save(context.Background(), &domain.User{
		ID:        1,
		Login:     "originaluser",
		Email:     "original@example.com",
		Password:  "$2a$10$test",
		Name:      &name,
		CreatedAt: &now,
		UpdatedAt: &now,
	}))
	require.NoError(t, rbacRepo.SaveRole(context.Background(), &domain.Role{Name: "user"}))

	return handler, recorder
}

func doPutUser(t *testing.T, handler *Handler, body any) *httptest.ResponseRecorder {
	t.Helper()

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/users/1", bytes.NewReader(bodyBytes))
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	return w
}

// TestHandler_Audit_UserUpdateWithRolesEmitsBothEvents covers OWASP
// API8:2023. Updating a user while supplying roles must emit a user.update
// event AND a user.roles.assign event, both with outcome success, category
// admin_op, scoped to the target user, and attributed to the acting admin.
func TestHandler_Audit_UserUpdateWithRolesEmitsBothEvents(t *testing.T) {
	// ARRANGE
	handler, recorder := setupPutUserAudit(t)

	// ACT
	w := doPutUser(t, handler, updateUserInput{
		Email:   "updated@example.com",
		Name:    new("Updated User"),
		Roles:   []string{"user"},
		Servers: []flexible.Uint{},
	})

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "update must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()

	upd, ok := findEvent(events, audit.EventUserUpdate)
	require.True(t, ok, "a successful user update must leave a user.update audit event")
	assert.Equal(t, audit.OutcomeSuccess, upd.Outcome)
	assert.Equal(t, audit.CategoryAdminOp, upd.Category)
	assert.Equal(t, "user", upd.ResourceType)
	assert.Equal(t, "1", upd.ResourceID, "the updated user id must be recorded")
	assert.Equal(t, "update", upd.Action)
	assert.Equal(t, testUser1.ID, upd.ActorID, "the acting admin must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, upd.AuthMethod)

	roles, ok := findEvent(events, audit.EventUserRolesAssign)
	require.True(t, ok, "supplying roles must leave a user.roles.assign audit event")
	assert.Equal(t, audit.OutcomeSuccess, roles.Outcome)
	assert.Equal(t, audit.CategoryAdminOp, roles.Category)
	assert.Equal(t, "user", roles.ResourceType)
	assert.Equal(t, "1", roles.ResourceID)
	assert.Equal(t, "role_assign", roles.Action)
	assert.Equal(t, testUser1.ID, roles.ActorID)
}

// TestHandler_Audit_UserUpdateWithoutRolesEmitsOnlyUpdate covers OWASP
// API8:2023. Updating a user without supplying any roles must emit the
// user.update event but must NOT emit a user.roles.assign event (the audit
// trail must not claim a privilege change that did not happen).
func TestHandler_Audit_UserUpdateWithoutRolesEmitsOnlyUpdate(t *testing.T) {
	// ARRANGE
	handler, recorder := setupPutUserAudit(t)

	// ACT
	w := doPutUser(t, handler, updateUserInput{
		Email:   "updated@example.com",
		Name:    new("Updated User"),
		Roles:   []string{},
		Servers: []flexible.Uint{},
	})

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "update must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()
	assert.Equal(t, 1, countEvents(events, audit.EventUserUpdate),
		"exactly one user.update event must be emitted")
	assert.Equal(t, 0, countEvents(events, audit.EventUserRolesAssign),
		"no roles supplied means no user.roles.assign event may be recorded")
}
