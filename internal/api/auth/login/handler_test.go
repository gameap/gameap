package login

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the login handler emits (mirrors router_security_auditlog_test.go).
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

func TestHandler_ServeHTTP(t *testing.T) {
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

	tests := []struct {
		name           string
		setupRepo      func(*inmemory.UserRepository)
		requestBody    string
		expectedStatus int
		wantError      string
		checkResponse  func(*testing.T, map[string]any)
	}{
		{
			name: "successful login with username",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), testUser)
			},
			requestBody: `{
				"login": "testuser",
				"password": "password123"
			}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.Contains(t, response, "token")
				assert.NotEmpty(t, response["token"])
				assert.Contains(t, response, "expires_in")
				assert.Contains(t, response, "user")

				user, ok := response["user"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "testuser", user["login"])
				assert.Equal(t, "test@example.com", user["email"])
			},
		},
		{
			name: "successful login with email",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), testUser)
			},
			requestBody: `{
				"email": "test@example.com",
				"password": "password123"
			}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.Contains(t, response, "token")
				assert.NotEmpty(t, response["token"])
			},
		},
		{
			// OWASP API2:2023 Broken Authentication — a correct password for a
			// 2FA-enabled account must NOT yield a session token; only a
			// challenge to be completed at /api/auth/2fa/verify.
			name: "login_with_2fa_enabled_returns_challenge_not_token",
			setupRepo: func(repo *inmemory.UserRepository) {
				u := *testUser
				u.ID = 0
				u.Login = "twofauser"
				u.Email = "twofa@example.com"
				u.TwoFactorEnabled = true
				secret := "encrypted-secret"
				u.TwoFactorSecret = &secret
				_ = repo.Save(context.Background(), &u)
			},
			requestBody: `{
				"login": "twofauser",
				"password": "password123"
			}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response map[string]any) {
				t.Helper()

				assert.NotContains(t, response, "token")
				assert.Equal(t, true, response["two_factor_required"])
				challengeToken, ok := response["challenge_token"].(string)
				require.True(t, ok)
				assert.True(t, strings.HasPrefix(challengeToken, "g2fa_"))
			},
		},
		{
			name: "invalid password",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), testUser)
			},
			requestBody: `{
				"login": "testuser",
				"password": "wrongpassword"
			}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid credentials",
		},
		{
			name: "user not found",
			setupRepo: func(_ *inmemory.UserRepository) {
				// Don't add any users
			},
			requestBody: `{
				"login": "nonexistent",
				"password": "password123"
			}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid credentials",
		},
		{
			name:      "missing login field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"password": "password123"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "login or email fields are required",
		},
		{
			name:      "missing password field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"login": "testuser"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "password field is required",
		},
		{
			name:      "empty login field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"login": "",
				"password": "password123"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "login or email fields are required",
		},
		{
			name:      "empty password field",
			setupRepo: func(_ *inmemory.UserRepository) {},
			requestBody: `{
				"login": "testuser",
				"password": ""
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "password field is required",
		},
		{
			name:           "invalid JSON body",
			setupRepo:      func(_ *inmemory.UserRepository) {},
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewUserRepository()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			responder := api.NewResponder()
			handler := NewHandler(
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil,
			)

			body := []byte(tt.requestBody)

			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else if tt.checkResponse != nil {
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestHandler_MultipleUsers(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil,
	)

	// Create multiple users
	hashedPassword1, _ := auth.HashPassword("pass1")
	hashedPassword2, _ := auth.HashPassword("pass2")
	hashedPassword3, _ := auth.HashPassword("pass3")
	now := time.Now()

	user1 := &domain.User{
		ID:        1,
		Login:     "user1",
		Email:     "user1@example.com",
		Password:  hashedPassword1,
		Name:      new("User One"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	user2 := &domain.User{
		ID:        2,
		Login:     "user2",
		Email:     "user2@example.com",
		Password:  hashedPassword2,
		Name:      new("User Two"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	user3 := &domain.User{
		ID:        3,
		Login:     "user3",
		Email:     "user3@example.com",
		Password:  hashedPassword3,
		Name:      new("User Three"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	_ = repo.Save(context.Background(), user1)
	_ = repo.Save(context.Background(), user2)
	_ = repo.Save(context.Background(), user3)

	tests := []struct {
		name     string
		login    string
		email    string
		password string
	}{
		{
			name:     "login as user1 with username",
			login:    "user1",
			password: "pass1",
		},
		{
			name:     "login as user2 with username",
			login:    "user2",
			password: "pass2",
		},
		{
			name:     "login as user1 with email",
			email:    "user1@example.com",
			password: "pass1",
		},
		{
			name:     "login as user2 with email",
			email:    "user2@example.com",
			password: "pass2",
		},
		{
			name:     "login as user3 with username",
			login:    "user3",
			password: "pass3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			loginData := map[string]string{
				"login":    tt.login,
				"email":    tt.email,
				"password": tt.password,
			}
			body, _ := json.Marshal(loginData)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusOK, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			_, ok := response["user"].(map[string]any)
			require.True(t, ok)
		})
	}
}

func TestHandler_SpecialCharacters(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil,
	)

	// Create user with special characters
	specialPassword := "p@$$w0rd!#%&*()"
	hashedPassword, _ := auth.HashPassword(specialPassword)
	now := time.Now()

	user := &domain.User{
		ID:        1,
		Login:     "special.user-name_123",
		Email:     "special+tag@example.com",
		Password:  hashedPassword,
		Name:      new("Special User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	_ = repo.Save(context.Background(), user)

	tests := []struct {
		name           string
		login          string
		email          string
		password       string
		expectedStatus int
	}{
		{
			name:           "login with special characters in username",
			login:          "special.user-name_123",
			password:       specialPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "login with special characters in email",
			email:          "special+tag@example.com",
			password:       specialPassword,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			loginData := map[string]string{
				"login":    tt.login,
				"email":    tt.email,
				"password": tt.password,
			}
			body, _ := json.Marshal(loginData)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandler_TokenValidation(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil,
	)

	hashedPassword, _ := auth.HashPassword("testpass")
	now := time.Now()
	user := &domain.User{
		ID:        42,
		Login:     "tokenuser",
		Email:     "token@test.com",
		Password:  hashedPassword,
		Name:      new("Token User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_ = repo.Save(context.Background(), user)

	// ACT
	loginData := map[string]string{
		"login":    "tokenuser",
		"password": "testpass",
	}
	body, err := json.Marshal(loginData)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	// Verify token structure
	token, ok := response["token"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, token)

	// JWT tokens have three parts separated by dots
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3, "JWT token should have three parts")

	// Verify expires_in is reasonable (24 hours = 86400 seconds)
	expiresIn, ok := response["expires_in"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(86400), expiresIn)
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — a successful interactive login must
//     leave an auth.login.success audit event attributed to the authenticated
//     principal (OWASP ASVS §7.1.3). The session is not yet in context at the
//     emission point, so the handler must pass the actor explicitly; this test
//     pins that. A failed login must NOT be recorded as a success.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestHandler_Audit_SuccessfulLoginIsRecorded covers OWASP API2:2023. A
// successful login (by username or by email) must emit exactly one
// auth.login.success event with outcome success, auth_method session, and the
// authenticated user's id/login as the actor.
func TestHandler_Audit_SuccessfulLoginIsRecorded(t *testing.T) {
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	user := &domain.User{
		ID:        42,
		Login:     "audituser",
		Email:     "audit@example.com",
		Password:  hashedPassword,
		Name:      new("Audit User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	tests := []struct {
		name        string
		requestBody string
	}{
		{
			name:        "login_by_username_attributes_actor",
			requestBody: `{"login":"audituser","password":"password123"}`,
		},
		{
			name:        "login_by_email_attributes_actor",
			requestBody: `{"email":"audit@example.com","password":"password123"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewUserRepository()
			require.NoError(t, repo.Save(context.Background(), user))
			recorder := &auditCapture{}
			handler := NewHandler(
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), api.NewResponder(), recorder,
			)

			req := httptest.NewRequest(
				http.MethodPost, "/auth/login", bytes.NewBufferString(tt.requestBody),
			)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusOK, w.Code, "login must succeed; body=%s", w.Body.String())

			events := recorder.snapshot()
			require.Equal(t, 1, countEvents(events, audit.EventLoginSuccess),
				"exactly one login-success event must be emitted per successful login")

			ev, ok := findEvent(events, audit.EventLoginSuccess)
			require.True(t, ok, "a successful login must leave an auth.login.success event")
			assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed login records success")
			assert.Equal(t, audit.CategoryAuthentication, ev.Category)
			assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod,
				"an interactive login establishes a session")
			assert.Equal(t, user.ID, ev.ActorID,
				"the authenticated user's id must be attributed as the actor")
			assert.Equal(t, user.Login, ev.ActorLogin,
				"the actor must be the resolved login, even when authenticating by email")
		})
	}
}

// TestHandler_Audit_FailedLoginIsNotRecordedAsSuccess covers OWASP API2:2023.
// An invalid-credentials attempt must not emit an auth.login.success event —
// only a genuine authentication may produce the success record.
func TestHandler_Audit_FailedLoginIsNotRecordedAsSuccess(t *testing.T) {
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	user := &domain.User{
		ID:        1,
		Login:     "audituser",
		Email:     "audit@example.com",
		Password:  hashedPassword,
		Name:      new("Audit User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	tests := []struct {
		name        string
		setupRepo   func(*inmemory.UserRepository)
		requestBody string
		wantStatus  int
	}{
		{
			name: "wrong_password_records_no_success",
			setupRepo: func(repo *inmemory.UserRepository) {
				require.NoError(t, repo.Save(context.Background(), user))
			},
			requestBody: `{"login":"audituser","password":"wrongpassword"}`,
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "unknown_user_records_no_success",
			setupRepo:   func(_ *inmemory.UserRepository) {},
			requestBody: `{"login":"ghost","password":"password123"}`,
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)
			recorder := &auditCapture{}
			handler := NewHandler(
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), api.NewResponder(), recorder,
			)

			req := httptest.NewRequest(
				http.MethodPost, "/auth/login", bytes.NewBufferString(tt.requestBody),
			)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())
			assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventLoginSuccess),
				"a failed login must never be recorded as auth.login.success")
		})
	}
}
