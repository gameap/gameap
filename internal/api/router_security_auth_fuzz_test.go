// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa2-broken-authentication/
//
// This file complements router_security_auth_test.go with fuzz testing.
// Run with:
//
//	go test -run NONE -fuzz=FuzzAuthMiddleware_TokenParsing       -fuzztime=60s ./internal/api/
//	go test -run NONE -fuzz=FuzzAuthMiddleware_AuthorizationHeader -fuzztime=60s ./internal/api/
//	go test -run NONE -fuzz=FuzzAuthMiddleware_AdminEndpointBypass -fuzztime=60s ./internal/api/
//
// For -fuzztime >= 10m also pass -timeout=0 (go test does not extend -timeout to fit -fuzztime).
// Without -fuzz, the seed corpus runs as a fast smoke test in `go test`.

package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/require"
)

// fuzzAuthEnv is a process-wide environment shared across all fuzz iterations.
// Building it once is critical: SetupFixtures takes ~5ms, multiplied by hundreds
// of thousands of iterations that becomes the bottleneck. Every iteration only
// reads from the env, never mutates it, so sharing is safe.
//
// We hold the *known-valid* PASETO and PAT tokens too, so the assertion code
// can decide whether a 200 reply is "expected (matched a valid token)" or
// "an actual authentication bypass that fuzz just found".
//
//nolint:gochecknoglobals
var (
	fuzzAuthEnvOnce sync.Once
	fuzzAuthEnvVal  *fuzzAuthEnv
)

type fuzzAuthEnv struct {
	env              *securityTestEnv
	validAdminPASETO string
	validUserPASETO  string
	validAdminPAT    string
	validUserPAT     string
}

func loadFuzzAuthEnv(tb testing.TB) *fuzzAuthEnv {
	tb.Helper()

	fuzzAuthEnvOnce.Do(func() {
		env := setupSecurityTest(tb)
		fuzzAuthEnvVal = &fuzzAuthEnv{
			env:              env,
			validAdminPASETO: issuePASETOToken(tb, env, env.fixtures.AdminUser),
			validUserPASETO:  issuePASETOToken(tb, env, env.fixtures.RegularUser),
			validAdminPAT: issuePAT(tb, env, env.fixtures.AdminUser, []domain.PATAbility{
				domain.PATAbilityServerList,
			}),
			validUserPAT: issuePAT(tb, env, env.fixtures.RegularUser, []domain.PATAbility{
				domain.PATAbilityServerList,
			}),
		}
	})

	return fuzzAuthEnvVal
}

// validTokenSet returns the set of tokens that legitimately authenticate.
func (e *fuzzAuthEnv) validTokenSet() map[string]struct{} {
	return map[string]struct{}{
		e.validAdminPASETO: {},
		e.validUserPASETO:  {},
		e.validAdminPAT:    {},
		e.validUserPAT:     {},
	}
}

// authTokenSeeds returns a representative seed corpus for token-fuzzing.
// The seeds focus on every known parsing branch in auth.go (PASETO/JWT/PAT),
// every classic bypass attempt (CRLF, null bytes, double-Bearer, scheme spoof),
// and the four lengths that historically break parsers.
func authTokenSeeds() []string {
	return []string{
		"",
		" ",
		"\x00",
		"\r\n",
		"Bearer",
		"Bearer ",
		"bearer abc",
		"BEARER abc",
		"BeArEr abc",
		"Basic dXNlcjpwYXNz",
		"Digest username=\"a\"",
		"Token abc",

		// JWT family
		"eyJhbGciOiJub25lIn0.e30.",                  // alg:none header (CVE-class)
		"eyJhbGciOiJIUzI1NiJ9.e30.signature",        // looks like JWT but invalid signature
		"eyJ" + strings.Repeat("A", 4096),           // very long JWT-prefixed
		"eyJ\x00\x00\x00",                           // null bytes inside JWT-prefix
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9", // missing signature segment

		// PASETO family
		"v4.local.",
		"v4.public.",
		"v4.local." + strings.Repeat("a", 4096),
		"v4.local.\x00\x00",
		"v4.public.eyJleHAiOiIyMDM5In0",

		// Personal Access Token family — look-alike formats
		"1|",
		"|secret",
		"0|secret",
		"-1|secret",
		"99999999999999999999|secret",
		"1|secret|extra",
		"abc|secret",
		"1|" + strings.Repeat("A", 4096),

		// CRLF / header smuggling
		"Bearer abc\r\nX-Admin: true",
		"Bearer abc\nLocation: evil",
		"Bearer abc injection",
		"Bearer\rabc",

		// Unicode / encoding edge cases
		"Bearer \u202eabc", // RTL override
		"Bearer 𝓪𝓭𝓶𝓲𝓷",     // mathematical alphanumeric
		"Bearer admin\x00", // null byte at end of token
		"Bearer ad\x00min", // null byte in the middle of token
		"Bearer админ",     // cyrillic

		// Length boundaries
		strings.Repeat("A", 1),
		strings.Repeat("A", 64),
		strings.Repeat("A", 1024),
		strings.Repeat("A", 65536),

		// Multiple valid-looking prefixes
		"Bearer eyJ.v4.local.1|x",
	}
}

// FuzzAuthMiddleware_AuthorizationHeader fuzzes the value of the Authorization
// HTTP header on a vanilla user-level endpoint. The invariants pinned here:
//
//  1. The middleware must NEVER produce a 5xx response (= no parsing panic).
//  2. The middleware must NEVER authenticate a request unless the supplied
//     header value matches one of the tokens we minted at setup time —
//     otherwise the response code must be 401 Unauthorized.
//
// Run: `go test -run NONE -fuzz=FuzzAuthMiddleware_AuthorizationHeader -fuzztime=30s ./internal/api/`.
func FuzzAuthMiddleware_AuthorizationHeader(f *testing.F) {
	for _, seed := range authTokenSeeds() {
		f.Add(seed)
	}

	// NOTE: do NOT seed real valid tokens here. Fuzz workers run in a separate
	// process where sync.Once re-initialises with fresh tokens, so a token
	// serialised at seed time will not match the worker's validTokenSet.
	// The mutator cannot forge an HMAC-signed PASETO/JWT by chance, so the
	// assertion below is sound: any 200 reply during fuzz is a true bypass.

	f.Fuzz(func(t *testing.T, headerValue string) {
		env := loadFuzzAuthEnv(t)

		req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
		// Bypass net/http header validation by writing into the map directly:
		// real-world clients can and do send arbitrary bytes here.
		req.Header["Authorization"] = []string{headerValue}

		w := doRequestRaw(t, env.env, req)

		// Invariant 1: no panic / internal server error from auth code.
		require.NotEqualf(t, http.StatusInternalServerError, w.Code,
			"auth middleware panicked on header=%q body=%s", clamp(headerValue), w.Body.String())

		// Invariant 2: 200 only with a known-valid token wrapped in a Bearer scheme.
		if w.Code == http.StatusOK {
			extracted := extractBearerToken(headerValue)
			if _, valid := env.validTokenSet()[extracted]; !valid {
				t.Fatalf("AUTHENTICATION BYPASS: header=%q produced HTTP 200 without a valid bearer token; "+
					"extracted=%q body=%s", clamp(headerValue), clamp(extracted), w.Body.String())
			}
		}
	})
}

// FuzzAuthMiddleware_TokenParsing fuzzes the *raw* token value across the three
// transport channels supported by the auth middleware (Authorization Bearer,
// `?token=` query, and `token` cookie). It pins the same two invariants as
// FuzzAuthMiddleware_AuthorizationHeader but exercises the multi-channel path
// where a fuzz mutator can find sneaky bypasses (e.g. cookie that "looks like"
// a JWT or query string that bleeds into a header).
//
// Run: `go test -run NONE -fuzz=FuzzAuthMiddleware_TokenParsing -fuzztime=30s ./internal/api/`.
func FuzzAuthMiddleware_TokenParsing(f *testing.F) {
	for _, seed := range authTokenSeeds() {
		// Run each seed across all four channels, combining with channel index.
		for ch := uint8(0); ch <= 3; ch++ {
			f.Add(seed, ch)
		}
	}
	// NOTE: see FuzzAuthMiddleware_AuthorizationHeader for why we don't seed
	// with real valid tokens — they live in different processes from the fuzzer.

	f.Fuzz(func(t *testing.T, token string, channel uint8) {
		env := loadFuzzAuthEnv(t)
		req := buildAuthFuzzRequest(t, "/api/user", token, channel)

		w := doRequestRaw(t, env.env, req)

		// Invariant 1: no panic.
		require.NotEqualf(t, http.StatusInternalServerError, w.Code,
			"panic on token=%q channel=%d body=%s", clamp(token), channel, w.Body.String())

		// Invariant 2: 200 only if the supplied raw token is actually valid.
		if w.Code == http.StatusOK {
			if _, valid := env.validTokenSet()[token]; !valid {
				t.Fatalf("AUTHENTICATION BYPASS: token=%q channel=%d produced 200 without a known token; body=%s",
					clamp(token), channel, w.Body.String())
			}
		}
	})
}

// FuzzAuthMiddleware_AdminEndpointBypass fuzzes the same channels but against an
// admin-only endpoint. The invariant is stricter:
//
//  1. No 5xx (no panic).
//  2. The response is 200 only if the supplied token is the AdminUser's token —
//     never for the regular user, never for any garbage.
//
// This catches bugs where IsAdminMiddleware mis-evaluates a tampered token.
//
// Run: `go test -run NONE -fuzz=FuzzAuthMiddleware_AdminEndpointBypass -fuzztime=30s ./internal/api/`.
func FuzzAuthMiddleware_AdminEndpointBypass(f *testing.F) {
	for _, seed := range authTokenSeeds() {
		for ch := uint8(0); ch <= 3; ch++ {
			f.Add(seed, ch)
		}
	}

	f.Fuzz(func(t *testing.T, token string, channel uint8) {
		env := loadFuzzAuthEnv(t)
		req := buildAuthFuzzRequest(t, "/api/users", token, channel)

		w := doRequestRaw(t, env.env, req)

		require.NotEqualf(t, http.StatusInternalServerError, w.Code,
			"admin endpoint panicked: token=%q channel=%d body=%s", clamp(token), channel, w.Body.String())

		// On /api/users, only the admin tokens (validAdminPASETO/validAdminPAT)
		// should produce 200. Any other token producing 200 is a BFLA bypass.
		if w.Code == http.StatusOK {
			adminSet := map[string]struct{}{
				env.validAdminPASETO: {},
				env.validAdminPAT:    {},
			}
			if _, isAdmin := adminSet[token]; !isAdmin {
				t.Fatalf("BFLA BYPASS: admin endpoint /api/users returned 200 for non-admin token %q channel=%d body=%s",
					clamp(token), channel, w.Body.String())
			}
		}
	})
}

// buildAuthFuzzRequest constructs a request that delivers `token` through one of:
//   - 0: Authorization: Bearer <token>
//   - 1: ?token=<token>  (URL-encoded)
//   - 2: Cookie: token=<token>
//   - 3: Authorization: <token>  (raw, no Bearer prefix — exercises non-Bearer schemes)
func buildAuthFuzzRequest(tb testing.TB, path, token string, channel uint8) *http.Request {
	tb.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)

	switch channel & 0b11 {
	case 0:
		req.Header["Authorization"] = []string{"Bearer " + token}
	case 1:
		// Build query string by hand because url.Values.Encode normalises the form.
		req.URL.RawQuery = "token=" + url.QueryEscape(token)
	case 2:
		req.Header["Cookie"] = []string{"token=" + token}
	case 3:
		req.Header["Authorization"] = []string{token}
	}

	return req
}

// extractBearerToken pulls the token out of a Bearer header value, mirroring
// the logic in auth.go:128-152 (case-insensitive scheme match, single space).
// Returns "" if no Bearer scheme is present — those should never authenticate.
func extractBearerToken(header string) string {
	const prefix = "bearer "
	if len(header) < len(prefix) {
		return ""
	}

	if !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}

	return header[len(prefix):]
}

// clamp shortens a token to the first 80 runes for safe inclusion in error messages —
// fuzz inputs can be megabytes long.
func clamp(s string) string {
	const maxRunes = 80
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	out := make([]rune, 0, maxRunes+1)
	count := 0
	for _, r := range s {
		if count >= maxRunes {
			out = append(out, '…')

			break
		}
		out = append(out, r)
		count++
	}

	return string(out)
}
