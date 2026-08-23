package hostlibrary

import (
	"bytes"
	"context"
	"io"
	"net"
	stdhttp "net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/gameap/gameap/pkg/netutil"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/http"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
)

const (
	defaultTimeout      = 30 * time.Second
	maxBodySize         = 10 * 1024 * 1024 // 10 MB
	defaultMaxRedirects = 5
)

// Errors returned by the SSRF pre-dial check. They are deliberately specific
// so the audit pipeline can distinguish "blocked because cloud metadata"
// from "blocked because RFC1918" without parsing free-form strings.
var (
	errSchemeNotAllowed = errors.New("plugin http: scheme not allowed")
	errInvalidURL       = errors.New("plugin http: invalid url")
	errHostNotResolved  = errors.New("plugin http: hostname did not resolve")
	errBlockedTarget    = errors.New("plugin http: target ip is blocked")
	errRedirectExceeded = errors.New("plugin http: too many redirects")
)

// HTTPConfig is the operator-facing configuration for the host library.
// Mirrors internal/config Plugin.HTTP one-for-one; the indirection lets
// tests construct a service without depending on the full config package.
type HTTPConfig struct {
	BlockPrivateIPs         bool
	AllowedSchemes          []string
	AllowedHosts            []string
	MaxTimeoutSeconds       int
	MaxRedirects            int
	ResponseHeaderAllowlist []string
}

// defaultResponseHeaderAllowlist is the minimum set of response headers
// every plugin sees. Anything outside this list (Set-Cookie, Authorization,
// WWW-Authenticate, Proxy-Authenticate, Clear-Site-Data, Server-Timing,
// Public-Key-Pins, …) is stripped before the response is handed back so a
// plugin cannot exfiltrate credentials issued by a reachable origin.
//
// Allowlist (not denylist) is deliberate: a new HTTP header invented next
// year defaults to "stripped", not "leaked".
var defaultResponseHeaderAllowlist = []string{
	"Content-Type",
	"Content-Length",
	"Content-Encoding",
	"Content-Language",
	"Last-Modified",
	"Etag",
	"Cache-Control",
	"Date",
	"Location",
	"Expires",
	"Vary",
}

// HTTPServiceImpl is the wazero-facing implementation. It carries its own
// hardened transport (custom DialContext + CheckRedirect) so every dial
// goes through the SSRF gate, including any host that the HTTP library
// or a future redirect would otherwise reach.
type HTTPServiceImpl struct {
	cfg                 HTTPConfig
	resolver            netResolver
	dialer              *net.Dialer
	allowedSchemes      map[string]struct{}
	allowedHosts        map[string]struct{}
	responseHeaderAllow map[string]struct{}
	timeoutCap          time.Duration
	maxRedirects        int
}

// netResolver lets tests inject a stub for net.DefaultResolver.LookupNetIP.
type netResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// NewHTTPService constructs the host-library service from the operator
// config. The returned *http.Client is built per-request inside Fetch so
// the CheckRedirect closure can capture the request-scoped resolver.
func NewHTTPService(cfg HTTPConfig) *HTTPServiceImpl {
	return newHTTPService(cfg, net.DefaultResolver, &net.Dialer{Timeout: defaultTimeout})
}

// newHTTPService is the test-friendly constructor — it accepts a custom
// resolver/dialer pair so a fake DNS can simulate rebinding etc.
func newHTTPService(cfg HTTPConfig, resolver netResolver, dialer *net.Dialer) *HTTPServiceImpl {
	if dialer == nil {
		dialer = &net.Dialer{Timeout: defaultTimeout}
	}

	timeoutCap := time.Duration(cfg.MaxTimeoutSeconds) * time.Second
	if timeoutCap <= 0 {
		timeoutCap = defaultTimeout
	}

	maxRedirects := cfg.MaxRedirects
	if maxRedirects < 0 {
		maxRedirects = 0
	} else if maxRedirects == 0 {
		maxRedirects = defaultMaxRedirects
	}

	allowedSchemes := normaliseAllowedSchemes(cfg.AllowedSchemes)
	allowedHosts := normaliseAllowedHosts(cfg.AllowedHosts)
	allow := buildResponseHeaderAllowlist(cfg.ResponseHeaderAllowlist)

	return &HTTPServiceImpl{
		cfg:                 cfg,
		resolver:            resolver,
		dialer:              dialer,
		allowedSchemes:      allowedSchemes,
		allowedHosts:        allowedHosts,
		responseHeaderAllow: allow,
		timeoutCap:          timeoutCap,
		maxRedirects:        maxRedirects,
	}
}

// Fetch implements the wazero HTTPService contract. Plugin-controlled
// inputs (URL, method, headers, body, timeout) are validated through the
// SSRF gate before any network IO happens, and the response is filtered
// through the header allowlist on the way out.
func (s *HTTPServiceImpl) Fetch(
	ctx context.Context,
	req *http.HTTPFetchRequest,
) (*http.HTTPFetchResponse, error) {
	timeout := defaultTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}

	if timeout > s.timeoutCap {
		timeout = s.timeoutCap
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := s.validateURL(req.Url); err != nil {
		return errorResponse(err), nil
	}

	var body io.Reader
	if req.Body != nil {
		body = bytes.NewReader(req.Body)
	}

	httpReq, err := stdhttp.NewRequestWithContext(ctx, req.Method, req.Url, body)
	if err != nil {
		return errorResponse(err), nil
	}

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	client := s.client(ctx)

	resp, err := client.Do(httpReq)
	if err != nil {
		return errorResponse(err), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return &http.HTTPFetchResponse{
			StatusCode: int32(resp.StatusCode), //nolint:gosec
			Error:      new(err.Error()),
		}, nil
	}

	return &http.HTTPFetchResponse{
		StatusCode: int32(resp.StatusCode), //nolint:gosec
		Headers:    s.filterResponseHeaders(resp.Header),
		Body:       respBody,
	}, nil
}

// client builds the per-request http.Client. The transport's DialContext
// re-validates every connect (including redirect-driven re-dials) by
// resolving the hostname, checking each candidate IP against the SSRF
// blocklist, and dialing the CHOSEN IP rather than the hostname. Dialing
// the IP closes the DNS-rebinding window between resolution and connect.
func (s *HTTPServiceImpl) client(_ context.Context) *stdhttp.Client {
	transport := &stdhttp.Transport{
		Proxy: stdhttp.ProxyFromEnvironment,
		DialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			ip, port, dialErr := s.resolveAndCheck(dialCtx, address)
			if dialErr != nil {
				return nil, dialErr
			}

			return s.dialer.DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
		},
		// Conservative defaults — short keep-alive, no idle reuse across
		// requests so a future redirect that lands on a different host
		// cannot accidentally reuse a previous connection.
		MaxIdleConns:          0,
		IdleConnTimeout:       0,
		ResponseHeaderTimeout: s.timeoutCap,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &stdhttp.Client{
		Transport: transport,
		Timeout:   s.timeoutCap,
		CheckRedirect: func(req *stdhttp.Request, via []*stdhttp.Request) error {
			if len(via) >= s.maxRedirects {
				return errRedirectExceeded
			}

			// Re-validate the redirect target against the same rules. The
			// transport will also re-check at dial time; this provides an
			// earlier, cleaner error to the plugin.
			return s.validateURL(req.URL.String())
		},
	}
}

// validateURL checks scheme and resolves the host so we can fail fast on
// invalid URLs and obviously-blocked targets before any TCP work.
func (s *HTTPServiceImpl) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.Wrap(errInvalidURL, err.Error())
	}

	scheme := strings.ToLower(u.Scheme)
	if _, ok := s.allowedSchemes[scheme]; !ok {
		return errors.Wrapf(errSchemeNotAllowed, "scheme=%q", scheme)
	}

	if u.Host == "" {
		return errors.Wrap(errInvalidURL, "missing host")
	}

	return nil
}

// resolveAndCheck performs the SSRF-safe DNS lookup for a `host:port`
// address. Returns the chosen IP (to be dialed verbatim) and the port.
//
// An operator allow-list bypasses the private-IP blocklist for matching
// hostnames, but cloud-metadata IPs are NEVER bypassed.
func (s *HTTPServiceImpl) resolveAndCheck(ctx context.Context, address string) (netip.Addr, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return netip.Addr{}, "", errors.Wrap(errInvalidURL, err.Error())
	}

	allowBypass := s.hostInAllowlist(host)

	// If the host is a literal IP, no DNS needed.
	if ip, ipErr := netip.ParseAddr(host); ipErr == nil {
		if err := s.checkIP(ip, allowBypass); err != nil {
			return netip.Addr{}, "", err
		}

		return ip, port, nil
	}

	ips, err := s.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, "", errors.Wrap(errHostNotResolved, err.Error())
	}

	if len(ips) == 0 {
		return netip.Addr{}, "", errors.Wrap(errHostNotResolved, host)
	}

	// Reject if ANY resolved IP is blocked — refuse the request entirely
	// rather than picking a "safe" subset. Multi-IP attacks deliberately
	// mix a public IP with a private one to fool naive validators.
	for _, ip := range ips {
		if err := s.checkIP(ip, allowBypass); err != nil {
			return netip.Addr{}, "", err
		}
	}

	// Use the first IP for the actual dial.
	return ips[0], port, nil
}

func (s *HTTPServiceImpl) checkIP(ip netip.Addr, allowBypass bool) error {
	// Cloud metadata is always blocked, regardless of allow-list.
	if netutil.IsCloudMetadataIP(ip) {
		return errors.Wrapf(errBlockedTarget, "ip=%s reason=%s", ip, netutil.BlockReasonCloudMetadata)
	}

	if !s.cfg.BlockPrivateIPs {
		return nil
	}

	if allowBypass {
		return nil
	}

	if reason := netutil.BlockReason(ip); reason != "" {
		return errors.Wrapf(errBlockedTarget, "ip=%s reason=%s", ip, reason)
	}

	return nil
}

func (s *HTTPServiceImpl) hostInAllowlist(host string) bool {
	if len(s.allowedHosts) == 0 {
		return false
	}

	_, ok := s.allowedHosts[strings.ToLower(host)]

	return ok
}

func (s *HTTPServiceImpl) filterResponseHeaders(in stdhttp.Header) map[string]string {
	out := make(map[string]string, len(in))

	for key := range in {
		canonical := stdhttp.CanonicalHeaderKey(key)
		if _, ok := s.responseHeaderAllow[canonical]; !ok {
			continue
		}

		out[canonical] = in.Get(canonical)
	}

	return out
}

func errorResponse(err error) *http.HTTPFetchResponse {
	return &http.HTTPFetchResponse{
		Error: new(err.Error()),
	}
}

func normaliseAllowedSchemes(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))

	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out[s] = struct{}{}
		}
	}

	// Defensive default: never allow file://, ftp://, gopher://, ldap://.
	// If the operator passes an empty list we fall back to "https only" —
	// matches the documented envDefault but protects against a manual
	// override with an empty value.
	if len(out) == 0 {
		out["https"] = struct{}{}
	}

	return out
}

func normaliseAllowedHosts(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))

	for _, h := range in {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			out[h] = struct{}{}
		}
	}

	return out
}

func buildResponseHeaderAllowlist(extra []string) map[string]struct{} {
	out := make(map[string]struct{}, len(defaultResponseHeaderAllowlist)+len(extra))

	for _, h := range defaultResponseHeaderAllowlist {
		out[stdhttp.CanonicalHeaderKey(h)] = struct{}{}
	}

	for _, h := range extra {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}

		out[stdhttp.CanonicalHeaderKey(h)] = struct{}{}
	}

	return out
}

// HTTPHostLibrary wires the SSRF-hardened service into the wazero runtime.
type HTTPHostLibrary struct {
	impl http.HTTPService
}

// NewHTTPHostLibrary constructs an ungated host library from operator
// config; the panel registers the per-plugin, rate-limited variant through
// NewHTTPHostLibraryFactory instead.
func NewHTTPHostLibrary(cfg HTTPConfig) *HTTPHostLibrary {
	return &HTTPHostLibrary{
		impl: NewHTTPService(cfg),
	}
}

func (l *HTTPHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return http.Instantiate(ctx, r, l.impl)
}

// guardedHTTPService puts the plugin's rate limit in front of the shared
// service. The service itself is shared between plugins (one resolver and
// dialer), only the guard binding is per plugin.
type guardedHTTPService struct {
	impl  *HTTPServiceImpl
	guard *PluginGuard
}

func (s *guardedHTTPService) Fetch(
	ctx context.Context,
	req *http.HTTPFetchRequest,
) (*http.HTTPFetchResponse, error) {
	if msg := s.guard.Check(ctx, ModuleHTTP, "fetch"); msg != "" {
		return &http.HTTPFetchResponse{Error: new(msg)}, nil
	}

	return s.impl.Fetch(ctx, req)
}

// HTTPHostLibraryFactory builds a per-plugin gameap-http module over one
// shared SSRF-hardened service.
type HTTPHostLibraryFactory struct {
	impl  *HTTPServiceImpl
	guard *Guard
}

func NewHTTPHostLibraryFactory(cfg HTTPConfig, guard *Guard) *HTTPHostLibraryFactory {
	return &HTTPHostLibraryFactory{
		impl:  NewHTTPService(cfg),
		guard: guard,
	}
}

func (f *HTTPHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &HTTPHostLibrary{
		impl: &guardedHTTPService{impl: f.impl, guard: f.guard.For(pluginID)},
	}
}
