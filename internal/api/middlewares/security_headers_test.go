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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/services/captcha"
	webstatic "github.com/gameap/gameap/web/static"
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
// (plugin loader uses dynamic import of a blob: URL), 'unsafe-inline' in
// style-src only (Naive UI css-render runtime <style>) and never in
// script-src (where it would gut XSS protection).
func TestSecurityHeaders_CSPCoreDirectives(t *testing.T) {
	cfg := baseSecureConfig()

	resp := runMiddleware(t, cfg, newTestFS(t), nil, nil)
	directives := splitDirectives(resp.Get("Content-Security-Policy"))

	require.Contains(t, directives, "script-src")
	assert.Contains(t, directives["script-src"], "blob:", "plugin loader uses import(blob:…)")
	assert.NotContains(t, directives["script-src"], "'unsafe-inline'",
		"script-src must not allow inline; that would defeat the inline-hash strategy")

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
