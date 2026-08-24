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
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingPluginRepository struct{}

func (failingPluginRepository) FindAll(context.Context, []filters.Sorting, *filters.Pagination) ([]domain.Plugin, error) {
	return nil, errors.New("database is down")
}

func (failingPluginRepository) Find(
	context.Context, *filters.FindPlugin, []filters.Sorting, *filters.Pagination,
) ([]domain.Plugin, error) {
	return nil, errors.New("database is down")
}

func (failingPluginRepository) Save(context.Context, *domain.Plugin) error { return nil }

func (failingPluginRepository) UpdateLoadState(context.Context, domain.Uint64ID, domain.PluginLoadState) error {
	return nil
}

func (failingPluginRepository) Delete(context.Context, domain.Uint64ID) error { return nil }

func (failingPluginRepository) Exists(context.Context, *filters.FindPlugin) (bool, error) {
	return false, errors.New("database is down")
}

func listPlugins(t *testing.T, h *getloaded.Handler) []map[string]any {
	t.Helper()

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

	return resp.Data
}

func TestLoaded_reports_runtime_state_of_loaded_plugins(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()
	dbID := pkgplugin.ParsePluginID("healthy")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: dbID, Name: "Healthy", Version: "1.0.0", Status: domain.PluginStatusActive,
	}))

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{{
			Info:    &proto.PluginInfo{Id: "healthy", Name: "Healthy", Version: "1.0.0"},
			Enabled: true,
			DBID:    uint64(dbID),
		}}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 1)
	assert.Equal(t, true, data[0]["enabled"])
	assert.Equal(t, true, data[0]["loaded"])
	assert.Equal(t, "active", data[0]["status"])
	assert.Nil(t, data[0]["error"])
	assert.Nil(t, data[0]["error_at"])
	assert.Nil(t, data[0]["memory_bytes"], "mock instances expose no module memory")
}

func TestLoaded_includes_installed_but_not_loaded_plugins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pluginRepo := inmemory.NewPluginRepository()
	errorAt := time.Date(2026, 8, 22, 9, 30, 0, 0, time.UTC)

	loadedID := pkgplugin.ParsePluginID("loadedplugin")
	require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
		ID: loadedID, Name: "Loaded", Version: "1.0.0", Status: domain.PluginStatusActive,
		Source: new("file://1.wasm"),
	}))

	broken := &domain.Plugin{
		ID: 4242, Name: "Broken", Version: "0.9.0", Description: "fails to load",
		Status: domain.PluginStatusError, Source: new("file://4242.wasm"),
		LastError: new("plugin file not found: plugins/4242.wasm"), LastErrorAt: &errorAt,
	}
	require.NoError(t, pluginRepo.Save(ctx, broken))

	disabled := &domain.Plugin{
		ID: 4343, Name: "Disabled", Version: "2.0.0", Status: domain.PluginStatusDisabled,
		Source: new("https://plugins.gameap.dev/api/plugins/disabled"),
	}
	require.NoError(t, pluginRepo.Save(ctx, disabled))

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{{
			Info:    &proto.PluginInfo{Id: "loadedplugin", Name: "Loaded", Version: "1.0.0"},
			Enabled: true,
			DBID:    uint64(loadedID),
		}}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 3)

	assert.Equal(t, "Loaded", data[0]["name"], "loaded plugins come first")
	assert.Equal(t, true, data[0]["loaded"])

	byName := map[string]map[string]any{}
	for _, entry := range data[1:] {
		byName[entry["name"].(string)] = entry
	}

	brokenEntry := byName["Broken"]
	require.NotNil(t, brokenEntry)
	assert.Equal(t, pkgplugin.CompactPluginID(4242), brokenEntry["id"])
	assert.Equal(t, "0.9.0", brokenEntry["version"])
	assert.Equal(t, "fails to load", brokenEntry["description"])
	assert.Equal(t, "file", brokenEntry["source_type"])
	assert.Equal(t, false, brokenEntry["enabled"])
	assert.Equal(t, false, brokenEntry["loaded"])
	assert.Equal(t, "error", brokenEntry["status"])
	assert.Equal(t, "plugin file not found: plugins/4242.wasm", brokenEntry["error"])
	assert.Equal(t, errorAt.Format(time.RFC3339), brokenEntry["error_at"])
	assert.Equal(t, false, brokenEntry["has_frontend_bundle"])
	assert.Nil(t, brokenEntry["http_routes"])
	assert.Nil(t, brokenEntry["memory_bytes"])

	disabledEntry := byName["Disabled"]
	require.NotNil(t, disabledEntry)
	assert.Equal(t, "disabled", disabledEntry["status"])
	assert.Equal(t, "store", disabledEntry["source_type"])
	assert.Equal(t, false, disabledEntry["loaded"])
	assert.Nil(t, disabledEntry["error"])
}

func TestLoaded_disabled_in_memory_reports_reason(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pluginRepo := inmemory.NewPluginRepository()
	errorAt := time.Date(2026, 8, 22, 9, 30, 0, 0, time.UTC)
	dbID := pkgplugin.ParsePluginID("hanging")
	require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
		ID: dbID, Name: "Hanging", Version: "1.0.0", Status: domain.PluginStatusError,
		LastError: new("recorded reason"), LastErrorAt: &errorAt,
	}))

	hanging := &pkgplugin.LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "hanging", Name: "Hanging", Version: "1.0.0"},
		Enabled: true,
		DBID:    uint64(dbID),
	}
	hanging.DisableWithReason("http handler timed out (GET /hang)")

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{hanging}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 1)
	assert.Equal(t, false, data[0]["enabled"])
	assert.Equal(t, true, data[0]["loaded"])
	assert.Equal(t, "error", data[0]["status"])
	assert.Equal(t, "http handler timed out (GET /hang)", data[0]["error"], "the live reason wins over the recorded one")
	assert.Equal(t, errorAt.Format(time.RFC3339), data[0]["error_at"])
}

func TestLoaded_silently_disabled_plugin_falls_back_to_record(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pluginRepo := inmemory.NewPluginRepository()
	dbID := pkgplugin.ParsePluginID("unloading")
	require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
		ID: dbID, Name: "Unloading", Version: "1.0.0", Status: domain.PluginStatusDisabled,
	}))

	unloading := &pkgplugin.LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "unloading", Name: "Unloading", Version: "1.0.0"},
		Enabled: true,
		DBID:    uint64(dbID),
	}
	unloading.Disable()

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{unloading}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 1)
	assert.Equal(t, "disabled", data[0]["status"], "an operator-managed status is reported as is")
	assert.Nil(t, data[0]["error"])
}

func TestLoaded_repository_failure_falls_back_to_loaded_plugins(t *testing.T) {
	t.Parallel()
	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{{
			Info:    &proto.PluginInfo{Id: "survivor", Name: "Survivor", Version: "1.0.0"},
			Enabled: true,
		}}
	}}, nil, failingPluginRepository{}, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 1)
	assert.Equal(t, "Survivor", data[0]["name"])
	assert.Equal(t, "active", data[0]["status"])
	assert.Equal(t, "store", data[0]["source_type"])
}

func TestLoaded_maps_loaded_plugin_to_record_by_declared_id(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pluginRepo := inmemory.NewPluginRepository()
	require.NoError(t, pluginRepo.Save(ctx, &domain.Plugin{
		ID: pkgplugin.ParsePluginID("legacyplugin"), Name: "Legacy", Version: "1.0.0",
		Status: domain.PluginStatusActive, Source: new("file://legacy.wasm"),
	}))

	// A LoadedPlugin built without a database ID (older call sites, tests)
	// still pairs with its record through the declared plugin ID.
	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{{
			Info:    &proto.PluginInfo{Id: "legacyplugin", Name: "Legacy", Version: "1.0.0"},
			Enabled: true,
		}}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 1, "the record must not be listed a second time as not loaded")
	assert.Equal(t, "file", data[0]["source_type"])
}

func TestLoaded_reports_permissions(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()
	dbID := pkgplugin.ParsePluginID("gated")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: dbID, Name: "Gated", Version: "1.0.0", Status: domain.PluginStatusActive,
		RequiredPermissions: []domain.PluginPermission{domain.PluginPermissionFiles},
		AllowedPermissions:  []domain.PluginPermission{domain.PluginPermissionFiles, domain.PluginPermissionListenEvents},
	}))
	notLoadedID := pkgplugin.ParsePluginID("dormant")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: notLoadedID, Name: "Dormant", Version: "1.0.0", Status: domain.PluginStatusDisabled,
		AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionSecrets},
	}))

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{{
			Info:    &proto.PluginInfo{Id: "gated", Name: "Gated", Version: "1.0.0"},
			Enabled: true,
			DBID:    uint64(dbID),
			HostImports: []pkgplugin.HostImport{
				{Module: "gameap-nodecmd", Function: "execute_command"},
				{Module: "gameap-nodefs", Function: "upload"},
				{Module: "gameap-servers", Function: "find_servers"},
			},
			SubscribedEvents: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
		}}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 2)

	assert.Equal(t, []any{"files"}, data[0]["required_permissions"])
	assert.Equal(t, []any{"files", "listen_events"}, data[0]["allowed_permissions"])
	assert.Equal(t, []any{"files", "listen_events", "node_commands"}, data[0]["used_permissions"],
		"derived from the gated imports and the subscriptions; open modules do not count")
	assert.Equal(t, []any{"node_commands"}, data[0]["missing_permissions"])

	assert.Equal(t, []any{}, data[1]["required_permissions"], "empty list, not null")
	assert.Equal(t, []any{"secrets"}, data[1]["allowed_permissions"])
	assert.Nil(t, data[1]["used_permissions"], "unknown for a plugin that is not loaded here")
	assert.Nil(t, data[1]["missing_permissions"])
}

func TestLoaded_reports_configuration_and_health(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()

	configuredID := pkgplugin.ParsePluginID("configured")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: configuredID, Name: "Configured", Version: "1.0.0", Status: domain.PluginStatusActive,
		Config:       map[string]any{"port": float64(9000), "api_key": map[string]any{"$secret": "enc:abc"}},
		ConfigSchema: new(`{"properties": {"port": {"type": "integer"}}}`),
	}))

	brokenID := pkgplugin.ParsePluginID("broken-schema-plugin")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: brokenID, Name: "Broken", Version: "1.0.0", Status: domain.PluginStatusError,
		ConfigSchema: new(`{`),
	}))

	loaded := &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{Id: "configured", Name: "Configured", Version: "1.0.0"}, Enabled: true,
		DBID: uint64(configuredID),
	}
	loaded.SetHealth(pkgplugin.HealthReport{Status: pkgplugin.HealthDegraded, Message: "warming up",
		Details: map[string]string{"stage": "cache"}})

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{loaded}
	}}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 2)

	byName := make(map[string]map[string]any)
	for _, row := range data {
		byName[row["name"].(string)] = row //nolint:forcetypeassert
	}

	configured := byName["Configured"]
	assert.Equal(t, true, configured["has_config_schema"])
	assert.Equal(t, []any{"api_key", "port"}, configured["config_keys"], "names only, never values")
	assert.NotContains(t, configured, "config_schema_error")

	health, ok := configured["health"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "degraded", health["status"])
	assert.Equal(t, "warming up", health["message"])
	assert.Equal(t, map[string]any{"stage": "cache"}, health["details"])
	assert.NotEmpty(t, health["reported_at"])

	broken := byName["Broken"]
	assert.Equal(t, true, broken["has_config_schema"])
	assert.NotEmpty(t, broken["config_schema_error"])
	assert.NotContains(t, broken, "health")
	assert.NotContains(t, broken, "config_keys")

	body := listBody(t, h)
	assert.NotContains(t, body, "enc:abc")
	assert.NotContains(t, body, "9000")
}

func listBody(t *testing.T, h *getloaded.Handler) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/plugins/loaded", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	return recorder.Body.String()
}

type fakeSyncStatus struct {
	snapshot map[domain.Uint64ID]pluginsync.Status
}

func (f fakeSyncStatus) Snapshot() map[domain.Uint64ID]pluginsync.Status { return f.snapshot }

func TestLoaded_reports_sync_state_and_local_failures(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()

	runningID := pkgplugin.ParsePluginID("running-plugin")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: runningID, Name: "Running", Version: "1.0.0", Status: domain.PluginStatusActive,
	}))

	brokenID := pkgplugin.ParsePluginID("broken-here-plugin")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: brokenID, Name: "BrokenHere", Version: "1.0.0", Status: domain.PluginStatusActive,
	}))

	attempt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	retry := attempt.Add(30 * time.Second)

	h := getloaded.NewHandler(&mockLoaderManager{getPluginsFunc: func() []*pkgplugin.LoadedPlugin {
		return []*pkgplugin.LoadedPlugin{{
			Info: &proto.PluginInfo{Id: "running-plugin", Name: "Running", Version: "1.0.0"}, Enabled: true,
			DBID: uint64(runningID),
		}}
	}}, nil, pluginRepo, true, api.NewResponder(), getloaded.WithSyncStatus(fakeSyncStatus{snapshot: map[domain.Uint64ID]pluginsync.Status{
		runningID: {PluginID: runningID, State: pluginsync.StateInSync},
		brokenID: {
			PluginID: brokenID, State: pluginsync.StateRetrying, Failures: 2,
			LastError:   "plugin file is missing and the plugin was not installed from the store",
			LastAttempt: attempt, NextAttempt: retry,
		},
	}}))

	data := listPlugins(t, h)
	require.Len(t, data, 2)

	byName := make(map[string]map[string]any)
	for _, row := range data {
		byName[row["name"].(string)] = row //nolint:forcetypeassert
	}

	running := byName["Running"]
	assert.Equal(t, "active", running["status"])
	assert.Equal(t, map[string]any{"state": "in_sync"}, running["sync"])

	broken := byName["BrokenHere"]
	assert.Equal(t, "error", broken["status"], "the answering instance's failure is shown, whatever the shared row says")
	assert.Contains(t, broken["error"], "plugin file is missing")
	assert.Equal(t, false, broken["loaded"])

	sync, ok := broken["sync"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "retrying", sync["state"])
	assert.Equal(t, float64(2), sync["failures"])
	assert.Equal(t, "2026-08-23T12:00:00Z", sync["last_attempt_at"])
	assert.Equal(t, "2026-08-23T12:00:30Z", sync["next_retry_at"])
}

func TestLoaded_without_sync_provider_omits_sync(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID: pkgplugin.ParsePluginID("lonely-plugin"), Name: "Lonely", Version: "1.0.0", Status: domain.PluginStatusActive,
	}))

	h := getloaded.NewHandler(&mockLoaderManager{}, nil, pluginRepo, true, api.NewResponder())

	data := listPlugins(t, h)
	require.Len(t, data, 1)
	assert.NotContains(t, data[0], "sync")
	assert.Equal(t, "active", data[0]["status"])
}
