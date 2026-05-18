// API Security Tests for OWASP API Security Top 10:2023.
// Categories covered:
//   - API3:2023 — Broken Object Property Level Authorization (mass-assignment of `roles`,
//     `permissions`, `is_admin` etc. via PUT /api/users/{id})
//   - API5:2023 — Broken Function Level Authorization (admin-only endpoint /api/users/{id})
//
// References:
//   - https://owasp.org/API-Security/editions/2023/en/0xa3-broken-object-property-level-authorization/
//   - https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/
//
// This file complements router_security_escalation_test.go with fuzz testing.
// Run with:
//
//	go test -run NONE -fuzz=FuzzPATAbilities_PostToken_SingleAbility -fuzztime=60s ./internal/api/
//	go test -run NONE -fuzz=FuzzPATAbilities_PostToken_RawBody       -fuzztime=60s ./internal/api/
//	go test -run NONE -fuzz=FuzzPutUserBody_MassAssignment           -fuzztime=60s ./internal/api/
//
// For -fuzztime >= 10m also pass -timeout=0 (go test does not extend -timeout to fit -fuzztime).
// Without -fuzz, the seed corpus runs as a fast smoke test in `go test`.

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/stretchr/testify/require"
)

//nolint:gochecknoglobals
var (
	fuzzEscalationEnvOnce sync.Once
	fuzzEscalationEnvVal  *fuzzEscalationEnv
)

type fuzzEscalationEnv struct {
	env              *securityTestEnv
	regularUserToken string
	regularUserID    uint
	adminUserID      uint
}

func loadFuzzEscalationEnv(tb testing.TB) *fuzzEscalationEnv {
	tb.Helper()

	fuzzEscalationEnvOnce.Do(func() {
		env := setupSecurityTest(tb)
		fuzzEscalationEnvVal = &fuzzEscalationEnv{
			env:              env,
			regularUserToken: issuePASETOToken(tb, env, env.fixtures.RegularUser),
			regularUserID:    env.fixtures.RegularUser.ID,
			adminUserID:      env.fixtures.AdminUser.ID,
		}
	})

	return fuzzEscalationEnvVal
}

// adminAbilitySet is the immutable set of PATAbility values that require admin
// privileges. Any of them appearing on a PAT minted by a regular user signals
// a privilege-escalation bypass.
//
//nolint:gochecknoglobals
var adminAbilitySet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(domain.GetAdminAbilities()))
	for _, a := range domain.GetAdminAbilities() {
		out[string(a)] = struct{}{}
	}

	return out
}()

// containsAdminAbility checks whether `abilities` includes ANY admin-level
// ability — case-sensitive on the canonical form, but also case-insensitive
// for safety against any future case-folding bugs in the validator.
func containsAdminAbility(abilities []domain.PATAbility) (bool, string) {
	for _, ab := range abilities {
		s := string(ab)
		if _, ok := adminAbilitySet[s]; ok {
			return true, s
		}
		// Defence in depth: also flag any ability that lower-cases to a known admin one.
		for known := range adminAbilitySet {
			if strings.EqualFold(s, known) {
				return true, s
			}
		}
		// Final dragnet: lower-cased prefix `admin:` should never make it through
		// for a regular user, even if the validator adds new admin abilities later.
		if strings.HasPrefix(strings.ToLower(s), "admin:") {
			return true, s
		}
	}

	return false, ""
}

// fetchPATFromIssuedToken takes the wire token returned by POST /api/tokens
// (`<id>|<secret>`), parses the id, and reads the persisted record back from
// the in-memory repository so its `Abilities` slice can be inspected.
func fetchPATFromIssuedToken(tb testing.TB, env *fuzzEscalationEnv, issued string) *domain.PersonalAccessToken {
	tb.Helper()

	pipe := strings.IndexByte(issued, '|')
	if pipe <= 0 {
		tb.Fatalf("issued token does not match `<id>|<secret>` format: %q", clamp(issued))
	}

	id, err := strconv.Atoi(issued[:pipe])
	if err != nil || id <= 0 {
		tb.Fatalf("issued token has bogus id segment %q: %v", clamp(issued), err)
	}

	tokens, err := env.env.container.PersonalAccessTokenRepository().Find(
		env.env.ctx,
		filters.FindPersonalAccessTokenByIDs(uint(id)),
		nil,
		&filters.Pagination{Limit: 1},
	)
	require.NoErrorf(tb, err, "find PAT id=%d", id)
	require.Lenf(tb, tokens, 1, "PAT id=%d not found in repo after 2xx response", id)

	return &tokens[0]
}

// patAbilitySingleSeeds returns a representative seed corpus for a single
// PATAbility string parameter — focusing on every observed bypass pattern
// against case-folding, Unicode normalisation, separator confusion, and
// classic injection forms.
func patAbilitySingleSeeds() []string {
	return []string{
		// Canonical admin
		"admin:server:create",
		"admin:gdaemon-task:read",

		// Case variations
		"ADMIN:SERVER:CREATE",
		"Admin:Server:Create",
		"aDmIn:SeRvEr:cReAtE",

		// Whitespace tricks
		" admin:server:create",
		"admin:server:create ",
		"admin:server:create\t",
		"admin:server:create\n",
		"admin:server:create\r\n",

		// Null bytes
		"admin:server:create\x00",
		"admin\x00:server:create",
		"\x00admin:server:create",

		// Unicode lookalikes / NFC-confusables
		" admin:server:create",      // leading non-breaking space
		"admin\u00a0:server:create", // leading non-breaking space
		"admin\u200b:server:create", // zero-width space
		"admin\u202e:server:create", // RTL override
		"admin:server:create\ufeff", // BOM at end

		// Separator / encoding tricks
		"admin%3Aserver%3Acreate",         // percent-encoded colons
		"admin\\:server\\:create",         // backslash escapes
		"admin/server/create",             // wrong separator
		"admin..server..create",           // wrong separator with traversal hint
		"admin:server:create#suffix",      // fragment-style suffix
		"admin:server:create?x=1",         // query-string-style suffix
		"admin:server:create,server:list", // CSV smuggle

		// Empty / primitives
		"",
		" ",
		"null",
		"true",

		// Valid baseline (not a bypass — used as oracle truth check)
		"server:list",

		// Length boundaries
		strings.Repeat("a", 1024),
		strings.Repeat("a", 8192),
	}
}

// FuzzPATAbilities_PostToken_SingleAbility fuzzes a single ability string and
// places it into a JSON body, then POSTs to /api/tokens as a regular user.
// The handler is supposed to reject any admin-level ability via
// validateAdminAbilities (handler.go:128). This fuzz target hammers the validator
// with case-folded, Unicode-confusable, and encoding-tricked inputs — any ability
// that bypasses the validator will appear on the persisted PAT, which we read
// back from the repo and inspect.
//
// Run: `go test -run NONE -fuzz=FuzzPATAbilities_PostToken_SingleAbility -fuzztime=60s ./internal/api/`.
func FuzzPATAbilities_PostToken_SingleAbility(f *testing.F) {
	for _, seed := range patAbilitySingleSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, ability string) {
		env := loadFuzzEscalationEnv(t)

		// Build a syntactically clean JSON body — any malformed shape would
		// just be a parser test, which the RawBody fuzz target already covers.
		bodyBytes, err := json.Marshal(map[string]any{
			"token_name": "fuzz-single",
			"abilities":  []string{ability},
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(string(bodyBytes)))
		req.Header.Set("Authorization", "Bearer "+env.regularUserToken)
		req.Header.Set("Content-Type", "application/json")

		w := doRequestRaw(t, env.env, req)

		// Invariant 1: handler must not panic.
		require.NotEqualf(t, http.StatusInternalServerError, w.Code,
			"POST /api/tokens panicked on ability=%q body=%s", clamp(ability), w.Body.String())

		// Invariant 2: if a token was issued, it must NOT carry an admin ability.
		if w.Code >= 200 && w.Code < 300 {
			var resp struct {
				Token string `json:"token"`
			}
			require.NoErrorf(t, json.Unmarshal(w.Body.Bytes(), &resp),
				"parse token response on 2xx: body=%s", w.Body.String())

			pat := fetchPATFromIssuedToken(t, env, resp.Token)
			if pat.Abilities == nil {
				return
			}

			has, which := containsAdminAbility(*pat.Abilities)
			if has {
				t.Fatalf("PRIVILEGE ESCALATION via PostToken: regular user got admin ability %q "+
					"on the persisted PAT; input ability=%q response=%s",
					which, clamp(ability), w.Body.String())
			}
		}
	})
}

// patBodyRawSeeds returns whole-body JSON seeds — including type-confusion
// shapes, primitive roots, and key-case variations — that should never let a
// regular user mint a PAT with an admin-level ability.
func patBodyRawSeeds() []string {
	return []string{
		// Sanity baseline
		`{"token_name":"x","abilities":["server:list"]}`,

		// Direct attack
		`{"token_name":"x","abilities":["admin:server:create"]}`,
		`{"token_name":"x","abilities":["admin:gdaemon-task:read"]}`,

		// Mixed
		`{"token_name":"x","abilities":["server:list","admin:server:create"]}`,
		`{"token_name":"x","abilities":["admin:server:create","server:list"]}`,

		// Key reordering
		`{"abilities":["admin:server:create"],"token_name":"x"}`,

		// Key-case
		`{"TOKEN_NAME":"x","ABILITIES":["admin:server:create"]}`,
		`{"Token_Name":"x","Abilities":["admin:server:create"]}`,

		// Type confusion: array of arrays
		`{"token_name":"x","abilities":[["admin:server:create"]]}`,
		// Type confusion: string instead of array
		`{"token_name":"x","abilities":"admin:server:create"}`,
		// Type confusion: object instead of array
		`{"token_name":"x","abilities":{"0":"admin:server:create"}}`,
		// null element in array
		`{"token_name":"x","abilities":[null,"admin:server:create"]}`,
		// duplicates
		`{"token_name":"x","abilities":["admin:server:create","admin:server:create"]}`,

		// Whitespace + encoding inside the value
		`{"token_name":"x","abilities":[" admin:server:create"]}`,
		"{\"token_name\":\"x\",\"abilities\":[\"admin:server:create\\u0000\"]}", // null byte
		`{"token_name":"x","abilities":["admin:server:create"]}`,                // 'a' as \u escape

		// Primitive roots / empty / oddities
		`{}`,
		`[]`,
		`null`,
		`true`,
		`1`,
		`"a string"`,
		``,
		`{"token_name":"","abilities":["admin:server:create"]}`,
		`{"token_name":"x","abilities":[]}`,

		// JSON injection-ish
		`{"token_name":"x","abilities":["server:list"],"_admin":true}`,
		`{"token_name":"x","abilities":["server:list"],"is_admin":true}`,
		`{"token_name":"x","abilities":["server:list"],"roles":["admin"]}`,
	}
}

// FuzzPATAbilities_PostToken_RawBody fuzzes the full JSON body for POST /api/tokens
// against a regular-user session. It exercises both the JSON parser and the
// abilities validator simultaneously, looking for any path that lets an
// admin-level ability slip through onto the persisted PAT.
//
// Run: `go test -run NONE -fuzz=FuzzPATAbilities_PostToken_RawBody -fuzztime=60s ./internal/api/`.
func FuzzPATAbilities_PostToken_RawBody(f *testing.F) {
	for _, seed := range patBodyRawSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body string) {
		env := loadFuzzEscalationEnv(t)

		req := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+env.regularUserToken)
		req.Header.Set("Content-Type", "application/json")

		w := doRequestRaw(t, env.env, req)

		// Invariant 1: handler must not panic on any body shape.
		require.NotEqualf(t, http.StatusInternalServerError, w.Code,
			"POST /api/tokens panicked on body=%q resp=%s", clamp(body), w.Body.String())

		// Invariant 2: if a token was issued, no admin ability may appear.
		if w.Code >= 200 && w.Code < 300 {
			var resp struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Token == "" {
				// 2xx without a token field is suspicious but not an escalation by itself.
				return
			}

			pat := fetchPATFromIssuedToken(t, env, resp.Token)
			if pat.Abilities == nil {
				return
			}

			has, which := containsAdminAbility(*pat.Abilities)
			if has {
				t.Fatalf("PRIVILEGE ESCALATION via PostToken (raw body): regular user minted PAT "+
					"with admin ability %q; input body=%q response=%s",
					which, clamp(body), w.Body.String())
			}
		}
	})
}

// putUserBodySeeds returns JSON shapes targeted at mass-assignment vulnerabilities
// in PUT /api/users/{id}. Even a perfect handler must never service these
// because the endpoint is gated by IsAdminMiddleware — no regular user, no
// matter what they send, may be granted any role/permission/ID change.
func putUserBodySeeds() []string {
	return []string{
		// Empty / no-op
		`{}`,
		`null`,
		`[]`,

		// Benign field
		`{"login":"hijack"}`,
		`{"email":"new@new"}`,

		// Direct privilege escalation
		`{"roles":["admin"]}`,
		`{"role":"admin"}`,
		`{"is_admin":true}`,
		`{"isAdmin":true}`,
		`{"is-admin":true}`,
		`{"admin":true}`,
		`{"permissions":["admin:server:create"]}`,
		`{"abilities":["admin:server:create"]}`,
		`{"login":"x","roles":["admin","user"]}`,

		// Nested escalation (in case any handler walks nested JSON)
		`{"user":{"roles":["admin"]}}`,
		`{"profile":{"roles":["admin"]}}`,
		`{"data":{"role":"admin"}}`,

		// Prototype-pollution shapes
		`{"__proto__":{"role":"admin"}}`,
		`{"constructor":{"prototype":{"role":"admin"}}}`,

		// Identity changes
		`{"id":1}`,
		`{"ID":1}`,
		`{"user_id":1}`,
		`{"login":"admin"}`,

		// Type confusion
		`true`,
		`false`,
		`1`,
		`"string"`,
		`""`,

		// Length extreme
		`{"login":"` + strings.Repeat("A", 10000) + `"}`,
		`{"roles":[` + strings.Repeat(`"admin",`, 100) + `"user"]}`,

		// Unicode key tricks
		`{"roles":["admin"]}`, // \u-encoded "roles"
		`{"ROLES":["admin"]}`,
		`{"Roles":["admin"]}`,
	}
}

// FuzzPutUserBody_MassAssignment fuzzes the JSON body for `PUT /api/users/{self}`
// from a regular-user session. The endpoint is admin-only, so the IsAdminMiddleware
// must reject every variant with 403 long before the handler reads the body.
//
// Inv 1: status code must never be 5xx (= no panic in middleware).
// Inv 2: status code must never be 2xx (= mass-assignment bypass).
//
// We accept 401/403/400/422 and the various 4xx codes as legitimate rejections.
//
// Run: `go test -run NONE -fuzz=FuzzPutUserBody_MassAssignment -fuzztime=60s ./internal/api/`.
func FuzzPutUserBody_MassAssignment(f *testing.F) {
	for _, seed := range putUserBodySeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, body string) {
		env := loadFuzzEscalationEnv(t)

		path := fmt.Sprintf("/api/users/%d", env.regularUserID)
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+env.regularUserToken)
		req.Header.Set("Content-Type", "application/json")

		w := doRequestRaw(t, env.env, req)

		// Invariant 1: no panic in middleware/handler chain.
		require.NotEqualf(t, http.StatusInternalServerError, w.Code,
			"PUT /api/users/{self} panicked on body=%q resp=%s", clamp(body), w.Body.String())

		// Invariant 2: admin endpoint must NEVER allow a non-admin caller through.
		if w.Code >= 200 && w.Code < 300 {
			t.Fatalf("MASS ASSIGNMENT / BFLA BYPASS: PUT %s succeeded for regular user; "+
				"body=%q status=%d response=%s",
				path, clamp(body), w.Code, w.Body.String())
		}
	})
}
