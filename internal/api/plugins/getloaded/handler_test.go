package getloaded_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/plugins/getloaded"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/pluginsync"
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
	pluginRepo := inmemory.NewPluginRepository()

	h := getloaded.NewHandler(
		&mockLoaderManager{
			getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
				return nil
			},
		},
		nil,
		pluginRepo,
		nil,
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
		nil,
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
		nil,
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

type fakeSyncStatus struct {
	statuses map[domain.Uint64ID]pluginsync.Status
}

func (f *fakeSyncStatus) Snapshot() map[domain.Uint64ID]pluginsync.Status {
	return f.statuses
}

func TestLoaded_sync_state(t *testing.T) {
	ctx := context.Background()
	nextAttempt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("reports_the_state_of_a_healthy_plugin", func(t *testing.T) {
		pluginRepo := inmemory.NewPluginRepository()
		dbID := pkgplugin.ParsePluginID("healthyplugin")
		require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
			ID:      dbID,
			Name:    "Healthy Plugin",
			Version: "1.0.0",
			Status:  domain.PluginStatusActive,
		}))

		h := getloaded.NewHandler(
			&mockLoaderManager{
				getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
					return []*pkgplugin.LoadedPlugin{
						{
							Info:    &proto.PluginInfo{Id: "healthyplugin", Name: "Healthy Plugin", Version: "1.0.0"},
							Enabled: true,
						},
					}
				},
			},
			nil,
			pluginRepo,
			&fakeSyncStatus{statuses: map[domain.Uint64ID]pluginsync.Status{
				dbID: {PluginID: dbID, State: pluginsync.SyncStateInSync},
			}},
			api.NewResponder(),
		)

		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil))

		require.Equal(t, http.StatusOK, recorder.Code)

		data := decodeData(t, recorder)
		require.Len(t, data, 1)
		assert.Equal(t, "in_sync", data[0]["sync_state"])
		assert.Equal(t, "active", data[0]["db_status"])
		assert.Equal(t, true, data[0]["enabled"])
	})

	t.Run("lists_an_active_plugin_this_instance_could_not_load", func(t *testing.T) {
		pluginRepo := inmemory.NewPluginRepository()
		dbID := pkgplugin.ParsePluginID("brokenplugin")
		require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
			ID:      dbID,
			Name:    "Broken Plugin",
			Version: "2.0.0",
			Status:  domain.PluginStatusActive,
		}))

		h := getloaded.NewHandler(
			&mockLoaderManager{},
			nil,
			pluginRepo,
			&fakeSyncStatus{statuses: map[domain.Uint64ID]pluginsync.Status{
				dbID: {
					PluginID:    dbID,
					State:       pluginsync.SyncStateRetrying,
					Failures:    3,
					LastError:   "plugin file not found",
					NextAttempt: nextAttempt,
				},
			}},
			api.NewResponder(),
		)

		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil))

		require.Equal(t, http.StatusOK, recorder.Code)

		data := decodeData(t, recorder)
		require.Len(t, data, 1, "a broken plugin must not be invisible")
		assert.Equal(t, "Broken Plugin", data[0]["name"])
		assert.Equal(t, false, data[0]["enabled"])
		assert.Equal(t, "retrying", data[0]["sync_state"])
		assert.Equal(t, "plugin file not found", data[0]["sync_error"])
		assert.InDelta(t, 3.0, data[0]["sync_failures"], 0.001)
		assert.Equal(t, nextAttempt.Format(time.RFC3339), data[0]["next_attempt_at"])
	})

	t.Run("marks_a_module_without_a_row_as_an_orphan", func(t *testing.T) {
		h := getloaded.NewHandler(
			&mockLoaderManager{
				getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
					return []*pkgplugin.LoadedPlugin{
						{
							Info:    &proto.PluginInfo{Id: "orphanplugin", Name: "Orphan", Version: "1.0.0"},
							Enabled: true,
						},
					}
				},
			},
			nil,
			inmemory.NewPluginRepository(),
			&fakeSyncStatus{},
			api.NewResponder(),
		)

		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil))

		require.Equal(t, http.StatusOK, recorder.Code)

		data := decodeData(t, recorder)
		require.Len(t, data, 1)
		assert.Equal(t, "orphan", data[0]["sync_state"])
		assert.Empty(t, data[0]["db_status"])
	})

	t.Run("disabled_rows_are_not_listed", func(t *testing.T) {
		pluginRepo := inmemory.NewPluginRepository()
		require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
			ID:      pkgplugin.ParsePluginID("disabledplugin"),
			Name:    "Disabled Plugin",
			Version: "1.0.0",
			Status:  domain.PluginStatusDisabled,
		}))

		h := getloaded.NewHandler(&mockLoaderManager{}, nil, pluginRepo, &fakeSyncStatus{}, api.NewResponder())

		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Empty(t, decodeData(t, recorder))
	})
}

func decodeData(t *testing.T, recorder *httptest.ResponseRecorder) []map[string]any {
	t.Helper()

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

	return resp.Data
}
