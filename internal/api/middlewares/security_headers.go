package middlewares

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/services/captcha"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
)

// SecurityHeadersMiddleware emits the global HTTP response security headers
// (HSTS, X-Content-Type-Options, X-Frame-Options, Referrer-Policy and
// Content-Security-Policy). All header values are precomputed at construction
// so the per-request hot path is a fixed sequence of Header().Set() calls.
//
// Headers are set before delegating to the next handler so that downstream
// handlers that explicitly set their own header (e.g. the file-manager stream
// path that emits "Content-Security-Policy: sandbox" for user file content)
// override the global value for their response.
type SecurityHeadersMiddleware struct {
	cfg *config.Config

	hstsValue         string
	frameOptions      string
	referrerPolicy    string
	cspHeader         string
	cspValue          string
	sensitivePrefixes []string
}

const (
	// sensitiveCacheControl is the value applied to responses whose request
	// path begins with one of cfg.Security.SensitivePathPrefixes. "no-store"
	// alone is the modern directive; "no-cache, must-revalidate, private"
	// covers HTTP/1.1 intermediates that handle no-store inconsistently;
	// the legacy "Pragma: no-cache" is added so HTTP/1.0 proxies (still
	// present in some enterprise networks) cooperate.
	sensitiveCacheControl = "no-store, no-cache, must-revalidate, private"
	sensitivePragma       = "no-cache"
)

const (
	// indexHTMLPath is the served SPA entrypoint with two static IIFE scripts
	// (theme + i18n bootstrap) inlined in a single <script> block.
	indexHTMLPath = "index.html"

	// streamSaverMitmPath registers the same-origin service worker used by the
	// file-manager download flow; its inline <script> must run under CSP or
	// downloads break.
	streamSaverMitmPath = "streamsaver/mitm.html"
)

// Captcha CSP sources. Self-hosting is not viable: api.js is a loader that
// pulls versioned sub-resources from www.gstatic.com and renders the
// challenge inside an iframe bound to the provider origin. We allow-list only
// the configured provider; with captcha disabled (default) no third-party
// origins appear in the policy.
var (
	recaptchaScriptSrc = []string{"https://www.google.com/recaptcha/", "https://www.gstatic.com/recaptcha/"}
	recaptchaFrameSrc  = []string{"https://www.google.com/recaptcha/", "https://recaptcha.google.com/recaptcha/"}
	turnstileScriptSrc = []string{"https://challenges.cloudflare.com"}
	turnstileFrameSrc  = []string{"https://challenges.cloudflare.com"}
)

// NewSecurityHeadersMiddleware precomputes every header value. It returns an
// error (and never partially-builds the middleware) when CSP is enabled with
// a generated policy but the embedded static FS is missing one of the HTML
// files whose inline-script hash the policy depends on — a corrupt build
// should fail at startup, not at the first request that triggers a violation.
func NewSecurityHeadersMiddleware(cfg *config.Config, staticFS fs.FS) (*SecurityHeadersMiddleware, error) {
	m := &SecurityHeadersMiddleware{cfg: cfg}

	if !cfg.Security.Enabled {
		return m, nil
	}

	m.hstsValue = buildHSTSValue(
		cfg.Security.HSTS.MaxAge,
		cfg.Security.HSTS.IncludeSubDomains,
		cfg.Security.HSTS.Preload,
	)
	m.frameOptions = cfg.Security.FrameOptions
	m.referrerPolicy = cfg.Security.ReferrerPolicy
	m.sensitivePrefixes = normalizeSensitivePrefixes(cfg.Security.SensitivePathPrefixes)

	if cfg.Security.CSP.Enabled {
		cspValue, err := buildPolicy(cfg, staticFS)
		if err != nil {
			return nil, errors.WithMessage(err, "build CSP")
		}

		m.cspValue = cspValue
		m.cspHeader = "Content-Security-Policy"
		if cfg.Security.CSP.ReportOnly {
			m.cspHeader = "Content-Security-Policy-Report-Only"
		}
	}

	return m, nil
}

// Middleware returns a handler that stamps the precomputed security headers
// on every response. When the master switch is off it returns next unchanged
// so disabled deployments pay zero per-request cost.
func (m *SecurityHeadersMiddleware) Middleware(next http.Handler) http.Handler {
	if !m.cfg.Security.Enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		if m.cfg.Security.ContentTypeOptions {
			h.Set("X-Content-Type-Options", "nosniff")
		}

		if m.frameOptions != "" {
			h.Set("X-Frame-Options", m.frameOptions)
		}

		if m.referrerPolicy != "" {
			h.Set("Referrer-Policy", m.referrerPolicy)
		}

		if m.cspValue != "" {
			h.Set(m.cspHeader, m.cspValue)
		}

		if m.cfg.Security.HSTS.Enabled && isHTTPSRequest(r, m.cfg) {
			h.Set("Strict-Transport-Security", m.hstsValue)
		}

		if m.shouldNoStore(r.URL.Path) {
			// Only set if the response does not already carry an explicit
			// Cache-Control — a downstream handler that emits its own value
			// wins. We do not overwrite "no-store" with "no-store" either;
			// the guard is cheap and keeps the response header section
			// idempotent across stacked middleware.
			if h.Get("Cache-Control") == "" {
				h.Set("Cache-Control", sensitiveCacheControl)
			}
			if h.Get("Pragma") == "" {
				h.Set("Pragma", sensitivePragma)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// shouldNoStore reports whether the request path begins with one of the
// configured sensitive prefixes and therefore must not be cached.
func (m *SecurityHeadersMiddleware) shouldNoStore(path string) bool {
	for _, prefix := range m.sensitivePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// normalizeSensitivePrefixes trims and drops empty entries from the configured
// list. An empty envDefault on a comma-separated slice surfaces as []string{""}
// for caarlos0/env, which would otherwise match every path.
func normalizeSensitivePrefixes(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// isHTTPSRequest reports whether the request reached the panel over TLS.
// X-Forwarded-Proto is honored so HSTS still works behind a TLS-terminating
// reverse proxy; plain HTTP dev sessions never receive HSTS (the directive
// would poison localhost for a year).
func isHTTPSRequest(r *http.Request, cfg *config.Config) bool {
	if r.TLS != nil {
		return true
	}

	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}

	return cfg.TLS.ForceHTTPS
}

func buildHSTSValue(maxAge int, includeSubDomains, preload bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "max-age=%d", maxAge)

	if includeSubDomains {
		b.WriteString("; includeSubDomains")
	}

	if preload {
		b.WriteString("; preload")
	}

	return b.String()
}

func buildPolicy(cfg *config.Config, staticFS fs.FS) (string, error) {
	if cfg.Security.CSP.Policy != "" {
		return cfg.Security.CSP.Policy, nil
	}

	hashes, err := collectInlineScriptHashes(staticFS, []string{indexHTMLPath, streamSaverMitmPath})
	if err != nil {
		return "", err
	}

	return buildGeneratedCSP(cfg, hashes), nil
}

func buildGeneratedCSP(cfg *config.Config, scriptHashes []string) string {
	csp := &cfg.Security.CSP

	captchaScript, captchaFrame := captchaCSPSources(cfg.Captcha.Provider)

	directives := []string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'self'",
		"form-action 'self'",
		// 'wasm-unsafe-eval' allows WebAssembly.compile only (JS eval stays
		// blocked): the file-manager upload flow hashes files client-side via
		// hash-wasm, whose inlined wasm module cannot compile without it and
		// every upload then dies in the hashing phase.
		joinTokens("script-src",
			[]string{"'self'", "blob:", "'wasm-unsafe-eval'"}, scriptHashes, captchaScript, csp.ExtraScriptSrc),
		joinTokens("style-src", []string{"'self'", "'unsafe-inline'"}, csp.ExtraStyleSrc),
		joinTokens("img-src", []string{"'self'", "data:", "blob:"}, csp.ExtraImgSrc),
		joinTokens("font-src", []string{"'self'"}, csp.ExtraFontSrc),
		joinTokens("connect-src", []string{"'self'"}, csp.ExtraConnectSrc),
		joinTokens("frame-src", []string{"'self'"}, captchaFrame, csp.ExtraFrameSrc),
		joinTokens("worker-src", []string{"'self'", "blob:"}),
	}

	if csp.ReportURI != "" {
		directives = append(directives, "report-uri "+csp.ReportURI)
	}

	return strings.Join(directives, "; ")
}

func captchaCSPSources(provider string) (script, frame []string) {
	switch captcha.Provider(provider) {
	case captcha.ProviderReCAPTCHAV2, captcha.ProviderReCAPTCHAV3:
		return recaptchaScriptSrc, recaptchaFrameSrc
	case captcha.ProviderTurnstile:
		return turnstileScriptSrc, turnstileFrameSrc
	default:
		return nil, nil
	}
}

// joinTokens writes "<name> <token1> <token2> …" by flattening any number of
// variadic source slices. Empty tokens are dropped so callers can pass an
// unconditional captcha slice that is nil when no provider is configured.
func joinTokens(name string, groups ...[]string) string {
	var b strings.Builder
	b.WriteString(name)

	for _, group := range groups {
		for _, t := range group {
			if t == "" {
				continue
			}

			b.WriteByte(' ')
			b.WriteString(t)
		}
	}

	return b.String()
}

func collectInlineScriptHashes(staticFS fs.FS, paths []string) ([]string, error) {
	var hashes []string

	for _, p := range paths {
		fileHashes, err := extractInlineScriptHashes(staticFS, p)
		if err != nil {
			return nil, errors.WithMessagef(err, "extract inline script hashes from %s", p)
		}

		hashes = append(hashes, fileHashes...)
	}

	return hashes, nil
}

// extractInlineScriptHashes walks an HTML document and returns a CSP token
// ('sha256-<base64>') for every <script> element without a src attribute.
// The HTML tokenizer surfaces script body as a raw text node (matching what
// a browser hashes), so no entity decoding or whitespace munging is needed.
func extractInlineScriptHashes(staticFS fs.FS, path string) ([]string, error) {
	f, err := staticFS.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	defer func() {
		_ = f.Close()
	}()

	tokenizer := html.NewTokenizer(f)

	var (
		hashes  []string
		inline  bool
		content strings.Builder
	)

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return hashes, nil
			}

			return nil, errors.Wrap(tokenizer.Err(), "tokenize")

		case html.StartTagToken:
			name, hasAttr := tokenizer.TagName()
			if string(name) != "script" {
				continue
			}

			inline = true
			content.Reset()

			for hasAttr {
				var k, v []byte
				k, v, hasAttr = tokenizer.TagAttr()
				if string(k) == "src" && len(v) > 0 {
					inline = false

					break
				}
			}

		case html.TextToken:
			if inline {
				content.Write(tokenizer.Text())
			}

		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if string(name) != "script" || !inline {
				continue
			}

			sum := sha256.Sum256([]byte(content.String()))
			hashes = append(hashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
			inline = false
			content.Reset()
		}
	}
}
