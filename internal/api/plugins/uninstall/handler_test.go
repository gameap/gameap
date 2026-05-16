package uninstall_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/uninstall"
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
