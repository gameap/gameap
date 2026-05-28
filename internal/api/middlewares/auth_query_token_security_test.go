// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins the C-4 (ASVS_L2 §C-4) source-aware token-extraction policy:
//   - PASETO / JWT / PAT in ?token= must be rejected (URLs leak into proxy
//     logs, browser history, Referer headers — only the single-use ≤10 s
//     glst_ short-lived token may travel in the query string).
//   - PAT in a `token` cookie must be rejected (PAT is a machine credential
//     for the API, not a browser session).
//   - The corresponding Authorization-header transport for the same token
//     continues to work, proving the rejection is per-source and not a
//     blanket break of the credential.
//
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa2-broken-authentication/
package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// patShape returns a syntactically valid PAT (`<id>|<48 chars>`). It does NOT
// need to verify successfully: the C-4 source gate rejects the credential
// before any repository lookup, so an opaque value of the right shape is
// sufficient to exercise the rejection path.
func patShape() string {
	return "42|RsTuVwXyZ0123456789abcdefghijklmnopqrstuvwxyzABCD"
}

// buildAuthMiddleware wires the in-memory dependencies for a single test.
func buildAuthMiddleware(t *testing.T, user *domain.User) *AuthMiddleware {
	t.Helper()

	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	tokenRepo := inmemory.NewPersonalAccessTokenRepository()

	jwt := auth.NewJWTService([]byte(testJWTSecret))

	return NewAuthMiddleware(jwt, userRepo, tokenRepo, auth.NoopRevocation{}, nil, api.NewResponder(), nil)
}

// runWithRequest exercises the middleware against a prepared request and
// returns the response recorder. Downstream handler unconditionally writes
// 200 so any non-200 status is the middleware's rejection.
func runWithRequest(t *testing.T, mw *AuthMiddleware, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	return w
}

// TestAuthMiddleware_C4_PASETOInQueryRejected — OWASP API2:2023 — a valid,
// long-lived PASETO (issued at login) MUST be rejected when supplied via
// ?token= because URLs are non-private (logs/history/Referer). The same
// PASETO via Authorization header passes, proving the rejection is
// source-scoped.
func TestAuthMiddleware_C4_PASETOInQueryRejected(t *testing.T) {
	now := time.Now()
	hashed, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &domain.User{
		ID:        7,
		Login:     "alice",
		Email:     "alice@example.test",
		Password:  hashed,
		Name:      new("Alice"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	mw := buildAuthMiddleware(t, user)

	jwt := auth.NewJWTService([]byte(testJWTSecret))
	paseto, err := jwt.GenerateTokenForUser(user, time.Hour)
	require.NoError(t, err)

	t.Run("query_rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected?token="+paseto, nil)
		w := runWithRequest(t, mw, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code,
			"PASETO in ?token= must be rejected as missing-token")
	})

	t.Run("header_still_works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+paseto)
		w := runWithRequest(t, mw, req)

		assert.Equal(t, http.StatusOK, w.Code,
			"same PASETO via Authorization header must still authenticate")
	})
}

// TestAuthMiddleware_C4_PATInQueryRejected — OWASP API2:2023 — a personal
// access token in ?token= is rejected for the same logging-exposure reasons.
// Crucially, an attacker who captures a query log finds a useless string;
// the same PAT in Authorization keeps working for legitimate API clients.
func TestAuthMiddleware_C4_PATInQueryRejected(t *testing.T) {
	now := time.Now()
	hashed, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &domain.User{
		ID:        8,
		Login:     "bob",
		Email:     "bob@example.test",
		Password:  hashed,
		Name:      new("Bob"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	mw := buildAuthMiddleware(t, user)

	req := httptest.NewRequest(http.MethodGet, "/protected?token="+patShape(), nil)
	w := runWithRequest(t, mw, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"PAT in ?token= must be rejected; URLs leak credentials to proxy logs")
}

// TestAuthMiddleware_C4_PATInCookieRejected — OWASP API2:2023 — PATs are
// machine credentials for the API and must not be carried in a browser
// `token` cookie. The cookie source still accepts session PASETOs.
func TestAuthMiddleware_C4_PATInCookieRejected(t *testing.T) {
	now := time.Now()
	hashed, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &domain.User{
		ID:        9,
		Login:     "carol",
		Email:     "carol@example.test",
		Password:  hashed,
		Name:      new("Carol"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	mw := buildAuthMiddleware(t, user)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: patShape()})

	w := runWithRequest(t, mw, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"PAT in cookie must be rejected; PATs are API credentials, not browser sessions")
}

// TestSourceAllowsTokenType_Matrix — OWASP API2:2023 — direct unit test of
// the per-source allow-list. Pinning the matrix here keeps changes to the
// extraction policy reviewable in one place.
func TestSourceAllowsTokenType_Matrix(t *testing.T) {
	cases := []struct {
		name   string
		source tokenSource
		kind   tokenType
		allow  bool
	}{
		{"header_paseto", tokenSourceHeader, tokenTypeUserAuth, true},
		{"header_pat", tokenSourceHeader, tokenTypePersonalAccess, true},
		{"header_shortlived", tokenSourceHeader, tokenTypeShortLived, true},
		{"header_unknown_pass_through", tokenSourceHeader, tokenTypeUnknown, true},

		{"query_shortlived_only", tokenSourceQuery, tokenTypeShortLived, true},
		{"query_paseto_rejected", tokenSourceQuery, tokenTypeUserAuth, false},
		{"query_pat_rejected", tokenSourceQuery, tokenTypePersonalAccess, false},
		{"query_unknown_pass_through", tokenSourceQuery, tokenTypeUnknown, true},

		{"cookie_paseto", tokenSourceCookie, tokenTypeUserAuth, true},
		{"cookie_shortlived", tokenSourceCookie, tokenTypeShortLived, true},
		{"cookie_pat_rejected", tokenSourceCookie, tokenTypePersonalAccess, false},
		{"cookie_unknown_pass_through", tokenSourceCookie, tokenTypeUnknown, true},

		{"unknown_source_rejected", tokenSourceUnknown, tokenTypeUserAuth, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.allow, sourceAllowsTokenType(tc.source, tc.kind))
		})
	}
}
