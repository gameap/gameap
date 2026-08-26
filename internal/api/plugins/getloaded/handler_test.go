package getloaded_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/getloaded"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLoaderManager struct {
	getPluginsFunc func() []*pkgplugin.LoadedPlugin
}

func (m *mockLoaderManager) GetPlugins() []*pkgplugin.LoadedPlugin {
	if m.getPluginsFunc != nil {
		return m.getPluginsFunc()
	}

	return nil
}

func TestLoaded_empty_list(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()

	h := getloaded.NewHandler(
		&mockLoaderManager{
			getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
				return nil
			},
		},
		nil,
		pluginRepo,
		true,
		api.NewResponder(),
	)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil)

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]any)
	require.True(t, ok)
	assert.Empty(t, data)
}

func TestLoaded_with_plugins(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()

	plugin1 := &domain.Plugin{
		ID:      pkgplugin.ParsePluginID("testplugin1"),
		Name:    "Test Plugin 1",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
		Source:  new("file://12345.wasm"),
	}
	err := pluginRepo.Save(context.Background(), plugin1)
	require.NoError(t, err)

	plugin2 := &domain.Plugin{
		ID:      pkgplugin.ParsePluginID("testplugin2"),
		Name:    "Test Plugin 2",
		Version: "2.0.0",
		Status:  domain.PluginStatusActive,
		Source:  new("https://plugins.gameap.dev/api/plugins/testplugin2"),
	}
	err = pluginRepo.Save(context.Background(), plugin2)
	require.NoError(t, err)

	h := getloaded.NewHandler(
		&mockLoaderManager{
			getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
				return []*pkgplugin.LoadedPlugin{
					{
						Info: &proto.PluginInfo{
							Id:          "testplugin1",
							Name:        "Test Plugin 1",
							Version:     "1.0.0",
							Description: "Test Plugin 1 Description",
							Author:      "Test Author",
							ApiVersion:  "v1",
						},
						Enabled: true,
						HTTPRoutes: []*proto.HTTPRoute{
							{Path: "/stats", Methods: []string{"GET"}},
						},
						ServerAbilities: []*proto.ServerAbility{
							{Name: "custom_restart", Title: "Custom Restart"},
						},
						FrontendBundle: []byte{1, 2, 3},
					},
					{
						Info: &proto.PluginInfo{
							Id:          "testplugin2",
							Name:        "Test Plugin 2",
							Version:     "2.0.0",
							Description: "Test Plugin 2 Description",
							Author:      "Test Author 2",
							ApiVersion:  "v1",
						},
						Enabled: true,
					},
				}
			},
		},
		nil,
		pluginRepo,
		true,
		api.NewResponder(),
	)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil)

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	err = json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 2)

	plugin1Data := data[0].(map[string]any)
	assert.Equal(t, "Test Plugin 1", plugin1Data["name"])
	assert.Equal(t, "1.0.0", plugin1Data["version"])
	assert.Equal(t, "Test Plugin 1 Description", plugin1Data["description"])
	assert.Equal(t, "file", plugin1Data["source_type"])
	assert.Equal(t, true, plugin1Data["enabled"])
	assert.Equal(t, true, plugin1Data["has_frontend_bundle"])

	routes := plugin1Data["http_routes"].([]any)
	require.Len(t, routes, 1)
	assert.Equal(t, "/stats", routes[0].(map[string]any)["path"])

	abilities := plugin1Data["server_abilities"].([]any)
	require.Len(t, abilities, 1)
	assert.Equal(t, "custom_restart", abilities[0].(map[string]any)["name"])

	plugin2Data := data[1].(map[string]any)
	assert.Equal(t, "Test Plugin 2", plugin2Data["name"])
	assert.Equal(t, "2.0.0", plugin2Data["version"])
	assert.Equal(t, "Test Plugin 2 Description", plugin2Data["description"])
	assert.Equal(t, "store", plugin2Data["source_type"])
	assert.Equal(t, true, plugin2Data["enabled"])
	assert.Equal(t, false, plugin2Data["has_frontend_bundle"])
}

func TestLoaded_plugin_not_in_db(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()

	h := getloaded.NewHandler(
		&mockLoaderManager{
			getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
				return []*pkgplugin.LoadedPlugin{
					{
						Info: &proto.PluginInfo{
							Id:          "autoload_plugin",
							Name:        "Autoload Plugin",
							Version:     "1.0.0",
							Description: "Plugin loaded from autoload",
							Author:      "Test Author",
							ApiVersion:  "v1",
						},
						Enabled: true,
					},
				}
			},
		},
		nil,
		pluginRepo,
		true,
		api.NewResponder(),
	)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil)

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)

	pluginData := data[0].(map[string]any)
	assert.Equal(t, "Autoload Plugin", pluginData["name"])
	assert.Equal(t, "1.0.0", pluginData["version"])
	assert.Equal(t, "store", pluginData["source_type"])
}

func TestLoaded_reports_whether_permissions_are_enforced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		enforced bool
	}{
		{name: "enforced", enforced: true},
		{name: "not_enforced", enforced: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := getloaded.NewHandler(
				&mockLoaderManager{},
				nil,
				inmemory.NewPluginRepository(),
				tt.enforced,
				api.NewResponder(),
			)

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil)

			h.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusOK, recorder.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
			assert.Equal(t, tt.enforced, resp["permissions_enforced"],
				"the UI decides from this flag whether to warn that grants are not applied")
		})
	}
}
