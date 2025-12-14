package plugin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin/proto"
	gameapProto "github.com/gameap/gameap/pkg/proto"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

const (
	DefaultTimeout     = 30 * time.Second
	DefaultMaxBodySize = 1 << 20 // 1MB
)

// AuthMiddleware interface for authentication middleware.
type AuthMiddleware interface {
	Middleware(next http.Handler) http.Handler
}

// AdminMiddleware interface for admin check middleware.
type AdminMiddleware interface {
	Middleware(next http.Handler) http.Handler
}

// HTTPHandler handles HTTP requests for plugins.
type HTTPHandler struct {
	manager         *Manager
	authMiddleware  AuthMiddleware
	adminMiddleware AdminMiddleware
	timeout         time.Duration
	maxBody         int64
}

// NewHTTPHandler creates a new HTTP handler for plugin routes.
func NewHTTPHandler(
	manager *Manager,
	authMiddleware AuthMiddleware,
	adminMiddleware AdminMiddleware,
) *HTTPHandler {
	return &HTTPHandler{
		manager:         manager,
		authMiddleware:  authMiddleware,
		adminMiddleware: adminMiddleware,
		timeout:         DefaultTimeout,
		maxBody:         DefaultMaxBodySize,
	}
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

	if !plugin.Enabled {
		http.Error(w, "plugin is disabled", http.StatusServiceUnavailable)
		return
	}

	pluginPath := extractPluginPath(r.URL.Path, pluginID)

	route, pathParams := h.matchRoute(plugin, r.Method, pluginPath)
	if route == nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handlePluginRequest(w, r, plugin, pluginPath, pathParams)
	})

	var finalHandler http.Handler = handler

	if route.AdminOnly {
		finalHandler = h.adminMiddleware.Middleware(finalHandler)
	}

	if route.RequiresAuth {
		finalHandler = h.authMiddleware.Middleware(finalHandler)
	}

	finalHandler.ServeHTTP(w, r)
}

func (h *HTTPHandler) handlePluginRequest(
	w http.ResponseWriter,
	r *http.Request,
	plugin *LoadedPlugin,
	pluginPath string,
	pathParams map[string]string,
) {
	ctx := r.Context()

	protoReq, err := h.buildProtoRequest(r, plugin.Info.Id, pluginPath, pathParams)
	if err != nil {
		slog.Error("failed to build proto request",
			slog.String("plugin_id", plugin.Info.Id),
			slog.String("error", err.Error()),
		)
		http.Error(w, "failed to process request", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	resp, err := h.callPlugin(ctx, plugin, protoReq)
	if err != nil {
		slog.Error("plugin request failed",
			slog.String("plugin_id", plugin.Info.Id),
			slog.String("path", pluginPath),
			slog.String("error", err.Error()),
		)

		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "request timeout", http.StatusGatewayTimeout)
			return
		}

		http.Error(w, "plugin error", http.StatusInternalServerError)
		return
	}

	h.writeResponse(w, resp)
}

func extractPluginPath(fullPath, pluginID string) string {
	prefix := "/api/plugins/" + pluginID
	if strings.HasPrefix(fullPath, prefix) {
		path := strings.TrimPrefix(fullPath, prefix)
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
	r *http.Request,
	pluginID string,
	pluginPath string,
	pathParams map[string]string,
) (*proto.HTTPRequest, error) {
	body, err := h.readBody(r)
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

	return &proto.HTTPRequest{
		Context: &proto.PluginContext{
			PluginId:  pluginID,
			RequestId: r.Header.Get("X-Request-ID"),
		},
		Method:      r.Method,
		Path:        pluginPath,
		Headers:     headers,
		PathParams:  pathParams,
		QueryParams: queryParams,
		Body:        body,
		Session:     session,
	}, nil
}

func (h *HTTPHandler) readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	limitedReader := io.LimitReader(r.Body, h.maxBody+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read body")
	}

	if int64(len(body)) > h.maxBody {
		return nil, errors.New("request body too large")
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
		protoSession.User.CreatedAt = lo.ToPtr(authSession.User.CreatedAt.Unix())
	}
	if authSession.User.UpdatedAt != nil {
		protoSession.User.UpdatedAt = lo.ToPtr(authSession.User.UpdatedAt.Unix())
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
		protoToken.LastUsedAt = lo.ToPtr(token.LastUsedAt.Unix())
	}
	if token.CreatedAt != nil {
		protoToken.CreatedAt = lo.ToPtr(token.CreatedAt.Unix())
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

// pathParamRegex matches path parameters like {id} in route patterns.
var pathParamRegex = regexp.MustCompile(`\{([^}]+)\}`)
