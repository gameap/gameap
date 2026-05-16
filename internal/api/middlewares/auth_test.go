package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-secret-key-for-testing"

func TestAuthMiddleware_Middleware(t *testing.T) {
	// Setup test user
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	testUser := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Password:  hashedPassword,
		Name:      new("Test User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_ = userRepo.Save(context.Background(), testUser)

	// Setup JWT service and generate token
	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	validToken, _ := jwtService.GenerateTokenForUser(testUser, 24*time.Hour)
	expiredToken, _ := jwtService.GenerateTokenForUser(testUser, -1*time.Hour)
	invalidUserToken, _ := jwtService.GenerateTokenForUser(&domain.User{ID: 999, Login: "invalid"}, 24*time.Hour)

	tests := []struct {
		name       string
		authHeader string
		queryParam string
		cookie     *http.Cookie
		wantStatus int
		wantUser   bool
		wantError  string
	}{
		{
			name:       "valid token in Authorization header",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "valid token without Bearer prefix",
			authHeader: validToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
		{
			name:       "valid token in query parameter",
			queryParam: validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name: "valid token in cookie",
			cookie: &http.Cookie{
				Name:  "token",
				Value: validToken,
			},
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "missing token",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
		{
			name:       "expired token",
			authHeader: "Bearer " + expiredToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "invalid or expired token",
		},
		{
			name:       "invalid token format",
			authHeader: "Bearer invalid-token",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "invalid or expired token",
		},
		{
			name:       "token for non-existent user",
			authHeader: "Bearer " + invalidUserToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "user not found",
		},
		{
			name:       "malformed Authorization header",
			authHeader: "InvalidScheme " + validToken,
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
		{
			name:       "empty Bearer token",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
			wantError:  "missing authentication token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup middleware
			responder := api.NewResponder()
			authMiddleware := NewAuthMiddleware(
				auth.NewJWTService([]byte(testJWTSecret)),
				userRepo,
				tokenRepo,
				auth.NoopRevocation{},
				nil,
				responder,
				nil,
			)

			var session *auth.Session

			// Create test handler that will be protected
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				session = auth.SessionFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			// Apply middleware
			protectedHandler := authMiddleware.Middleware(testHandler)

			// Create request
			url := "/protected"
			if tt.queryParam != "" {
				url += "?token=" + tt.queryParam
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}

			// Execute request
			w := httptest.NewRecorder()
			protectedHandler.ServeHTTP(w, req)

			// Assert status
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantUser {
				require.NotNil(t, session)
				assert.Equal(t, testUser.Login, session.Login)
				assert.Equal(t, testUser.Email, session.Email)
			} else {
				assert.Nil(t, session)
			}

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				assert.Contains(t, response["error"], tt.wantError)
			}
		})
	}
}

func TestAuthMiddleware_OptionalMiddleware(t *testing.T) {
	// Setup test user
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	testUser := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Password:  hashedPassword,
		Name:      new("Test User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_ = userRepo.Save(context.Background(), testUser)

	// Setup JWT service and generate tokens
	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	validToken, _ := jwtService.GenerateTokenForUser(testUser, 24*time.Hour)
	expiredToken, _ := jwtService.GenerateTokenForUser(testUser, -1*time.Hour)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantError  string
		wantUser   bool
	}{
		{
			name:       "valid token",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "no token - should still pass",
			authHeader: "",
			wantStatus: http.StatusOK,
			wantUser:   false,
		},
		{
			name:       "expired token - should still pass but without user",
			authHeader: "Bearer " + expiredToken,
			wantStatus: http.StatusOK,
			wantUser:   false,
		},
		{
			name:       "invalid token - shouldn't pass",
			authHeader: "Bearer invalid-token",
			wantStatus: http.StatusUnauthorized,
			wantError:  "invalid or expired token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup middleware
			responder := api.NewResponder()
			authMiddleware := NewAuthMiddleware(auth.NewJWTService([]byte(testJWTSecret)), userRepo, tokenRepo, auth.NoopRevocation{}, nil, responder, nil)

			var session *auth.Session
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// capturedUser, _ = GetUserFromContext(r.Context())
				session = auth.SessionFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			// Apply optional middleware
			protectedHandler := authMiddleware.OptionalMiddleware(testHandler)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/optional", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Execute request
			w := httptest.NewRecorder()
			protectedHandler.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantError == "" {
				assert.Equal(t, "success", w.Body.String())
			} else {
				assert.Contains(t, w.Body.String(), tt.wantError)
			}

			if tt.wantUser {
				assert.NotNil(t, session)
				assert.Equal(t, testUser.Login, session.Login)
				assert.Equal(t, testUser.Email, session.Email)
			} else {
				assert.Nil(t, session)
			}
		})
	}
}

func TestTokenExtractionPriority(t *testing.T) {
	// Setup
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	testUser1 := &domain.User{
		ID:        1,
		Login:     "user1",
		Email:     "user1@example.com",
		Password:  hashedPassword,
		Name:      new("User1"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_ = userRepo.Save(context.Background(), testUser1)

	// Do not save user2 to simulate non-existent users
	testUser2 := &domain.User{
		ID:        2,
		Login:     "user2",
		Email:     "user2@example.com",
		Password:  hashedPassword,
		Name:      new("User2"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	// Do not save user3 to simulate non-existent users
	testUser3 := &domain.User{
		ID:        3,
		Login:     "user3",
		Email:     "user3@example.com",
		Password:  hashedPassword,
		Name:      new("User3"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	tokenHeader, _ := jwtService.GenerateTokenForUser(testUser1, 24*time.Hour)
	tokenQuery, _ := jwtService.GenerateTokenForUser(testUser2, 24*time.Hour)
	tokenCookie, _ := jwtService.GenerateTokenForUser(testUser3, 24*time.Hour)

	responder := api.NewResponder()
	authMiddleware := NewAuthMiddleware(auth.NewJWTService([]byte(testJWTSecret)), userRepo, tokenRepo, auth.NoopRevocation{}, nil, responder, nil)

	// Test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := auth.SessionFromContext(r.Context())
		if session != nil {
			response := map[string]string{"login": session.Login}
			require.NoError(t, json.NewEncoder(w).Encode(response))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("no user in context"))
		}
	})

	protectedHandler := authMiddleware.Middleware(testHandler)

	// Test 1: Authorization header should have highest priority
	req := httptest.NewRequest(http.MethodGet, "/test?token="+tokenQuery, nil)
	req.Header.Set("Authorization", "Bearer "+tokenHeader)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})

	w := httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	var response map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "user1", response["login"], "Authorization header should have highest priority")

	// Test 2: Query parameter should be used when no Authorization header
	req = httptest.NewRequest(http.MethodGet, "/test?token="+tokenQuery, nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})

	w = httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	// Since user with ID 2 doesn't exist, this should fail
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Test 3: Cookie should be used when no Authorization header or query param
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})

	w = httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	// Since user with ID 3 doesn't exist, this should fail
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — every rejected bearer/PAT
//     authentication attempt must leave an auth.token.rejected audit event
//     with outcome failure and a stable, non-sensitive reason for forensics
//     (OWASP ASVS §7.1.3). A silent rejection is a detective-control gap.
//   - API1:2023 BOLA / API5:2023 BFLA — an authenticated principal denied
//     the admin gate must leave an access.denied event attributed to that
//     principal (OWASP ASVS §4.1.5).
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// setupAuthMiddlewareAudit builds an AuthMiddleware with a single seeded user
// and a capturing audit logger so a test can assert on the emitted events.
func setupAuthMiddlewareAudit(t *testing.T) (*AuthMiddleware, *auditCapture) {
	t.Helper()

	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Password:  hashedPassword,
		Name:      new("Test User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	recorder := &auditCapture{}
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		tokenRepo,
		auth.NoopRevocation{},
		nil,
		api.NewResponder(),
		recorder,
	)

	return mw, recorder
}

// noopNextHandler is a 200-OK terminal handler used to prove a middleware
// passed the request through (and therefore did NOT emit a rejection).
func noopNextHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestAuthMiddleware_Audit_RejectionEmitsEvent covers OWASP API2:2023.
// Every rejection branch of the required (non-optional) auth middleware must
// emit exactly one auth.token.rejected event with outcome failure and the
// branch-specific stable reason token.
func TestAuthMiddleware_Audit_RejectionEmitsEvent(t *testing.T) {
	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	expiredToken, _ := jwtService.GenerateTokenForUser(&domain.User{ID: 1, Login: "testuser"}, -time.Hour)

	tests := []struct {
		name       string
		authHeader string
		wantReason string
	}{
		{
			name:       "missing_token_has_missing_token_reason",
			authHeader: "",
			wantReason: "missing_token",
		},
		{
			name:       "expired_jwt_has_invalid_or_expired_token_reason",
			authHeader: "Bearer " + expiredToken,
			wantReason: "invalid_or_expired_token",
		},
		{
			name:       "unrecognised_token_shape_has_unknown_token_type_reason",
			authHeader: "Bearer not-a-real-token",
			wantReason: "unknown_token_type",
		},
		{
			name:       "jwt_shaped_garbage_has_invalid_or_expired_token_reason",
			authHeader: "Bearer eyJ-not-a-real-jwt",
			wantReason: "invalid_or_expired_token",
		},
		{
			name:       "unknown_pat_id_has_pat_not_found_reason",
			authHeader: "Bearer 999|deadbeefdeadbeefdeadbeefdeadbeef",
			wantReason: "pat_not_found",
		},
		{
			name:       "malformed_pat_id_has_invalid_pat_id_reason",
			authHeader: "Bearer 0|deadbeef",
			wantReason: "invalid_pat_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			mw, recorder := setupAuthMiddlewareAudit(t)
			handler := mw.Middleware(noopNextHandler())

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusUnauthorized, w.Code,
				"a rejected request must not reach the protected handler; body=%s", w.Body.String())

			events := recorder.snapshot()
			require.Equal(t, 1, countEvents(events, audit.EventAuthTokenRejected),
				"exactly one rejected-token event must be emitted per rejected request")

			ev, ok := findEvent(events, audit.EventAuthTokenRejected)
			require.True(t, ok, "a rejected-token audit event must be emitted")
			assert.Equal(t, audit.OutcomeFailure, ev.Outcome, "a rejected auth attempt is a failure")
			assert.Equal(t, audit.CategoryAuthentication, ev.Category)
			assert.Equal(t, tt.wantReason, ev.Reason,
				"the failure reason must be the branch-specific stable token")
			assert.Equal(t, audit.AuthMethodAnonymous, ev.AuthMethod,
				"a caller that never authenticated must be attributed anonymous, never a user")
			assert.Zero(t, ev.ActorID, "no actor id may be attributed to an unauthenticated rejection")
		})
	}
}

// TestAuthMiddleware_Audit_TokenRevokedEmitsEvent covers OWASP API2:2023.
// A credential that verifies but is on the revocation denylist must still be
// rejected AND audited with the stable token_revoked reason.
func TestAuthMiddleware_Audit_TokenRevokedEmitsEvent(t *testing.T) {
	// ARRANGE
	userRepo := inmemory.NewUserRepository()
	tokenRepo := inmemory.NewPersonalAccessTokenRepository()
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	user := &domain.User{
		ID:        1,
		Login:     "testuser",
		Email:     "test@example.com",
		Password:  hashedPassword,
		Name:      new("Test User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	validToken, _ := jwtService.GenerateTokenForUser(user, time.Hour)

	recorder := &auditCapture{}
	mw := NewAuthMiddleware(
		jwtService,
		userRepo,
		tokenRepo,
		revokeAllRevocation{},
		nil,
		api.NewResponder(),
		recorder,
	)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code,
		"a revoked token must be rejected even though its signature is valid; body=%s", w.Body.String())

	ev, ok := findEvent(recorder.snapshot(), audit.EventAuthTokenRejected)
	require.True(t, ok, "a revoked token must leave a rejected-token audit event")
	assert.Equal(t, audit.OutcomeFailure, ev.Outcome)
	assert.Equal(t, "token_revoked", ev.Reason,
		"a denylisted credential must be recorded with the stable token_revoked reason")
}

// revokeAllRevocation is a TokenRevocation that treats every token as revoked,
// exercising the post-verification revocation branch of the auth middleware.
type revokeAllRevocation struct{}

func (revokeAllRevocation) Revoke(context.Context, string, time.Duration) error { return nil }

func (revokeAllRevocation) IsRevoked(context.Context, string) (bool, error) { return true, nil }

// TestAuthMiddleware_Audit_OptionalMiddlewareNuance covers OWASP API2:2023.
// The optional middleware must NOT record a rejection when no token is
// presented (anonymous access is legitimate) but MUST record one when a
// token is presented and turns out to be invalid (a real failed attempt).
func TestAuthMiddleware_Audit_OptionalMiddlewareNuance(t *testing.T) {
	jwtService := auth.NewJWTService([]byte(testJWTSecret))
	expiredToken, _ := jwtService.GenerateTokenForUser(&domain.User{ID: 1, Login: "testuser"}, -time.Hour)

	tests := []struct {
		name        string
		authHeader  string
		wantEmitted bool
		wantReason  string
	}{
		{
			name:        "bare_missing_token_must_not_emit",
			authHeader:  "",
			wantEmitted: false,
		},
		{
			name:        "presented_but_invalid_token_must_emit",
			authHeader:  "Bearer " + expiredToken,
			wantEmitted: true,
			wantReason:  "invalid_or_expired_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			mw, recorder := setupAuthMiddlewareAudit(t)
			handler := mw.OptionalMiddleware(noopNextHandler())

			req := httptest.NewRequest(http.MethodGet, "/optional", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusOK, w.Code,
				"optional middleware must always pass the request through; body=%s", w.Body.String())

			events := recorder.snapshot()
			if tt.wantEmitted {
				ev, ok := findEvent(events, audit.EventAuthTokenRejected)
				require.True(t, ok,
					"a presented-but-invalid token is a real failed attempt and must be audited")
				assert.Equal(t, audit.OutcomeFailure, ev.Outcome)
				assert.Equal(t, tt.wantReason, ev.Reason)
			} else {
				assert.Equal(t, 0, countEvents(events, audit.EventAuthTokenRejected),
					"anonymous access via optional middleware must NOT be recorded as a rejection")
			}
		})
	}
}

// TestIsAdminMiddleware_Audit_DenialEmitsEvent covers OWASP API1:2023 /
// API5:2023. The admin gate must emit an access.denied event with outcome
// denied and category authorization for both the unauthenticated and the
// authenticated-non-admin cases, with the branch-specific stable reason.
func TestIsAdminMiddleware_Audit_DenialEmitsEvent(t *testing.T) {
	tests := []struct {
		name       string
		setupAuth  func() context.Context
		wantStatus int
		wantReason string
		wantActor  uint
	}{
		{
			name:       "unauthenticated_request_denied_with_not_authenticated_reason",
			setupAuth:  context.Background,
			wantStatus: http.StatusUnauthorized,
			wantReason: "not_authenticated",
			wantActor:  0,
		},
		{
			name: "authenticated_non_admin_denied_with_admin_required_reason",
			setupAuth: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					Login: "regular",
					Email: "regular@example.com",
					User:  &domain.User{ID: 7, Login: "regular"},
				})
			},
			wantStatus: http.StatusForbidden,
			wantReason: "admin_required",
			wantActor:  7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			recorder := &auditCapture{}
			mw := NewIsAdminMiddleware(rbacService, api.NewResponder(), recorder)
			handler := mw.Middleware(noopNextHandler())

			req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
			req = req.WithContext(tt.setupAuth())
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code,
				"a non-admin must be denied the admin route; body=%s", w.Body.String())

			ev, ok := findEvent(recorder.snapshot(), audit.EventAccessDenied)
			require.True(t, ok, "an access-denied event must be emitted for a refused admin-route access")
			assert.Equal(t, audit.OutcomeDenied, ev.Outcome, "an authorization refusal is a denial")
			assert.Equal(t, audit.CategoryAuthorization, ev.Category)
			assert.Equal(t, "admin", ev.ResourceType, "the admin gate guards the admin resource")
			assert.Equal(t, tt.wantReason, ev.Reason, "the stable denial reason must be recorded")
			assert.Equal(t, tt.wantActor, ev.ActorID,
				"the denied principal (or none, when unauthenticated) must be attributed")
		})
	}
}

// TestIsAdminMiddleware_Audit_AdminIsNotDenied covers OWASP API5:2023. A user
// who actually holds the admin ability must pass the gate and must NOT leave
// an access.denied event (no false-positive denials in the audit trail).
func TestIsAdminMiddleware_Audit_AdminIsNotDenied(t *testing.T) {
	// ARRANGE
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

	adminRole := &domain.Role{Name: "admin"}
	require.NoError(t, rbacRepo.SaveRole(context.Background(), adminRole))
	ability := &domain.Ability{Name: domain.AbilityNameAdminRolesPermissions}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))
	require.NoError(t, rbacRepo.Allow(
		context.Background(), adminRole.ID, domain.EntityTypeRole, []domain.Ability{*ability},
	))
	require.NoError(t, rbacRepo.AssignRolesForEntity(
		context.Background(), 9, domain.EntityTypeUser,
		[]domain.RestrictedRole{domain.NewRestrictedRoleFromRole(*adminRole)},
	))

	recorder := &auditCapture{}
	mw := NewIsAdminMiddleware(rbacService, api.NewResponder(), recorder)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin",
		User:  &domain.User{ID: 9, Login: "admin"},
	}))
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code,
		"an admin must pass the gate; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventAccessDenied),
		"a successful admin must not produce a false-positive access-denied audit event")
}
