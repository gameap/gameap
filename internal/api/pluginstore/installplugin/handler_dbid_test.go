package installplugin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/pluginstore/installplugin"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingLoaderManager struct {
	gotPluginID uint64
}

func (m *recordingLoaderManager) Load(
	_ context.Context,
	_ []byte,
	_ map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	m.gotPluginID = pluginID

	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{Id: testPluginID, Name: "Test Plugin", Version: "1.0.0"},
	}, nil
}

func (m *recordingLoaderManager) LoadTransient(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return m.Load(ctx, wasmBytes, config, pluginID)
}

func (m *recordingLoaderManager) Unload(_ context.Context, _ string) error { return nil }
func (m *recordingLoaderManager) GetPlugin(_ string) (*pkgplugin.LoadedPlugin, bool) {
	return nil, false
}
func (m *recordingLoaderManager) GetPlugins() []*pkgplugin.LoadedPlugin { return nil }
func (m *recordingLoaderManager) Shutdown(_ context.Context) error      { return nil }

func (m *recordingLoaderManager) Register(_ *pkgplugin.LoadedPlugin) error { return nil }

func (m *recordingLoaderManager) Replace(_ *pkgplugin.LoadedPlugin) (*pkgplugin.LoadedPlugin, error) {
	return nil, nil
}

func (m *recordingLoaderManager) ShutdownPlugin(_ context.Context, _ *pkgplugin.LoadedPlugin) error {
	return nil
}

func TestInstallPlugin_loader_receives_db_plugin_id(t *testing.T) {
	// ARRANGE
	mockServer := newUpstreamServer(t, upstreamConfig{
		pluginDetails: defaultPluginDetails(false),
		versions:      defaultVersions(),
		downloadBody:  testWasmContent,
	})
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	repo := inmemory.NewPluginRepository()
	fm := newFakeFileManager()
	manager := &recordingLoaderManager{}
	loader := plugin.NewLoader(manager, fm, repo, nil, "plugins")

	h := installplugin.NewHandler(storeService, repo, fm, loader, nil, nil, "plugins", api.NewResponder())

	req := httptest.NewRequest(http.MethodPost, "/api/plugin-store/plugins/"+testPluginID+"/install", nil)
	req = mux.SetURLVars(req, map[string]string{"id": testPluginID})
	w := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(w, req)

	// ASSERT: the loader must pass the DB plugin id into the manager so
	// per-plugin host libraries (storage, log) are scoped to the real plugin.
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, uint64(pkgplugin.ParsePluginID(testPluginID)), manager.gotPluginID)
}
