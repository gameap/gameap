package uninstall_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/uninstall"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

type mockPluginManager struct {
	plugins      map[string]*pkgplugin.LoadedPlugin
	unloadCalled bool
	unloadErr    error
}

func newMockPluginManager() *mockPluginManager {
	return &mockPluginManager{
		plugins: make(map[string]*pkgplugin.LoadedPlugin),
	}
}

func (m *mockPluginManager) GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool) {
	p, ok := m.plugins[pluginID]

	return p, ok
}

func (m *mockPluginManager) Unload(_ context.Context, _ string) error {
	m.unloadCalled = true

	return m.unloadErr
}

func (m *mockPluginManager) addPlugin(pluginID string) {
	m.plugins[pluginID] = &pkgplugin.LoadedPlugin{}
}

func TestUninstall_successful(t *testing.T) {
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	existingPlugin := domain.Plugin{
		ID:       pkgplugin.ParsePluginID("testplugin123"),
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin123.wasm"),
		Status:   domain.PluginStatusActive,
	}
	err := pluginRepo.Save(context.Background(), &existingPlugin)
	require.NoError(t, err)

	err = fileManager.Write(context.Background(), "plugins/testplugin123.wasm", []byte("wasm content"))
	require.NoError(t, err)

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		nil,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin123"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNoContent, recorder.Code)

	remaining, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, remaining)

	assert.False(t, fileManager.Exists(context.Background(), "plugins/testplugin123.wasm"))
}

func TestUninstall_not_installed(t *testing.T) {
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		nil,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/nonexistent", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "nonexistent"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestUninstall_with_manager(t *testing.T) {
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	manager := newMockPluginManager()

	dbID := pkgplugin.ParsePluginID("testplugin456")
	managerID := pkgplugin.CompactPluginID(dbID)
	manager.addPlugin(managerID)

	existingPlugin := domain.Plugin{
		ID:       dbID,
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin456.wasm"),
		Status:   domain.PluginStatusActive,
	}
	err := pluginRepo.Save(context.Background(), &existingPlugin)
	require.NoError(t, err)

	err = fileManager.Write(context.Background(), "plugins/testplugin456.wasm", []byte("wasm content"))
	require.NoError(t, err)

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		manager,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin456", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin456"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.True(t, manager.unloadCalled)

	remaining, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestUninstall_manager_unload_error(t *testing.T) {
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	manager := newMockPluginManager()
	manager.unloadErr = errors.New("unload failed")

	dbID := pkgplugin.ParsePluginID("testplugin789")
	managerID := pkgplugin.CompactPluginID(dbID)
	manager.addPlugin(managerID)

	existingPlugin := domain.Plugin{
		ID:       dbID,
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin789.wasm"),
		Status:   domain.PluginStatusActive,
	}
	err := pluginRepo.Save(context.Background(), &existingPlugin)
	require.NoError(t, err)

	err = fileManager.Write(context.Background(), "plugins/testplugin789.wasm", []byte("wasm content"))
	require.NoError(t, err)

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		manager,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin789", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin789"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.True(t, manager.unloadCalled)

	remaining, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
}

func TestUninstall_plugin_not_loaded_in_manager(t *testing.T) {
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	manager := newMockPluginManager()

	dbID := pkgplugin.ParsePluginID("testpluginnotloaded")

	existingPlugin := domain.Plugin{
		ID:       dbID,
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testpluginnotloaded.wasm"),
		Status:   domain.PluginStatusActive,
	}
	err := pluginRepo.Save(context.Background(), &existingPlugin)
	require.NoError(t, err)

	err = fileManager.Write(context.Background(), "plugins/testpluginnotloaded.wasm", []byte("wasm content"))
	require.NoError(t, err)

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		manager,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testpluginnotloaded", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testpluginnotloaded"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.False(t, manager.unloadCalled)

	remaining, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — uninstalling a plugin removes
//     executable code and its DB record; it must be recorded (OWASP ASVS
//     §7.2.1) so the platform's code inventory changes are auditable.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestUninstall_Audit_SuccessfulUninstallIsRecorded covers OWASP API8:2023. A
// successful plugin uninstall must emit exactly one plugin.uninstall event
// with outcome success, category plugin_op, and the plugin id as the resource.
func TestUninstall_Audit_SuccessfulUninstallIsRecorded(t *testing.T) {
	// ARRANGE
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID:       pkgplugin.ParsePluginID("testplugin123"),
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin123.wasm"),
		Status:   domain.PluginStatusActive,
	}))
	require.NoError(t, fileManager.Write(
		context.Background(), "plugins/testplugin123.wasm", []byte("wasm content"),
	))

	recorder := &auditCapture{}
	h := uninstall.NewHandler(
		pluginRepo, fileManager, nil, nil, nil, nil, "plugins", api.NewResponder(), recorder,
	)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin123"})

	// ACT
	h.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusNoContent, w.Code,
		"uninstall must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventPluginUninstall),
		"exactly one plugin.uninstall event must be emitted per successful uninstall")

	ev, ok := findEvent(events, audit.EventPluginUninstall)
	require.True(t, ok, "a successful uninstall must leave a plugin.uninstall audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryPluginOp, ev.Category)
	assert.Equal(t, "plugin", ev.ResourceType)
	assert.NotEmpty(t, ev.ResourceID, "the uninstalled plugin id must be recorded as resource_id")
	assert.Equal(t, "uninstall", ev.Action)
}

// TestUninstall_Audit_NotInstalledIsNotRecorded covers OWASP API8:2023. An
// uninstall request for a plugin that is not installed must NOT emit a
// plugin.uninstall event (nothing was removed).
func TestUninstall_Audit_NotInstalledIsNotRecorded(t *testing.T) {
	// ARRANGE
	recorder := &auditCapture{}
	h := uninstall.NewHandler(
		inmemory.NewPluginRepository(), files.NewInMemoryFileManager(), nil, nil, nil, nil, "plugins",
		api.NewResponder(), recorder,
	)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/ghostplugin", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "ghostplugin"})

	// ACT
	h.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusNotFound, w.Code,
		"uninstalling a missing plugin must 404; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventPluginUninstall),
		"a no-op uninstall must not be recorded as a successful plugin.uninstall")
}

type fakeTaskScheduler struct {
	mu      sync.Mutex
	removed []domain.Uint64ID
	err     error
}

func (f *fakeTaskScheduler) RemovePluginTasks(_ context.Context, pluginID domain.Uint64ID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, pluginID)

	return 1, f.err
}

func (f *fakeTaskScheduler) snapshot() []domain.Uint64ID {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]domain.Uint64ID(nil), f.removed...)
}

func TestUninstall_RemovesScheduledTasks(t *testing.T) {
	// ARRANGE
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	scheduler := &fakeTaskScheduler{}

	existingPlugin := domain.Plugin{
		ID:       pkgplugin.ParsePluginID("testplugin123"),
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin123.wasm"),
		Status:   domain.PluginStatusActive,
	}
	require.NoError(t, pluginRepo.Save(context.Background(), &existingPlugin))

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		nil,
		nil,
		nil,
		scheduler,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin123"})

	// ACT
	h.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusNoContent, w.Code)
	removed := scheduler.snapshot()
	require.Len(t, removed, 1)
	assert.Equal(t, existingPlugin.ID, removed[0])
}

func TestUninstall_SchedulerErrorDoesNotBreakUninstall(t *testing.T) {
	// ARRANGE
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	scheduler := &fakeTaskScheduler{err: errors.New("db is down")}

	existingPlugin := domain.Plugin{
		ID:       pkgplugin.ParsePluginID("testplugin123"),
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin123.wasm"),
		Status:   domain.PluginStatusActive,
	}
	require.NoError(t, pluginRepo.Save(context.Background(), &existingPlugin))

	h := uninstall.NewHandler(
		pluginRepo,
		fileManager,
		nil,
		nil,
		nil,
		scheduler,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin123", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin123"})

	// ACT
	h.ServeHTTP(w, req)

	// ASSERT: task cleanup is best-effort, the uninstall itself succeeds
	require.Equal(t, http.StatusNoContent, w.Code)

	remaining, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, remaining)
}
