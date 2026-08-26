package plugin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin/proto"
	gameapProto "github.com/gameap/gameap/pkg/proto"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPluginServiceHTTP struct {
	mockPluginService

	handleHTTPRequestFunc func(ctx context.Context, req *proto.HTTPRequest) (*proto.HTTPResponse, error)
}

func (m *mockPluginServiceHTTP) HandleHTTPRequest(
	ctx context.Context,
	req *proto.HTTPRequest,
) (*proto.HTTPResponse, error) {
	if m.handleHTTPRequestFunc != nil {
		return m.handleHTTPRequestFunc(ctx, req)
	}

	return &proto.HTTPResponse{StatusCode: 200}, nil
}

type mockMiddleware struct {
	called bool
}

func (m *mockMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.called = true
		next.ServeHTTP(w, r)
	})
}

func TestNewHTTPHandler(t *testing.T) {
	t.Parallel()
	t.Run("creates_handler_with_default_values", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := NewManager(ManagerConfig{})
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}

		// ACT
		handler := NewHTTPHandler(manager, authMw, adminMw)

		// ASSERT
		require.NotNil(t, handler)
		assert.Equal(t, manager, handler.manager)
		assert.Equal(t, authMw, handler.authMiddleware)
		assert.Equal(t, adminMw, handler.adminMiddleware)
		assert.Equal(t, DefaultTimeout, handler.timeout)
		assert.Equal(t, int64(DefaultMaxBodySize), handler.maxBody)
	})
}

func TestHTTPHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	t.Run("missing_plugin_id", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := NewManager(ManagerConfig{})
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": ""})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "plugin ID is required")
	})

	t.Run("plugin_not_found", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := NewManager(ManagerConfig{})
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/nonexistent/test", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": "nonexistent"})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("plugin_disabled", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := NewManager(ManagerConfig{})
		pluginID := CompactPluginID(ParsePluginID("disabled-plugin"))
		manager.plugins[pluginID] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pluginID},
			Enabled: false,
		}
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/disabled-plugin/test", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": "disabled-plugin"})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.Contains(t, rr.Body.String(), "plugin is disabled")
	})

	t.Run("route_not_found", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := NewManager(ManagerConfig{})
		pluginID := CompactPluginID(ParsePluginID("test-plugin"))
		manager.plugins[pluginID] = &LoadedPlugin{
			Info:       &proto.PluginInfo{Id: pluginID},
			Enabled:    true,
			HTTPRoutes: []*proto.HTTPRoute{},
			Instance:   &mockPluginServiceHTTP{},
		}
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/test-plugin/nonexistent", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": "test-plugin"})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "route not found")
	})

	t.Run("successful_request", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := NewManager(ManagerConfig{})
		pluginID := CompactPluginID(ParsePluginID("test-plugin"))
		manager.plugins[pluginID] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pluginID},
			Enabled: true,
			HTTPRoutes: []*proto.HTTPRoute{
				{Path: "/test", Methods: []string{"GET"}},
			},
			Instance: &mockPluginServiceHTTP{
				handleHTTPRequestFunc: func(_ context.Context, _ *proto.HTTPRequest) (*proto.HTTPResponse, error) {
					return &proto.HTTPResponse{
						StatusCode: 200,
						Body:       []byte(`{"success":true}`),
						Headers:    map[string]string{"Content-Type": "application/json"},
					}, nil
				},
			},
		}
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/"+pluginID+"/test", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": pluginID})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `{"success":true}`)
	})

	t.Run("auth_middleware_applied", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := NewManager(ManagerConfig{})
		pluginID := CompactPluginID(ParsePluginID("auth-plugin"))
		manager.plugins[pluginID] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pluginID},
			Enabled: true,
			HTTPRoutes: []*proto.HTTPRoute{
				{Path: "/secure", Methods: []string{"GET"}, RequiresAuth: true},
			},
			Instance: &mockPluginServiceHTTP{},
		}
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/"+pluginID+"/secure", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": pluginID})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, authMw.called, "auth middleware should be called")
	})

	t.Run("admin_middleware_applied", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := NewManager(ManagerConfig{})
		pluginID := CompactPluginID(ParsePluginID("admin-plugin"))
		manager.plugins[pluginID] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pluginID},
			Enabled: true,
			HTTPRoutes: []*proto.HTTPRoute{
				{Path: "/admin", Methods: []string{"GET"}, AdminOnly: true},
			},
			Instance: &mockPluginServiceHTTP{},
		}
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/"+pluginID+"/admin", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": pluginID})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, adminMw.called, "admin middleware should be called")
	})

	t.Run("both_auth_and_admin_middleware_applied", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := NewManager(ManagerConfig{})
		pluginID := CompactPluginID(ParsePluginID("secure-admin-plugin"))
		manager.plugins[pluginID] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pluginID},
			Enabled: true,
			HTTPRoutes: []*proto.HTTPRoute{
				{Path: "/secure-admin", Methods: []string{"GET"}, RequiresAuth: true, AdminOnly: true},
			},
			Instance: &mockPluginServiceHTTP{},
		}
		authMw := &mockMiddleware{}
		adminMw := &mockMiddleware{}
		handler := NewHTTPHandler(manager, authMw, adminMw)

		req := httptest.NewRequest(http.MethodGet, "/api/plugins/"+pluginID+"/secure-admin", nil)
		req = mux.SetURLVars(req, map[string]string{"plugin_id": pluginID})
		rr := httptest.NewRecorder()

		// ACT
		handler.ServeHTTP(rr, req)

		// ASSERT
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, authMw.called, "auth middleware should be called")
		assert.True(t, adminMw.called, "admin middleware should be called")
	})
}

func TestExtractPluginPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fullPath string
		pluginID string
		want     string
	}{
		{
			name:     "full_path_with_route",
			fullPath: "/api/plugins/my-plugin/users",
			pluginID: "my-plugin",
			want:     "/users",
		},
		{
			name:     "root_path",
			fullPath: "/api/plugins/my-plugin",
			pluginID: "my-plugin",
			want:     "/",
		},
		{
			name:     "nested_path",
			fullPath: "/api/plugins/my-plugin/api/v1/data",
			pluginID: "my-plugin",
			want:     "/api/v1/data",
		},
		{
			name:     "path_without_prefix",
			fullPath: "/other/path",
			pluginID: "my-plugin",
			want:     "/",
		},
		{
			name:     "trailing_slash_on_root",
			fullPath: "/api/plugins/my-plugin/",
			pluginID: "my-plugin",
			want:     "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := extractPluginPath(tt.fullPath, tt.pluginID)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsMethod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		methods []string
		method  string
		want    bool
	}{
		{
			name:    "exact_match",
			methods: []string{"GET", "POST"},
			method:  "GET",
			want:    true,
		},
		{
			name:    "case_insensitive",
			methods: []string{"GET", "POST"},
			method:  "get",
			want:    true,
		},
		{
			name:    "not_found",
			methods: []string{"GET", "POST"},
			method:  "DELETE",
			want:    false,
		},
		{
			name:    "empty_slice",
			methods: []string{},
			method:  "GET",
			want:    false,
		},
		{
			name:    "nil_slice",
			methods: nil,
			method:  "GET",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := containsMethod(tt.methods, tt.method)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		pattern    string
		path       string
		wantMatch  bool
		wantParams map[string]string
	}{
		{
			name:       "exact_root_match",
			pattern:    "/",
			path:       "/",
			wantMatch:  true,
			wantParams: map[string]string{},
		},
		{
			name:       "exact_path_match",
			pattern:    "/users",
			path:       "/users",
			wantMatch:  true,
			wantParams: map[string]string{},
		},
		{
			name:      "param_extraction",
			pattern:   "/users/{id}",
			path:      "/users/123",
			wantMatch: true,
			wantParams: map[string]string{
				"id": "123",
			},
		},
		{
			name:      "multiple_params",
			pattern:   "/users/{userId}/posts/{postId}",
			path:      "/users/1/posts/42",
			wantMatch: true,
			wantParams: map[string]string{
				"userId": "1",
				"postId": "42",
			},
		},
		{
			name:      "length_mismatch",
			pattern:   "/users/{id}",
			path:      "/users/123/extra",
			wantMatch: false,
		},
		{
			name:      "segment_mismatch",
			pattern:   "/users",
			path:      "/posts",
			wantMatch: false,
		},
		{
			name:       "nested_path_match",
			pattern:    "/api/v1/users",
			path:       "/api/v1/users",
			wantMatch:  true,
			wantParams: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			params, match := matchPath(tt.pattern, tt.path)

			// ASSERT
			assert.Equal(t, tt.wantMatch, match)
			if tt.wantMatch {
				assert.Equal(t, tt.wantParams, params)
			}
		})
	}
}

func TestReadBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		body      io.Reader
		maxBody   int64
		wantBody  []byte
		wantError string
	}{
		{
			name:     "nil_body",
			body:     nil,
			maxBody:  1024,
			wantBody: nil,
		},
		{
			name:     "empty_body",
			body:     strings.NewReader(""),
			maxBody:  1024,
			wantBody: []byte{},
		},
		{
			name:     "normal_body",
			body:     strings.NewReader("test body content"),
			maxBody:  1024,
			wantBody: []byte("test body content"),
		},
		{
			name:     "body_at_max_size",
			body:     strings.NewReader("12345"),
			maxBody:  5,
			wantBody: []byte("12345"),
		},
		{
			name:      "body_exceeds_max_size",
			body:      strings.NewReader("123456"),
			maxBody:   5,
			wantError: "request body too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := &HTTPHandler{maxBody: tt.maxBody}
			var req *http.Request
			if tt.body == nil {
				req = httptest.NewRequest(http.MethodPost, "/test", nil)
				req.Body = nil
			} else {
				req = httptest.NewRequest(http.MethodPost, "/test", tt.body)
			}

			// ACT
			body, err := handler.readBody(req)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantBody, body)
			}
		})
	}
}

func TestBuildProtoRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupReq    func() *http.Request
		pluginID    string
		pluginPath  string
		pathParams  map[string]string
		maxBody     int64
		checkResult func(*testing.T, *proto.HTTPRequest)
		wantError   string
	}{
		{
			name: "basic_request",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/plugins/test/users", nil)
				req.Header.Set("Accept", "application/json")

				return req
			},
			pluginID:   "test-plugin",
			pluginPath: "/users",
			pathParams: map[string]string{},
			maxBody:    DefaultMaxBodySize,
			checkResult: func(t *testing.T, req *proto.HTTPRequest) {
				t.Helper()
				assert.Equal(t, http.MethodGet, req.Method)
				assert.Equal(t, "/users", req.Path)
				assert.Equal(t, "test-plugin", req.Context.PluginId)
				assert.Equal(t, "application/json", req.Headers["Accept"])
			},
		},
		{
			name: "with_query_params",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/plugins/test/users?page=1&limit=10", nil)
			},
			pluginID:   "test-plugin",
			pluginPath: "/users",
			pathParams: map[string]string{},
			maxBody:    DefaultMaxBodySize,
			checkResult: func(t *testing.T, req *proto.HTTPRequest) {
				t.Helper()
				require.Contains(t, req.QueryParams, "page")
				require.Contains(t, req.QueryParams, "limit")
				require.Len(t, req.QueryParams["page"].Values, 1)
				assert.Equal(t, "1", req.QueryParams["page"].Values[0])
				assert.Equal(t, "10", req.QueryParams["limit"].Values[0])
			},
		},
		{
			name: "comma_separated_query",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/plugins/test/users?ids=1,2,3", nil)
			},
			pluginID:   "test-plugin",
			pluginPath: "/users",
			pathParams: map[string]string{},
			maxBody:    DefaultMaxBodySize,
			checkResult: func(t *testing.T, req *proto.HTTPRequest) {
				t.Helper()
				require.Contains(t, req.QueryParams, "ids")
				require.Len(t, req.QueryParams["ids"].Values, 3)
				assert.Equal(t, []string{"1", "2", "3"}, req.QueryParams["ids"].Values)
			},
		},
		{
			name: "with_path_params",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/plugins/test/users/123", nil)
			},
			pluginID:   "test-plugin",
			pluginPath: "/users/123",
			pathParams: map[string]string{"id": "123"},
			maxBody:    DefaultMaxBodySize,
			checkResult: func(t *testing.T, req *proto.HTTPRequest) {
				t.Helper()
				require.Contains(t, req.PathParams, "id")
				assert.Equal(t, "123", req.PathParams["id"])
			},
		},
		{
			name: "with_request_id_header",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/plugins/test/users", nil)
				req.Header.Set("X-Request-ID", "req-123")

				return req
			},
			pluginID:   "test-plugin",
			pluginPath: "/users",
			pathParams: map[string]string{},
			maxBody:    DefaultMaxBodySize,
			checkResult: func(t *testing.T, req *proto.HTTPRequest) {
				t.Helper()
				assert.Equal(t, "req-123", req.Context.RequestId)
			},
		},
		{
			name: "with_authenticated_session",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/plugins/test/users", nil)
				now := time.Now()
				session := &auth.Session{
					ID: "session-123",
					User: &domain.User{
						ID:        1,
						Login:     "testuser",
						Email:     "test@example.com",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
				}
				ctx := auth.ContextWithSession(req.Context(), session)

				return req.WithContext(ctx)
			},
			pluginID:   "test-plugin",
			pluginPath: "/users",
			pathParams: map[string]string{},
			maxBody:    DefaultMaxBodySize,
			checkResult: func(t *testing.T, req *proto.HTTPRequest) {
				t.Helper()
				require.NotNil(t, req.Session)
				assert.Equal(t, "session-123", req.Session.Id)
				require.NotNil(t, req.Session.User)
				assert.Equal(t, uint64(1), req.Session.User.Id)
				assert.Equal(t, "testuser", req.Session.User.Login)
				assert.Equal(t, "test@example.com", req.Session.User.Email)
			},
		},
		{
			name: "body_too_large",
			setupReq: func() *http.Request {
				body := strings.Repeat("x", 100)

				return httptest.NewRequest(http.MethodPost, "/api/plugins/test/data", strings.NewReader(body))
			},
			pluginID:   "test-plugin",
			pluginPath: "/data",
			pathParams: map[string]string{},
			maxBody:    10,
			wantError:  "request body too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := &HTTPHandler{maxBody: tt.maxBody}
			req := tt.setupReq()

			// ACT
			protoReq, err := handler.buildProtoRequest(req, tt.pluginID, tt.pluginPath, tt.pathParams)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
				require.NotNil(t, protoReq)
				tt.checkResult(t, protoReq)
			}
		})
	}
}

func TestBuildProtoSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupCtx    func() context.Context
		checkResult func(*testing.T, *proto.Session)
	}{
		{
			name:     "nil_session",
			setupCtx: context.Background,
			checkResult: func(t *testing.T, session *proto.Session) {
				t.Helper()
				assert.Nil(t, session)
			},
		},
		{
			name: "unauthenticated_session",
			setupCtx: func() context.Context {
				session := &auth.Session{
					ID:   "session-123",
					User: &domain.User{ID: 0},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			checkResult: func(t *testing.T, session *proto.Session) {
				t.Helper()
				assert.Nil(t, session)
			},
		},
		{
			name: "authenticated_user_session",
			setupCtx: func() context.Context {
				name := "Test User"
				session := &auth.Session{
					ID: "session-123",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
						Name:  &name,
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			checkResult: func(t *testing.T, session *proto.Session) {
				t.Helper()
				require.NotNil(t, session)
				assert.Equal(t, "session-123", session.Id)
				require.NotNil(t, session.User)
				assert.Equal(t, uint64(1), session.User.Id)
				assert.Equal(t, "testuser", session.User.Login)
				assert.Equal(t, "test@example.com", session.User.Email)
				assert.Equal(t, "Test User", session.User.GetName())
			},
		},
		{
			name: "token_session",
			setupCtx: func() context.Context {
				abilities := []domain.PATAbility{"server:list", "server:start"}
				session := &auth.Session{
					ID: "session-123",
					User: &domain.User{
						ID:    1,
						Login: "testuser",
						Email: "test@example.com",
					},
					Token: &domain.PersonalAccessToken{
						ID:          10,
						TokenableID: 1,
						Name:        "test-token",
						Abilities:   &abilities,
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			checkResult: func(t *testing.T, session *proto.Session) {
				t.Helper()
				require.NotNil(t, session)
				require.NotNil(t, session.Token)
				assert.Equal(t, uint64(10), session.Token.Id)
				assert.Equal(t, uint64(1), session.Token.TokenableId)
				assert.Equal(t, "test-token", session.Token.Name)
				require.Len(t, session.Token.Abilities, 2)
				assert.Contains(t, session.Token.Abilities, "server:list")
				assert.Contains(t, session.Token.Abilities, "server:start")
			},
		},
		{
			name: "user_with_timestamps",
			setupCtx: func() context.Context {
				now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
				session := &auth.Session{
					ID: "session-123",
					User: &domain.User{
						ID:        1,
						Login:     "testuser",
						Email:     "test@example.com",
						CreatedAt: &now,
						UpdatedAt: &now,
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			checkResult: func(t *testing.T, session *proto.Session) {
				t.Helper()
				require.NotNil(t, session)
				require.NotNil(t, session.User)
				expectedUnix := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
				assert.Equal(t, expectedUnix, session.User.GetCreatedAt())
				assert.Equal(t, expectedUnix, session.User.GetUpdatedAt())
			},
		},
		{
			name: "user_without_timestamps",
			setupCtx: func() context.Context {
				session := &auth.Session{
					ID: "session-123",
					User: &domain.User{
						ID:        1,
						Login:     "testuser",
						Email:     "test@example.com",
						CreatedAt: nil,
						UpdatedAt: nil,
					},
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			checkResult: func(t *testing.T, session *proto.Session) {
				t.Helper()
				require.NotNil(t, session)
				require.NotNil(t, session.User)
				assert.Nil(t, session.User.CreatedAt)
				assert.Nil(t, session.User.UpdatedAt)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := &HTTPHandler{}
			ctx := tt.setupCtx()

			// ACT
			session := handler.buildProtoSession(ctx)

			// ASSERT
			tt.checkResult(t, session)
		})
	}
}

func TestWriteResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		response            *proto.HTTPResponse
		expectedStatus      int
		expectedContentType string
		expectedBody        string
		expectedHeaders     map[string]string
	}{
		{
			name: "sets_headers",
			response: &proto.HTTPResponse{
				StatusCode: 200,
				Headers:    map[string]string{"X-Custom": "value"},
			},
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/json",
			expectedHeaders:     map[string]string{"X-Custom": "value"},
		},
		{
			name: "default_content_type",
			response: &proto.HTTPResponse{
				StatusCode: 200,
				Headers:    map[string]string{},
			},
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/json",
		},
		{
			name: "custom_content_type",
			response: &proto.HTTPResponse{
				StatusCode: 200,
				Headers:    map[string]string{"Content-Type": "text/plain"},
			},
			expectedStatus:      http.StatusOK,
			expectedContentType: "text/plain",
		},
		{
			name: "default_status_code",
			response: &proto.HTTPResponse{
				StatusCode: 0,
			},
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/json",
		},
		{
			name: "custom_status_code",
			response: &proto.HTTPResponse{
				StatusCode: 201,
			},
			expectedStatus:      http.StatusCreated,
			expectedContentType: "application/json",
		},
		{
			name: "writes_body",
			response: &proto.HTTPResponse{
				StatusCode: 200,
				Body:       []byte(`{"data":"test"}`),
			},
			expectedStatus:      http.StatusOK,
			expectedContentType: "application/json",
			expectedBody:        `{"data":"test"}`,
		},
		{
			name: "empty_body",
			response: &proto.HTTPResponse{
				StatusCode: 204,
				Body:       []byte{},
			},
			expectedStatus:      http.StatusNoContent,
			expectedContentType: "application/json",
			expectedBody:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := &HTTPHandler{}
			rr := httptest.NewRecorder()

			// ACT
			handler.writeResponse(rr, tt.response)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, rr.Code)
			assert.Equal(t, tt.expectedContentType, rr.Header().Get("Content-Type"))
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, rr.Body.String())
			}
			for key, value := range tt.expectedHeaders {
				assert.Equal(t, value, rr.Header().Get(key))
			}
		})
	}
}

func TestExpandQueryValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{
			name:   "single_value",
			values: []string{"a"},
			want:   []string{"a"},
		},
		{
			name:   "comma_separated",
			values: []string{"a,b,c"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "mixed",
			values: []string{"a", "b,c"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "empty",
			values: []string{},
			want:   []string{},
		},
		{
			name:   "nil_input",
			values: nil,
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := expandQueryValues(tt.values)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainEntityTypeToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		entityType domain.EntityType
		want       gameapProto.EntityType
	}{
		{
			name:       "user_type",
			entityType: domain.EntityTypeUser,
			want:       gameapProto.EntityType_ENTITY_TYPE_USER,
		},
		{
			name:       "node_type",
			entityType: domain.EntityTypeNode,
			want:       gameapProto.EntityType_ENTITY_TYPE_NODE,
		},
		{
			name:       "client_certificate_type",
			entityType: domain.EntityTypeClientCertificate,
			want:       gameapProto.EntityType_ENTITY_TYPE_CLIENT_CERTIFICATE,
		},
		{
			name:       "game_type",
			entityType: domain.EntityTypeGame,
			want:       gameapProto.EntityType_ENTITY_TYPE_GAME,
		},
		{
			name:       "game_mod_type",
			entityType: domain.EntityTypeGameMod,
			want:       gameapProto.EntityType_ENTITY_TYPE_GAME_MOD,
		},
		{
			name:       "server_type",
			entityType: domain.EntityTypeServer,
			want:       gameapProto.EntityType_ENTITY_TYPE_SERVER,
		},
		{
			name:       "role_type",
			entityType: domain.EntityTypeRole,
			want:       gameapProto.EntityType_ENTITY_TYPE_ROLE,
		},
		{
			name:       "unknown_type",
			entityType: domain.EntityType("unknown"),
			want:       gameapProto.EntityType_ENTITY_TYPE_UNSPECIFIED,
		},
		{
			name:       "empty_type",
			entityType: domain.EntityTypeEmpty,
			want:       gameapProto.EntityType_ENTITY_TYPE_UNSPECIFIED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := domainEntityTypeToProto(tt.entityType)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildProtoToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		token       *domain.PersonalAccessToken
		checkResult func(*testing.T, *gameapProto.PersonalAccessToken)
	}{
		{
			name:  "nil_token",
			token: nil,
			checkResult: func(t *testing.T, token *gameapProto.PersonalAccessToken) {
				t.Helper()
				assert.Nil(t, token)
			},
		},
		{
			name: "basic_token",
			token: &domain.PersonalAccessToken{
				ID:          1,
				TokenableID: 10,
				Name:        "test-token",
			},
			checkResult: func(t *testing.T, token *gameapProto.PersonalAccessToken) {
				t.Helper()
				require.NotNil(t, token)
				assert.Equal(t, uint64(1), token.Id)
				assert.Equal(t, uint64(10), token.TokenableId)
				assert.Equal(t, "test-token", token.Name)
			},
		},
		{
			name: "with_abilities",
			token: &domain.PersonalAccessToken{
				ID:        1,
				Name:      "test-token",
				Abilities: new([]domain.PATAbility{"server:list", "server:start"}),
			},
			checkResult: func(t *testing.T, token *gameapProto.PersonalAccessToken) {
				t.Helper()
				require.NotNil(t, token)
				require.Len(t, token.Abilities, 2)
				assert.Contains(t, token.Abilities, "server:list")
				assert.Contains(t, token.Abilities, "server:start")
			},
		},
		{
			name: "with_timestamps",
			token: &domain.PersonalAccessToken{
				ID:         1,
				Name:       "test-token",
				LastUsedAt: new(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)),
				CreatedAt:  new(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
			checkResult: func(t *testing.T, token *gameapProto.PersonalAccessToken) {
				t.Helper()
				require.NotNil(t, token)
				assert.Equal(t, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix(), token.GetLastUsedAt())
				assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), token.GetCreatedAt())
			},
		},
		{
			name: "without_timestamps",
			token: &domain.PersonalAccessToken{
				ID:         1,
				Name:       "test-token",
				LastUsedAt: nil,
				CreatedAt:  nil,
			},
			checkResult: func(t *testing.T, token *gameapProto.PersonalAccessToken) {
				t.Helper()
				require.NotNil(t, token)
				assert.Nil(t, token.LastUsedAt)
				assert.Nil(t, token.CreatedAt)
			},
		},
		{
			name: "with_tokenable_type",
			token: &domain.PersonalAccessToken{
				ID:            1,
				Name:          "test-token",
				TokenableType: domain.EntityTypeUser,
			},
			checkResult: func(t *testing.T, token *gameapProto.PersonalAccessToken) {
				t.Helper()
				require.NotNil(t, token)
				assert.Equal(t, gameapProto.EntityType_ENTITY_TYPE_USER, token.TokenableType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := buildProtoToken(tt.token)

			// ASSERT
			tt.checkResult(t, got)
		})
	}
}

func TestHandlePluginRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setupPlugin    func() *LoadedPlugin
		requestBody    string
		maxBody        int64
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful_request",
			setupPlugin: func() *LoadedPlugin {
				return &LoadedPlugin{
					Info: &proto.PluginInfo{Id: "test-plugin"},
					Instance: &mockPluginServiceHTTP{
						handleHTTPRequestFunc: func(_ context.Context, _ *proto.HTTPRequest) (*proto.HTTPResponse, error) {
							return &proto.HTTPResponse{
								StatusCode: 200,
								Body:       []byte(`{"result":"success"}`),
							}, nil
						},
					},
				}
			},
			maxBody:        DefaultMaxBodySize,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"result":"success"}`,
		},
		{
			name: "build_request_error",
			setupPlugin: func() *LoadedPlugin {
				return &LoadedPlugin{
					Info:     &proto.PluginInfo{Id: "test-plugin"},
					Instance: &mockPluginServiceHTTP{},
				}
			},
			requestBody:    strings.Repeat("x", 100),
			maxBody:        10,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "failed to process request",
		},
		{
			name: "plugin_error",
			setupPlugin: func() *LoadedPlugin {
				return &LoadedPlugin{
					Info: &proto.PluginInfo{Id: "test-plugin"},
					Instance: &mockPluginServiceHTTP{
						handleHTTPRequestFunc: func(_ context.Context, _ *proto.HTTPRequest) (*proto.HTTPResponse, error) {
							return nil, errors.New("plugin internal error")
						},
					},
				}
			},
			maxBody:        DefaultMaxBodySize,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "plugin error",
		},
		{
			name: "timeout_error",
			setupPlugin: func() *LoadedPlugin {
				return &LoadedPlugin{
					Info: &proto.PluginInfo{Id: "test-plugin"},
					Instance: &mockPluginServiceHTTP{
						handleHTTPRequestFunc: func(_ context.Context, _ *proto.HTTPRequest) (*proto.HTTPResponse, error) {
							return nil, context.DeadlineExceeded
						},
					},
				}
			},
			maxBody:        DefaultMaxBodySize,
			expectedStatus: http.StatusGatewayTimeout,
			expectedBody:   "request timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := &HTTPHandler{
				maxBody: tt.maxBody,
				timeout: 30 * time.Second,
			}
			plugin := tt.setupPlugin()

			var body io.Reader
			if tt.requestBody != "" {
				body = bytes.NewBufferString(tt.requestBody)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/plugins/test-plugin/data", body)
			rr := httptest.NewRecorder()

			// ACT
			handler.handlePluginRequest(rr, req, plugin, "/data", map[string]string{})

			// ASSERT
			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}

			if tt.expectedStatus == http.StatusGatewayTimeout {
				assert.False(t, plugin.IsEnabled())
				reason, ok := plugin.DisabledReason()
				require.True(t, ok)
				assert.Equal(t, "http handler timed out (POST /data)", reason)
			} else {
				_, ok := plugin.DisabledReason()
				assert.False(t, ok, "only a timeout disables the plugin")
			}
		})
	}
}

func TestMatchRoute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		plugin     *LoadedPlugin
		method     string
		path       string
		wantRoute  bool
		wantParams map[string]string
	}{
		{
			name: "exact_match",
			plugin: &LoadedPlugin{
				HTTPRoutes: []*proto.HTTPRoute{
					{Path: "/users", Methods: []string{"GET", "POST"}},
				},
			},
			method:     "GET",
			path:       "/users",
			wantRoute:  true,
			wantParams: map[string]string{},
		},
		{
			name: "method_case_insensitive",
			plugin: &LoadedPlugin{
				HTTPRoutes: []*proto.HTTPRoute{
					{Path: "/users", Methods: []string{"GET"}},
				},
			},
			method:     "get",
			path:       "/users",
			wantRoute:  true,
			wantParams: map[string]string{},
		},
		{
			name: "no_matching_method",
			plugin: &LoadedPlugin{
				HTTPRoutes: []*proto.HTTPRoute{
					{Path: "/users", Methods: []string{"GET"}},
				},
			},
			method:    "POST",
			path:      "/users",
			wantRoute: false,
		},
		{
			name: "no_matching_path",
			plugin: &LoadedPlugin{
				HTTPRoutes: []*proto.HTTPRoute{
					{Path: "/users", Methods: []string{"GET"}},
				},
			},
			method:    "GET",
			path:      "/posts",
			wantRoute: false,
		},
		{
			name: "path_with_params",
			plugin: &LoadedPlugin{
				HTTPRoutes: []*proto.HTTPRoute{
					{Path: "/users/{id}", Methods: []string{"GET"}},
				},
			},
			method:    "GET",
			path:      "/users/123",
			wantRoute: true,
			wantParams: map[string]string{
				"id": "123",
			},
		},
		{
			name: "multiple_routes_first_match",
			plugin: &LoadedPlugin{
				HTTPRoutes: []*proto.HTTPRoute{
					{Path: "/users", Methods: []string{"GET"}},
					{Path: "/users/{id}", Methods: []string{"GET"}},
				},
			},
			method:     "GET",
			path:       "/users",
			wantRoute:  true,
			wantParams: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := &HTTPHandler{}

			// ACT
			route, params := handler.matchRoute(tt.plugin, tt.method, tt.path)

			// ASSERT
			if tt.wantRoute {
				require.NotNil(t, route)
				assert.Equal(t, tt.wantParams, params)
			} else {
				assert.Nil(t, route)
			}
		})
	}
}
