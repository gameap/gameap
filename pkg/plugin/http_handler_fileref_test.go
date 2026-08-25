package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFileRefServer struct {
	mu    sync.Mutex
	calls []FileRefRequest
	err   error
	body  string
}

func (f *fakeFileRefServer) ServeFileRef(w http.ResponseWriter, _ *http.Request, req FileRefRequest) error {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()

	if f.err != nil {
		return f.err
	}

	w.Header().Set("Content-Disposition", `attachment; filename="export.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(f.body))

	return nil
}

func (f *fakeFileRefServer) snapshot() []FileRefRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]FileRefRequest(nil), f.calls...)
}

func fileRefPlugin(resp *proto.HTTPResponse) *LoadedPlugin {
	return &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "exporter"},
		Enabled: true,
		DBID:    77,
		Instance: &mockPluginServiceHTTP{
			handleHTTPRequestFunc: func(context.Context, *proto.HTTPRequest) (*proto.HTTPResponse, error) {
				return resp, nil
			},
		},
	}
}

func TestNewHTTPHandler_WithFileRefServer(t *testing.T) {
	t.Parallel()
	server := &fakeFileRefServer{}

	handler := NewHTTPHandler(NewManager(ManagerConfig{}), &mockMiddleware{}, &mockMiddleware{}, WithFileRefServer(server))

	assert.Equal(t, server, handler.fileRefs)
}

func TestHandlePluginRequest_FileRef(t *testing.T) {
	t.Parallel()

	fileResponse := &proto.HTTPResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "text/csv"},
		Body:       []byte("inline body that must not be written"),
		File:       &proto.FileRef{NodeId: 3, Path: "/srv/gameap/servers/cs2/export.csv", Filename: "export.csv"},
	}

	tests := []struct {
		name       string
		server     *fakeFileRefServer
		wantStatus int
		wantBody   string
		wantCall   bool
	}{
		{
			name:       "without_server_answers_501",
			server:     nil,
			wantStatus: http.StatusNotImplemented,
			wantBody:   "file responses are not enabled",
		},
		{
			name:       "delegates_to_server",
			server:     &fakeFileRefServer{body: "streamed,file\n"},
			wantStatus: http.StatusOK,
			wantBody:   "streamed,file\n",
			wantCall:   true,
		},
		{
			name:       "status_error_is_mapped",
			server:     &fakeFileRefServer{err: api.WrapHTTPError(errors.New("plugin permission files required"), http.StatusForbidden)},
			wantStatus: http.StatusForbidden,
			wantBody:   "plugin permission files required",
			wantCall:   true,
		},
		{
			name:       "generic_error_is_500_without_detail",
			server:     &fakeFileRefServer{err: errors.New("daemon exploded: /srv/gameap/secret")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to serve file",
			wantCall:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := &HTTPHandler{maxBody: DefaultMaxBodySize, timeout: 30 * time.Second}
			if tt.server != nil {
				handler.fileRefs = tt.server
			}

			req := httptest.NewRequest(http.MethodGet, "/api/plugins/exporter/export", nil)
			rr := httptest.NewRecorder()

			handler.handlePluginRequest(rr, req, fileRefPlugin(fileResponse), "/export", map[string]string{})

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.wantBody)
			assert.NotContains(t, rr.Body.String(), "inline body", "the plugin body is ignored for file responses")
			assert.NotContains(t, rr.Body.String(), "/srv/gameap/secret", "internal errors stay out of the response")

			if tt.server == nil {
				return
			}

			calls := tt.server.snapshot()
			if !tt.wantCall {
				assert.Empty(t, calls)

				return
			}

			require.Len(t, calls, 1)
			assert.Equal(t, uint64(77), calls[0].PluginID)
			assert.Equal(t, "exporter", calls[0].PluginName)
			assert.Equal(t, "/srv/gameap/servers/cs2/export.csv", calls[0].Ref.Path)
			assert.Equal(t, uint64(3), calls[0].Ref.NodeId)
			assert.Equal(t, "text/csv", calls[0].Headers["Content-Type"])
			assert.Equal(t, 200, calls[0].StatusCode)
		})
	}
}

func TestHandlePluginRequest_body_response_does_not_touch_file_server(t *testing.T) {
	t.Parallel()
	server := &fakeFileRefServer{body: "never"}
	handler := &HTTPHandler{maxBody: DefaultMaxBodySize, timeout: 30 * time.Second, fileRefs: server}

	req := httptest.NewRequest(http.MethodGet, "/api/plugins/exporter/status", nil)
	rr := httptest.NewRecorder()

	handler.handlePluginRequest(rr, req, fileRefPlugin(&proto.HTTPResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}),
		"/status", map[string]string{})

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, `{"ok":true}`, rr.Body.String())
	assert.Empty(t, server.snapshot())
}
