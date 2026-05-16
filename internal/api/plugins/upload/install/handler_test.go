package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/upload/install"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLoaderManager struct {
	loadFunc   func(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	unloadFunc func(ctx context.Context, pluginID string) error
}

func (m *mockLoaderManager) Load(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, wasmBytes, config, pluginID)
	}

	return nil, nil
}

func (m *mockLoaderManager) Unload(ctx context.Context, pluginID string) error {
	if m.unloadFunc != nil {
		return m.unloadFunc(ctx, pluginID)
	}

	return nil
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

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/install", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func validWASMBytes() []byte {
	return []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
}

func TestInstall(t *testing.T) {
	tests := []struct {
		name           string
		wasmContent    []byte
		mockManager    *mockLoaderManager
		wantStatus     int
		wantName       string
		wantVersion    string
		wantErrorMatch string
	}{
		{
			name:        "successful_install",
			wasmContent: validWASMBytes(),
			mockManager: &mockLoaderManager{
				loadFunc: func(_ context.Context, _ []byte, _ map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
					assert.Equal(t, uint64(0), pluginID)

					return &pkgplugin.LoadedPlugin{
						Info: &proto.PluginInfo{
							Id:          "testplugin",
							Name:        "Test Plugin",
							Version:     "1.0.0",
							Description: "A test plugin",
							Author:      "Test Author",
							ApiVersion:  "v1",
						},
					}, nil
				},
				unloadFunc: func(_ context.Context, _ string) error {
					return nil
				},
			},
			wantStatus:  http.StatusOK,
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
			name:        "load_returns_error",
			wasmContent: validWASMBytes(),
			mockManager: &mockLoaderManager{
				loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
					return nil, errors.New("failed to compile WASM")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pluginRepo := inmemory.NewPluginRepository()
			fileManager := files.NewInMemoryFileManager()

			h := install.NewHandler(
				tt.mockManager,
				pluginRepo,
				fileManager,
				nil,
				"plugins",
				api.NewResponder(),
				nil,
			)
			recorder := httptest.NewRecorder()

			req := createMultipartRequest(t, "plugin.wasm", tt.wasmContent)
			h.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				err := json.Unmarshal(recorder.Body.Bytes(), &resp)
				require.NoError(t, err)

				assert.NotNil(t, resp["id"])
				assert.Equal(t, tt.wantName, resp["name"])
				assert.Equal(t, tt.wantVersion, resp["version"])
				assert.Equal(t, "active", resp["status"])
				assert.NotNil(t, resp["installed_at"])

				installed, err := pluginRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)
				assert.Equal(t, tt.wantName, installed[0].Name)
				assert.Equal(t, tt.wantVersion, installed[0].Version)
				assert.NotNil(t, installed[0].Source)
				assert.Contains(t, *installed[0].Source, "file://")
			}

			if tt.wantErrorMatch != "" {
				assert.Contains(t, recorder.Body.String(), tt.wantErrorMatch)
			}
		})
	}
}

func TestInstall_already_installed_returns_409(t *testing.T) {
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	existingPlugin := domain.Plugin{
		ID:      pkgplugin.ParsePluginID("testplugin"),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}
	err := pluginRepo.Save(context.Background(), &existingPlugin)
	require.NoError(t, err)

	mockManager := &mockLoaderManager{
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
			}, nil
		},
		unloadFunc: func(_ context.Context, _ string) error {
			return nil
		},
	}

	h := install.NewHandler(
		mockManager,
		pluginRepo,
		fileManager,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := createMultipartRequest(t, "plugin.wasm", validWASMBytes())
	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "plugin already installed")
}

func TestInstall_no_file_uploaded(t *testing.T) {
	h := install.NewHandler(
		&mockLoaderManager{},
		inmemory.NewPluginRepository(),
		files.NewInMemoryFileManager(),
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/install", nil)
	req.Header.Set("Content-Type", "multipart/form-data")

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
