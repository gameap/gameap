// Security tests for the plugin HTTP route handler.
//
// OWASP API Security Top 10:2023:
//   - API4:2023 Unrestricted Resource Consumption — requests to plugin routes
//     are bounded per client (rate limit), per plugin and per instance
//     (request caps), and in size (body, query); a flood must never take a
//     plugin down, because a request that queued for most of its deadline is
//     refused instead of entering the guest with a budget it cannot honour.
//   - API2:2023 Broken Authentication — the operator can refuse anonymous
//     plugin routes whatever the plugin declared.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/ratelimit"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/api"
)

const hardeningPluginDBID uint64 = 7

// fakeMemory answers every read with an empty buffer: the fake guest returns
// an empty (zero-value) response, which the handler serves as 200.
type fakeMemory struct {
	api.Memory
}

func (fakeMemory) Write(uint32, []byte) bool          { return true }
func (fakeMemory) Read(uint32, uint32) ([]byte, bool) { return []byte{}, true }
func (fakeMemory) Size() uint32                       { return 0 }

type fakeModule struct {
	api.Module
}

func (fakeModule) Memory() api.Memory { return fakeMemory{} }
func (fakeModule) IsClosed() bool     { return false }

type fakeDefinition struct {
	api.FunctionDefinition

	name string
}

func (d fakeDefinition) ExportNames() []string { return []string{d.name} }

type fakeFunction struct {
	api.Function

	name string
	call func(ctx context.Context) ([]uint64, error)
}

func (f fakeFunction) Definition() api.FunctionDefinition { return fakeDefinition{name: f.name} }

func (f fakeFunction) Call(ctx context.Context, _ ...uint64) ([]uint64, error) {
	return f.call(ctx)
}

// newFakeGuestWrapper builds a pluginServiceWrapper over a fake module, so
// HandleHTTPRequest goes through the real call gate, queue timeout, budget
// floor and deadline handling without a wasm runtime. handle is the guest
// work; an error it returns is what a closed module would answer with.
func newFakeGuestWrapper(handle func(ctx context.Context) error) *pluginServiceWrapper {
	return &pluginServiceWrapper{
		gate:   make(chan struct{}, 1),
		module: fakeModule{},
		malloc: fakeFunction{name: "malloc", call: func(context.Context) ([]uint64, error) {
			return []uint64{8}, nil
		}},
		free: fakeFunction{name: "free", call: func(context.Context) ([]uint64, error) {
			return nil, nil
		}},
		handlehttprequest: fakeFunction{
			name: "plugin_service_handle_http_request",
			call: func(ctx context.Context) ([]uint64, error) {
				if err := handle(ctx); err != nil {
					return nil, err
				}

				return []uint64{0}, nil
			},
		},
	}
}

// httpAuditRecorder keeps every audit event the handler emitted.
type httpAuditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *httpAuditRecorder) Record(_ context.Context, e audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *httpAuditRecorder) all() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.events)
}

// newHardeningHandler registers one plugin with an anonymous GET /status
// route and builds a handler over it with the given options.
func newHardeningHandler(
	t *testing.T,
	instance proto.PluginService,
	opts ...HTTPHandlerOption,
) (*HTTPHandler, *LoadedPlugin, *mockMiddleware) {
	t.Helper()

	manager := NewManager(ManagerConfig{})
	pluginID := CompactPluginID(ParsePluginID("hardened"))
	plugin := &LoadedPlugin{
		Info:     &proto.PluginInfo{Id: pluginID},
		DBID:     hardeningPluginDBID,
		Enabled:  true,
		Instance: instance,
		HTTPRoutes: []*proto.HTTPRoute{
			{Path: "/status", Methods: []string{http.MethodGet, http.MethodPost}},
		},
	}
	manager.plugins[pluginID] = plugin

	authMw := &mockMiddleware{}
	handler := NewHTTPHandler(manager, authMw, &mockMiddleware{}, opts...)

	return handler, plugin, authMw
}

func hardeningRequest(t *testing.T, method, target string, body string) *http.Request {
	t.Helper()

	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, target, reader)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}

	return mux.SetURLVars(req, map[string]string{"plugin_id": "hardened"})
}

func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	return rr
}

// TestHTTPHandler_flood_of_cheap_requests_never_disables_the_plugin covers
// OWASP API4:2023. Every guest call is fast, yet under a flood the callers
// queue for most of their deadline; a call admitted with less budget than
// the guest needs must be refused, not started — starting it would end in
// a deadline close and a disabled plugin.
func TestHTTPHandler_flood_of_cheap_requests_never_disables_the_plugin(t *testing.T) {
	t.Parallel()

	// ARRANGE
	const guestWork = 50 * time.Millisecond

	wrapper := newFakeGuestWrapper(func(ctx context.Context) error {
		select {
		case <-time.After(guestWork):
			return nil
		case <-ctx.Done():
			// The runtime closes the module when the deadline passes.
			return ctx.Err()
		}
	})

	observer := &observerRecorder{}
	handler, plugin, _ := newHardeningHandler(t, wrapper,
		WithRequestTimeout(6*guestWork),
		WithQueueTimeout(0),
		WithGuestBudget(2*guestWork),
		WithMaxQueue(0),
		WithMaxInFlight(0),
		WithObserver(observer),
	)

	const clients = 200

	statuses := make(chan int, clients)

	var wg sync.WaitGroup

	// ACT
	for range clients {
		wg.Go(func() {
			rr := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))
			statuses <- rr.Code
		})
	}

	wg.Wait()
	close(statuses)

	// ASSERT
	served, refused := 0, 0

	for code := range statuses {
		switch code {
		case http.StatusOK:
			served++
		case http.StatusServiceUnavailable:
			refused++
		default:
			t.Errorf("unexpected status %d under flood", code)
		}
	}

	assert.Positive(t, served, "the plugin keeps answering the requests it can")
	assert.Positive(t, refused, "the rest are refused as busy")

	assert.True(t, plugin.IsEnabled(), "a flood of cheap requests must not disable the plugin")

	_, disabled := plugin.DisabledReason()
	assert.False(t, disabled)

	results := observer.httpResults()
	assert.NotContains(t, results, "plugin:"+HTTPResultTimeout, "no guest call may run into the deadline")
	assert.Contains(t, results, "plugin:"+HTTPResultBusy)
	assert.Contains(t, results, "plugin:"+HTTPResultOK)
}

// TestHTTPHandler_busy_answer_carries_retry_after covers OWASP API4:2023: a
// request that gave up on the gate is refused with a retry hint and is
// never cached.
func TestHTTPHandler_busy_answer_carries_retry_after(t *testing.T) {
	t.Parallel()

	// ARRANGE
	wrapper := newFakeGuestWrapper(func(context.Context) error { return nil })
	wrapper.gate <- struct{}{} // another call holds the gate for good

	handler, plugin, _ := newHardeningHandler(t, wrapper,
		WithRequestTimeout(time.Second),
		WithQueueTimeout(20*time.Millisecond),
	)

	// ACT
	rr := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))

	// ASSERT
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "plugin is busy")
	assert.Equal(t, retryAfterBusy, rr.Header().Get("Retry-After"))
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	assert.True(t, plugin.IsEnabled())
}

// TestHTTPHandler_rate_limits_anonymous_clients_per_ip covers OWASP
// API4:2023: anonymous clients share nothing — one address exhausting its
// bucket does not touch another — and the refusal is audited once per
// interval while the observer counts every refusal.
func TestHTTPHandler_rate_limits_anonymous_clients_per_ip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	observer := &observerRecorder{}
	auditLog := &httpAuditRecorder{}
	handler, _, _ := newHardeningHandler(t, &mockPluginServiceHTTP{},
		WithClientRateLimits(ratelimit.Limit{RPS: 1, Burst: 2}, ratelimit.Limit{}),
		WithObserver(observer),
		WithAuditLogger(auditLog),
	)

	from := func(addr string) *http.Request {
		req := hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", "")
		req.RemoteAddr = addr

		return req
	}

	// ACT
	first := serve(handler, from("203.0.113.5:40000"))
	second := serve(handler, from("203.0.113.5:40001"))
	third := serve(handler, from("203.0.113.5:40002"))
	fourth := serve(handler, from("203.0.113.5:40003"))
	other := serve(handler, from("203.0.113.9:40000"))

	// ASSERT
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, http.StatusTooManyRequests, third.Code, "the burst is spent")
	assert.Equal(t, http.StatusTooManyRequests, fourth.Code)
	assert.Equal(t, http.StatusOK, other.Code, "another address has its own bucket")

	assert.Contains(t, third.Body.String(), "too many requests")
	assert.Equal(t, "1", third.Header().Get("Retry-After"))
	assert.Equal(t, "no-store", third.Header().Get("Cache-Control"))

	events := auditLog.all()
	require.Len(t, events, 1, "the second refusal of the same client within the interval is not audited")
	assert.Equal(t, audit.EventPluginHTTPRateLimited, events[0].Type)
	assert.Equal(t, audit.CategoryRateLimit, events[0].Category)
	assert.Equal(t, audit.OutcomeBlocked, events[0].Outcome)
	assert.Equal(t, audit.AuthMethodAnonymous, events[0].AuthMethod)
	assert.Equal(t, "plugin", events[0].ResourceType)
	assert.Equal(t, "7", events[0].ResourceID)
	assert.Equal(t, "rate_limited", events[0].Reason)
	assert.Contains(t, attrValues(events[0]), "ip:203.0.113.5")
	assert.Contains(t, attrValues(events[0]), "/status")

	assert.Equal(t, 2, countResults(observer.httpResults(), HTTPResultRateLimited),
		"the observer counts every refusal")
	assert.Equal(t, 3, countResults(observer.httpResults(), HTTPResultOK))
}

// TestHTTPHandler_rate_limits_authenticated_clients_per_user covers OWASP
// API4:2023: a session is keyed by its user, not by the address, so a user
// cannot widen the budget by changing addresses and an anonymous client is
// not charged for it.
func TestHTTPHandler_rate_limits_authenticated_clients_per_user(t *testing.T) {
	t.Parallel()

	// ARRANGE
	auditLog := &httpAuditRecorder{}
	handler, _, _ := newHardeningHandler(t, &mockPluginServiceHTTP{},
		WithClientRateLimits(ratelimit.Limit{}, ratelimit.Limit{RPS: 1, Burst: 1}),
		WithAuditLogger(auditLog),
	)

	asUser := func(id uint, addr string) *http.Request {
		req := hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", "")
		req.RemoteAddr = addr
		session := &auth.Session{ID: "s", User: &domain.User{ID: id, Login: "user"}}

		return req.WithContext(auth.ContextWithSession(req.Context(), session))
	}

	// ACT
	first := serve(handler, asUser(5, "203.0.113.5:1"))
	second := serve(handler, asUser(5, "203.0.113.6:1"))
	otherUser := serve(handler, asUser(6, "203.0.113.5:1"))
	anonymous := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))

	// ASSERT
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusTooManyRequests, second.Code, "the same user from another address shares the bucket")
	assert.Equal(t, http.StatusOK, otherUser.Code)
	assert.Equal(t, http.StatusOK, anonymous.Code, "anonymous clients are not limited when only the user limit is set")

	events := auditLog.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.AuthMethodSession, events[0].AuthMethod, "the actor is the session that was refused")
	assert.Equal(t, uint(5), events[0].ActorID)
	assert.Contains(t, attrValues(events[0]), "user:5")
}

// TestHTTPHandler_honours_the_trusted_client_ip_header covers OWASP
// API4:2023: behind a reverse proxy every client shares the proxy's address,
// so the configured header is the client's identity for the limiter.
func TestHTTPHandler_honours_the_trusted_client_ip_header(t *testing.T) {
	t.Parallel()

	// ARRANGE
	handler, _, _ := newHardeningHandler(t, &mockPluginServiceHTTP{},
		WithClientRateLimits(ratelimit.Limit{RPS: 1, Burst: 1}, ratelimit.Limit{}),
		WithClientIPHeader("X-Real-IP"),
	)

	via := func(ip string) *http.Request {
		req := hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", "")
		req.Header.Set("X-Real-IP", ip)

		return req
	}

	// ACT
	first := serve(handler, via("198.51.100.1"))
	second := serve(handler, via("198.51.100.2"))
	third := serve(handler, via("198.51.100.1"))

	// ASSERT
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code, "a different client behind the same proxy has its own bucket")
	assert.Equal(t, http.StatusTooManyRequests, third.Code)
}

// TestHTTPHandler_caps_requests_per_plugin covers OWASP API4:2023: beyond
// the per-plugin cap a request is refused at once, so a slow plugin cannot
// pile up goroutines and buffered bodies for its whole timeout.
func TestHTTPHandler_caps_requests_per_plugin(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var started sync.Once

	entered := make(chan struct{})
	release := make(chan struct{})

	instance := &mockPluginServiceHTTP{
		handleHTTPRequestFunc: func(context.Context, *proto.HTTPRequest) (*proto.HTTPResponse, error) {
			started.Do(func() { close(entered) })
			<-release

			return &proto.HTTPResponse{StatusCode: 200}, nil
		},
	}

	observer := &observerRecorder{}
	handler, plugin, _ := newHardeningHandler(t, instance, WithMaxQueue(1), WithObserver(observer))

	firstDone := make(chan int, 1)

	// ACT
	go func() {
		firstDone <- serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", "")).Code
	}()

	<-entered

	queued := handler.QueuedRequests(plugin.Info.Id)
	second := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))

	close(release)

	// ASSERT
	assert.Equal(t, 1, queued)
	assert.Equal(t, http.StatusServiceUnavailable, second.Code)
	assert.Contains(t, second.Body.String(), "plugin request queue is full")
	assert.Equal(t, retryAfterBusy, second.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusOK, <-firstDone, "the request inside the cap completes")
	assert.Equal(t, 0, handler.QueuedRequests(plugin.Info.Id), "slots are released")
	assert.Contains(t, observer.httpResults(), "plugin:"+HTTPResultQueueFull)
}

// TestHTTPHandler_caps_requests_in_flight covers OWASP API4:2023: the
// per-instance cap bounds the goroutines and bodies the handler holds across
// every plugin.
func TestHTTPHandler_caps_requests_in_flight(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var started sync.Once

	entered := make(chan struct{})
	release := make(chan struct{})

	instance := &mockPluginServiceHTTP{
		handleHTTPRequestFunc: func(context.Context, *proto.HTTPRequest) (*proto.HTTPResponse, error) {
			started.Do(func() { close(entered) })
			<-release

			return &proto.HTTPResponse{StatusCode: 200}, nil
		},
	}

	observer := &observerRecorder{}
	handler, _, _ := newHardeningHandler(t, instance, WithMaxInFlight(1), WithMaxQueue(0), WithObserver(observer))

	firstDone := make(chan int, 1)

	// ACT
	go func() {
		firstDone <- serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", "")).Code
	}()

	<-entered

	second := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))

	close(release)

	// ASSERT
	assert.Equal(t, http.StatusServiceUnavailable, second.Code)
	assert.Contains(t, second.Body.String(), "too many plugin requests in flight")
	assert.Equal(t, retryAfterBusy, second.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusOK, <-firstDone)
	assert.Contains(t, observer.httpResults(), "plugin:"+HTTPResultInFlightFull)

	third := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))
	assert.Equal(t, http.StatusOK, third.Code, "the slot is released once the request completes")
}

// TestHTTPHandler_refuses_oversized_input covers OWASP API4:2023: the body
// cap answers 413 without draining the upload, the query cap answers 414
// before the query is expanded for the guest.
func TestHTTPHandler_refuses_oversized_input(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "body_over_the_cap",
			target:     "/api/plugins/hardened/status",
			body:       strings.Repeat("x", 9),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   "request body too large",
		},
		{
			name:       "body_at_the_cap",
			target:     "/api/plugins/hardened/status",
			body:       strings.Repeat("x", 8),
			wantStatus: http.StatusOK,
		},
		{
			name:       "query_over_the_cap",
			target:     "/api/plugins/hardened/status?q=1,2,3,4,5,6,7,8,9",
			wantStatus: http.StatusRequestURITooLong,
			wantBody:   "query string too long",
		},
		{
			name:       "query_at_the_cap",
			target:     "/api/plugins/hardened/status?q=1,2,3,4,5,6",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var seen bool

			instance := &mockPluginServiceHTTP{
				handleHTTPRequestFunc: func(context.Context, *proto.HTTPRequest) (*proto.HTTPResponse, error) {
					seen = true

					return &proto.HTTPResponse{StatusCode: 200}, nil
				},
			}

			observer := &observerRecorder{}
			handler, _, _ := newHardeningHandler(t, instance, WithMaxBody(8), WithMaxQuery(15), WithObserver(observer))

			// ACT
			rr := serve(handler, hardeningRequest(t, http.MethodPost, tt.target, tt.body))

			// ASSERT
			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBody)
				assert.False(t, seen, "an oversized request never reaches the guest")
				assert.Contains(t, observer.httpResults(), "plugin:"+HTTPResultTooLarge)

				return
			}

			assert.True(t, seen)
		})
	}
}

// TestHTTPHandler_anonymous_routes_switch covers OWASP API2:2023: with
// anonymous routes off, a route the plugin declared without requires_auth
// still goes through the auth middleware.
func TestHTTPHandler_anonymous_routes_switch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		allowed  bool
		wantAuth bool
	}{
		{name: "allowed_serves_the_route_without_auth", allowed: true, wantAuth: false},
		{name: "refused_sends_the_route_through_auth", allowed: false, wantAuth: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			handler, _, authMw := newHardeningHandler(t, &mockPluginServiceHTTP{}, WithAnonymousRoutes(tt.allowed))

			// ACT
			rr := serve(handler, hardeningRequest(t, http.MethodGet, "/api/plugins/hardened/status", ""))

			// ASSERT
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.wantAuth, authMw.called)
		})
	}
}

func TestNewHTTPHandler_defaults_and_options(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		handler := NewHTTPHandler(NewManager(ManagerConfig{}), &mockMiddleware{}, &mockMiddleware{})

		assert.Equal(t, DefaultQueueTimeout, handler.queueTimeout)
		assert.Equal(t, DefaultMinCallBudget, handler.minCallBudget)
		assert.Equal(t, int64(DefaultMaxQuerySize), handler.maxQuery)
		assert.Equal(t, int32(DefaultMaxQueue), handler.maxQueue)
		assert.Equal(t, DefaultMaxInFlight, cap(handler.inflight))
		assert.True(t, handler.anonymousRoutes)
		assert.Nil(t, handler.anonLimiter, "rate limits are opt-in for the handler; the panel sets them from config")
		assert.Nil(t, handler.userLimiter)
	})

	t.Run("non_positive_values_keep_or_remove_the_bound", func(t *testing.T) {
		t.Parallel()

		handler := NewHTTPHandler(NewManager(ManagerConfig{}), &mockMiddleware{}, &mockMiddleware{},
			WithRequestTimeout(0),
			WithGuestBudget(-1),
			WithMaxBody(0),
			WithQueueTimeout(0),
			WithMaxQuery(0),
			WithMaxQueue(0),
			WithMaxInFlight(0),
			WithClientRateLimits(ratelimit.Limit{}, ratelimit.Limit{RPS: 1}),
		)

		assert.Equal(t, DefaultTimeout, handler.timeout, "a timeout of zero keeps the default")
		assert.Equal(t, DefaultMinCallBudget, handler.minCallBudget)
		assert.Equal(t, int64(DefaultMaxBodySize), handler.maxBody, "a body cap of zero keeps the default")
		assert.Equal(t, time.Duration(0), handler.queueTimeout, "zero waits up to the request timeout")
		assert.Equal(t, int64(0), handler.maxQuery, "zero removes the query cap")
		assert.Equal(t, int32(0), handler.maxQueue)
		assert.Nil(t, handler.inflight)
		assert.Nil(t, handler.anonLimiter)
		assert.NotNil(t, handler.userLimiter)
	})
}

func attrValues(event audit.Event) []string {
	values := make([]string, 0, len(event.Extra))
	for _, attr := range event.Extra {
		values = append(values, attr.Value.String())
	}

	return values
}

func countResults(results []string, result string) int {
	count := 0

	for _, r := range results {
		if r == "plugin:"+result {
			count++
		}
	}

	return count
}

// TestHTTPHandler_real_guest_survives_a_flood covers OWASP API4:2023 end to
// end: the example plugin's anonymous /status route served by a real wasm
// module, with the runtime's own deadline handling. Whatever the mix of
// served and refused requests, the plugin stays enabled.
func TestHTTPHandler_real_guest_survives_a_flood(t *testing.T) {
	t.Parallel()

	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)
	require.NotEmpty(t, plugin.HTTPRoutes, "the example plugin declares its routes on load")

	observer := &observerRecorder{}
	handler := NewHTTPHandler(sharedManager, &mockMiddleware{}, &mockMiddleware{},
		WithRequestTimeout(200*time.Millisecond),
		WithQueueTimeout(0),
		WithGuestBudget(20*time.Millisecond),
		WithMaxQueue(0),
		WithMaxInFlight(0),
		WithObserver(observer),
	)

	target := "/api/plugins/" + plugin.Info.Id + "/status"

	const clients = 200

	codes := make(chan int, clients)
	bodies := make(chan string, clients)

	var wg sync.WaitGroup

	// ACT
	for range clients {
		wg.Go(func() {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req = mux.SetURLVars(req, map[string]string{"plugin_id": plugin.Info.Id})

			rr := serve(handler, req)
			codes <- rr.Code
			bodies <- rr.Body.String()
		})
	}

	wg.Wait()
	close(codes)
	close(bodies)

	// ASSERT
	served := 0

	for code := range codes {
		switch code {
		case http.StatusOK:
			served++
		case http.StatusServiceUnavailable:
		default:
			t.Errorf("unexpected status %d under flood", code)
		}
	}

	assert.Positive(t, served)

	for body := range bodies {
		if strings.Contains(body, `"status":"ok"`) {
			served--
		}
	}

	assert.Equal(t, 0, served, "every 200 carries the plugin's own answer")
	assert.True(t, plugin.IsEnabled(), "a flood must not disable the plugin")
	assert.NotContains(t, observer.httpResults(), "transient:"+HTTPResultTimeout)
}
