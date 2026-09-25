package idempotency

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	defaultKeyTTL = 24 * time.Hour

	lockTTL             = time.Minute
	lockRefreshInterval = 20 * time.Second

	// executionTimeout bounds a request that keeps running after its client
	// hung up.
	executionTimeout = 2 * time.Minute

	storeTimeout = 10 * time.Second
	saveAttempts = 3
	saveBackoff  = 200 * time.Millisecond

	maxRequestBody  = 1 << 20
	maxResponseBody = 1 << 20
	maxRequestPath  = 255

	inProgressRetryAfter  = "1"
	unavailableRetryAfter = "5"

	lockKeyPrefix    = "idempotency:"
	keyHashLogPrefix = 12
)

// Middleware stores the outcome of a mutating request sent with an
// Idempotency-Key and replays it to retries carrying the same key, so a retry
// never runs the request twice. Keys are scoped to the user. It must wrap the
// handler inside the authentication and authorization middlewares: a replay
// is served only to a request that passed them.
type Middleware struct {
	store     Store
	locks     locker.Locker
	responder responder
	hasher    hasher
	recovery  func(http.Handler) http.Handler
	keyTTL    time.Duration
	now       func() time.Time
	logger    *slog.Logger
	disabled  bool
}

type Option func(m *Middleware)

// WithKeyTTL sets how long an outcome is replayed. A non-positive value keeps
// the default of 24 hours.
func WithKeyTTL(ttl time.Duration) Option {
	return func(m *Middleware) {
		if ttl > 0 {
			m.keyTTL = ttl
		}
	}
}

// WithRecovery wraps the handler with the panic recovery middleware, so a
// panic is stored as the 500 it produces instead of leaving no outcome for
// the retry to find.
func WithRecovery(recovery func(http.Handler) http.Handler) Option {
	return func(m *Middleware) {
		m.recovery = recovery
	}
}

func WithClock(now func() time.Time) Option {
	return func(m *Middleware) {
		m.now = now
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(m *Middleware) {
		m.logger = logger
	}
}

func NewMiddleware(
	store Store,
	locks locker.Locker,
	responder responder,
	secret []byte,
	opts ...Option,
) *Middleware {
	m := &Middleware{
		store:     store,
		locks:     locks,
		responder: responder,
		hasher:    newHasher(secret),
		recovery: func(next http.Handler) http.Handler {
			return next
		},
		keyTTL: defaultKeyTTL,
		now:    time.Now,
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Disabled returns a middleware that leaves routes untouched: the
// Idempotency-Key header is ignored (IDEMPOTENCY_DRIVER=none).
func Disabled() *Middleware {
	return &Middleware{disabled: true}
}

func (m *Middleware) Middleware(next http.Handler) http.Handler {
	if m.disabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.serve(w, r, next)
	})
}

type request struct {
	userID      uint
	keyHash     string
	fingerprint string
	method      string
	path        string
}

func (req request) lockKey() string {
	return lockKeyPrefix + strconv.FormatUint(uint64(req.userID), 10) + ":" + req.keyHash
}

func (req request) logAttrs(attrs ...any) []any {
	return append([]any{
		slog.Uint64("user_id", uint64(req.userID)),
		slog.String("key_hash", req.keyHash[:keyHashLogPrefix]),
		slog.String("method", req.method),
		slog.String("path", req.path),
	}, attrs...)
}

func (m *Middleware) serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	ctx := r.Context()

	if !isMutating(r.Method) {
		next.ServeHTTP(w, r)

		return
	}

	key, ok, err := ParseKey(r.Header)
	if err != nil {
		m.responder.WriteError(ctx, w, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	session := auth.SessionFromContext(ctx)
	if !ok || !session.IsAuthenticated() {
		next.ServeHTTP(w, r)

		return
	}

	body, err := readBody(w, r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			m.responder.WriteError(ctx, w, api.WrapHTTPError(ErrBodyTooLarge, http.StatusRequestEntityTooLarge))

			return
		}

		m.responder.WriteError(ctx, w, api.WrapHTTPError(
			errors.WithMessage(err, "failed to read request body"),
			http.StatusBadRequest,
		))

		return
	}

	req := request{
		userID:      session.User.ID,
		keyHash:     m.hasher.keyHash(key),
		fingerprint: m.hasher.fingerprint(r, body),
		method:      r.Method,
		path:        truncate(r.URL.EscapedPath(), maxRequestPath),
	}

	// Store calls outlive a client that hangs up: its retry still needs the
	// outcome.
	storeCtx := context.WithoutCancel(ctx)

	if m.replayStored(storeCtx, w, r, next, req) {
		return
	}

	acquireCtx, cancelAcquire := context.WithTimeout(storeCtx, storeTimeout)
	lock, err := m.locks.Acquire(acquireCtx, req.lockKey(), lockTTL)
	cancelAcquire()

	if err != nil {
		if errors.Is(err, locker.ErrLocked) {
			w.Header().Set("Retry-After", inProgressRetryAfter)
			m.responder.WriteError(ctx, w, api.WrapHTTPError(ErrRequestInProgress, http.StatusConflict))

			return
		}

		m.writeUnavailable(ctx, w, errors.WithMessage(err, "failed to acquire idempotency lock"))

		return
	}
	defer m.release(storeCtx, lock, req)

	// The original request may have finished between the lookup above and
	// taking the lock.
	if m.replayStored(storeCtx, w, r, next, req) {
		return
	}

	m.execute(w, r, next, req, lock)
}

// replayStored answers the request from a stored outcome and reports whether
// it wrote a response.
func (m *Middleware) replayStored(
	ctx context.Context, w http.ResponseWriter, r *http.Request, next http.Handler, req request,
) bool {
	findCtx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()

	record, err := m.store.Find(findCtx, req.userID, req.keyHash, m.now())
	if err != nil {
		m.writeUnavailable(ctx, w, errors.WithMessage(err, "idempotency store lookup failed"))

		return true
	}

	if record == nil {
		return false
	}

	if record.RequestFingerprint != req.fingerprint {
		m.responder.WriteError(ctx, w, api.WrapHTTPError(ErrKeyReused, http.StatusUnprocessableEntity))

		return true
	}

	if authorizer, ok := next.(ReplayAuthorizer); ok {
		if err := authorizer.AuthorizeIdempotencyReplay(r.WithContext(findCtx)); err != nil {
			m.writeReplayRefused(ctx, w, err)

			return true
		}
	}

	m.logger.DebugContext(ctx, "idempotency: replaying stored outcome",
		req.logAttrs(slog.Int("status", record.ResponseStatus))...)

	replay(w, record)

	return true
}

func (m *Middleware) execute(w http.ResponseWriter, r *http.Request, next http.Handler, req request, lock locker.Lock) {
	// Once accepted, the request runs to completion even if its client hangs
	// up; cancelling it halfway would record "context canceled" as the
	// outcome of a request that may already have changed something.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), executionTimeout)
	defer cancel()

	stopRefresh := m.keepLock(ctx, lock, req)
	defer stopRefresh()

	rec := newRecorder(w, maxResponseBody)

	m.recovery(next).ServeHTTP(rec, r.WithContext(ctx))

	stopRefresh()
	rec.finish()

	m.save(context.WithoutCancel(r.Context()), rec, req)
}

// keepLock refreshes the lock while the handler runs, so a slow request is
// not mistaken for a crashed one and run again by a retry.
func (m *Middleware) keepLock(ctx context.Context, lock locker.Lock, req request) func() {
	done := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		defer close(finished)

		ticker := time.NewTicker(lockRefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
				err := lock.Refresh(refreshCtx, lockTTL)
				cancel()

				if errors.Is(err, locker.ErrLockLost) {
					m.logger.WarnContext(ctx, "idempotency: lock lost while the request runs, a retry may run it again",
						req.logAttrs()...)

					return
				}

				if err != nil {
					m.logger.WarnContext(ctx, "idempotency: failed to refresh the lock",
						req.logAttrs(slog.String("error", err.Error()))...)
				}
			}
		}
	}()

	var once sync.Once

	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}

// save stores every outcome the handler produced, 5xx included: a failure
// may come after a change was made, so running the request again on retry
// could make it twice.
func (m *Middleware) save(ctx context.Context, rec *recorder, req request) {
	if rec.overflow {
		m.logger.WarnContext(ctx, "idempotency: response too large to store, a retry will run the request again",
			req.logAttrs(slog.Int("status", rec.status))...)

		return
	}

	now := m.now()
	record := &domain.IdempotencyKey{
		UserID:             req.userID,
		KeyHash:            req.keyHash,
		RequestMethod:      req.method,
		RequestPath:        req.path,
		RequestFingerprint: req.fingerprint,
		ResponseStatus:     rec.status,
		ResponseHeaders:    rec.header,
		ResponseBody:       rec.body.Bytes(),
		CreatedAt:          now,
		ExpiresAt:          now.Add(m.keyTTL),
	}

	var err error

	for attempt := range saveAttempts {
		if attempt > 0 {
			time.Sleep(saveBackoff * time.Duration(attempt))
		}

		saveCtx, cancel := context.WithTimeout(ctx, storeTimeout)

		var saved bool
		saved, err = m.store.Save(saveCtx, record)

		cancel()

		if err == nil {
			if !saved {
				m.logger.WarnContext(ctx, "idempotency: another outcome already holds the key, the lock was lost",
					req.logAttrs(slog.Int("status", rec.status))...)
			}

			return
		}
	}

	m.logger.ErrorContext(ctx, "idempotency: failed to store the outcome, a retry will run the request again",
		req.logAttrs(slog.Int("status", rec.status), slog.String("error", err.Error()))...)
}

func (m *Middleware) release(ctx context.Context, lock locker.Lock, req request) {
	releaseCtx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()

	if err := lock.Release(releaseCtx); err != nil {
		m.logger.WarnContext(ctx, "idempotency: failed to release the lock",
			req.logAttrs(slog.String("error", err.Error()))...)
	}
}

// writeUnavailable answers a request that was not executed because its
// idempotency cannot be ensured right now; a retry with the same key is safe.
func (m *Middleware) writeUnavailable(ctx context.Context, w http.ResponseWriter, err error) {
	w.Header().Set("Retry-After", unavailableRetryAfter)
	m.responder.WriteError(ctx, w, api.WrapHTTPError(err, http.StatusServiceUnavailable))
}

// writeReplayRefused passes a 4xx through: it is the verdict of the access
// check. Any other failure means access could not be checked right now.
// Nothing ran, so it is a 503 the client retries with the same key, not a
// 500 that reads as an unknown outcome.
func (m *Middleware) writeReplayRefused(ctx context.Context, w http.ResponseWriter, err error) {
	var statusErr statusError
	if errors.As(err, &statusErr) {
		if status := statusErr.HTTPStatus(); status >= http.StatusBadRequest && status < http.StatusInternalServerError {
			m.responder.WriteError(ctx, w, err)

			return
		}
	}

	m.writeUnavailable(ctx, w, errors.WithMessage(err, "failed to authorize the replay"))
}

func replay(w http.ResponseWriter, record *domain.IdempotencyKey) {
	header := w.Header()

	for name, values := range record.ResponseHeaders {
		header.Del(name)

		for _, value := range values {
			header.Add(name, value)
		}
	}

	header.Set(HeaderReplayed, "true")
	w.WriteHeader(record.ResponseStatus)
	_, _ = w.Write(record.ResponseBody)
}

// readBody buffers the body for the fingerprint and hands the handler an
// identical copy.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		return nil, err
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	return body, nil
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func truncate(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}

	return s[:maxLength]
}
