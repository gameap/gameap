package idempotency_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/idempotency"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const maxBody = 1 << 20

var errStoreDown = errors.New("store is down")

// failingStore wraps a working store and fails the operations it was told to.
type failingStore struct {
	idempotency.Store

	findErr error
	saveErr error
	saves   atomic.Int32
}

func (s *failingStore) Find(
	ctx context.Context,
	userID uint,
	keyHash string,
	now time.Time,
) (*domain.IdempotencyKey, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}

	return s.Store.Find(ctx, userID, keyHash, now)
}

func (s *failingStore) Save(ctx context.Context, record *domain.IdempotencyKey) (bool, error) {
	s.saves.Add(1)

	if s.saveErr != nil {
		return false, s.saveErr
	}

	return s.Store.Save(ctx, record)
}

type failingLocker struct {
	err error
}

func (l failingLocker) Acquire(context.Context, string, time.Duration) (locker.Lock, error) {
	return nil, l.err
}

func newTestMiddleware(
	t *testing.T,
	store idempotency.Store,
	locks locker.Locker,
	opts ...idempotency.Option,
) *idempotency.Middleware {
	t.Helper()

	if store == nil {
		store = inmemory.NewIdempotencyKeyRepository()
	}

	if locks == nil {
		locks = locker.NewInMemoryLocker()
	}

	return idempotency.NewMiddleware(store, locks, api.NewResponder(), []byte("test-secret"), opts...)
}

// createHandler answers like POST /api/servers: 201 with the id of the server
// the call made, so a second execution is visible in the body.
func createHandler(calls *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)

		id := strconv.Itoa(int(calls.Add(1)))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/api/servers/"+id)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"serverId":` + id + `}`))
	})
}

func newKeyedRequest(method, target, key, body string, userID uint) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))

	if key != "" {
		r.Header.Set(idempotency.HeaderKey, key)
	}

	if userID != 0 {
		r = r.WithContext(auth.ContextWithSession(r.Context(), &auth.Session{
			User: &domain.User{ID: userID},
		}))
	}

	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	return w
}

func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error string `json:"error"`
	}

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	return body.Error
}

func TestMiddleware_replays_completed_request(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

	first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{"name":"cs"}`, 1))
	second := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{"name":"cs"}`, 1))

	assert.Equal(t, int32(1), calls.Load())

	assert.Equal(t, http.StatusCreated, first.Code)
	assert.JSONEq(t, `{"serverId":1}`, first.Body.String())
	assert.Empty(t, first.Header().Get(idempotency.HeaderReplayed))

	assert.Equal(t, http.StatusCreated, second.Code)
	assert.JSONEq(t, `{"serverId":1}`, second.Body.String())
	assert.Equal(t, "true", second.Header().Get(idempotency.HeaderReplayed))
	assert.Equal(t, "application/json", second.Header().Get("Content-Type"))
	assert.Equal(t, "/api/servers/1", second.Header().Get("Location"))
}

func TestMiddleware_matches_equivalent_request(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		firstTarget string
		firstBody   string
		retryTarget string
		retryBody   string
	}{
		{
			name:        "identical_json_body",
			firstTarget: "/api/servers",
			firstBody:   `{"name":"cs","server_port":27015,"vars":{"b":1,"a":2}}`,
			retryTarget: "/api/servers",
			retryBody:   `{"name":"cs","server_port":27015,"vars":{"b":1,"a":2}}`,
		},
		{
			name:        "reordered_query_parameters",
			firstTarget: "/api/servers?install=1&notify=0",
			firstBody:   `{}`,
			retryTarget: "/api/servers?notify=0&install=1",
			retryBody:   `{}`,
		},
		{
			name:        "identical_non_json_body",
			firstTarget: "/api/servers",
			firstBody:   "name=cs&port=27015",
			retryTarget: "/api/servers",
			retryBody:   "name=cs&port=27015",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

			serve(h, newKeyedRequest(http.MethodPost, tt.firstTarget, "order-1", tt.firstBody, 1))
			retry := serve(h, newKeyedRequest(http.MethodPost, tt.retryTarget, "order-1", tt.retryBody, 1))

			assert.Equal(t, int32(1), calls.Load())
			assert.Equal(t, http.StatusCreated, retry.Code)
			assert.JSONEq(t, `{"serverId":1}`, retry.Body.String())
			assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
		})
	}
}

func TestMiddleware_rejects_key_reused_for_different_request(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		retryMethod string
		retryTarget string
		retryBody   string
	}{
		{
			name:        "different_body",
			retryMethod: http.MethodPost,
			retryTarget: "/api/servers",
			retryBody:   `{"name":"tf2"}`,
		},
		{
			name:        "different_number_literal",
			retryMethod: http.MethodPost,
			retryTarget: "/api/servers",
			retryBody:   `{"name":"cs","port":27015.0}`,
		},
		{
			name:        "reordered_json_fields",
			retryMethod: http.MethodPost,
			retryTarget: "/api/servers",
			retryBody:   `{"port":27015,"name":"cs"}`,
		},
		{
			name:        "different_json_whitespace",
			retryMethod: http.MethodPost,
			retryTarget: "/api/servers",
			retryBody:   `{ "name": "cs", "port": 27015 }`,
		},
		{
			name:        "different_path",
			retryMethod: http.MethodPost,
			retryTarget: "/api/users",
			retryBody:   `{"name":"cs","port":27015}`,
		},
		{
			name:        "different_method",
			retryMethod: http.MethodPut,
			retryTarget: "/api/servers",
			retryBody:   `{"name":"cs","port":27015}`,
		},
		{
			name:        "different_query",
			retryMethod: http.MethodPost,
			retryTarget: "/api/servers?install=1",
			retryBody:   `{"name":"cs","port":27015}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

			serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{"name":"cs","port":27015}`, 1))
			retry := serve(h, newKeyedRequest(tt.retryMethod, tt.retryTarget, "order-1", tt.retryBody, 1))

			assert.Equal(t, int32(1), calls.Load())
			assert.Equal(t, http.StatusUnprocessableEntity, retry.Code)
			assert.Contains(t, errorMessage(t, retry), "idempotency key has already been used for a different request")
			assert.Empty(t, retry.Header().Get(idempotency.HeaderReplayed))
		})
	}
}

func TestMiddleware_passes_through_requests_without_idempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		key     string
		userID  uint
		headers map[string]string
	}{
		{
			name:   "no_header",
			method: http.MethodPost,
			userID: 1,
		},
		{
			name:    "empty_header",
			method:  http.MethodPost,
			userID:  1,
			headers: map[string]string{idempotency.HeaderKey: "  "},
		},
		{
			name:   "read_only_method",
			method: http.MethodGet,
			key:    "order-1",
			userID: 1,
		},
		{
			name:   "guest_request",
			method: http.MethodPost,
			key:    "order-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

			for range 2 {
				r := newKeyedRequest(tt.method, "/api/servers", tt.key, `{"name":"cs"}`, tt.userID)
				for name, value := range tt.headers {
					r.Header.Set(name, value)
				}

				w := serve(h, r)

				assert.Equal(t, http.StatusCreated, w.Code)
				assert.Empty(t, w.Header().Get(idempotency.HeaderReplayed))
			}

			assert.Equal(t, int32(2), calls.Load())
		})
	}
}

func TestMiddleware_rejects_invalid_key(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    []string
		wantError string
	}{
		{
			name:      "key_over_max_length",
			values:    []string{strings.Repeat("k", 256)},
			wantError: "the key must be at most 255 characters long: invalid Idempotency-Key header",
		},
		{
			name:      "repeated_header",
			values:    []string{"first", "second"},
			wantError: "the header must be sent once: invalid Idempotency-Key header",
		},
		{
			name:      "unterminated_quoted_key",
			values:    []string{`"order-1`},
			wantError: "the quoted key is not terminated: invalid Idempotency-Key header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

			r := newKeyedRequest(http.MethodPost, "/api/servers", "", `{}`, 1)
			for _, value := range tt.values {
				r.Header.Add(idempotency.HeaderKey, value)
			}

			w := serve(h, r)

			assert.Equal(t, int32(0), calls.Load())
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, errorMessage(t, w), tt.wantError)
		})
	}
}

func TestMiddleware_scopes_keys_by_user(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

	first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	other := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 2))
	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, int32(2), calls.Load())
	assert.JSONEq(t, `{"serverId":1}`, first.Body.String())
	assert.JSONEq(t, `{"serverId":2}`, other.Body.String())
	assert.Empty(t, other.Header().Get(idempotency.HeaderReplayed))
	assert.JSONEq(t, `{"serverId":1}`, retry.Body.String())
	assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
}

func TestMiddleware_conflicts_while_original_runs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	started := make(chan struct{})
	proceed := make(chan struct{})

	h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-proceed

		createHandler(&calls).ServeHTTP(w, r)
	}))

	done := make(chan *httptest.ResponseRecorder)
	go func() {
		done <- serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	}()

	<-started

	concurrent := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	close(proceed)
	original := <-done

	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, http.StatusConflict, concurrent.Code)
	assert.Equal(t, "1", concurrent.Header().Get("Retry-After"))
	assert.Contains(t, errorMessage(t, concurrent), "a request with this idempotency key is still being processed")

	assert.Equal(t, http.StatusCreated, original.Code)
	assert.Equal(t, http.StatusCreated, retry.Code)
	assert.JSONEq(t, `{"serverId":1}`, retry.Body.String())
	assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
	assert.Equal(t, int32(1), calls.Load())
}

func TestMiddleware_stores_handler_errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		err    error
	}{
		{
			name:   "client_error",
			status: http.StatusUnprocessableEntity,
			err:    api.NewValidationError("game mod does not belong to the specified game"),
		},
		{
			name:   "server_error",
			status: http.StatusInternalServerError,
			err:    errors.New("failed to create install task"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			responder := api.NewResponder()
			h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					responder.WriteError(r.Context(), w, tt.err)
				},
			))

			first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
			retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

			assert.Equal(t, int32(1), calls.Load())
			assert.Equal(t, tt.status, first.Code)
			assert.Equal(t, tt.status, retry.Code)
			assert.JSONEq(t, first.Body.String(), retry.Body.String())
			assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
		})
	}
}

func TestMiddleware_stores_recovered_panic(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	recovery := middlewares.NewRecoveryMiddleware(api.NewResponder())
	m := newTestMiddleware(t, nil, nil, idempotency.WithRecovery(recovery.Middleware))

	h := m.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
		panic("nil server")
	}))

	first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, http.StatusInternalServerError, first.Code)
	assert.Equal(t, http.StatusInternalServerError, retry.Code)
	assert.Equal(t, "Internal Server Error", errorMessage(t, retry))
	assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
}

func TestMiddleware_does_not_store_aborted_handler(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	recovery := middlewares.NewRecoveryMiddleware(api.NewResponder())
	m := newTestMiddleware(t, nil, nil, idempotency.WithRecovery(recovery.Middleware))

	h := m.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		if calls.Add(1) == 1 {
			panic(http.ErrAbortHandler)
		}
	}))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	})

	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, http.StatusOK, retry.Code)
	assert.Empty(t, retry.Header().Get(idempotency.HeaderReplayed))
}

func TestMiddleware_completes_request_after_client_disconnects(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	started := make(chan struct{})
	proceed := make(chan struct{})

	h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-proceed

		if err := r.Context().Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		createHandler(&calls).ServeHTTP(w, r)
	}))

	r := newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1)
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)

	done := make(chan *httptest.ResponseRecorder)
	go func() {
		done <- serve(h, r)
	}()

	<-started
	cancel()
	close(proceed)

	original := <-done
	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, http.StatusCreated, original.Code)
	assert.Equal(t, http.StatusCreated, retry.Code)
	assert.JSONEq(t, `{"serverId":1}`, retry.Body.String())
	assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
	assert.Equal(t, int32(1), calls.Load())
}

func TestMiddleware_replays_headers_as_sent(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)

		w.Header().Set("Location", "/api/servers/1")
		w.Header().Set("Set-Cookie", "session=secret")
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		// Set after WriteHeader, so net/http never sends it.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"serverId":1}`))
	}))

	first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, int32(1), calls.Load())
	assert.Empty(t, first.Result().Header.Get("Content-Type"))

	result := retry.Result()
	assert.Equal(t, http.StatusCreated, result.StatusCode)
	assert.Equal(t, "/api/servers/1", result.Header.Get("Location"))
	assert.Empty(t, result.Header.Get("Content-Type"))
	assert.Empty(t, result.Header.Get("Set-Cookie"))
	assert.Empty(t, result.Header.Get("X-Custom"))
	assert.JSONEq(t, `{"serverId":1}`, retry.Body.String())
}

func TestMiddleware_stores_empty_response(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))

	first := serve(h, newKeyedRequest(http.MethodDelete, "/api/servers/1", "order-1", "", 1))
	retry := serve(h, newKeyedRequest(http.MethodDelete, "/api/servers/1", "order-1", "", 1))

	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, retry.Code)
	assert.Empty(t, retry.Body.String())
	assert.Equal(t, "true", retry.Header().Get(idempotency.HeaderReplayed))
}

func TestMiddleware_does_not_store_oversized_response(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	payload := strings.Repeat("x", maxBody+1)

	h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(payload))
	}))

	first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, maxBody+1, first.Body.Len())
	assert.Equal(t, maxBody+1, retry.Body.Len())
	assert.Empty(t, retry.Header().Get(idempotency.HeaderReplayed))
	assert.Equal(t, int32(2), calls.Load())
}

func TestMiddleware_rejects_oversized_request_body(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := newTestMiddleware(t, nil, nil).Middleware(createHandler(&calls))

	w := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", strings.Repeat("x", maxBody+1), 1))

	assert.Equal(t, int32(0), calls.Load())
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, errorMessage(t, w), "request body is too large for an idempotent request")
}

func TestMiddleware_passes_request_body_to_handler(t *testing.T) {
	t.Parallel()

	var received string

	h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		received = string(body)
		w.WriteHeader(http.StatusCreated)
	}))

	w := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{"name":"cs"}`, 1))

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.JSONEq(t, `{"name":"cs"}`, received)
}

func TestMiddleware_fails_closed_when_store_is_unavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		store func() idempotency.Store
		locks locker.Locker
	}{
		{
			name: "store_lookup_fails",
			store: func() idempotency.Store {
				return &failingStore{Store: inmemory.NewIdempotencyKeyRepository(), findErr: errStoreDown}
			},
		},
		{
			name: "lock_backend_fails",
			store: func() idempotency.Store {
				return inmemory.NewIdempotencyKeyRepository()
			},
			locks: failingLocker{err: errStoreDown},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			h := newTestMiddleware(t, tt.store(), tt.locks).Middleware(createHandler(&calls))

			w := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

			assert.Equal(t, int32(0), calls.Load())
			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			assert.Equal(t, "5", w.Header().Get("Retry-After"))
			assert.Equal(t, "Service Unavailable", errorMessage(t, w))
		})
	}
}

func TestMiddleware_runs_again_when_outcome_was_not_stored(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		store := &failingStore{Store: inmemory.NewIdempotencyKeyRepository(), saveErr: errStoreDown}
		h := newTestMiddleware(t, store, nil).Middleware(createHandler(&calls))

		first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
		retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

		assert.Equal(t, http.StatusCreated, first.Code)
		assert.Equal(t, http.StatusCreated, retry.Code)
		assert.JSONEq(t, `{"serverId":2}`, retry.Body.String())
		assert.Equal(t, int32(2), calls.Load())
		assert.Equal(t, int32(6), store.saves.Load(), "every outcome is saved in three attempts")
	})
}

func TestMiddleware_runs_again_after_outcome_expires(t *testing.T) {
	t.Parallel()

	var (
		mu  sync.Mutex
		now = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	)

	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()

		return now
	}

	var calls atomic.Int32
	m := newTestMiddleware(t, nil, nil, idempotency.WithClock(clock), idempotency.WithKeyTTL(time.Hour))
	h := m.Middleware(createHandler(&calls))

	serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	mu.Lock()
	now = now.Add(time.Hour)
	mu.Unlock()

	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{"name":"other"}`, 1))

	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, http.StatusCreated, retry.Code)
	assert.JSONEq(t, `{"serverId":2}`, retry.Body.String())
	assert.Empty(t, retry.Header().Get(idempotency.HeaderReplayed))
}

func TestMiddleware_keeps_lock_while_slow_request_runs(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32

		h := newTestMiddleware(t, nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Longer than the one-minute lock TTL: only the refresh keeps
			// the lock alive.
			time.Sleep(3 * time.Minute)
			createHandler(&calls).ServeHTTP(w, r)
		}))

		done := make(chan *httptest.ResponseRecorder)
		go func() {
			done <- serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
		}()

		time.Sleep(150 * time.Second)

		concurrent := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
		original := <-done

		assert.Equal(t, http.StatusConflict, concurrent.Code)
		assert.Equal(t, http.StatusCreated, original.Code)
		assert.Equal(t, int32(1), calls.Load())
	})
}

func TestDisabled_ignores_the_header(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	h := idempotency.Disabled().Middleware(createHandler(&calls))

	first := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))
	retry := serve(h, newKeyedRequest(http.MethodPost, "/api/servers", "order-1", `{}`, 1))

	assert.Equal(t, int32(2), calls.Load())
	assert.JSONEq(t, `{"serverId":1}`, first.Body.String())
	assert.JSONEq(t, `{"serverId":2}`, retry.Body.String())
	assert.Empty(t, retry.Header().Get(idempotency.HeaderReplayed))
}
