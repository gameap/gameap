package plugin

import (
	"context"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackingManager keeps the modules it loaded, like the real manager, so the
// loader's "is a module running for this row" decisions can be exercised.
type trackingManager struct {
	mockPluginManager

	mu      sync.Mutex
	loaded  map[string]*pkgplugin.LoadedPlugin
	loads   []uint64
	unloads []string
	failFor map[uint64]error
}

func newTrackingManager() *trackingManager {
	m := &trackingManager{loaded: make(map[string]*pkgplugin.LoadedPlugin), failFor: make(map[uint64]error)}

	m.loadFunc = func(_ context.Context, wasmBytes []byte, _ map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
		m.mu.Lock()
		defer m.mu.Unlock()

		m.loads = append(m.loads, pluginID)

		if err := m.failFor[pluginID]; err != nil {
			return nil, err
		}

		plugin := &pkgplugin.LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pluginIDString(pluginID), Name: string(wasmBytes), Version: "1.0.0"},
			Enabled: true,
			DBID:    pluginID,
		}
		m.loaded[normalizeTestID(plugin.Info.Id)] = plugin

		return plugin, nil
	}

	m.unloadFunc = func(_ context.Context, pluginID string) error {
		m.mu.Lock()
		defer m.mu.Unlock()

		m.unloads = append(m.unloads, pluginID)

		if _, ok := m.loaded[normalizeTestID(pluginID)]; !ok {
			return pkgplugin.ErrPluginNotFound
		}

		delete(m.loaded, normalizeTestID(pluginID))

		return nil
	}

	m.getPlugin = func(pluginID string) (*pkgplugin.LoadedPlugin, bool) {
		m.mu.Lock()
		defer m.mu.Unlock()

		plugin, ok := m.loaded[normalizeTestID(pluginID)]

		return plugin, ok
	}

	return m
}

func normalizeTestID(id string) string {
	return pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(id))
}

func (m *trackingManager) loadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.loads)
}

func (m *trackingManager) unloadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.unloads)
}

type eventRecorder struct {
	mu     sync.Mutex
	events []recordedEvent
}

type recordedEvent struct {
	Type    proto.EventType
	Info    pkgplugin.EventInfo
	Trigger string
}

func (r *eventRecorder) DispatchPluginEventAsync(
	_ context.Context,
	eventType proto.EventType,
	info pkgplugin.EventInfo,
	extraData map[string]string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, recordedEvent{Type: eventType, Info: info, Trigger: extraData["trigger"]})
}

func (r *eventRecorder) all() []recordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]recordedEvent(nil), r.events...)
}

type syncEnv struct {
	ctx       context.Context
	repo      *inmemory.PluginRepository
	files     files.FileManager
	manager   *trackingManager
	events    *eventRecorder
	refresher *refreshRecorder
	loader    *Loader
}

func newSyncEnv(t *testing.T) *syncEnv {
	t.Helper()

	env := &syncEnv{
		ctx:       context.Background(),
		repo:      inmemory.NewPluginRepository(),
		files:     files.NewInMemoryFileManager(),
		manager:   newTrackingManager(),
		events:    &eventRecorder{},
		refresher: &refreshRecorder{},
	}
	env.loader = NewLoader(env.manager, env.files, env.repo, nil, "plugins",
		WithSubscriptionRefresher(env.refresher), WithLifecycleEvents(env.events))

	return env
}

func (e *syncEnv) install(t *testing.T, id domain.Uint64ID, status domain.PluginStatus, content string) *domain.Plugin {
	t.Helper()

	plugin := seedPlugin(e.ctx, t, e.repo, id, status)
	require.NoError(t, e.files.Write(e.ctx, "plugins/"+*plugin.Filename, []byte(content)))

	return plugin
}

func TestLoader_ApplyRecord_loads_absent_plugin_without_writing_the_row(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 701, domain.PluginStatusActive, "fine")
	before := findPlugin(env.ctx, t, env.repo, plugin.ID)

	changed, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)
	assert.True(t, changed)

	state := env.loader.RuntimeState(plugin.ID)
	assert.True(t, state.Present)
	assert.True(t, state.Enabled)
	assert.Equal(t, Fingerprint(plugin), state.Fingerprint)
	assert.Equal(t, Fingerprint(plugin), state.Attempted)

	after := findPlugin(env.ctx, t, env.repo, plugin.ID)
	assert.Equal(t, before.LastLoadedAt, after.LastLoadedAt, "the reconciler never writes the row")
	assert.Equal(t, before.Status, after.Status)
	assert.Equal(t, 1, env.refresher.count())

	events := env.events.all()
	require.Len(t, events, 1)
	assert.Equal(t, proto.EventType_EVENT_TYPE_PLUGIN_LOADED, events[0].Type)
	assert.Equal(t, TriggerSync, events[0].Trigger)
	assert.Equal(t, uint64(plugin.ID), events[0].Info.DBID)
	assert.Equal(t, "active", events[0].Info.Status)

	changed, err = env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)
	assert.False(t, changed, "an up-to-date module is left alone")
	assert.Equal(t, 1, env.manager.loadCount())
}

func TestLoader_ApplyRecord_replaces_module_when_fingerprint_differs(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 702, domain.PluginStatusActive, "v1")

	_, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)

	plugin.Version = "2.0.0"
	require.NoError(t, env.files.Write(env.ctx, "plugins/"+*plugin.Filename, []byte("v2")))

	changed, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, 1, env.manager.unloadCount())
	assert.Equal(t, 2, env.manager.loadCount())
	assert.Equal(t, Fingerprint(plugin), env.loader.RuntimeState(plugin.ID).Fingerprint)

	running, ok := env.manager.GetPlugin(pluginIDString(uint64(plugin.ID)))
	require.True(t, ok)
	assert.Equal(t, "v2", running.Info.Name, "the module was built from the new file")
}

func TestLoader_ApplyRecord_reloads_module_disabled_at_runtime(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 703, domain.PluginStatusActive, "fine")

	_, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)

	running, ok := env.manager.GetPlugin(pluginIDString(uint64(plugin.ID)))
	require.True(t, ok)
	running.Disable()

	assert.False(t, env.loader.RuntimeState(plugin.ID).Enabled)

	changed, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.True(t, env.loader.RuntimeState(plugin.ID).Enabled)
}

func TestLoader_ApplyRecord_failure_records_attempt_and_event_without_writing_the_row(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 704, domain.PluginStatusActive, "broken")
	env.manager.failFor[uint64(plugin.ID)] = errors.New("simulated failure")

	changed, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.Error(t, err)
	assert.False(t, changed)

	state := env.loader.RuntimeState(plugin.ID)
	assert.False(t, state.Present)
	assert.Empty(t, state.Fingerprint)
	assert.Equal(t, Fingerprint(plugin), state.Attempted)

	row := findPlugin(env.ctx, t, env.repo, plugin.ID)
	assert.Equal(t, domain.PluginStatusActive, row.Status, "the shared row is not written by the reconciler")
	assert.Nil(t, row.LastError)

	events := env.events.all()
	require.Len(t, events, 1)
	assert.Equal(t, proto.EventType_EVENT_TYPE_PLUGIN_ERROR, events[0].Type)
	require.NotNil(t, events[0].Info.Error)
	assert.Contains(t, *events[0].Info.Error, "simulated failure")
}

func TestLoader_ApplyRecord_refuses_held_plugin(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 705, domain.PluginStatusActive, "fine")

	release := env.loader.Hold(plugin.ID)
	nested := env.loader.Hold(plugin.ID)

	_, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.ErrorIs(t, err, ErrPluginHeld)

	nested()
	nested()

	_, err = env.loader.ApplyRecord(env.ctx, plugin)
	require.ErrorIs(t, err, ErrPluginHeld, "the outer hold is still in place")

	release()

	changed, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)
	assert.True(t, changed)
}

func TestLoader_UnloadRecord(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 706, domain.PluginStatusActive, "fine")

	unloaded, err := env.loader.UnloadRecord(env.ctx, plugin.ID, TriggerSync)
	require.NoError(t, err)
	assert.False(t, unloaded, "nothing runs yet")

	_, err = env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)

	unloaded, err = env.loader.UnloadRecord(env.ctx, plugin.ID, TriggerUninstall)
	require.NoError(t, err)
	assert.True(t, unloaded)

	state := env.loader.RuntimeState(plugin.ID)
	assert.False(t, state.Present)
	assert.Empty(t, state.Fingerprint)

	_, mapped := env.loader.GetPluginManagerID(plugin.ID)
	assert.False(t, mapped)

	events := env.events.all()
	require.Len(t, events, 2)
	assert.Equal(t, proto.EventType_EVENT_TYPE_PLUGIN_UNLOADED, events[1].Type)
	assert.Equal(t, "uninstalled", events[1].Info.Status)
	assert.Equal(t, TriggerUninstall, events[1].Trigger)
	assert.Equal(t, "fine", events[1].Info.Name)
}

func TestLoader_LoadRecord_adopts_identical_running_module_and_marks_row_active(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 707, domain.PluginStatusError, "fine")

	_, err := env.loader.ApplyRecord(env.ctx, plugin)
	require.NoError(t, err)
	require.Equal(t, 1, env.manager.loadCount())

	loaded, err := env.loader.LoadRecord(env.ctx, plugin)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, 1, env.manager.loadCount(), "the running module is adopted, not rebuilt")

	row := findPlugin(env.ctx, t, env.repo, plugin.ID)
	assert.Equal(t, domain.PluginStatusActive, row.Status)
	assert.NotNil(t, row.LastLoadedAt)

	events := env.events.all()
	require.Len(t, events, 1, "adoption publishes nothing")
}

func TestLoader_LoadRecord_emits_loaded_with_install_trigger(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 708, domain.PluginStatusActive, "fine")

	_, err := env.loader.LoadRecord(env.ctx, plugin)
	require.NoError(t, err)

	events := env.events.all()
	require.Len(t, events, 1)
	assert.Equal(t, proto.EventType_EVENT_TYPE_PLUGIN_LOADED, events[0].Type)
	assert.Equal(t, TriggerInstall, events[0].Trigger)
}

func TestLoader_LoadAll_emits_only_errors(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	env.install(t, 709, domain.PluginStatusActive, "fine")
	broken := env.install(t, 710, domain.PluginStatusActive, "broken")
	env.manager.failFor[uint64(broken.ID)] = errors.New("simulated failure")

	require.NoError(t, env.loader.LoadAll(env.ctx))

	events := env.events.all()
	require.Len(t, events, 1, "no subscriber exists during startup, so no loaded event")
	assert.Equal(t, proto.EventType_EVENT_TYPE_PLUGIN_ERROR, events[0].Type)
	assert.Equal(t, uint64(broken.ID), events[0].Info.DBID)
	assert.Equal(t, 0, env.refresher.count(), "LoadAll leaves the single refresh to the application")
}

func TestLoader_Reload_bumps_generation_and_persists_it(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 711, domain.PluginStatusActive, "fine")
	_, err := env.loader.LoadRecord(env.ctx, plugin)
	require.NoError(t, err)

	row, loaded, err := env.loader.Reload(env.ctx, plugin.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, 1, row.Generation)

	stored := findPlugin(env.ctx, t, env.repo, plugin.ID)
	assert.Equal(t, 1, stored.Generation)
	assert.Equal(t, Fingerprint(&stored), env.loader.RuntimeState(plugin.ID).Fingerprint,
		"the recorded fingerprint includes the bumped generation")

	events := env.events.all()
	require.Len(t, events, 2)
	assert.Equal(t, TriggerReload, events[1].Trigger)
}

func TestLoader_Reload_failure_still_persists_generation(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 712, domain.PluginStatusActive, "fine")
	env.manager.failFor[uint64(plugin.ID)] = errors.New("simulated failure")

	_, _, err := env.loader.Reload(env.ctx, plugin.ID)
	require.Error(t, err)

	stored := findPlugin(env.ctx, t, env.repo, plugin.ID)
	assert.Equal(t, 1, stored.Generation, "other instances must still restart the module")
	assert.Equal(t, domain.PluginStatusError, stored.Status)
}

func TestLoader_Unload_by_manager_id_emits_unloaded(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	plugin := env.install(t, 713, domain.PluginStatusActive, "fine")
	loaded, err := env.loader.LoadRecord(env.ctx, plugin)
	require.NoError(t, err)

	require.NoError(t, env.loader.Unload(env.ctx, loaded.Info.Id))

	state := env.loader.RuntimeState(plugin.ID)
	assert.False(t, state.Present)
	assert.Empty(t, state.Fingerprint)

	events := env.events.all()
	require.Len(t, events, 2)
	assert.Equal(t, proto.EventType_EVENT_TYPE_PLUGIN_UNLOADED, events[1].Type)
	assert.Equal(t, TriggerUnload, events[1].Trigger)
	assert.Equal(t, "unloaded", events[1].Info.Status)
}

func TestLoader_RuntimeState_of_unknown_plugin(t *testing.T) {
	t.Parallel()
	env := newSyncEnv(t)

	assert.Equal(t, RuntimeState{}, env.loader.RuntimeState(999))

	var nilLoader *Loader
	assert.Equal(t, RuntimeState{}, nilLoader.RuntimeState(1))
}
