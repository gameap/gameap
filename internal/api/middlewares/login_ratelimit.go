package middlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/api"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
)

// Defaults for the login rate limiter. They are intentionally generous enough
// to absorb the occasional typing mistake but tight enough to make a brute-force
// guess campaign unworkable in practice.
const (
	defaultLoginRateLimitWindow         = 15 * time.Minute
	defaultLoginRateLimitMaxPerIP       = 20
	defaultLoginRateLimitMaxPerUsername = 5

	loginRateLimitKeyPrefix = "auth:login-fail:"
	loginRateLimitMaxBody   = 1 << 20 // 1 MiB safety cap on the JSON body
)

var (
	errLoginRateLimitedByIP       = errors.New("too many failed login attempts from this client")
	errLoginRateLimitedByUsername = errors.New("too many failed login attempts for this account")
)

// LoginRateLimitMiddleware throttles repeated failed authentication attempts
// against /api/auth/login, mitigating OWASP API2:2023 / API4:2023 and CWE-307.
//
// The limiter increments two counters in the cache after every failed login:
// one keyed by client IP, one keyed by submitted login/email. Either counter
// crossing its limit causes future attempts to be rejected with 429 until the
// window elapses. Successful logins reset only the username counter; the IP
// counter continues to count attempts so a compromised IP can still be slowed.
type LoginRateLimitMiddleware struct {
	cache          cache.Cache
	responder      base.Responder
	window         time.Duration
	maxPerIP       int
	maxPerUser     int
	clock          func() time.Time
	clientIPHeader string // optional header to consult before RemoteAddr (e.g. X-Real-IP); empty disables
	audit          audit.Logger
	keyPrefix      string // cache key namespace; overridable so a reusing route gets its own counters
}

// LoginRateLimitOption configures the middleware at construction time.
type LoginRateLimitOption func(*LoginRateLimitMiddleware)

// WithLoginRateLimitWindow overrides the sliding window duration.
func WithLoginRateLimitWindow(d time.Duration) LoginRateLimitOption {
	return func(m *LoginRateLimitMiddleware) { m.window = d }
}

// WithLoginRateLimitPerIP overrides the per-IP failure limit.
func WithLoginRateLimitPerIP(n int) LoginRateLimitOption {
	return func(m *LoginRateLimitMiddleware) { m.maxPerIP = n }
}

// WithLoginRateLimitPerUsername overrides the per-username failure limit.
func WithLoginRateLimitPerUsername(n int) LoginRateLimitOption {
	return func(m *LoginRateLimitMiddleware) { m.maxPerUser = n }
}

// WithLoginRateLimitClientIPHeader picks an HTTP header to use as the trusted
// client IP (when running behind a reverse proxy that sets X-Real-IP or similar).
func WithLoginRateLimitClientIPHeader(h string) LoginRateLimitOption {
	return func(m *LoginRateLimitMiddleware) { m.clientIPHeader = h }
}

// WithLoginRateLimitAuditLogger wires the security audit logger so failed
// and rate-limit-blocked login attempts are recorded.
func WithLoginRateLimitAuditLogger(l audit.Logger) LoginRateLimitOption {
	return func(m *LoginRateLimitMiddleware) {
		if l != nil {
			m.audit = l
		}
	}
}

// WithLoginRateLimitKeyPrefix overrides the cache key namespace. A route that
// reuses this limiter (the SSO ticket exchange) must pass its own prefix so its
// failures do not share counters with /api/auth/login — otherwise a customer
// retrying a stale magic link would lock themselves out of password login.
func WithLoginRateLimitKeyPrefix(prefix string) LoginRateLimitOption {
	return func(m *LoginRateLimitMiddleware) {
		if prefix != "" {
			m.keyPrefix = prefix
		}
	}
}

func NewLoginRateLimitMiddleware(
	c cache.Cache,
	responder base.Responder,
	opts ...LoginRateLimitOption,
) *LoginRateLimitMiddleware {
	m := &LoginRateLimitMiddleware{
		cache:      c,
		responder:  responder,
		window:     defaultLoginRateLimitWindow,
		maxPerIP:   defaultLoginRateLimitMaxPerIP,
		maxPerUser: defaultLoginRateLimitMaxPerUsername,
		clock:      time.Now,
		audit:      audit.NopLogger{},
		keyPrefix:  loginRateLimitKeyPrefix,
	}
	for _, opt := range opts {
		opt(m)
	}

	return m
}

func (m *LoginRateLimitMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		ip := audit.ClientIP(r, m.clientIPHeader)
		username, body := m.peekUsernameAndRestoreBody(r)

		// Pre-check both counters; if either is already over the limit, refuse
		// to even attempt the credential check so an attacker cannot side-step
		// the limit by varying the username while attacking a single account
		// (or vice versa).
		if retryAfter, blocked, reason := m.shouldBlock(ctx, ip, username); blocked {
			blockedBy := "username"
			if errors.Is(reason, errLoginRateLimitedByIP) {
				blockedBy = "ip"
			}
			audit.LoginBlocked(ctx, m.audit, username, blockedBy)
			m.respond429(ctx, w, retryAfter, reason)

			_ = body

			return
		}

		recorder := &statusRecordingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		switch recorder.statusCode {
		case http.StatusUnauthorized:
			audit.LoginFailure(ctx, m.audit, username, "invalid_credentials")
			m.recordFailure(ctx, ip, username)
		case http.StatusOK:
			// Only reset the username counter; the IP counter intentionally keeps
			// counting so a noisy origin remains noisy even when one of the
			// usernames it tested happens to succeed.
			if username != "" {
				_ = m.cache.Delete(ctx, m.keyPrefix+"user:"+pkgstrings.SHA256(username))
			}
		}

		_ = body // silence linter on the unused return; kept for future audit-log hooks
	})
}

func (m *LoginRateLimitMiddleware) shouldBlock(
	ctx context.Context, ip, username string,
) (retryAfter int, blocked bool, err error) {
	if ip != "" {
		count, _ := m.readCount(ctx, "ip:"+ip)
		if count >= m.maxPerIP {
			return int(m.window.Seconds()), true, errLoginRateLimitedByIP
		}
	}
	if username != "" {
		count, _ := m.readCount(ctx, "user:"+pkgstrings.SHA256(username))
		if count >= m.maxPerUser {
			return int(m.window.Seconds()), true, errLoginRateLimitedByUsername
		}
	}

	return 0, false, nil
}

func (m *LoginRateLimitMiddleware) recordFailure(ctx context.Context, ip, username string) {
	if ip != "" {
		_ = m.increment(ctx, "ip:"+ip)
	}
	if username != "" {
		_ = m.increment(ctx, "user:"+pkgstrings.SHA256(username))
	}
}

func (m *LoginRateLimitMiddleware) increment(ctx context.Context, suffix string) error {
	key := m.keyPrefix + suffix
	count, _ := m.readCount(ctx, suffix)
	count++

	return m.cache.Set(ctx, key, count, cache.WithExpiration(m.window))
}

// readCount tolerates the JSON-roundtrip behaviour of the Redis cache (which
// returns numbers as float64) and the in-memory cache (which returns the original
// type). Anything else is treated as zero.
func (m *LoginRateLimitMiddleware) readCount(ctx context.Context, suffix string) (int, error) {
	raw, err := m.cache.Get(ctx, m.keyPrefix+suffix)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return 0, nil
		}

		return 0, err
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case json.Number:
		n, _ := v.Int64()

		return int(n), nil
	default:
		return 0, nil
	}
}

func (m *LoginRateLimitMiddleware) respond429(
	ctx context.Context, w http.ResponseWriter, retryAfter int, reason error,
) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	}
	m.responder.WriteError(ctx, w, api.WrapHTTPError(reason, http.StatusTooManyRequests))
}

// peekUsernameAndRestoreBody reads the JSON body, extracts the lowercased
// login or email (whichever is present), and replaces r.Body with a new reader
// so the downstream handler can decode the body again.
func (m *LoginRateLimitMiddleware) peekUsernameAndRestoreBody(r *http.Request) (string, []byte) {
	if r.Body == nil {
		return "", nil
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, loginRateLimitMaxBody))
	_ = r.Body.Close()
	if err != nil || len(body) == 0 {
		r.Body = io.NopCloser(bytes.NewReader(body))

		return "", body
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	var peek struct {
		Login string `json:"login"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &peek); err != nil {
		return "", body
	}

	if peek.Email != "" {
		return strings.ToLower(strings.TrimSpace(peek.Email)), body
	}

	return strings.ToLower(strings.TrimSpace(peek.Login)), body
}

// statusRecordingResponseWriter remembers the HTTP status code passed to
// WriteHeader so the middleware can react to authentication outcomes after the
// handler has run. Pre-1.20 Go has no built-in for this.
type statusRecordingResponseWriter struct {
	http.ResponseWriter

	statusCode  int
	wroteHeader bool
}

func (rw *statusRecordingResponseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *statusRecordingResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}

	return rw.ResponseWriter.Write(b)
}
