package getprofile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectProfile  bool
	}{
		{
			name: "successful profile retrieval",
			setupAuth: func() context.Context {
				now := time.Now()
				userName := "Test User"
				user := &domain.User{
					ID:        1,
					Login:     "testuser",
					Email:     "test@example.com",
					Name:      &userName,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  user,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(rbacRepo *inmemory.RBACRepository) {
				// Setup user role
				role := &domain.Role{
					ID:   1,
					Name: "User",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), role))

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   1,
					EntityType: domain.EntityTypeUser,
				}
				require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), assignedRole))
			},
			expectedStatus: http.StatusOK,
			expectProfile:  true,
		},
		{
			name: "user not authenticated",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo:      func(_ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectProfile:  false,
		},
		{
			name: "profile with admin role",
			setupAuth: func() context.Context {
				now := time.Now()
				userName := "Admin User"
				user := &domain.User{
					ID:        2,
					Login:     "adminuser",
					Email:     "admin@example.com",
					Name:      &userName,
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				session := &auth.Session{
					Login: "adminuser",
					Email: "admin@example.com",
					User:  user,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(rbacRepo *inmemory.RBACRepository) {
				// Setup admin role
				role := &domain.Role{
					ID:   2,
					Name: "Admin",
				}
				require.NoError(t, rbacRepo.SaveRole(context.Background(), role))

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   2,
					EntityType: domain.EntityTypeUser,
				}
				require.NoError(t, rbacRepo.SaveAssignedRole(context.Background(), assignedRole))
			},
			expectedStatus: http.StatusOK,
			expectProfile:  true,
		},
		{
			name: "profile with minimal fields",
			setupAuth: func() context.Context {
				user := &domain.User{
					ID:    3,
					Login: "minimaluser",
					Email: "minimal@example.com",
				}

				session := &auth.Session{
					Login: "minimaluser",
					Email: "minimal@example.com",
					User:  user,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusOK,
			expectProfile:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbac := inmemory.NewRBACRepository()
			responder := api.NewResponder()
			handler := NewHandler(rbac, nil, nil, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(rbac)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/profile/getprofile", nil)
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

			if tt.expectProfile {
				var profileResp profileResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profileResp))
				assert.NotZero(t, profileResp.ID)
				assert.NotEmpty(t, profileResp.Login)
				assert.NotEmpty(t, profileResp.Email)
			}
		})
	}
}

func TestHandler_ProfileResponseFields(t *testing.T) {
	rbac := inmemory.NewRBACRepository()
	responder := api.NewResponder()
	handler := NewHandler(rbac, nil, nil, responder)

	now := time.Now()
	userName := "John Doe"
	user := &domain.User{
		ID:        1,
		Login:     "johndoe",
		Email:     "john@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	// Setup role for the user
	role := &domain.Role{
		ID:   1,
		Name: "User",
	}
	require.NoError(t, rbac.SaveRole(context.Background(), role))

	assignedRole := &domain.AssignedRole{
		RoleID:     role.ID,
		EntityID:   user.ID,
		EntityType: domain.EntityTypeUser,
	}
	require.NoError(t, rbac.SaveAssignedRole(context.Background(), assignedRole))

	session := &auth.Session{
		Login: "johndoe",
		Email: "john@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/profile/getprofile", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var profileResp profileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profileResp))

	assert.Equal(t, uint(1), profileResp.ID)
	assert.Equal(t, "johndoe", profileResp.Login)
	assert.Equal(t, "john@example.com", profileResp.Email)
	require.NotNil(t, profileResp.Name)
	assert.Equal(t, "John Doe", *profileResp.Name)
	assert.Equal(t, []string{"User"}, profileResp.Roles)
}

func TestHandler_ProfileResponseWithNilFields(t *testing.T) {
	rbac := inmemory.NewRBACRepository()
	responder := api.NewResponder()
	handler := NewHandler(rbac, nil, nil, responder)

	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      nil,
		CreatedAt: nil,
		UpdatedAt: nil,
	}

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/profile/getprofile", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var profileResp profileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profileResp))

	assert.Equal(t, uint(1), profileResp.ID)
	assert.Equal(t, "testuser", profileResp.Login)
	assert.Equal(t, "test@example.com", profileResp.Email)
	assert.Nil(t, profileResp.Name)
	assert.Empty(t, profileResp.Roles)
}

func TestHandler_ProfileWithMultipleRoles(t *testing.T) {
	rbac := inmemory.NewRBACRepository()
	responder := api.NewResponder()
	handler := NewHandler(rbac, nil, nil, responder)

	now := time.Now()
	userName := "Multi Role User"
	user := &domain.User{
		ID:        1,
		Login:     "multiuser",
		Email:     "multi@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	// Setup multiple roles
	userRole := &domain.Role{
		ID:   1,
		Name: "User",
	}
	adminRole := &domain.Role{
		ID:   2,
		Name: "Admin",
	}
	require.NoError(t, rbac.SaveRole(context.Background(), userRole))
	require.NoError(t, rbac.SaveRole(context.Background(), adminRole))

	// Assign both roles to user
	assignedUserRole := &domain.AssignedRole{
		RoleID:     userRole.ID,
		EntityID:   user.ID,
		EntityType: domain.EntityTypeUser,
	}
	assignedAdminRole := &domain.AssignedRole{
		RoleID:     adminRole.ID,
		EntityID:   user.ID,
		EntityType: domain.EntityTypeUser,
	}
	require.NoError(t, rbac.SaveAssignedRole(context.Background(), assignedUserRole))
	require.NoError(t, rbac.SaveAssignedRole(context.Background(), assignedAdminRole))

	session := &auth.Session{
		Login: "multiuser",
		Email: "multi@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/profile/getprofile", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var profileResp profileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &profileResp))

	assert.Equal(t, uint(1), profileResp.ID)
	assert.Equal(t, "multiuser", profileResp.Login)
	assert.Equal(t, "multi@example.com", profileResp.Email)
	require.NotNil(t, profileResp.Name)
	assert.Equal(t, "Multi Role User", *profileResp.Name)
	assert.Len(t, profileResp.Roles, 2)
	assert.Contains(t, profileResp.Roles, "User")
	assert.Contains(t, profileResp.Roles, "Admin")
}

func TestHandler_NewHandler(t *testing.T) {
	rbac := inmemory.NewRBACRepository()
	responder := api.NewResponder()

	handler := NewHandler(rbac, nil, nil, responder)

	require.NotNil(t, handler)
	assert.Equal(t, rbac, handler.rbac)
	assert.Equal(t, responder, handler.responder)
}

func TestNewProfileResponseFromUser(t *testing.T) {
	now := time.Now()
	userName := "Test User"
	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	roles := []domain.RestrictedRole{
		{
			Role: domain.Role{
				Name: "admin",
			},
		},
	}

	response := newProfileResponseFromUser(user, roles, nil)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "testuser", response.Login)
	assert.Equal(t, "test@example.com", response.Email)
	require.NotNil(t, response.Name)
	assert.Equal(t, "Test User", *response.Name)
	assert.Equal(t, []string{"admin"}, response.Roles)
}

func TestNewProfileResponseFromUserWithNilFields(t *testing.T) {
	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Name:      nil,
		CreatedAt: nil,
		UpdatedAt: nil,
	}

	response := newProfileResponseFromUser(user, nil, nil)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "testuser", response.Login)
	assert.Equal(t, "test@example.com", response.Email)
	assert.Nil(t, response.Name)
	assert.Empty(t, response.Roles)
}

type stubAdminChecker struct{ isAdmin bool }

func (s stubAdminChecker) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return s.isAdmin, nil
}

// TestHandler_MFANudgeInProfile covers OWASP API2:2023: an admin without 2FA
// receives the MFA nudge block in their profile when the operator has enabled
// AUTH_REQUIRE_MFA_FOR_ADMINS, so the frontend can surface the enforcement
// modal on every page load. A non-admin gets no nudge.
func TestHandler_MFANudgeInProfile(t *testing.T) {
	var cfg config.Config
	cfg.Auth.RequireMFAForAdmins = true
	cfg.Auth.MFAHardFailDays = 30
	clock := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }

	user := &domain.User{ID: 7, Login: "admin", Email: "admin@example.com"}
	ctx := auth.ContextWithSession(context.Background(), &auth.Session{User: user, Login: user.Login})

	serve := func(t *testing.T, admin adminChecker) profileResponse {
		t.Helper()

		handler := NewHandler(
			inmemory.NewRBACRepository(), mfanudge.New(cfg, clock), admin, api.NewResponder(),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/profile", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp profileResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		return resp
	}

	t.Run("admin_without_2fa_receives_nudge", func(t *testing.T) {
		resp := serve(t, stubAdminChecker{isAdmin: true})
		require.NotNil(t, resp.MFANudge)
		assert.True(t, resp.MFANudge.Required)
		assert.True(t, resp.MFANudge.ShowNow)
	})

	t.Run("non_admin_has_no_nudge", func(t *testing.T) {
		resp := serve(t, stubAdminChecker{isAdmin: false})
		assert.Nil(t, resp.MFANudge)
	})
}

// TestHandler_MFANudgeInProfile_States covers OWASP API2:2023. It pins the
// remaining nudge states surfaced by the side-effect-free profile read: the
// operator switch being off, the user already having 2FA, an active snooze
// suppressing the modal, and — critically — that the GET never persists the
// first-shown timestamp (the login flow owns that write).
func TestHandler_MFANudgeInProfile_States(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC) }

	// serveUser runs the profile read for the given user under cfg and returns
	// the decoded response. The same user pointer is observable afterwards so a
	// caller can assert the handler did not mutate it.
	serveUser := func(t *testing.T, cfg config.Config, user *domain.User, admin adminChecker) profileResponse {
		t.Helper()

		ctx := auth.ContextWithSession(context.Background(), &auth.Session{User: user, Login: user.Login})
		handler := NewHandler(inmemory.NewRBACRepository(), mfanudge.New(cfg, clock), admin, api.NewResponder())
		req := httptest.NewRequest(http.MethodGet, "/api/profile", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp profileResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		return resp
	}

	enabledCfg := func() config.Config {
		var cfg config.Config
		cfg.Auth.RequireMFAForAdmins = true
		cfg.Auth.MFAHardFailDays = 30

		return cfg
	}

	t.Run("feature_disabled_no_nudge", func(t *testing.T) {
		var cfg config.Config // RequireMFAForAdmins defaults to false
		cfg.Auth.MFAHardFailDays = 30

		resp := serveUser(t, cfg, &domain.User{ID: 7, Login: "admin"}, stubAdminChecker{isAdmin: true})

		assert.Nil(t, resp.MFANudge, "the operator switch being off must suppress the nudge entirely")
	})

	t.Run("two_factor_enabled_no_nudge", func(t *testing.T) {
		user := &domain.User{ID: 7, Login: "admin", TwoFactorEnabled: true}

		resp := serveUser(t, enabledCfg(), user, stubAdminChecker{isAdmin: true})

		assert.Nil(t, resp.MFANudge, "a user who already has 2FA must not be nudged")
	})

	t.Run("snoozed_nudge_hidden", func(t *testing.T) {
		user := &domain.User{ID: 7, Login: "admin"}
		shown := clock().Add(-3 * 24 * time.Hour)
		snoozed := clock().Add(10 * time.Hour) // snooze still active
		user.SetMFAFirstShownAt(&shown)
		user.SetMFASnoozedUntil(&snoozed)

		resp := serveUser(t, enabledCfg(), user, stubAdminChecker{isAdmin: true})

		require.NotNil(t, resp.MFANudge, "the nudge block is present while snoozed")
		assert.True(t, resp.MFANudge.Required)
		assert.False(t, resp.MFANudge.ShowNow, "an active snooze must suppress the modal on the profile read")
	})

	t.Run("profile_get_does_not_persist", func(t *testing.T) {
		user := &domain.User{ID: 7, Login: "admin"} // no first-shown in metadata
		require.Nil(t, user.MFAFirstShownAt(), "precondition: the admin has no recorded first-shown timestamp")

		resp := serveUser(t, enabledCfg(), user, stubAdminChecker{isAdmin: true})

		require.NotNil(t, resp.MFANudge, "an admin without 2FA still receives the nudge on first contact")
		assert.True(t, resp.MFANudge.ShowNow)
		assert.Nil(t, user.MFAFirstShownAt(),
			"the profile GET must be side-effect-free: it must not record the first-shown timestamp")
	})
}
