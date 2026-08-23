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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRefresher struct {
	calls int
}

func (f *fakeRefresher) RefreshSubscriptions(_ context.Context) error {
	f.calls++

	return nil
}

type fakeResolver struct {
	managerID string
}

func (f *fakeResolver) GetPluginManagerID(_ domain.Uint64ID) (string, bool) {
	if f.managerID == "" {
		return "", false
	}

	return f.managerID, true
}

func TestUninstall_refreshes_subscriptions_after_unload(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()
	manager := newMockPluginManager()

	dbID := pkgplugin.ParsePluginID("testplugin789")
	manager.addPlugin(pkgplugin.CompactPluginID(dbID))
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID:     dbID,
		Name:   "Test Plugin",
		Status: domain.PluginStatusActive,
	}))

	refresher := &fakeRefresher{}
	h := uninstall.NewHandler(
		pluginRepo,
		files.NewInMemoryFileManager(),
		manager,
		nil,
		refresher,
		nil,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin789", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin789"})

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, manager.unloadCalled)
	assert.Equal(t, 1, refresher.calls, "subscriptions must be refreshed after uninstall")
}

func TestUninstall_resolves_manager_id_via_loader(t *testing.T) {
	t.Parallel()
	// The wasm's own info ID may differ from the store ID; the resolver
	// mapping must win over the compact DB id fallback.
	pluginRepo := inmemory.NewPluginRepository()
	manager := newMockPluginManager()
	manager.addPlugin("custom-manager-id")

	dbID := pkgplugin.ParsePluginID("storeplugin42")
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID:     dbID,
		Name:   "Store Plugin",
		Status: domain.PluginStatusActive,
	}))

	refresher := &fakeRefresher{}
	h := uninstall.NewHandler(
		pluginRepo,
		files.NewInMemoryFileManager(),
		manager,
		&fakeResolver{managerID: "custom-manager-id"},
		refresher,
		nil,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/storeplugin42", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "storeplugin42"})

	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, manager.unloadCalled, "plugin registered under a custom id must be unloaded via the resolver mapping")
	assert.Equal(t, 1, refresher.calls)
}
