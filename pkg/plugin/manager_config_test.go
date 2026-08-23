package plugin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

const testConfigSchema = `{"properties": {
	"region": {"type": "string", "default": "eu"},
	"port": {"type": "integer", "default": 8080},
	"api_key": {"type": "string", "format": "secret"}
}}`

func initializeWithMock(t *testing.T, service *mockPluginService, config map[string]string) *LoadedPlugin {
	t.Helper()

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })

	manager := NewManager(ManagerConfig{})
	loaded := &LoadedPlugin{Config: config, Enabled: true, DBID: 1, guestLogs: newGuestLogs(nil)}

	require.NoError(t, manager.initializePlugin(ctx, r, service, loaded, loaded.guestLogs))

	return loaded
}

func TestInitializePlugin_overlays_schema_defaults_under_caller_values(t *testing.T) {
	t.Parallel()

	var received map[string]string
	service := &mockPluginService{
		infoFunc: func(context.Context, *proto.GetInfoRequest) (*proto.PluginInfo, error) {
			return &proto.PluginInfo{Id: "configured", ConfigSchema: testConfigSchema}, nil
		},
		initializeFunc: func(_ context.Context, req *proto.InitializeRequest) (*proto.InitializeResponse, error) {
			received = req.Config

			return &proto.InitializeResponse{}, nil
		},
	}

	loaded := initializeWithMock(t, service, map[string]string{"port": "9000", "api_key": "k"})

	want := map[string]string{"region": "eu", "port": "9000", "api_key": "k"}
	assert.Equal(t, want, received, "Initialize sees defaults for absent keys, the caller's values win")
	assert.Equal(t, want, loaded.Config)
	assert.Empty(t, loaded.ConfigSchemaError)
}

func TestInitializePlugin_without_schema_passes_config_through(t *testing.T) {
	t.Parallel()

	var received map[string]string
	service := &mockPluginService{
		initializeFunc: func(_ context.Context, req *proto.InitializeRequest) (*proto.InitializeResponse, error) {
			received = req.Config

			return &proto.InitializeResponse{}, nil
		},
	}

	loaded := initializeWithMock(t, service, map[string]string{"free": "form"})

	assert.Equal(t, map[string]string{"free": "form"}, received)
	assert.Equal(t, map[string]string{"free": "form"}, loaded.Config)

	loaded = initializeWithMock(t, service, nil)
	assert.Nil(t, received)
	assert.Nil(t, loaded.Config)
}

func TestInitializePlugin_invalid_schema_is_reported_and_plugin_still_loads(t *testing.T) {
	t.Parallel()

	initialized := false
	service := &mockPluginService{
		infoFunc: func(context.Context, *proto.GetInfoRequest) (*proto.PluginInfo, error) {
			return &proto.PluginInfo{Id: "broken", ConfigSchema: `{"properties": {"a": {"type": "object"}}}`}, nil
		},
		initializeFunc: func(_ context.Context, req *proto.InitializeRequest) (*proto.InitializeResponse, error) {
			initialized = true
			assert.Equal(t, map[string]string{"a": "x"}, req.Config)

			return &proto.InitializeResponse{}, nil
		},
	}

	loaded := initializeWithMock(t, service, map[string]string{"a": "x"})

	assert.True(t, initialized)
	assert.Contains(t, loaded.ConfigSchemaError, `unsupported type "object"`)
}

func TestLoad_records_host_modules_and_indexes_by_database_id(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{Libraries: importingLibraries(&stubNodeCmd{})})

	loaded, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	assert.Equal(t, []string{"gameap-nodecmd", "gameap-nodefs", "gameap-servers"}, loaded.HostModules,
		"every host module instantiated for the plugin, sorted; WASI and env are not host modules")

	modules, ok := manager.HostModules(42)
	require.True(t, ok)
	assert.Equal(t, loaded.HostModules, modules)

	byID, ok := manager.PluginByDBID(42)
	require.True(t, ok)
	assert.Same(t, loaded, byID)

	_, ok = manager.PluginByDBID(0)
	assert.False(t, ok)

	require.NoError(t, manager.Unload(context.Background(), loaded.Info.Id))

	_, ok = manager.PluginByDBID(42)
	assert.False(t, ok, "unload drops the index entry")

	assert.False(t, manager.SetHealth(42, HealthReport{Status: HealthHealthy}))
}

func TestLoadTransient_is_not_indexed_by_database_id(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{Libraries: importingLibraries(&stubNodeCmd{})})

	loaded, err := manager.LoadTransient(context.Background(), importingWASM, nil, 7)
	require.NoError(t, err)
	t.Cleanup(func() { _ = loaded.Close(context.Background()) })

	_, ok := manager.PluginByDBID(7)
	assert.False(t, ok)
	assert.Equal(t, []string{"gameap-nodecmd", "gameap-nodefs", "gameap-servers"}, loaded.HostModules)
}

func TestLoad_duplicate_declared_id_keeps_the_running_plugin_indexed(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{Libraries: importingLibraries(&stubNodeCmd{})})

	first, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	_, err = manager.Load(context.Background(), importingWASM, nil, 43)
	require.ErrorIs(t, err, ErrPluginAlreadyLoaded)

	_, ok := manager.PluginByDBID(43)
	assert.False(t, ok, "the rejected load leaves no index entry")

	byID, ok := manager.PluginByDBID(42)
	require.True(t, ok)
	assert.Same(t, first, byID)
}

func TestLoad_failed_load_restores_the_previous_index_entry(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{Libraries: importingLibraries(&stubNodeCmd{})})

	first, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	_, err = manager.Load(context.Background(), []byte("not wasm"), nil, 42)
	require.Error(t, err)

	byID, ok := manager.PluginByDBID(42)
	require.True(t, ok)
	assert.Same(t, first, byID, "a failed reload attempt must not hide the module that is still running")
}

func TestSetHealth_reachable_from_a_host_library_during_load(t *testing.T) {
	t.Parallel()

	var manager *Manager
	accepted := false

	reporting := hostStubLibrary{func(context.Context, wazero.Runtime) error {
		accepted = manager.SetHealth(42, HealthReport{Status: HealthDegraded, Message: "warming up"})

		return nil
	}}

	manager = NewManager(ManagerConfig{Libraries: append(importingLibraries(&stubNodeCmd{}), reporting)})

	loaded, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	assert.True(t, accepted, "the plugin is indexed before its runtime starts, while the manager lock is held")

	report, ok := loaded.Health()
	require.True(t, ok)
	assert.Equal(t, HealthDegraded, report.Status)
	assert.Equal(t, "warming up", report.Message)
	assert.False(t, report.ReportedAt.IsZero())
}

func TestShutdown_clears_the_database_id_index(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{Libraries: importingLibraries(&stubNodeCmd{})})

	_, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	require.NoError(t, manager.Shutdown(context.Background()))

	_, ok := manager.PluginByDBID(42)
	assert.False(t, ok)
}

func TestHealthReport_is_bounded(t *testing.T) {
	t.Parallel()

	details := map[string]string{strings.Repeat("k", MaxHealthDetailKeyLen+1): "dropped"}
	for i := range MaxHealthDetails + 5 {
		details["key-"+string(rune('a'+i))] = strings.Repeat("v", 300)
	}

	plugin := &LoadedPlugin{}
	plugin.SetHealth(HealthReport{
		Status:     HealthUnhealthy,
		Message:    strings.Repeat("m", MaxHealthMessageLen+10),
		Details:    details,
		ReportedAt: time.Unix(100, 0),
	})

	report, ok := plugin.Health()
	require.True(t, ok)
	assert.Len(t, report.Message, MaxHealthMessageLen)
	assert.Len(t, report.Details, MaxHealthDetails)
	assert.Equal(t, time.Unix(100, 0), report.ReportedAt)

	for key, value := range report.Details {
		assert.LessOrEqual(t, len(key), MaxHealthDetailKeyLen)
		assert.Len(t, value, MaxHealthDetailValueLen)
	}

	assert.NotContains(t, report.Details, strings.Repeat("k", MaxHealthDetailKeyLen+1))

	_, ok = (&LoadedPlugin{}).Health()
	assert.False(t, ok)

	assert.Equal(t, "healthy", HealthHealthy.String())
	assert.Equal(t, "degraded", HealthDegraded.String())
	assert.Equal(t, "unhealthy", HealthUnhealthy.String())
	assert.Equal(t, "unknown", HealthUnknown.String())
	assert.Equal(t, "unknown", HealthStatus(42).String())
}
