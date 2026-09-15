package plugin

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin/proto"
	gameapProto "github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/ratelimit"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

const (
	DefaultTimeout     = 30 * time.Second
	DefaultMaxBodySize = 1 << 20 // 1MB
	// DefaultQueueTimeout bounds the wait for the plugin's call gate; a
	// request that waited this long answers 503 and never reaches the guest.
	DefaultQueueTimeout = 10 * time.Second
	// DefaultMinCallBudget is what must remain of the request timeout once
	// the call wins the gate; with less the request answers 503 rather than
	// starting a guest call the deadline would cut short.
	DefaultMinCallBudget = 5 * time.Second
	// DefaultMaxQuerySize caps the raw query string handed to the guest.
	DefaultMaxQuerySize = 64 << 10
	// DefaultMaxQueue caps the requests of one plugin inside the handler on
	// one panel instance, waiting for the gate or executing.
	DefaultMaxQueue = 32
	// DefaultMaxInFlight caps plugin requests inside the handler across all
	// plugins on one panel instance.
	DefaultMaxInFlight = 256

	// retryAfterBusy is the Retry-After of a 503 for a busy plugin or a full
	// queue: the condition clears with the next guest call.
	retryAfterBusy = "1"
	// rateLimitAuditInterval spaces out the audit records for one client and
	// plugin; the metric still counts every refusal.
	rateLimitAuditInterval = time.Minute
	// rateLimitAuditKeys bounds the audit throttle table, so a flood spread
	// over many clients cannot grow it without bound.
	rateLimitAuditKeys = 4096
)

var errBodyTooLarge = errors.New("request body too large")

type Middleware interface {
	Middleware(next http.Handler) http.Handler
}

// HTTPHandler handles HTTP requests for plugins.
type HTTPHandler struct {
	manager         *Manager
	authMiddleware  Middleware
	adminMiddleware Middleware
	timeout         time.Duration
	queueTimeout    time.Duration
	minCallBudget   time.Duration
	maxBody         int64
	maxQuery        int64
	maxQueue        int32
	anonymousRoutes bool
	clientIPHeader  string
	fileRefs        FileRefServer
	observer        Observer
	audit           audit.Logger

	// inflight caps the requests inside the handler on this instance; nil
	// leaves them uncapped.
	inflight chan struct{}
	// queued counts, per plugin id, the requests waiting for the call gate
	// or executing; the counters outlive a reload of the plugin.
	queued sync.Map

	anonLimiter   *ratelimit.KeyedLimiter
	userLimiter   *ratelimit.KeyedLimiter
	auditThrottle *throttle[rateLimitAuditKey]
}

type rateLimitAuditKey struct {
	pluginID uint64
	client   string
}

// HTTPHandlerOption configures a handler created by NewHTTPHandler.
type HTTPHandlerOption func(*HTTPHandler)

// WithFileRefServer enables HTTPResponse.file: the server streams the
// referenced node file to the client. Without it such responses answer 501.
func WithFileRefServer(server FileRefServer) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.fileRefs = server
	}
}

// WithRequestTimeout bounds one request end to end: the wait for the
// plugin's call gate plus the guest call. Non-positive keeps the default.
func WithRequestTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if timeout > 0 {
			h.timeout = timeout
		}
	}
}

// WithQueueTimeout bounds the wait for the call gate alone; a request that
// waited this long answers 503 without touching the guest. Non-positive
// waits up to the request timeout.
func WithQueueTimeout(timeout time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.queueTimeout = max(timeout, 0)
	}
}

// WithGuestBudget sets what must remain of the request timeout once the
// call wins the gate (see DefaultMinCallBudget). Non-positive keeps the
// default.
func WithGuestBudget(budget time.Duration) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if budget > 0 {
			h.minCallBudget = budget
		}
	}
}

// WithMaxBody caps the request body handed to the guest; larger requests
// answer 413. Non-positive keeps the default.
func WithMaxBody(limit int64) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if limit > 0 {
			h.maxBody = limit
		}
	}
}

// WithMaxQuery caps the raw query string; longer ones answer 414.
// Non-positive removes the cap.
func WithMaxQuery(limit int64) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.maxQuery = max(limit, 0)
	}
}

// WithMaxQueue caps the requests of one plugin inside the handler, waiting
// for the gate or executing; further ones answer 503. Non-positive removes
// the cap.
func WithMaxQueue(limit int) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.maxQueue = int32(max(limit, 0)) //nolint:gosec // a queue cap fits an int32
	}
}

// WithMaxInFlight caps plugin requests inside the handler across all plugins;
// further ones answer 503. Non-positive removes the cap.
func WithMaxInFlight(limit int) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		if limit <= 0 {
			h.inflight = nil

			return
		}

		h.inflight = make(chan struct{}, limit)
	}
}

// WithAnonymousRoutes decides whether routes a plugin declares without
// requires_auth are served to clients without a session. Off, every plugin
// route goes through the auth middleware, whatever the plugin declared.
func WithAnonymousRoutes(allowed bool) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.anonymousRoutes = allowed
	}
}

// WithClientRateLimits installs per-client token buckets: anonymous clients
// are keyed by IP, authenticated ones by user. A disabled limit (RPS 0)
// leaves its class unlimited.
func WithClientRateLimits(anonymous, user ratelimit.Limit) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.anonLimiter, h.userLimiter = nil, nil

		if anonymous.Enabled() {
			h.anonLimiter = ratelimit.NewKeyed(anonymous)
		}

		if user.Enabled() {
			h.userLimiter = ratelimit.NewKeyed(user)
		}
	}
}

// WithClientIPHeader names the trusted reverse-proxy header the client IP
// is read from (empty: the connection's remote address).
func WithClientIPHeader(header string) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.clientIPHeader = header
	}
}

// WithObserver reports the outcome of every request (HTTPResult* values).
func WithObserver(observer Observer) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.observer = observerOrNop(observer)
	}
}

// WithAuditLogger records rate-limited requests in the audit log, throttled
// per client and plugin.
func WithAuditLogger(logger audit.Logger) HTTPHandlerOption {
	return func(h *HTTPHandler) {
		h.audit = logger
	}
}

// NewHTTPHandler creates a new HTTP handler for plugin routes.
func NewHTTPHandler(
	manager *Manager,
	authMiddleware Middleware,
	adminMiddleware Middleware,
	opts ...HTTPHandlerOption,
) *HTTPHandler {
	handler := &HTTPHandler{
		manager:         manager,
		authMiddleware:  authMiddleware,
		adminMiddleware: adminMiddleware,
		timeout:         DefaultTimeout,
		queueTimeout:    DefaultQueueTimeout,
		minCallBudget:   DefaultMinCallBudget,
		maxBody:         DefaultMaxBodySize,
		maxQuery:        DefaultMaxQuerySize,
		maxQueue:        DefaultMaxQueue,
		anonymousRoutes: true,
		observer:        NopObserver{},
		inflight:        make(chan struct{}, DefaultMaxInFlight),
		auditThrottle:   newThrottle[rateLimitAuditKey](rateLimitAuditInterval, rateLimitAuditKeys),
	}

	for _, opt := range opts {
		opt(handler)
	}

	return handler
}

// ServeHTTP handles HTTP requests for plugin routes.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pluginID := vars["plugin_id"]

	if pluginID == "" {
		http.Error(w, "plugin ID is required", http.StatusBadRequest)

		return
	}

	// Normalize plugin ID
	pluginID = CompactPluginID(ParsePluginID(pluginID))

	plugin, ok := h.manager.GetPlugin(pluginID)
	if !ok {
		http.NotFound(w, r)

		return
	}

	if !plugin.IsEnabled() {
		http.Error(w, "plugin is disabled", http.StatusServiceUnavailable)

		return
	}

	pluginPath := extractPluginPath(r.URL.Path, pluginID)

	route, pathParams := h.matchRoute(plugin, r.Method, pluginPath)
	if route == nil {
		http.Error(w, "route not found", http.StatusNotFound)

		return
	}

	if !h.admitClient(w, r, plugin, pluginPath) {
		return
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handlePluginRequest(w, r, plugin, pluginPath, pathParams)
	})

	var finalHandler http.Handler = handler

	if route.AdminOnly {
		finalHandler = h.adminMiddleware.Middleware(finalHandler)
	}

	// A route declared without requires_auth is anonymous only while the
	// operator allows anonymous plugin routes.
	if route.RequiresAuth || !h.anonymousRoutes {
		finalHandler = h.authMiddleware.Middleware(finalHandler)
	}

	finalHandler.ServeHTTP(w, r)
}

// admitClient applies the per-client rate limit: authenticated clients are
// keyed by user, anonymous ones by IP. A refused request is answered here
// (429 with Retry-After) and reported as false.
func (h *HTTPHandler) admitClient(
	w http.ResponseWriter,
	r *http.Request,
	plugin *LoadedPlugin,
	pluginPath string,
) bool {
	limiter, client := h.clientLimiter(r)

	allowed, retryAfter := limiter.Allow(client)
	if allowed {
		return true
	}

	h.observe(plugin, HTTPResultRateLimited)

	if h.auditThrottle.admit(rateLimitAuditKey{pluginID: plugin.DBID, client: client}) {
		audit.PluginHTTPRateLimited(r.Context(), h.audit, plugin.DBID, plugin.Info.Id, client, r.Method, pluginPath)
	}

	w.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "too many requests", http.StatusTooManyRequests)

	return false
}

// clientLimiter picks the bucket set and key for the request's client. The
// session, if any, was resolved by the optional auth middleware in front of
// the handler, before the route's own auth requirement is applied.
func (h *HTTPHandler) clientLimiter(r *http.Request) (*ratelimit.KeyedLimiter, string) {
	if session := auth.SessionFromContext(r.Context()); session.IsAuthenticated() {
		return h.userLimiter, "user:" + strconv.FormatUint(uint64(session.User.ID), 10)
	}

	return h.anonLimiter, "ip:" + audit.ClientIP(r, h.clientIPHeader)
}

func (h *HTTPHandler) handlePluginRequest(
	w http.ResponseWriter,
	r *http.Request,
	plugin *LoadedPlugin,
	pluginPath string,
	pathParams map[string]string,
) {
	release, ok := h.acquireInFlight()
	if !ok {
		h.rejectBusy(w, plugin, HTTPResultInFlightFull, "too many plugin requests in flight")

		return
	}
	defer release()

	releaseSlot, ok := h.acquireQueueSlot(plugin)
	if !ok {
		h.rejectBusy(w, plugin, HTTPResultQueueFull, "plugin request queue is full")

		return
	}
	defer releaseSlot()

	if h.maxQuery > 0 && int64(len(r.URL.RawQuery)) > h.maxQuery {
		h.observe(plugin, HTTPResultTooLarge)
		http.Error(w, "query string too long", http.StatusRequestURITooLong)

		return
	}

	protoReq, err := h.buildProtoRequest(w, r, plugin.Info.Id, pluginPath, pathParams)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			h.observe(plugin, HTTPResultTooLarge)
			http.Error(w, errBodyTooLarge.Error(), http.StatusRequestEntityTooLarge)

			return
		}

		slog.Error("failed to build proto request",
			slog.String("plugin_id", plugin.Info.Id),
			slog.String("error", err.Error()),
		)
		h.observe(plugin, HTTPResultError)
		http.Error(w, "failed to process request", http.StatusBadRequest)

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	ctx = WithCallQueueTimeout(ctx, h.queueTimeout)
	ctx = WithCallMinBudget(ctx, h.minCallBudget)

	resp, err := h.callPlugin(ctx, plugin, protoReq)
	if err != nil {
		h.handleCallError(ctx, w, r, plugin, pluginPath, err)

		return
	}

	h.observe(plugin, HTTPResultOK)

	if resp.File != nil {
		// The file is streamed on the request's own context, not the guest
		// call budget: the guest is done, the transfer may take longer.
		h.serveFileRef(w, r, plugin, resp)

		return
	}

	h.writeResponse(w, resp)
}

// handleCallError answers a failed guest call; ctx is the call context, whose
// deadline tells a timeout apart from a plugin error.
func (h *HTTPHandler) handleCallError(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	plugin *LoadedPlugin,
	pluginPath string,
	err error,
) {
	if errors.Is(err, ErrPluginBusy) {
		// The guest was never invoked and the plugin stays enabled: expected
		// under load, so it is not reported as a failure of the plugin.
		slog.Debug("plugin request refused, plugin is busy",
			slog.String("plugin_id", plugin.Info.Id),
			slog.String("path", pluginPath),
			slog.String("error", err.Error()),
		)
		h.rejectBusy(w, plugin, HTTPResultBusy, "plugin is busy")

		return
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		// The runtime closed the module on deadline; stop routing to it.
		plugin.DisableWithReason(DisableReasonHTTPTimeout + " (" + r.Method + " " + pluginPath + ")")

		slog.Error("plugin HTTP handler timed out, plugin disabled until reload",
			slog.String("plugin_id", plugin.Info.Id),
			slog.String("path", pluginPath),
			slog.String("error", err.Error()),
		)
		h.observe(plugin, HTTPResultTimeout)
		http.Error(w, "request timeout", http.StatusGatewayTimeout)

		return
	}

	slog.Error("plugin request failed",
		slog.String("plugin_id", plugin.Info.Id),
		slog.String("path", pluginPath),
		slog.String("error", err.Error()),
	)
	h.observe(plugin, HTTPResultError)
	http.Error(w, "plugin error", http.StatusInternalServerError)
}

// rejectBusy answers a request the handler will not run now: the plugin's
// gate is taken, or a request cap is reached. Nothing was sent to the guest.
func (h *HTTPHandler) rejectBusy(w http.ResponseWriter, plugin *LoadedPlugin, result, message string) {
	h.observe(plugin, result)

	w.Header().Set("Retry-After", retryAfterBusy)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, message, http.StatusServiceUnavailable)
}

// acquireInFlight takes a slot of the per-instance request cap without
// waiting; the returned func releases it.
func (h *HTTPHandler) acquireInFlight() (func(), bool) {
	if h.inflight == nil {
		return func() {}, true
	}

	select {
	case h.inflight <- struct{}{}:
		return func() { <-h.inflight }, true
	default:
		return nil, false
	}
}

// acquireQueueSlot counts the request against the plugin's cap; the returned
// func releases it.
func (h *HTTPHandler) acquireQueueSlot(plugin *LoadedPlugin) (func(), bool) {
	if h.maxQueue <= 0 {
		return func() {}, true
	}

	value, _ := h.queued.LoadOrStore(plugin.Info.Id, new(atomic.Int32))
	counter, _ := value.(*atomic.Int32)

	if counter.Add(1) > h.maxQueue {
		counter.Add(-1)

		return nil, false
	}

	return func() { counter.Add(-1) }, true
}

// QueuedRequests reports the requests of the plugin inside the handler on
// this instance, waiting for the call gate or executing.
func (h *HTTPHandler) QueuedRequests(pluginID string) int {
	value, ok := h.queued.Load(pluginID)
	if !ok {
		return 0
	}

	counter, _ := value.(*atomic.Int32)

	return int(counter.Load())
}

func (h *HTTPHandler) observe(plugin *LoadedPlugin, result string) {
	observerOrNop(h.observer).HTTPRequest(plugin.DBID, result)
}

func retryAfterSeconds(wait time.Duration) string {
	return strconv.Itoa(max(1, int(math.Ceil(wait.Seconds()))))
}

// serveFileRef hands a file response to the FileRefServer. The plugin's
// body is ignored by contract; its status and headers are passed along.
func (h *HTTPHandler) serveFileRef(
	w http.ResponseWriter,
	r *http.Request,
	plugin *LoadedPlugin,
	resp *proto.HTTPResponse,
) {
	if h.fileRefs == nil {
		slog.Error("plugin answered with a file reference but file responses are not enabled",
			slog.String("plugin_id", plugin.Info.Id),
		)
		http.Error(w, "file responses are not enabled", http.StatusNotImplemented)

		return
	}

	err := h.fileRefs.ServeFileRef(w, r, FileRefRequest{
		PluginID:   plugin.DBID,
		PluginName: plugin.Info.Id,
		Ref:        resp.File,
		Headers:    resp.Headers,
		StatusCode: int(resp.StatusCode),
	})
	if err == nil {
		return
	}

	status := fileRefErrorStatus(err)

	slog.Error("plugin file response failed",
		slog.String("plugin_id", plugin.Info.Id),
		slog.Uint64("node_id", resp.File.NodeId),
		slog.String("path", resp.File.Path),
		slog.Int("status", status),
		slog.String("error", err.Error()),
	)

	message := "failed to serve file"
	if status < http.StatusInternalServerError {
		message = err.Error()
	}

	http.Error(w, message, status)
}

// fileRefErrorStatus reads the status an error carries (pkg/api wrapped
// errors, daemon file errors); anything else is an internal failure.
func fileRefErrorStatus(err error) int {
	var withStatus interface{ HTTPStatus() int }
	if errors.As(err, &withStatus) {
		return withStatus.HTTPStatus()
	}

	return http.StatusInternalServerError
}

func extractPluginPath(fullPath, pluginID string) string {
	prefix := "/api/plugins/" + pluginID
	if after, ok := strings.CutPrefix(fullPath, prefix); ok {
		path := after
		if path == "" {
			return "/"
		}

		return path
	}

	return "/"
}

func (h *HTTPHandler) matchRoute(
	plugin *LoadedPlugin,
	method string,
	path string,
) (*proto.HTTPRoute, map[string]string) {
	for _, route := range plugin.HTTPRoutes {
		if !containsMethod(route.Methods, method) {
			continue
		}

		pathParams, ok := matchPath(route.Path, path)
		if ok {
			return route, pathParams
		}
	}

	return nil, nil
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}

	return false
}

func matchPath(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if pattern == "/" && path == "/" {
		return map[string]string{}, true
	}

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	params := make(map[string]string)
	for i, patternPart := range patternParts {
		pathPart := pathParts[i]

		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			paramName := patternPart[1 : len(patternPart)-1]
			params[paramName] = pathPart
		} else if patternPart != pathPart {
			return nil, false
		}
	}

	return params, true
}

func (h *HTTPHandler) buildProtoRequest(
	w http.ResponseWriter,
	r *http.Request,
	pluginID string,
	pluginPath string,
	pathParams map[string]string,
) (*proto.HTTPRequest, error) {
	body, err := h.readBody(w, r)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read request body")
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	queryParams := make(map[string]*proto.QueryParamValues)
	for key, values := range r.URL.Query() {
		expandedValues := expandQueryValues(values)
		queryParams[key] = &proto.QueryParamValues{
			Values: expandedValues,
		}
	}

	session := h.buildProtoSession(r.Context())

	pluginContext := &proto.PluginContext{
		PluginId:  pluginID,
		RequestId: r.Header.Get("X-Request-ID"),
	}
	if session != nil {
		pluginContext.UserId = new(session.User.Id)
	}

	return &proto.HTTPRequest{
		Context:     pluginContext,
		Method:      r.Method,
		Path:        pluginPath,
		Headers:     headers,
		PathParams:  pathParams,
		QueryParams: queryParams,
		Body:        body,
		Session:     session,
	}, nil
}

// readBody reads at most maxBody bytes. http.MaxBytesReader stops reading
// at the cap and tells the server to close the connection afterwards, so an
// oversized upload is not drained into memory before it is refused.
func (h *HTTPHandler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, errBodyTooLarge
		}

		return nil, errors.Wrap(err, "failed to read body")
	}

	return body, nil
}

func (h *HTTPHandler) buildProtoSession(ctx context.Context) *proto.Session {
	authSession := auth.SessionFromContext(ctx)
	if authSession == nil || !authSession.IsAuthenticated() {
		return nil
	}

	protoSession := &proto.Session{
		Id: authSession.ID,
		User: &gameapProto.User{
			Id:    uint64(authSession.User.ID),
			Login: authSession.User.Login,
			Email: authSession.User.Email,
			Name:  authSession.User.Name,
		},
	}

	if authSession.User.CreatedAt != nil {
		protoSession.User.CreatedAt = new(authSession.User.CreatedAt.Unix())
	}
	if authSession.User.UpdatedAt != nil {
		protoSession.User.UpdatedAt = new(authSession.User.UpdatedAt.Unix())
	}

	if authSession.IsTokenSession() {
		protoSession.Token = buildProtoToken(authSession.Token)
	}

	return protoSession
}

func buildProtoToken(token *domain.PersonalAccessToken) *gameapProto.PersonalAccessToken {
	if token == nil {
		return nil
	}

	protoToken := &gameapProto.PersonalAccessToken{
		Id:          uint64(token.ID),
		TokenableId: uint64(token.TokenableID),
		Name:        token.Name,
	}

	protoToken.TokenableType = domainEntityTypeToProto(token.TokenableType)

	if token.Abilities != nil {
		abilities := make([]string, 0, len(*token.Abilities))
		for _, ability := range *token.Abilities {
			abilities = append(abilities, string(ability))
		}
		protoToken.Abilities = abilities
	}

	if token.LastUsedAt != nil {
		protoToken.LastUsedAt = new(token.LastUsedAt.Unix())
	}
	if token.CreatedAt != nil {
		protoToken.CreatedAt = new(token.CreatedAt.Unix())
	}

	return protoToken
}

func (h *HTTPHandler) callPlugin(
	ctx context.Context,
	plugin *LoadedPlugin,
	req *proto.HTTPRequest,
) (*proto.HTTPResponse, error) {
	resp, err := plugin.Instance.HandleHTTPRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "plugin HandleHTTPRequest failed")
	}

	return resp, nil
}

func (h *HTTPHandler) writeResponse(w http.ResponseWriter, resp *proto.HTTPResponse) {
	for key, value := range resp.Headers {
		w.Header().Set(key, value)
	}

	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	statusCode := int(resp.StatusCode)
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)

	if len(resp.Body) > 0 {
		//nolint:gosec // G705: resp.Body is from trusted plugin, Content-Type is set
		_, err := w.Write(resp.Body)
		if err != nil {
			slog.Error("failed to write response body",
				slog.String("error", err.Error()),
			)
		}
	}
}

func expandQueryValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.Contains(value, ",") {
			parts := strings.Split(value, ",")
			result = append(result, parts...)
		} else {
			result = append(result, value)
		}
	}

	return result
}

func domainEntityTypeToProto(entityType domain.EntityType) gameapProto.EntityType {
	switch entityType {
	case domain.EntityTypeUser:
		return gameapProto.EntityType_ENTITY_TYPE_USER
	case domain.EntityTypeNode:
		return gameapProto.EntityType_ENTITY_TYPE_NODE
	case domain.EntityTypeClientCertificate:
		return gameapProto.EntityType_ENTITY_TYPE_CLIENT_CERTIFICATE
	case domain.EntityTypeGame:
		return gameapProto.EntityType_ENTITY_TYPE_GAME
	case domain.EntityTypeGameMod:
		return gameapProto.EntityType_ENTITY_TYPE_GAME_MOD
	case domain.EntityTypeServer:
		return gameapProto.EntityType_ENTITY_TYPE_SERVER
	case domain.EntityTypeRole:
		return gameapProto.EntityType_ENTITY_TYPE_ROLE
	default:
		return gameapProto.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}
