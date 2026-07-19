// API Security Tests for OWASP API Security Top 10:2023.
// Category: API8:2023 — Security Misconfiguration.
//
// These tests pin the global HTTP response security headers that
// SecurityHeadersMiddleware emits (HSTS, X-Content-Type-Options,
// X-Frame-Options, Referrer-Policy, Content-Security-Policy). A regression
// here would silently disable clickjacking / MIME-sniffing / TLS-downgrade /
// XSS / referrer-leakage protection panel-wide.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package middlewares_test

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/services/captcha"
	webstatic "github.com/gameap/gameap/web/static"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	indexHTMLInline = `(function(){ window.__theme = 'light'; })();`
	mitmHTMLInline  = `(function(){ navigator.serviceWorker.register('sw.js'); })();`
)

func newTestFS(t *testing.T) fstest.MapFS {
	t.Helper()

	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!doctype html><html><head><script>" + indexHTMLInline + "</script>" +
				`<script type="module" src="/assets/main.js"></script></head><body></body></html>`),
		},
		"streamsaver/mitm.html": &fstest.MapFile{
			Data: []byte("<!doctype html><html><body><script>" + mitmHTMLInline + "</script></body></html>"),
		},
	}
}

func expectedHash(t *testing.T, body string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(body))

	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func baseSecureConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Security.Enabled = true
	cfg.Security.HSTS.Enabled = true
	cfg.Security.HSTS.MaxAge = 31536000
	cfg.Security.ContentTypeOptions = true
	cfg.Security.FrameOptions = "SAMEORIGIN"
	cfg.Security.ReferrerPolicy = "strict-origin-when-cross-origin"
	cfg.Security.CSP.Enabled = true
	cfg.Security.SensitivePathPrefixes = []string{
		"/api/auth/",
		"/api/profile/",
		"/api/users/",
		"/api/tokens/",
	}

	return cfg
}

// runMiddleware wraps the middleware around a tiny handler and returns the
// captured response headers. The body is closed internally since these tests
// only care about headers.
func runMiddleware(
	t *testing.T,
	cfg *config.Config,
	staticFS fstest.MapFS,
	prepare func(r *http.Request),
	downstream http.HandlerFunc,
) http.Header {
	t.Helper()

	m, err := middlewares.NewSecurityHeadersMiddleware(cfg, staticFS)
	require.NoError(t, err)

	handler := downstream
	if handler == nil {
		handler = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
	}

	r := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if prepare != nil {
		prepare(r)
	}

	rr := httptest.NewRecorder()
	m.Middleware(handler).ServeHTTP(rr, r)

	resp := rr.Result()
	_ = resp.Body.Close()

	return resp.Header
}

// TestSecurityHeaders_Defaults — OWASP API8:2023 — verifies that the default
// configuration emits every protective header with the documented value and
// that the generated CSP carries the inline-script hash for both shipped
// HTML documents.
func TestSecurityHeaders_Defaults(t *testing.T) {
	cfg := baseSecureConfig()
	staticFS := newTestFS(t)

	resp := runMiddleware(t, cfg, staticFS, nil, nil)

	assert.Equal(t, "nosniff", resp.Get("X-Content-Type-Options"))
	assert.Equal(t, "SAMEORIGIN", resp.Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", resp.Get("Referrer-Policy"))
	assert.Empty(t, resp.Get("Strict-Transport-Security"), "no HSTS on plain HTTP")

	csp := resp.Get("Content-Security-Policy")
	require.NotEmpty(t, csp)
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "object-src 'none'")
	assert.Contains(t, csp, "frame-ancestors 'self'")
	assert.Contains(t, csp, expectedHash(t, indexHTMLInline))
	assert.Contains(t, csp, expectedHash(t, mitmHTMLInline))
	assert.Contains(t, csp, "worker-src 'self' blob:")
}

// TestSecurityHeaders_MasterSwitchOff — OWASP API8:2023 — confirms
// SECURITY_HEADERS_ENABLED=false fully bypasses the middleware so it cannot
// accidentally emit a stale policy in a deployment that disabled it.
func TestSecurityHeaders_MasterSwitchOff(t *testing.T) {
	cfg := baseSecureConfig()
	cfg.Security.Enabled = false

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

	for _, header := range []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
		"Content-Security-Policy-Report-Only",
		"Strict-Transport-Security",
	} {
		assert.Empty(t, resp.Get(header), header+" must not be set when disabled")
	}
}

// TestSecurityHeaders_HSTSEmission — OWASP API8:2023 — pins the HSTS
// emission rule: only on TLS, X-Forwarded-Proto: https, or when ForceHTTPS
// is on. Plain HTTP without a proxy hint must NEVER receive HSTS, otherwise
// a localhost dev request would lock the user out of http://localhost for a
// year.
func TestSecurityHeaders_HSTSEmission(t *testing.T) {
	cases := []struct {
		name         string
		forceHTTPS   bool
		tlsState     *tls.ConnectionState
		forwardedFor string
		wantHeader   string
	}{
		{
			name:       "plain_http_no_hsts",
			wantHeader: "",
		},
		{
			name:       "tls_request_hsts_set",
			tlsState:   &tls.ConnectionState{},
			wantHeader: "max-age=31536000",
		},
		{
			name:         "forwarded_proto_https_hsts_set",
			forwardedFor: "https",
			wantHeader:   "max-age=31536000",
		},
		{
			name:       "force_https_emits_hsts_even_on_plain_http",
			forceHTTPS: true,
			wantHeader: "max-age=31536000",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseSecureConfig()
			cfg.TLS.ForceHTTPS = tc.forceHTTPS

			resp := runMiddleware(t, cfg, newTestFS(t), func(r *http.Request) {
				if tc.tlsState != nil {
					r.TLS = tc.tlsState
				}
				if tc.forwardedFor != "" {
					r.Header.Set("X-Forwarded-Proto", tc.forwardedFor)
				}
			}, nil)

			assert.Equal(t, tc.wantHeader, resp.Get("Strict-Transport-Security"))
		})
	}
}

// TestSecurityHeaders_HSTSFormatting — OWASP API8:2023 — verifies the
// max-age / includeSubDomains / preload formatting so a misconfigured deploy
// cannot accidentally drop the directives.
func TestSecurityHeaders_HSTSFormatting(t *testing.T) {
	cases := []struct {
		name              string
		maxAge            int
		includeSubDomains bool
		preload           bool
		want              string
	}{
		{"max_age_only", 600, false, false, "max-age=600"},
		{"include_subdomains", 600, true, false, "max-age=600; includeSubDomains"},
		{"preload_only", 600, false, true, "max-age=600; preload"},
		{"all", 31536000, true, true, "max-age=31536000; includeSubDomains; preload"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseSecureConfig()
			cfg.Security.HSTS.MaxAge = tc.maxAge
			cfg.Security.HSTS.IncludeSubDomains = tc.includeSubDomains
			cfg.Security.HSTS.Preload = tc.preload
			cfg.TLS.ForceHTTPS = true

			resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

			assert.Equal(t, tc.want, resp.Get("Strict-Transport-Security"))
		})
	}
}

// TestSecurityHeaders_CSPReportOnly — OWASP API8:2023 — checks that
// SECURITY_CSP_REPORT_ONLY swaps the header name so admins can stage rollout
// without ever briefly enforcing a policy they have not validated.
func TestSecurityHeaders_CSPReportOnly(t *testing.T) {
	cfg := baseSecureConfig()
	cfg.Security.CSP.ReportOnly = true

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

	assert.Empty(t, resp.Get("Content-Security-Policy"))
	assert.NotEmpty(t, resp.Get("Content-Security-Policy-Report-Only"))
}

// TestSecurityHeaders_CSPVerbatimOverride — OWASP API8:2023 — when
// SECURITY_CSP_POLICY is set the generated policy (and captcha/extra-src
// logic) is bypassed so an operator can ship an exact custom policy.
func TestSecurityHeaders_CSPVerbatimOverride(t *testing.T) {
	cfg := baseSecureConfig()
	cfg.Security.CSP.Policy = "default-src 'none'; script-src 'self'"
	cfg.Captcha.Provider = string(captcha.ProviderTurnstile)

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

	csp := resp.Get("Content-Security-Policy")
	assert.Equal(t, "default-src 'none'; script-src 'self'", csp)
	assert.NotContains(t, csp, "challenges.cloudflare.com",
		"verbatim override must not be mixed with captcha sources")
}

// TestSecurityHeaders_CSPCaptchaProviderMatrix — OWASP API8:2023 — confirms
// the generated CSP appends exactly the right origins for the configured
// captcha provider (and nothing when it's disabled). This is the answer to
// the "don't permanently whitelist Google + Cloudflare just because someone
// might enable a captcha" concern.
func TestSecurityHeaders_CSPCaptchaProviderMatrix(t *testing.T) {
	cases := []struct {
		name       string
		provider   string
		wantTokens []string
		forbidden  []string
	}{
		{
			name:      "no_provider_no_third_party_sources",
			provider:  "",
			forbidden: []string{"google.com", "gstatic.com", "challenges.cloudflare.com"},
		},
		{
			name:     "recaptcha_v2_allows_google_only",
			provider: string(captcha.ProviderReCAPTCHAV2),
			wantTokens: []string{
				"https://www.google.com/recaptcha/",
				"https://www.gstatic.com/recaptcha/",
				"https://recaptcha.google.com/recaptcha/",
			},
			forbidden: []string{"challenges.cloudflare.com"},
		},
		{
			name:     "recaptcha_v3_allows_google_only",
			provider: string(captcha.ProviderReCAPTCHAV3),
			wantTokens: []string{
				"https://www.google.com/recaptcha/",
				"https://www.gstatic.com/recaptcha/",
			},
			forbidden: []string{"challenges.cloudflare.com"},
		},
		{
			name:       "turnstile_allows_cloudflare_only",
			provider:   string(captcha.ProviderTurnstile),
			wantTokens: []string{"https://challenges.cloudflare.com"},
			forbidden:  []string{"google.com", "gstatic.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseSecureConfig()
			cfg.Captcha.Provider = tc.provider

			resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)
			csp := resp.Get("Content-Security-Policy")

			for _, tok := range tc.wantTokens {
				assert.Contains(t, csp, tok, "expected token missing: %s", tok)
			}

			for _, forbidden := range tc.forbidden {
				assert.NotContains(t, csp, forbidden, "forbidden token leaked: %s", forbidden)
			}
		})
	}
}

// TestSecurityHeaders_CSPExtraSources — OWASP API8:2023 — verifies the
// additive EXTRA_*_SRC knobs append to the right directive so plugins /
// reverse-proxy setups can extend the policy without replacing it.
func TestSecurityHeaders_CSPExtraSources(t *testing.T) {
	cfg := baseSecureConfig()
	cfg.Security.CSP.ExtraScriptSrc = []string{"https://script.plugin.example"}
	cfg.Security.CSP.ExtraStyleSrc = []string{"https://style.plugin.example"}
	cfg.Security.CSP.ExtraConnectSrc = []string{"wss://ws.plugin.example"}
	cfg.Security.CSP.ExtraImgSrc = []string{"https://img.plugin.example"}
	cfg.Security.CSP.ExtraFrameSrc = []string{"https://frame.plugin.example"}
	cfg.Security.CSP.ExtraFontSrc = []string{"https://font.plugin.example"}
	cfg.Security.CSP.ReportURI = "https://csp.example/report"

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)
	csp := resp.Get("Content-Security-Policy")

	directives := splitDirectives(csp)

	assert.Contains(t, directives["script-src"], "https://script.plugin.example")
	assert.Contains(t, directives["style-src"], "https://style.plugin.example")
	assert.Contains(t, directives["connect-src"], "wss://ws.plugin.example")
	assert.Contains(t, directives["img-src"], "https://img.plugin.example")
	assert.Contains(t, directives["frame-src"], "https://frame.plugin.example")
	assert.Contains(t, directives["font-src"], "https://font.plugin.example")
	assert.Contains(t, csp, "report-uri https://csp.example/report")
}

// TestSecurityHeaders_CSPCoreDirectives — OWASP API8:2023 — pins the
// structural invariants of the generated CSP: blob: in script-src/worker-src
// (plugin loader uses dynamic import of a blob: URL), 'wasm-unsafe-eval' in
// script-src (file-manager upload hashes files via hash-wasm) while JS eval
// stays blocked, 'unsafe-inline' in style-src only (Naive UI css-render
// runtime <style>) and never in script-src (where it would gut XSS
// protection).
func TestSecurityHeaders_CSPCoreDirectives(t *testing.T) {
	cfg := baseSecureConfig()

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)
	directives := splitDirectives(resp.Get("Content-Security-Policy"))

	require.Contains(t, directives, "script-src")
	assert.Contains(t, directives["script-src"], "blob:", "plugin loader uses import(blob:…)")
	assert.Contains(t, directives["script-src"], "'wasm-unsafe-eval'",
		"file-manager upload hashes files with hash-wasm; without it WebAssembly.compile is blocked")
	assert.NotContains(t, directives["script-src"], "'unsafe-inline'",
		"script-src must not allow inline; that would defeat the inline-hash strategy")
	assert.NotContains(t, directives["script-src"], "'unsafe-eval'",
		"JS eval must stay blocked; 'wasm-unsafe-eval' covers wasm compilation alone")

	require.Contains(t, directives, "style-src")
	assert.Contains(t, directives["style-src"], "'unsafe-inline'", "Naive UI injects runtime <style>")

	require.Contains(t, directives, "worker-src")
	assert.Contains(t, directives["worker-src"], "blob:")

	require.Contains(t, directives, "img-src")
	assert.Contains(t, directives["img-src"], "data:")
	assert.Contains(t, directives["img-src"], "blob:")
}

// TestSecurityHeaders_DownstreamCanOverride — OWASP API8:2023 — guards the
// file-manager download flow: that handler emits its own restrictive
// "Content-Security-Policy: sandbox" for user-uploaded HTML/SVG, and the
// global middleware must not clobber it.
func TestSecurityHeaders_DownstreamCanOverride(t *testing.T) {
	cfg := baseSecureConfig()

	resp := runMiddleware(t, cfg, newTestFS(t), nil, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Security-Policy", "sandbox")
		w.WriteHeader(http.StatusOK)
	})

	assert.Equal(t, "sandbox", resp.Get("Content-Security-Policy"))
}

// TestSecurityHeaders_OptionalHeadersOmitted — OWASP API8:2023 — empty
// FrameOptions/ReferrerPolicy and ContentTypeOptions=false skip the
// respective header so an admin can opt out of a single directive without
// resorting to the master switch.
func TestSecurityHeaders_OptionalHeadersOmitted(t *testing.T) {
	cfg := baseSecureConfig()
	cfg.Security.ContentTypeOptions = false
	cfg.Security.FrameOptions = ""
	cfg.Security.ReferrerPolicy = ""
	cfg.Security.CSP.Enabled = false

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

	assert.Empty(t, resp.Get("X-Content-Type-Options"))
	assert.Empty(t, resp.Get("X-Frame-Options"))
	assert.Empty(t, resp.Get("Referrer-Policy"))
	assert.Empty(t, resp.Get("Content-Security-Policy"))
}

// TestSecurityHeaders_MissingStaticFile — OWASP API8:2023 — a corrupted
// build that lost index.html must fail at startup, not silently ship a CSP
// without the inline-script hash that the served HTML still relies on.
func TestSecurityHeaders_MissingStaticFile(t *testing.T) {
	cfg := baseSecureConfig()

	empty := fstest.MapFS{}
	_, err := middlewares.NewSecurityHeadersMiddleware(cfg, empty)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "index.html")
}

// TestSecurityHeaders_RealEmbeddedFS — OWASP API8:2023 — smoke-test against
// the real embedded SPA bundle: the inline scripts must still be parseable
// and produce non-empty hashes. Catches breakage from a Vite build that
// stops emitting the expected inline bootstrap.
func TestSecurityHeaders_RealEmbeddedFS(t *testing.T) {
	staticFS, err := webstatic.GetFS()
	require.NoError(t, err)

	cfg := baseSecureConfig()
	m, err := middlewares.NewSecurityHeadersMiddleware(cfg, staticFS)
	require.NoError(t, err)

	r := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rr := httptest.NewRecorder()

	m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, r)

	csp := rr.Header().Get("Content-Security-Policy")
	require.NotEmpty(t, csp)
	assert.Contains(t, csp, "'sha256-", "real index.html must produce at least one inline-script hash")
}

// TestSecurityHeaders_CSPInlineScriptDiscovery — OWASP API8:2023 — locks the
// rules for which <script> blocks contribute a CSP hash. A regression here
// would either (a) strip a legitimate hash and break the SPA bootstrap under
// a stricter browser, or (b) over-include a hash and dilute the inline-hash
// guarantee.
func TestSecurityHeaders_CSPInlineScriptDiscovery(t *testing.T) {
	// The streamsaver document is constant across cases so the hash count
	// math stays predictable: every case carries +1 from this file.
	mitmFile := &fstest.MapFile{
		Data: []byte("<!doctype html><html><body><script>" + mitmHTMLInline + "</script></body></html>"),
	}

	cases := []struct {
		name            string
		indexHTML       string
		wantPresent     []string
		wantAbsent      []string
		wantHashesCount int // total 'sha256- occurrences in the CSP (includes mitm)
	}{
		{
			name:            "empty_src_treated_as_inline",
			indexHTML:       `<!doctype html><html><head><script src="">alert(1)</script></head><body></body></html>`,
			wantPresent:     []string{expectedHash(t, "alert(1)"), expectedHash(t, mitmHTMLInline)},
			wantHashesCount: 2,
		},
		{
			name:            "non_empty_src_skipped",
			indexHTML:       `<!doctype html><html><head><script src="/foo.js">alert(1)</script></head><body></body></html>`,
			wantPresent:     []string{expectedHash(t, mitmHTMLInline)},
			wantAbsent:      []string{expectedHash(t, "alert(1)")},
			wantHashesCount: 1,
		},
		{
			name: "multiple_inline_scripts_all_hashed",
			indexHTML: `<!doctype html><html><head>` +
				`<script>alert(1)</script>` +
				`<script>alert(2)</script>` +
				`</head><body></body></html>`,
			wantPresent: []string{
				expectedHash(t, "alert(1)"),
				expectedHash(t, "alert(2)"),
				expectedHash(t, mitmHTMLInline),
			},
			wantHashesCount: 3,
		},
		{
			name:            "script_with_other_attrs_still_inline",
			indexHTML:       `<!doctype html><html><head><script type="module">alert(1)</script></head><body></body></html>`,
			wantPresent:     []string{expectedHash(t, "alert(1)"), expectedHash(t, mitmHTMLInline)},
			wantHashesCount: 2,
		},
		{
			name:            "empty_script_body_still_hashed",
			indexHTML:       `<!doctype html><html><head><script></script></head><body></body></html>`,
			wantPresent:     []string{expectedHash(t, ""), expectedHash(t, mitmHTMLInline)},
			wantHashesCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// ARRANGE
			cfg := baseSecureConfig()
			staticFS := fstest.MapFS{
				"index.html":            &fstest.MapFile{Data: []byte(tc.indexHTML)},
				"streamsaver/mitm.html": mitmFile,
			}

			// ACT
			resp := runMiddleware(t, cfg, staticFS, nil, nil)

			// ASSERT
			csp := resp.Get("Content-Security-Policy")
			require.NotEmpty(t, csp)

			for _, tok := range tc.wantPresent {
				assert.Contains(t, csp, tok, "expected inline hash missing: %s", tok)
			}

			for _, tok := range tc.wantAbsent {
				assert.NotContains(t, csp, tok, "unexpected inline hash present: %s", tok)
			}

			got := strings.Count(csp, "'sha256-")
			assert.Equal(t, tc.wantHashesCount, got,
				"inline-hash count mismatch (CSP=%q)", csp)
		})
	}
}

// TestSecurityHeaders_HSTSMaxAgeZero — OWASP API8:2023 — RFC 6797 §6.1.1
// allows max-age=0 as a deliberate reset signal that clears any previously
// cached HSTS state. The middleware must emit it verbatim so an operator can
// undo a misconfigured HSTS rollout without re-deploying the binary.
func TestSecurityHeaders_HSTSMaxAgeZero(t *testing.T) {
	// ARRANGE
	cfg := baseSecureConfig()
	cfg.Security.HSTS.MaxAge = 0
	cfg.TLS.ForceHTTPS = true

	// ACT
	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

	// ASSERT
	assert.Equal(t, "max-age=0", resp.Get("Strict-Transport-Security"),
		"max-age=0 must be emitted literally to act as an HSTS reset directive")
}

// TestSecurityHeaders_CSPReportOnlyWithReportURI — OWASP API8:2023 — when
// staging a policy admins typically set ReportOnly + ReportURI together so
// the browser surfaces violations without blocking. Each is covered in
// isolation elsewhere; this guards the combined path so the header name
// swap and the report-uri append don't regress against each other.
func TestSecurityHeaders_CSPReportOnlyWithReportURI(t *testing.T) {
	// ARRANGE
	cfg := baseSecureConfig()
	cfg.Security.CSP.ReportOnly = true
	cfg.Security.CSP.ReportURI = "https://r.example/csp"

	// ACT
	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)

	// ASSERT
	assert.Empty(t, resp.Get("Content-Security-Policy"),
		"enforcing header must be empty in report-only mode")

	reportOnly := resp.Get("Content-Security-Policy-Report-Only")
	require.NotEmpty(t, reportOnly,
		"report-only header must carry the policy in report-only mode")
	assert.Contains(t, reportOnly, "report-uri https://r.example/csp",
		"report-uri must be appended to the report-only policy")
}

// TestSecurityHeaders_InlineScriptHashTokenizerError — OWASP API8:2023 —
// when the html tokenizer fails with a non-EOF error mid-stream (e.g. the
// embedded FS surfaces a transient read error) the constructor must abort
// instead of silently shipping a CSP missing the inline-script hash the
// served HTML relies on.
func TestSecurityHeaders_InlineScriptHashTokenizerError(t *testing.T) {
	// ARRANGE
	cfg := baseSecureConfig()
	staticFS := errFS{files: map[string][]byte{
		// Unclosed <script>: the tokenizer keeps reading raw text inside
		// the script and hits the synthetic Read error before EOF.
		"index.html":            []byte("<!doctype html><html><body><script>"),
		"streamsaver/mitm.html": []byte("<!doctype html><html><body></body></html>"),
	}}

	// ACT
	_, err := middlewares.NewSecurityHeadersMiddleware(cfg, staticFS)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tokenize",
		"error must surface the tokenize stage so logs point at HTML parsing")
	assert.Contains(t, err.Error(), "index.html",
		"error must identify the offending file so logs point at the failing path")
}

// errSyntheticRead is the sentinel returned by errFile.Read after exhausting
// the in-memory data; it lets the html tokenizer surface a non-EOF
// ErrorToken so the production code's "tokenize" wrap path is reachable.
var errSyntheticRead = errors.New("synthetic read failure")

// errFile is a minimal fs.File whose Read returns errSyntheticRead after the
// in-memory data is exhausted, instead of io.EOF.
type errFile struct {
	name string
	data []byte
	pos  int
}

func (f *errFile) Stat() (fs.FileInfo, error) {
	return errFileInfo{name: f.name, size: int64(len(f.data))}, nil
}

func (f *errFile) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, errSyntheticRead
	}

	n := copy(p, f.data[f.pos:])
	f.pos += n

	return n, nil
}

func (f *errFile) Close() error {
	return nil
}

// errFileInfo is the fs.FileInfo paired with errFile; only Name and Size are
// load-bearing for the html tokenizer, the rest match a regular file.
type errFileInfo struct {
	name string
	size int64
}

func (i errFileInfo) Name() string       { return i.name }
func (i errFileInfo) Size() int64        { return i.size }
func (i errFileInfo) Mode() fs.FileMode  { return 0o644 }
func (i errFileInfo) ModTime() time.Time { return time.Time{} }
func (i errFileInfo) IsDir() bool        { return false }
func (i errFileInfo) Sys() any           { return nil }

// errFS is an fs.FS that opens errFile values backed by an in-memory map so
// tests can exercise the non-EOF tokenize-error branch in
// extractInlineScriptHashes.
type errFS struct {
	files map[string][]byte
}

func (e errFS) Open(name string) (fs.File, error) {
	data, ok := e.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}

	return &errFile{name: name, data: data}, nil
}

// runMiddlewareForPath is like runMiddleware but rewrites the request URL so
// the Cache-Control logic can be exercised for specific prefixes.
func runMiddlewareForPath(
	t *testing.T,
	cfg *config.Config,
	staticFS fstest.MapFS,
	path string,
	downstream http.HandlerFunc,
) http.Header {
	t.Helper()

	return runMiddleware(t, cfg, staticFS, func(r *http.Request) {
		r.URL.Path = path
	}, downstream)
}

// TestSecurityHeaders_CacheControl_OnSensitivePaths — OWASP API8:2023 —
// asserts that responses from /api/auth/*, /api/profile/*, /api/users/* and
// /api/tokens/* are stamped with Cache-Control: no-store and Pragma: no-cache
// so an intermediate cache (CDN, browser back/forward, shared corporate
// proxy) cannot serve another user's credentials or PII back to a different
// session. ASVS 8.1.2 / 8.2.1.
func TestSecurityHeaders_CacheControl_OnSensitivePaths(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"auth_login", "/api/auth/login"},
		{"auth_logout", "/api/auth/logout"},
		{"profile_me", "/api/profile/me"},
		{"users_list", "/api/users/"},
		{"users_specific", "/api/users/42"},
		{"tokens_create", "/api/tokens/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseSecureConfig()
			resp := runMiddlewareForPath(t, cfg, newTestFS(t), tc.path, nil)

			assert.Equal(t,
				"no-store, no-cache, must-revalidate, private",
				resp.Get("Cache-Control"),
				"sensitive path %s must emit no-store Cache-Control", tc.path)
			assert.Equal(t, "no-cache", resp.Get("Pragma"))
		})
	}
}

// TestSecurityHeaders_CacheControl_NotOnPublicPaths — OWASP API8:2023 —
// confirms the no-store stamp is restricted to the configured prefixes so
// non-sensitive endpoints (server lists, games catalogue, public assets)
// remain cacheable.
func TestSecurityHeaders_CacheControl_NotOnPublicPaths(t *testing.T) {
	cases := []string{
		"/api/games",
		"/api/servers/1",
		"/api/nodes",
		"/healthz",
		"/",
	}

	for _, path := range cases {
		t.Run(strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_"), func(t *testing.T) {
			cfg := baseSecureConfig()
			resp := runMiddlewareForPath(t, cfg, newTestFS(t), path, nil)

			assert.Empty(t, resp.Get("Cache-Control"),
				"public path %s must not receive Cache-Control: no-store", path)
			assert.Empty(t, resp.Get("Pragma"))
		})
	}
}

// TestSecurityHeaders_CacheControl_RespectsHandlerOverride — OWASP API8:2023
// — a downstream handler that intentionally emits its own Cache-Control
// (e.g. a long-lived public asset served through a sensitive prefix, or a
// file download that wants no-store with an extra directive) must not be
// silently overwritten by the middleware default.
func TestSecurityHeaders_CacheControl_RespectsHandlerOverride(t *testing.T) {
	cfg := baseSecureConfig()

	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=60")
		w.WriteHeader(http.StatusOK)
	}

	resp := runMiddlewareForPath(t, cfg, newTestFS(t), "/api/auth/something", handler)

	assert.Equal(t, "private, max-age=60", resp.Get("Cache-Control"),
		"middleware must not overwrite an explicit downstream Cache-Control")
}

// TestSecurityHeaders_CacheControl_EmptyPrefixListIsInert — OWASP API8:2023 —
// guards against an envDefault parsing quirk where an empty list could
// collapse to []string{""} and match every path. The middleware must do
// nothing when no prefixes are configured.
func TestSecurityHeaders_CacheControl_EmptyPrefixListIsInert(t *testing.T) {
	cfg := baseSecureConfig()
	cfg.Security.SensitivePathPrefixes = []string{"", "  "}

	resp := runMiddlewareForPath(t, cfg, newTestFS(t), "/api/auth/login", nil)

	assert.Empty(t, resp.Get("Cache-Control"),
		"empty/whitespace prefixes must NOT match any path")
}

// splitDirectives parses a CSP header value into a directive -> body map so
// tests can assert on individual tokens without false positives from a
// substring matching the wrong directive.
func splitDirectives(csp string) map[string]string {
	out := map[string]string{}
	for raw := range strings.SplitSeq(csp, ";") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		name, body, _ := strings.Cut(raw, " ")
		out[name] = body
	}

	return out
}
