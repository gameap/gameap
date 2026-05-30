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
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/captcha"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/gameap/gameap/internal/filters"
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

var errCaptchaUpstream = errors.New("upstream")

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
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil, nil, nil, nil, 0,
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

// stubCaptcha is a captchaVerifier double recording the token and remote IP
// it received.
type stubCaptcha struct {
	enabled bool
	err     error
	gotTok  string
	gotIP   string
}

func (s *stubCaptcha) Enabled() bool { return s.enabled }

func (s *stubCaptcha) Verify(_ context.Context, token, remoteIP string) error {
	s.gotTok = token
	s.gotIP = remoteIP

	return s.err
}

// TestHandler_Captcha pins the login/captcha contract: when a provider is
// configured the token is verified before the user store is touched, and a
// missing/rejected token blocks the login with a 422; a disabled verifier
// leaves the flow untouched.
func TestHandler_Captcha(t *testing.T) {
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	testUser := &domain.User{
		ID:        1,
		Login:     "captchauser",
		Email:     "captcha@example.com",
		Password:  hashedPassword,
		Name:      new("Captcha User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	tests := []struct {
		name           string
		captcha        *stubCaptcha
		nilVerifier    bool
		requestBody    string
		expectedStatus int
		wantError      string
		wantToken      string
	}{
		{
			name:           "disabled_verifier_does_not_block_login",
			captcha:        &stubCaptcha{enabled: false},
			requestBody:    `{"login":"captchauser","password":"password123"}`,
			expectedStatus: http.StatusOK,
		},
		{
			// The handler must tolerate a literal nil verifier (captcha not
			// wired at all) and proceed exactly like a disabled one.
			name:           "nil_verifier_does_not_block_login",
			nilVerifier:    true,
			requestBody:    `{"login":"captchauser","password":"password123"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "enabled_missing_token_rejected_before_user_lookup",
			captcha:        &stubCaptcha{enabled: true, err: captcha.ErrTokenRequired},
			requestBody:    `{"login":"captchauser","password":"password123"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "captcha token is required",
		},
		{
			name:           "enabled_invalid_token_rejected",
			captcha:        &stubCaptcha{enabled: true, err: captcha.ErrVerificationFailed},
			requestBody:    `{"login":"captchauser","password":"password123","captcha":"bad"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "captcha verification failed",
			wantToken:      "bad",
		},
		{
			name:           "enabled_valid_token_proceeds_to_password_check",
			captcha:        &stubCaptcha{enabled: true},
			requestBody:    `{"login":"captchauser","password":"password123","captcha":"good"}`,
			expectedStatus: http.StatusOK,
			wantToken:      "good",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewUserRepository()
			require.NoError(t, repo.Save(context.Background(), testUser))

			var verifier captchaVerifier
			if !tt.nilVerifier {
				verifier = tt.captcha
			}

			handler := NewHandler(
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
				api.NewResponder(), nil, verifier, nil, nil, 0,
			)

			req := httptest.NewRequest(
				http.MethodPost, "/auth/login", bytes.NewBufferString(tt.requestBody),
			)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errMsg, _ := response["error"].(string)
				assert.Contains(t, errMsg, tt.wantError)
			}

			if tt.wantToken != "" {
				assert.Equal(t, tt.wantToken, tt.captcha.gotTok,
					"the captcha token from the request body must reach the verifier")
			}
		})
	}
}

func TestHandler_MultipleUsers(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository()
	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil, nil, nil, nil, 0,
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
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil, nil, nil, nil, 0,
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
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), responder, nil, nil, nil, nil, 0,
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
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), api.NewResponder(), recorder, nil, nil, nil, 0,
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
				auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(), api.NewResponder(), recorder, nil, nil, nil, 0,
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

// ---------------------------------------------------------------------------
// Captcha bot-wall security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — the captcha gate must run before any
//     credential or account lookup so an attacker cannot probe account
//     existence from behind the wall (no oracle: a bad-captcha attempt on a
//     non-existent account must look exactly like one on a real account).
//   - API4:2023 Unrestricted Resource Consumption — rejecting unsolved
//     captchas before the user store keeps automated traffic from burning DB
//     lookups / password-hash work behind the wall.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestHandler_Captcha_GateRunsBeforeUserLookup covers OWASP API2:2023 and
// API4:2023. With an enabled verifier that rejects the token, a login for a
// user that does NOT exist must be stopped at the captcha wall (422 captcha
// error) and must NOT fall through to the 401 "invalid credentials" path —
// otherwise the differing responses would leak account existence and the DB
// lookup would still run.
func TestHandler_Captcha_GateRunsBeforeUserLookup(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewUserRepository() // intentionally empty: user does not exist
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
		api.NewResponder(), nil, &stubCaptcha{enabled: true, err: captcha.ErrVerificationFailed}, nil, nil, 0,
	)

	req := httptest.NewRequest(
		http.MethodPost, "/auth/login",
		bytes.NewBufferString(`{"login":"ghost-account","password":"whatever","captcha":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnprocessableEntity, w.Code,
		"a rejected captcha must stop the request at the bot wall; body=%s", w.Body.String())
	require.NotEqual(t, http.StatusUnauthorized, w.Code,
		"the request must not reach the credential check (no account-existence oracle)")

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	errMsg, _ := response["error"].(string)
	assert.Contains(t, errMsg, "captcha verification failed",
		"the response must be the captcha error, not invalid-credentials")
	assert.NotContains(t, errMsg, "invalid credentials",
		"a non-existent account must not be distinguishable behind the captcha wall")
}

// TestHandler_Captcha_UpstreamOutageMapsTo503 pins the fail-closed outage
// contract: when the verifier returns a 503-mapped wrapped error (provider
// unreachable, FailOpen disabled) the handler must answer 503 so the login is
// blocked rather than waved through.
func TestHandler_Captcha_UpstreamOutageMapsTo503(t *testing.T) {
	// ARRANGE
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	user := &domain.User{
		ID:        1,
		Login:     "captchauser",
		Email:     "captcha@example.com",
		Password:  hashedPassword,
		Name:      new("Captcha User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), user))

	outage := &stubCaptcha{
		enabled: true,
		err:     api.WrapHTTPError(errCaptchaUpstream, http.StatusServiceUnavailable),
	}
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
		api.NewResponder(), nil, outage, nil, nil, 0,
	)

	req := httptest.NewRequest(
		http.MethodPost, "/auth/login",
		bytes.NewBufferString(`{"login":"captchauser","password":"password123","captcha":"tok"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusServiceUnavailable, w.Code,
		"a fail-closed captcha outage must block the login with 503; body=%s", w.Body.String())
}

// TestHandler_Captcha_ForwardsClientIPFromContext covers the clientIP(ctx)
// positive path: when RequestContextMiddleware has populated the request
// info, the captured IP must be handed to the verifier as the remote IP.
func TestHandler_Captcha_ForwardsClientIPFromContext(t *testing.T) {
	// ARRANGE
	hashedPassword, _ := auth.HashPassword("password123")
	now := time.Now()
	user := &domain.User{
		ID:        1,
		Login:     "captchauser",
		Email:     "captcha@example.com",
		Password:  hashedPassword,
		Name:      new("Captcha User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), user))

	spy := &stubCaptcha{enabled: true}
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
		api.NewResponder(), nil, spy, nil, nil, 0,
	)

	req := httptest.NewRequest(
		http.MethodPost, "/auth/login",
		bytes.NewBufferString(`{"login":"captchauser","password":"password123","captcha":"tok"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(audit.ContextWithRequestInfo(
		req.Context(), &audit.RequestInfo{IP: "203.0.113.9"},
	))
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, "203.0.113.9", spy.gotIP,
		"the client IP from the request context must be forwarded to the captcha verifier")
}

// Pre-§2.1.2 accounts hold raw bcrypt(password) hashes. The login handler must
// keep authenticating them AND transparently upgrade the stored hash to the
// new SHA-256+bcrypt scheme so the legacy form is phased out over time
// (OWASP ASVS 4.0.3 §2.1.2 / §2.1.3).
func TestHandler_ServeHTTP_LegacyHashUpgradesOnLogin(t *testing.T) {
	const (
		legacyLogin    = "legacyuser"
		legacyEmail    = "legacy@example.com"
		legacyPassword = "legacy-pw-12"
	)

	ctx := context.Background()

	rawHash, err := bcrypt.GenerateFromPassword([]byte(legacyPassword), bcrypt.DefaultCost)
	require.NoError(t, err)

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(ctx, &domain.User{
		Login:    legacyLogin,
		Email:    legacyEmail,
		Password: string(rawHash),
	}))

	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")),
		repo,
		cache.NewInMemory(),
		responder,
		nil,
		nil,
		nil,
		nil,
		0,
	)

	body := []byte(`{"login":"` + legacyLogin + `","password":"` + legacyPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	users, err := repo.Find(ctx, &filters.FindUser{Logins: []string{legacyLogin}}, nil, &filters.Pagination{Limit: 1})
	require.NoError(t, err)
	require.Len(t, users, 1)

	assert.NotEqual(t, string(rawHash), users[0].Password,
		"the stored hash must have been replaced on successful legacy-scheme login")

	needsRehash, err := auth.VerifyPassword(users[0].Password, legacyPassword)
	assert.NoError(t, err, "the upgraded hash must verify against the original password")
	assert.False(t, needsRehash,
		"the upgraded hash must use the current SHA-256+bcrypt scheme and not request another upgrade")
}

// TestHandler_ServeHTTP_RehashesOnBcryptCostUpgrade pins ASVS §2.4.4: a
// stored hash with a cost below the operator-configured AUTH_BCRYPT_COST
// must be transparently upgraded on the next successful login. The handler
// must NOT downgrade (target < stored) and must preserve auth on first
// login (no 2nd request needed).
func TestHandler_ServeHTTP_RehashesOnBcryptCostUpgrade(t *testing.T) {
	const (
		userLogin    = "costupgrade"
		userEmail    = "costupgrade@example.test"
		userPassword = "valid-pw-cost-test"
		oldCost      = auth.MinBcryptCost     // 10
		newCost      = auth.MinBcryptCost + 2 // 12 — fast in tests, still > old
	)

	t.Cleanup(func() { _ = auth.SetDefaultBcryptCost(auth.DefaultBcryptCost) })

	ctx := context.Background()

	// Step 1: hash with the OLD cost, persist user.
	require.NoError(t, auth.SetDefaultBcryptCost(oldCost))
	oldHash, err := auth.HashPassword(userPassword)
	require.NoError(t, err)

	storedCost, err := auth.HashCost(oldHash)
	require.NoError(t, err)
	require.Equal(t, oldCost, storedCost, "precondition: starting hash must be at oldCost")

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(ctx, &domain.User{
		Login:    userLogin,
		Email:    userEmail,
		Password: oldHash,
	}))

	// Step 2: install the NEW target cost, exercise login.
	require.NoError(t, auth.SetDefaultBcryptCost(newCost))

	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")),
		repo,
		cache.NewInMemory(),
		responder,
		nil,
		nil,
		nil,
		nil,
		0,
	)

	body := []byte(`{"login":"` + userLogin + `","password":"` + userPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// Step 3: the stored hash must now use the NEW cost.
	users, err := repo.Find(ctx, &filters.FindUser{Logins: []string{userLogin}}, nil, &filters.Pagination{Limit: 1})
	require.NoError(t, err)
	require.Len(t, users, 1)

	upgradedCost, err := auth.HashCost(users[0].Password)
	require.NoError(t, err)
	assert.Equal(t, newCost, upgradedCost,
		"login at higher AUTH_BCRYPT_COST must transparently rehash the user's password")
}

// TestHandler_ServeHTTP_DoesNotDowngradeBcryptCost pins ASVS §2.4.4: when
// the operator-configured AUTH_BCRYPT_COST is LOWER than the stored hash's
// cost, the handler must NOT re-hash — a misconfigured downgrade can never
// weaken existing credentials. Authentication still succeeds.
func TestHandler_ServeHTTP_DoesNotDowngradeBcryptCost(t *testing.T) {
	const (
		userLogin    = "nocostdowngrade"
		userEmail    = "nodowngrade@example.test"
		userPassword = "valid-pw-no-downgrade"
		strongCost   = auth.MinBcryptCost + 2 // 12
		weakCost     = auth.MinBcryptCost     // 10 — operator misconfiguration
	)

	t.Cleanup(func() { _ = auth.SetDefaultBcryptCost(auth.DefaultBcryptCost) })

	ctx := context.Background()

	require.NoError(t, auth.SetDefaultBcryptCost(strongCost))
	strongHash, err := auth.HashPassword(userPassword)
	require.NoError(t, err)

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(ctx, &domain.User{
		Login:    userLogin,
		Email:    userEmail,
		Password: strongHash,
	}))

	// Operator drops to a weaker cost.
	require.NoError(t, auth.SetDefaultBcryptCost(weakCost))

	responder := api.NewResponder()
	handler := NewHandler(
		auth.NewJWTService([]byte("test-secret-key")),
		repo,
		cache.NewInMemory(),
		responder,
		nil,
		nil,
		nil,
		nil,
		0,
	)

	body := []byte(`{"login":"` + userLogin + `","password":"` + userPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	users, err := repo.Find(ctx, &filters.FindUser{Logins: []string{userLogin}}, nil, &filters.Pagination{Limit: 1})
	require.NoError(t, err)
	require.Len(t, users, 1)

	assert.Equal(t, strongHash, users[0].Password,
		"hash must remain at the original strong cost when configured cost is lower")
}

// ---------------------------------------------------------------------------
// Admin MFA-nudge tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — when AUTH_REQUIRE_MFA_FOR_ADMINS is on,
//     an admin without 2FA is nudged to enrol and, once the grace window has
//     elapsed, is issued a token scoped to the enrollment endpoints instead of
//     a full session (ASVS §4.3.1).
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// stubAdminChecker reports a fixed admin verdict for the MFA-nudge tests.
type stubAdminChecker struct {
	isAdmin bool
	err     error
}

func (s stubAdminChecker) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return s.isAdmin, s.err
}

func mfaNudgeConfig(require bool) config.Config {
	var cfg config.Config
	cfg.Auth.RequireMFAForAdmins = require
	cfg.Auth.MFAHardFailDays = 30

	return cfg
}

func doLogin(t *testing.T, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		bytes.NewBufferString(`{"login":"adminuser","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	return w
}

func TestHandler_MFANudge(t *testing.T) {
	hashedPassword, _ := auth.HashPassword("password123")

	newAdmin := func() *domain.User {
		now := time.Now()

		return &domain.User{
			Login:     "adminuser",
			Email:     "admin@example.com",
			Password:  hashedPassword,
			CreatedAt: &now,
			UpdatedAt: &now,
		}
	}

	clock := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }

	t.Run("admin_without_2fa_gets_soft_nudge_and_full_token", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		require.NoError(t, repo.Save(context.Background(), newAdmin()))

		handler := NewHandler(
			auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
			api.NewResponder(), nil, nil,
			mfanudge.New(mfaNudgeConfig(true), clock), stubAdminChecker{isAdmin: true}, 15*time.Minute,
		)

		w := doLogin(t, handler)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.NotEmpty(t, resp["token"], "a soft nudge still issues a full session token")
		assert.NotContains(t, resp, "mfa_enrollment_required")

		nudgeView, ok := resp["mfa_nudge"].(map[string]any)
		require.True(t, ok, "an admin without 2FA must receive the nudge block")
		assert.Equal(t, true, nudgeView["required"])
		assert.Equal(t, true, nudgeView["show_now"])
		assert.Equal(t, false, nudgeView["hard_fail"])

		users, err := repo.Find(
			context.Background(), &filters.FindUser{Logins: []string{"adminuser"}}, nil, &filters.Pagination{Limit: 1},
		)
		require.NoError(t, err)
		require.Len(t, users, 1)
		assert.NotNil(t, users[0].MFAFirstShownAt(), "the first-shown timestamp must be persisted on first contact")
	})

	t.Run("admin_past_grace_window_gets_enrollment_token", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		admin := newAdmin()
		shown := time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC) // 31 days before the clock
		admin.SetMFAFirstShownAt(&shown)
		require.NoError(t, repo.Save(context.Background(), admin))

		handler := NewHandler(
			auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
			api.NewResponder(), nil, nil,
			mfanudge.New(mfaNudgeConfig(true), clock), stubAdminChecker{isAdmin: true}, 15*time.Minute,
		)

		w := doLogin(t, handler)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		assert.Equal(t, true, resp["mfa_enrollment_required"],
			"an admin past the grace window must get an enrollment-scoped session")

		token, _ := resp["token"].(string)
		require.NotEmpty(t, token)

		claims, err := auth.NewJWTService([]byte("test-secret-key")).ValidateToken(token)
		require.NoError(t, err)
		scope, _ := claims.GetScope()
		assert.Equal(t, auth.ScopeMFAEnrollment, scope, "the issued token must carry the enrollment scope")

		nudgeView, ok := resp["mfa_nudge"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, nudgeView["hard_fail"])
	})

	t.Run("non_admin_gets_no_nudge", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		require.NoError(t, repo.Save(context.Background(), newAdmin()))

		handler := NewHandler(
			auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
			api.NewResponder(), nil, nil,
			mfanudge.New(mfaNudgeConfig(true), clock), stubAdminChecker{isAdmin: false}, 15*time.Minute,
		)

		w := doLogin(t, handler)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["token"])
		assert.NotContains(t, resp, "mfa_nudge")
		assert.NotContains(t, resp, "mfa_enrollment_required")
	})

	t.Run("feature_disabled_gets_no_nudge", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		require.NoError(t, repo.Save(context.Background(), newAdmin()))

		handler := NewHandler(
			auth.NewJWTService([]byte("test-secret-key")), repo, cache.NewInMemory(),
			api.NewResponder(), nil, nil,
			mfanudge.New(mfaNudgeConfig(false), clock), stubAdminChecker{isAdmin: true}, 15*time.Minute,
		)

		w := doLogin(t, handler)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotContains(t, resp, "mfa_nudge")
	})
}
