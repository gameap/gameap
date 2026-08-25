package dryrun_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/upload/dryrun"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLoaderManager struct {
	loadFunc func(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
}

func (m *mockLoaderManager) LoadTransient(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, wasmBytes, config, pluginID)
	}

	return nil, nil
}

func createMultipartRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)

	_, err = io.Copy(part, bytes.NewReader(content))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/dry-run", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func validWASMBytes() []byte {
	return []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
}

type mockPluginService struct {
	proto.PluginService
}

func (m *mockPluginService) GetSubscribedEvents(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
	return &proto.GetSubscribedEventsResponse{
		Events: []proto.EventType{
			proto.EventType_EVENT_TYPE_SERVER_POST_START,
		},
	}, nil
}

func TestDryRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		wasmContent    []byte
		mockManager    *mockLoaderManager
		wantStatus     int
		wantID         string
		wantName       string
		wantVersion    string
		wantErrorMatch string
	}{
		{
			name:        "successful_dry_run",
			wasmContent: validWASMBytes(),
			mockManager: &mockLoaderManager{
				loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
					return &pkgplugin.LoadedPlugin{
						Info: &proto.PluginInfo{
							Id:          "testplugin",
							Name:        "Test Plugin",
							Version:     "1.0.0",
							Description: "A test plugin",
							Author:      "Test Author",
							ApiVersion:  "v1",
						},
						Instance:        &mockPluginService{},
						HTTPRoutes:      []*proto.HTTPRoute{},
						ServerAbilities: []*proto.ServerAbility{},
						FrontendBundle:  []byte{1, 2, 3},
					}, nil
				},
			},
			wantStatus:  http.StatusOK,
			wantID:      "testplugin",
			wantName:    "Test Plugin",
			wantVersion: "1.0.0",
		},
		{
			name:        "invalid_wasm_magic",
			wasmContent: []byte{0x01, 0x02, 0x03, 0x04},
			mockManager: &mockLoaderManager{},
			wantStatus:  http.StatusInternalServerError,
		},
		{
			name:        "wasm_too_small",
			wasmContent: []byte{0x00, 0x61},
			mockManager: &mockLoaderManager{},
			wantStatus:  http.StatusInternalServerError,
		},
		{
			name:        "load_returns_error",
			wasmContent: validWASMBytes(),
			mockManager: &mockLoaderManager{
				loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
					return nil, errors.New("failed to compile WASM")
				},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := dryrun.NewHandler(tt.mockManager, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := createMultipartRequest(t, "plugin.wasm", tt.wasmContent)
			h.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				err := json.Unmarshal(recorder.Body.Bytes(), &resp)
				require.NoError(t, err)

				assert.Equal(t, tt.wantName, resp["name"])
				assert.Equal(t, tt.wantVersion, resp["version"])
				assert.Equal(t, true, resp["is_valid"])
				assert.NotNil(t, resp["errors"])
				assert.True(t, resp["has_frontend_bundle"].(bool))
			}

			if tt.wantErrorMatch != "" {
				assert.Contains(t, recorder.Body.String(), tt.wantErrorMatch)
			}
		})
	}
}

func TestDryRun_no_file_uploaded(t *testing.T) {
	t.Parallel()
	h := dryrun.NewHandler(&mockLoaderManager{}, api.NewResponder())
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/dry-run", nil)
	req.Header.Set("Content-Type", "multipart/form-data")

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestDryRun_reports_used_and_undeclared_permissions(t *testing.T) {
	t.Parallel()

	manager := &mockLoaderManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:                  "testplugin",
					Name:                "Test Plugin",
					Version:             "1.0.0",
					ApiVersion:          "1",
					RequiredPermissions: []string{"files"},
				},
				// The mock service subscribes to SERVER_POST_START.
				Instance: &mockPluginService{},
				HostImports: []pkgplugin.HostImport{
					{Module: "gameap-nodecmd", Function: "execute_command"},
					{Module: "gameap-nodefs", Function: "read_dir"},
					{Module: "gameap-log", Function: "log"},
				},
			}, nil
		},
	}

	h := dryrun.NewHandler(manager, api.NewResponder())
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, createMultipartRequest(t, "plugin.wasm", validWASMBytes()))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

	assert.Equal(t, []any{"files"}, resp["required_permissions"])
	assert.Equal(t, []any{"files_read", "listen_events", "node_commands"}, resp["used_permissions"],
		"the fixture only reads through gameap-nodefs")
	assert.Equal(t, []any{"listen_events", "node_commands"}, resp["undeclared_permissions"],
		"used but not declared: the install would not grant them; the declared files covers files_read")
}
