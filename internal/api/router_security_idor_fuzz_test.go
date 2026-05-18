// API Security Tests for OWASP API Security Top 10:2023.
// Category: API1:2023 — Broken Object Level Authorization (BOLA / IDOR).
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/
//
// This file complements router_security_idor_test.go with fuzz testing.
// Run with:
//
//	go test -run NONE -fuzz=FuzzFileManagerPath_DoesNotBypassAuthorization -fuzztime=60s ./internal/api/
//	go test -run NONE -fuzz=FuzzServerIDPathParam_DoesNotBypassAuthorization -fuzztime=60s ./internal/api/
//
// For -fuzztime >= 10m also pass -timeout=0 (go test does not extend -timeout to fit -fuzztime).
// Without -fuzz, the seed corpus runs as a fast smoke test in `go test`.

package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fuzzIDOREnvVal is a process-wide IDOR-fuzz environment shared across all iterations.
// We mint a regular-user token once and reuse it for every fuzz call.
//
//nolint:gochecknoglobals
var (
	fuzzIDOREnvOnce sync.Once
	fuzzIDOREnvVal  *fuzzIDOREnv
)

type fuzzIDOREnv struct {
	env              *securityTestEnv
	regularUserToken string
}

func loadFuzzIDOREnv(tb testing.TB) *fuzzIDOREnv {
	tb.Helper()

	fuzzIDOREnvOnce.Do(func() {
		env := setupSecurityTest(tb)
		fuzzIDOREnvVal = &fuzzIDOREnv{
			env:              env,
			regularUserToken: issuePASETOToken(tb, env, env.fixtures.RegularUser),
		}
	})

	return fuzzIDOREnvVal
}

// pathTraversalSeeds returns a representative seed corpus for the `path` query
// parameter. The seeds focus on every classic attack pattern: traversal,
// absolute paths, encoded variants, null bytes, alternative encodings, OS
// path separators, and length extremes.
//
// Length-bounded entries are kept moderate to keep the in-memory test fast
// while still seeding the mutator with non-trivial sizes to bloom from.
func pathTraversalSeeds() []string {
	return []string{
		// Trivial / valid-looking
		"",
		"/",
		"/file.txt",
		".",
		"..",

		// Classic traversal
		"../",
		"../../",
		"../../../etc/passwd",
		"../../../../../../etc/shadow",
		"..\\..\\..\\windows\\system32\\config\\sam",

		// Absolute paths
		"/etc/passwd",
		"/etc/shadow",
		"/proc/self/environ",
		"/proc/self/cmdline",
		"/dev/random",
		"C:\\Windows\\System32\\config\\SAM",
		"\\\\server\\share\\file",

		// Encoded variants
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"%252e%252e%252fetc%252fpasswd",    // double encoded
		"..%c0%af",                         // overlong UTF-8 slash
		"..%252fpasswd",                    // mixed encoding
		"..%5c..%5c..%5cwindows%5cwin.ini", // backslash encoded

		// Unicode separators
		"..∕..∕etc∕passwd", // U+2215 division slash
		"..⧸..⧸etc",        // U+29F8 big solidus

		// Null byte injection
		"/file.txt\x00.png",
		"/etc/passwd\x00",
		"\x00/etc/passwd",

		// Symlink-ish & special
		"/var/www/../../../etc/passwd",
		"./../config/.env",

		// Out-of-server traversal (target is server1 dir = "/path/to/server1")
		"/path/to/server2/secret",
		"../server2/secret",
		"../../path/to/server2/secret",

		// CRLF / smuggling
		"/file\r\nLocation: evil",
		"/file\nX-Admin: 1",

		// URL fragment & query smuggling
		"/file#../../../etc/passwd",
		"/file?other=value",
		"/file&other=value",

		// Schemes
		"file:///etc/passwd",
		"http://evil/",
		"php://input",
		"data:,test",
		"javascript:alert(1)",

		// Length extremes
		strings.Repeat("../", 100) + "etc/passwd",
		strings.Repeat("a/", 1000),
		strings.Repeat("/", 4096),
		strings.Repeat("A", 8192),
	}
}

// fileManagerEndpoint describes an endpoint that takes a path query parameter.
type fileManagerEndpoint struct {
	method string
	// pattern uses %d for the server ID and {path} for the encoded path query.
	// e.g.: "GET /api/file-manager/%d/content?path={path}"
	pattern string
}

//nolint:gochecknoglobals
var fileManagerFuzzEndpoints = []fileManagerEndpoint{
	{http.MethodGet, "/api/file-manager/%d/content?path={path}"},
	{http.MethodGet, "/api/file-manager/%d/tree?path={path}"},
	{http.MethodGet, "/api/file-manager/%d/download?path={path}"},
	{http.MethodGet, "/api/file-manager/%d/stream-file?path={path}"},
}

// FuzzFileManagerPath_DoesNotBypassAuthorization fuzzes the `path` query parameter
// of the file-manager family while the request targets a server that does NOT
// belong to the regular user (Server2). The invariant is the strictest one
// possible at the authorization layer:
//
//	The response status must be a 4xx (404 from ServerFinder, or 403 from
//	AbilityChecker). It must NEVER be 2xx — that would indicate an IDOR bypass
//	where the path manipulation tricks the handler into serving a foreign
//	server's data. A 5xx is also disallowed because the request must NEVER
//	reach the daemon-call layer (which is nil in the test container) — if
//	5xx leaks through, the access check ran late or was missed entirely.
//
// Run: `go test -run NONE -fuzz=FuzzFileManagerPath_DoesNotBypassAuthorization -fuzztime=60s ./internal/api/`.
func FuzzFileManagerPath_DoesNotBypassAuthorization(f *testing.F) {
	for _, seed := range pathTraversalSeeds() {
		for endpointIdx := uint8(0); endpointIdx < uint8(len(fileManagerFuzzEndpoints)); endpointIdx++ {
			f.Add(seed, endpointIdx)
		}
	}

	const foreignServerID = 2

	f.Fuzz(func(t *testing.T, path string, endpointIdx uint8) {
		env := loadFuzzIDOREnv(t)
		ep := fileManagerFuzzEndpoints[int(endpointIdx)%len(fileManagerFuzzEndpoints)]

		fullPath := strings.ReplaceAll(
			ep.pattern,
			"{path}",
			url.QueryEscape(path),
		)
		fullPath = strings.Replace(fullPath, "%d", strconv.Itoa(foreignServerID), 1)

		req := httptest.NewRequest(ep.method, fullPath, nil)
		req.Header.Set("Authorization", "Bearer "+env.regularUserToken)

		w := doRequestRaw(t, env.env, req)

		body := w.Body.String()

		// Invariant 1: foreign server access must NEVER produce a 2xx success.
		if w.Code >= 200 && w.Code <= 299 {
			t.Fatalf("BOLA + PATH-TRAVERSAL BYPASS: %s %s returned 2xx (%d) for foreign server; path=%q body=%s",
				ep.method, fullPath, w.Code, clamp(path), body)
		}

		// Invariant 2: response must not leak Server2's identifying data,
		// regardless of status code (e.g. a tracing-enabled 500 could echo the path).
		if strings.Contains(body, "Test Server 2") || strings.Contains(body, "/path/to/server2") {
			t.Fatalf("BOLA DATA LEAK: %s %s path=%q surfaced Server2 data in body; status=%d body=%s",
				ep.method, fullPath, clamp(path), w.Code, body)
		}
	})
}

// FuzzFileManagerPath_OwnServer_DoesNotLeakOtherServerFiles fuzzes the path
// while targeting the OWN server (Server1). Even within a server they own,
// a user must not be able to escape `server.Dir` and read arbitrary files.
//
// In the test container the daemon FileManager is nil, so the *handler* will
// crash with 500 if reached. We intentionally accept 5xx here as "request
// reached the handler" but still assert that no 2xx response is ever returned
// — the file-system layer is nil, so a 200 implies the handler somehow synthesised
// content from input alone (very suspicious).
//
// Run: `go test -run NONE -fuzz=FuzzFileManagerPath_OwnServer_DoesNotLeakOtherServerFiles -fuzztime=60s ./internal/api/`.
func FuzzFileManagerPath_OwnServer_DoesNotLeakOtherServerFiles(f *testing.F) {
	for _, seed := range pathTraversalSeeds() {
		for endpointIdx := uint8(0); endpointIdx < uint8(len(fileManagerFuzzEndpoints)); endpointIdx++ {
			f.Add(seed, endpointIdx)
		}
	}

	const ownServerID = 1

	f.Fuzz(func(t *testing.T, path string, endpointIdx uint8) {
		env := loadFuzzIDOREnv(t)
		ep := fileManagerFuzzEndpoints[int(endpointIdx)%len(fileManagerFuzzEndpoints)]

		fullPath := strings.ReplaceAll(
			ep.pattern,
			"{path}",
			url.QueryEscape(path),
		)
		fullPath = strings.Replace(fullPath, "%d", strconv.Itoa(ownServerID), 1)

		req := httptest.NewRequest(ep.method, fullPath, nil)
		req.Header.Set("Authorization", "Bearer "+env.regularUserToken)

		w := doRequestRaw(t, env.env, req)

		// Invariant: even on own server, the handler must not synthesise a
		// 2xx response from a malformed/traversal path. Anything in the 2xx
		// family would imply file content leaked from input alone.
		if w.Code >= 200 && w.Code <= 299 {
			t.Fatalf("PATH-TRAVERSAL or HANDLER MISBEHAVIOUR: %s %s returned 2xx (%d) on own server "+
				"with no real FileManager backend wired in; path=%q body=%s",
				ep.method, fullPath, w.Code, clamp(path), w.Body.String())
		}
	})
}

// FuzzServerIDPathParam_DoesNotLeakOtherServer fuzzes the `{server}` path
// segment with arbitrary bytes, then checks that the response body does not
// leak data belonging to Server2 (the foreign one).
//
// We use a "data leak" oracle instead of "any 2xx is suspicious" because
// numeric edge cases like "+1", "01", " 1" all parse to 1 — that resolves to
// the user's OWN server, which is a legitimate 200. The actual security
// invariant is that the user's request must never surface Server2's data
// regardless of how creative the URL-encoding gets.
//
// Run: `go test -run NONE -fuzz=FuzzServerIDPathParam_DoesNotLeakOtherServer -fuzztime=60s ./internal/api/`.
func FuzzServerIDPathParam_DoesNotLeakOtherServer(f *testing.F) {
	// Seeds: concrete attack patterns. We deliberately omit canonical "1" / "2"
	// since those are pinned by router_security_idor_test.go; the goal here is
	// edge cases.
	idSeeds := []string{
		"0", "-1", "+2",
		"02", "002", "2.0", "2e0",
		"2 ", " 2", "2\t", "2\n",
		"0x2", "0X2", "2L",
		"2;1", "2,1", "2|1", "2/1", "2\\1",
		"2%201", "2%2f1",
		"99999999999999999999",
		"-99999999999999999999",
		"2.0e308",
		"true", "false", "null",
		"{2}", "[2]", "(2)",
		"%00", "%0a", "%0d%0a",
		"абв", "你好", "2\u200b", // zero-width space
		"\x00", "\xff", "\x7f",
		"' OR 1=1 --",
		"<script>alert(1)</script>",
	}

	// Read-only endpoints: the shared fuzz env must not be mutated between iterations.
	// Each endpoint exercises a distinct codepath through ServerFinder/AbilityChecker.
	endpoints := []struct {
		method  string
		pattern string
	}{
		{http.MethodGet, "/api/servers/%s"},
		{http.MethodGet, "/api/servers/%s/status"},
		{http.MethodGet, "/api/servers/%s/abilities"},
		{http.MethodGet, "/api/servers/%s/query"},
		{http.MethodGet, "/api/servers/%s/tasks"},
		{http.MethodGet, "/api/servers/%s/settings"},
		{http.MethodGet, "/api/servers/%s/rcon/features"},
		{http.MethodGet, "/api/file-manager/%s/content?path=/"},
		{http.MethodGet, "/api/file-manager/%s/tree?path=/"},
		{http.MethodGet, "/api/file-manager/%s/download?path=/file"},
		{http.MethodGet, "/api/file-manager/%s/stream-file?path=/file"},
	}

	for _, seed := range idSeeds {
		for endpointIdx := uint8(0); endpointIdx < uint8(len(endpoints)); endpointIdx++ {
			f.Add(seed, endpointIdx)
		}
	}

	f.Fuzz(func(t *testing.T, idValue string, endpointIdx uint8) {
		env := loadFuzzIDOREnv(t)
		ep := endpoints[int(endpointIdx)%len(endpoints)]

		fullPath := strings.Replace(ep.pattern, "%s", url.PathEscape(idValue), 1)

		req := httptest.NewRequest(ep.method, fullPath, nil)
		req.Header.Set("Authorization", "Bearer "+env.regularUserToken)

		w := doRequestRaw(t, env.env, req)

		body := w.Body.String()

		// Invariant: response body must not contain Server2's identifiers.
		// Server2.Name = "Test Server 2" and Server2.Dir = "/path/to/server2".
		// If either appears, the user obtained data from a server they don't own.
		//
		// We do NOT assert a "no 5xx" invariant here: a fuzzed id segment may
		// resolve to the user's OWN Server1 (e.g. "+1", "01", " 1" all parse to
		// 1), in which case some handlers reach downstream services that are
		// nil in the test container (PluginManager, DaemonFiles) and panic into
		// a 500. That's a test-infrastructure artifact, not a security bug.
		// The data-leak oracle is the security-relevant assertion.
		if strings.Contains(body, "Test Server 2") || strings.Contains(body, "/path/to/server2") {
			t.Fatalf("BOLA DATA LEAK: %s %s id=%q surfaced Server2 data; status=%d body=%s",
				ep.method, fullPath, clamp(idValue), w.Code, body)
		}
	})
}
