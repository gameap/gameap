package plugin

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPluginManager struct {
	loadFunc    func(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	unloadFunc  func(ctx context.Context, pluginID string) error
	getPlugin   func(pluginID string) (*pkgplugin.LoadedPlugin, bool)
	getPlugins  func() []*pkgplugin.LoadedPlugin
	shutdownFn  func(ctx context.Context) error
	loadedCount int
}

func (m *mockPluginManager) Load(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	m.loadedCount++
	if m.loadFunc != nil {
		return m.loadFunc(ctx, wasmBytes, config, pluginID)
	}

	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{
			Id:      "test-plugin-id",
			Name:    "test-plugin",
			Version: "1.0.0",
		},
		Enabled: true,
	}, nil
}

func (m *mockPluginManager) LoadTransient(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return m.Load(ctx, wasmBytes, config, pluginID)
}

func (m *mockPluginManager) Unload(ctx context.Context, pluginID string) error {
	if m.unloadFunc != nil {
		return m.unloadFunc(ctx, pluginID)
	}

	return nil
}

func (m *mockPluginManager) GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool) {
	if m.getPlugin != nil {
		return m.getPlugin(pluginID)
	}

	return nil, false
}

func (m *mockPluginManager) GetPlugins() []*pkgplugin.LoadedPlugin {
	if m.getPlugins != nil {
		return m.getPlugins()
	}

	return nil
}

func (m *mockPluginManager) Shutdown(ctx context.Context) error {
	if m.shutdownFn != nil {
		return m.shutdownFn(ctx)
	}

	return nil
}

func TestLoader_LoadAll_FromRepository(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	_ = fileManager.Write(ctx, "plugins/test-plugin.wasm", []byte("wasm-content"))

	plugin := &domain.Plugin{
		ID:       456,
		Name:     "test-plugin",
		Version:  "1.0.0",
		Filename: new("test-plugin.wasm"),
		Status:   domain.PluginStatusActive,
	}
	err := pluginRepo.Save(ctx, plugin)
	require.NoError(t, err)

	loader := NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	err = loader.LoadAll(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, manager.loadedCount)

	mgrID, ok := loader.GetPluginManagerID(plugin.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-plugin-id", mgrID)
}

func TestLoader_LoadAll_WithAutoLoad(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:      "auto-plugin-id",
					Name:    "auto-plugin",
					Version: "1.0.0",
				},
				Enabled: true,
			}, nil
		},
	}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	_ = fileManager.Write(ctx, "plugins/auto-plugin.wasm", []byte("wasm-content"))

	loader := NewLoader(manager, fileManager, pluginRepo, []string{"auto-plugin.wasm"}, "plugins")

	err := loader.LoadAll(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, manager.loadedCount)

	plugins, err := pluginRepo.FindAll(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	assert.Equal(t, "auto-plugin", plugins[0].Name)
	assert.Equal(t, domain.PluginStatusActive, plugins[0].Status)
}

func TestLoader_Load_FileNotFound(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	loader := NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	_, err := loader.Load(ctx, "nonexistent.wasm")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin file not found")
}

func TestLoader_ProcessAutoLoad_AddsToDatabase(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	_ = fileManager.Write(ctx, "plugins/new-plugin.wasm", []byte("wasm-content"))

	loader := NewLoader(manager, fileManager, pluginRepo, []string{"new-plugin.wasm"}, "plugins")

	err := loader.processAutoLoad(ctx)
	require.NoError(t, err)

	plugins, err := pluginRepo.FindAll(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	assert.Equal(t, "test-plugin", plugins[0].Name)
	assert.Equal(t, domain.PluginStatusActive, plugins[0].Status)
	assert.NotNil(t, plugins[0].InstalledAt)
}

func TestLoader_ProcessAutoLoad_ActivatesExisting(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	_ = fileManager.Write(ctx, "plugins/existing-plugin.wasm", []byte("wasm-content"))

	plugin := &domain.Plugin{
		ID:      pkgplugin.ParsePluginID("test-plugin-id"),
		Name:    "existing-plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusDisabled,
	}
	err := pluginRepo.Save(ctx, plugin)
	require.NoError(t, err)

	loader := NewLoader(manager, fileManager, pluginRepo, []string{"existing-plugin.wasm"}, "plugins")

	err = loader.processAutoLoad(ctx)
	require.NoError(t, err)

	plugins, err := pluginRepo.FindAll(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	assert.Equal(t, domain.PluginStatusActive, plugins[0].Status)
}

func TestLoader_ProcessAutoLoad_MissingFile(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	loader := NewLoader(manager, fileManager, pluginRepo, []string{"missing-plugin.wasm"}, "plugins")

	err := loader.processAutoLoad(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "autoload plugin file not found")
}

func TestLoader_GetPluginManagerID(t *testing.T) {
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	loader := NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	loader.mu.Lock()
	loader.pluginIDs[123] = "manager-id-123"
	loader.mu.Unlock()

	mgrID, ok := loader.GetPluginManagerID(123)
	assert.True(t, ok)
	assert.Equal(t, "manager-id-123", mgrID)

	_, ok = loader.GetPluginManagerID(999)
	assert.False(t, ok)
}

func TestLoader_GetDBPluginID(t *testing.T) {
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	loader := NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	loader.mu.Lock()
	loader.pluginIDs[456] = "manager-id-456"
	loader.mu.Unlock()

	dbID, ok := loader.GetDBPluginID("manager-id-456")
	assert.True(t, ok)
	assert.Equal(t, domain.Uint64ID(456), dbID)

	_, ok = loader.GetDBPluginID("nonexistent-id")
	assert.False(t, ok)
}

func TestLoader_Unload(t *testing.T) {
	ctx := context.Background()
	unloadCalled := false
	manager := &mockPluginManager{
		unloadFunc: func(_ context.Context, pluginID string) error {
			unloadCalled = true
			assert.Equal(t, "plugin-to-unload", pluginID)

			return nil
		},
	}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	loader := NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	err := loader.Unload(ctx, "plugin-to-unload")
	require.NoError(t, err)
	assert.True(t, unloadCalled)
}

func TestParsePluginID_Numeric(t *testing.T) {
	id := pkgplugin.ParsePluginID("12345")
	assert.Equal(t, domain.Uint64ID(12345), id)
}

func TestParsePluginID_Base64(t *testing.T) {
	var num uint64 = 9876543210
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, num)
	encoded := pkgplugin.IDEncoding.EncodeToString(buf)

	id := pkgplugin.ParsePluginID(encoded)
	assert.Equal(t, domain.Uint64ID(num), id)
}

func TestParsePluginID_Hash(t *testing.T) {
	h := fnv.New64a()
	_, _ = h.Write([]byte("arbitrary-plugin-id"))
	expected := domain.Uint64ID(h.Sum64())

	id := pkgplugin.ParsePluginID("arbitrary-plugin-id")
	assert.Equal(t, expected, id)
}

func TestLoader_LoadAll_UpdatesLastLoadedAt(t *testing.T) {
	ctx := context.Background()
	manager := &mockPluginManager{}
	fileManager := files.NewInMemoryFileManager()
	pluginRepo := inmemory.NewPluginRepository()

	_ = fileManager.Write(ctx, "plugins/test-plugin.wasm", []byte("wasm-content"))

	plugin := &domain.Plugin{
		ID:       789,
		Name:     "test-plugin",
		Version:  "1.0.0",
		Filename: new("test-plugin.wasm"),
		Status:   domain.PluginStatusActive,
	}
	err := pluginRepo.Save(ctx, plugin)
	require.NoError(t, err)

	beforeLoad := time.Now()

	loader := NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	err = loader.LoadAll(ctx)
	require.NoError(t, err)

	plugins, err := pluginRepo.FindAll(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	assert.NotNil(t, plugins[0].LastLoadedAt)
	assert.True(t, lo.FromPtr(plugins[0].LastLoadedAt).After(beforeLoad) ||
		lo.FromPtr(plugins[0].LastLoadedAt).Equal(beforeLoad))
}
