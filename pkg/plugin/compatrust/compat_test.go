package compatrust

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/cache"
	"github.com/gameap/gameap/pkg/plugin/sdk/crypto"
	"github.com/gameap/gameap/pkg/plugin/sdk/daemontasks"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/http"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodes"
	"github.com/gameap/gameap/pkg/plugin/sdk/servercontrol"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/gameap/gameap/pkg/plugin/sdk/serversettings"
	"github.com/gameap/gameap/pkg/plugin/sdk/storage"
	"github.com/gameap/gameap/pkg/plugin/sdk/users"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// This file must compile on v4.3.5, v4.4.1 and master: only the common plugin
// API is used (Manager, LoadedPlugin.Info/Instance and the proto.PluginService
// methods that already existed on v4.3.5).

// readFixtureWASM reads and gunzips the fixture testdata/<name>.wasm.gz. The
// fixtures are built by the test-plugins project and committed to this
// package; a missing file fails the test.
func readFixtureWASM(t *testing.T, name string) []byte {
	t.Helper()

	fixturePath := filepath.Join("testdata", name+".wasm.gz")

	gzBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "fixture %s must be committed to the package (built by the test-plugins project)", fixturePath)

	gr, err := gzip.NewReader(bytes.NewReader(gzBytes))
	require.NoError(t, err, "fixture %s must be gzip-compressed", fixturePath)
	defer func() { _ = gr.Close() }()

	wasmBytes, err := io.ReadAll(gr)
	require.NoError(t, err, "fixture %s must gunzip cleanly", fixturePath)

	return wasmBytes
}

// loadFixturePlugin loads the named fixture with the given host libraries.
func loadFixturePlugin(t *testing.T, name string, libraries []plugin.HostLibrary) *plugin.LoadedPlugin {
	t.Helper()

	wasmBytes := readFixtureWASM(t, name)

	manager := plugin.NewManager(plugin.ManagerConfig{Libraries: libraries})

	loadedPlugin, err := manager.Load(context.Background(), wasmBytes, map[string]string{}, 0)
	require.NoError(t, err, "fixture %s must load against its host libraries", name)
	require.NotNil(t, loadedPlugin)

	t.Cleanup(func() {
		_ = loadedPlugin.Close(context.Background())
		_ = manager.Shutdown(context.Background())
	})

	return loadedPlugin
}

// assertLifecycle checks the lifecycle contract every fixture satisfies:
// non-empty plugin id, api version "1" and successful initialization.
func assertLifecycle(t *testing.T, loadedPlugin *plugin.LoadedPlugin) {
	t.Helper()

	// ACT
	info, err := loadedPlugin.Instance.GetInfo(context.Background(), &proto.GetInfoRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.NotEmpty(t, info.Id, "plugin must declare a non-empty id")
	assert.Equal(t, "1", info.ApiVersion, "plugin must be built against plugin service API version 1")

	// ACT
	initResp, err := loadedPlugin.Instance.Initialize(context.Background(), &proto.InitializeRequest{
		Context: &proto.PluginContext{PluginId: info.Id},
		Config:  map[string]string{},
	})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, initResp)
	require.NotNil(t, initResp.Result)
	assert.True(t, initResp.Result.Success, "plugin must initialize successfully")
}

// assertServerPostStartHandled sends a SERVER_POST_START server event and
// requires the plugin to handle it.
func assertServerPostStartHandled(t *testing.T, loadedPlugin *plugin.LoadedPlugin) {
	t.Helper()

	// ARRANGE
	event := &proto.Event{
		Type:      proto.EventType_EVENT_TYPE_SERVER_POST_START,
		Timestamp: 1700000000,
		Context:   &proto.PluginContext{PluginId: loadedPlugin.Info.Id, RequestId: "req-1"},
		Payload: &proto.Event_ServerEvent{
			ServerEvent: &proto.ServerEventPayload{
				Server: &domainproto.Server{
					Id:         42,
					Name:       "test-server",
					GameId:     "cs",
					GameModId:  1,
					ServerIp:   "127.0.0.1",
					ServerPort: 27015,
				},
			},
		},
	}

	// ACT
	result, err := loadedPlugin.Instance.HandleEvent(context.Background(), event)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Handled, "plugin must handle the SERVER_POST_START server event")
}

func TestRustPluginCompat(t *testing.T) {
	tests := []struct {
		name string
		// newStubs builds a fresh set of stub services per subtest and returns
		// the host libraries to load the fixture with, plus a verification
		// that the plugin actually exercised those libraries.
		newStubs func() (libraries []plugin.HostLibrary, verify func(t *testing.T))
		// scenario runs fixture-specific calls after the common lifecycle.
		scenario func(t *testing.T, loadedPlugin *plugin.LoadedPlugin)
	}{
		{
			name: "core-lifecycle",
			newStubs: func() ([]plugin.HostLibrary, func(t *testing.T)) {
				logs := &stubLogService{}

				libraries := []plugin.HostLibrary{
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return log.Instantiate(ctx, r, logs)
					}),
				}

				verify := func(t *testing.T) {
					t.Helper()
					assert.NotEmpty(t, logs.Entries(), "plugin must emit log lines through the log host library")
				}

				return libraries, verify
			},
			scenario: func(t *testing.T, loadedPlugin *plugin.LoadedPlugin) {
				t.Helper()

				// ASSERT (routes were already fetched and validated by Manager.Load)
				require.NotEmpty(t, loadedPlugin.HTTPRoutes, "core-lifecycle must declare HTTP routes")

				// ACT
				routes, err := loadedPlugin.Instance.GetHTTPRoutes(context.Background(), &proto.GetHTTPRoutesRequest{})

				// ASSERT
				require.NoError(t, err)
				require.NotNil(t, routes)
				assert.NotEmpty(t, routes.Routes)

				// ACT
				resp, err := loadedPlugin.Instance.HandleHTTPRequest(context.Background(), &proto.HTTPRequest{
					Method:  "GET",
					Path:    "/status",
					Context: &proto.PluginContext{PluginId: loadedPlugin.Info.Id},
				})

				// ASSERT
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, int32(200), resp.StatusCode, "the /status endpoint must answer 200")
			},
		},
		{
			name: "cache-storage",
			newStubs: func() ([]plugin.HostLibrary, func(t *testing.T)) {
				logs := &stubLogService{}
				cacheStub := &stubCacheService{}
				storageStub := &stubStorageService{}

				libraries := []plugin.HostLibrary{
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return log.Instantiate(ctx, r, logs)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return cache.Instantiate(ctx, r, cacheStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return storage.Instantiate(ctx, r, storageStub)
					}),
				}

				verify := func(t *testing.T) {
					t.Helper()
					assert.NotEmpty(t, cacheStub.Calls(), "plugin must call the cache host library")
					assert.NotEmpty(t, storageStub.Calls(), "plugin must call the storage host library")
				}

				return libraries, verify
			},
		},
		{
			name: "http-crypto",
			newStubs: func() ([]plugin.HostLibrary, func(t *testing.T)) {
				logs := &stubLogService{}
				httpStub := &stubHTTPService{}
				cryptoStub := &stubCryptoService{}

				libraries := []plugin.HostLibrary{
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return log.Instantiate(ctx, r, logs)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return http.Instantiate(ctx, r, httpStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return crypto.Instantiate(ctx, r, cryptoStub)
					}),
				}

				verify := func(t *testing.T) {
					t.Helper()
					assert.True(t, httpStub.Called("Fetch"), "plugin must fetch via the http host library")
					assert.NotEmpty(t, cryptoStub.Calls(), "plugin must call the crypto host library")
				}

				return libraries, verify
			},
		},
		{
			name: "repositories",
			newStubs: func() ([]plugin.HostLibrary, func(t *testing.T)) {
				logs := &stubLogService{}
				serversStub := &stubServersService{}
				usersStub := &stubUsersService{}
				gamesStub := &stubGamesService{}
				gameModsStub := &stubGameModsService{}
				nodesStub := &stubNodesService{}
				serverSettingsStub := &stubServerSettingsService{}
				daemonTasksStub := &stubDaemonTasksService{}

				libraries := []plugin.HostLibrary{
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return log.Instantiate(ctx, r, logs)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return servers.Instantiate(ctx, r, serversStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return users.Instantiate(ctx, r, usersStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return games.Instantiate(ctx, r, gamesStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return gamemods.Instantiate(ctx, r, gameModsStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return nodes.Instantiate(ctx, r, nodesStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return serversettings.Instantiate(ctx, r, serverSettingsStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return daemontasks.Instantiate(ctx, r, daemonTasksStub)
					}),
				}

				verify := func(t *testing.T) {
					t.Helper()
					assert.NotEmpty(t, serversStub.Calls(), "plugin must call the servers host library")
					assert.NotEmpty(t, usersStub.Calls(), "plugin must call the users host library")
					assert.NotEmpty(t, gamesStub.Calls(), "plugin must call the games host library")
					assert.NotEmpty(t, gameModsStub.Calls(), "plugin must call the gamemods host library")
					assert.NotEmpty(t, nodesStub.Calls(), "plugin must call the nodes host library")
					assert.NotEmpty(t, serverSettingsStub.Calls(), "plugin must call the serversettings host library")
					assert.NotEmpty(t, daemonTasksStub.Calls(), "plugin must call the daemontasks host library")
				}

				return libraries, verify
			},
		},
		{
			name: "servercontrol",
			newStubs: func() ([]plugin.HostLibrary, func(t *testing.T)) {
				logs := &stubLogService{}
				serversStub := &stubServersService{}
				serverControlStub := &stubServerControlService{}

				libraries := []plugin.HostLibrary{
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return log.Instantiate(ctx, r, logs)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return servers.Instantiate(ctx, r, serversStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return servercontrol.Instantiate(ctx, r, serverControlStub)
					}),
				}

				verify := func(t *testing.T) {
					t.Helper()
					assert.NotEmpty(t, serverControlStub.Calls(), "plugin must call the servercontrol host library")
				}

				return libraries, verify
			},
		},
		{
			name: "node-ops",
			newStubs: func() ([]plugin.HostLibrary, func(t *testing.T)) {
				logs := &stubLogService{}
				nodesStub := &stubNodesService{}
				nodeFSStub := &stubNodeFSService{}
				nodeCmdStub := &stubNodeCmdService{}

				libraries := []plugin.HostLibrary{
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return log.Instantiate(ctx, r, logs)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return nodes.Instantiate(ctx, r, nodesStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return nodefs.Instantiate(ctx, r, nodeFSStub)
					}),
					hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
						return nodecmd.Instantiate(ctx, r, nodeCmdStub)
					}),
				}

				verify := func(t *testing.T) {
					t.Helper()
					assert.NotEmpty(t, nodeFSStub.Calls(), "plugin must call the nodefs host library")
					assert.NotEmpty(t, nodeCmdStub.Calls(), "plugin must call the nodecmd host library")
				}

				return libraries, verify
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			libraries, verify := tt.newStubs()
			loadedPlugin := loadFixturePlugin(t, tt.name, libraries)

			// ACT: the common lifecycle every fixture satisfies
			assertLifecycle(t, loadedPlugin)
			assertServerPostStartHandled(t, loadedPlugin)

			if tt.scenario != nil {
				tt.scenario(t, loadedPlugin)
			}

			// ASSERT: the plugin really called its host libraries
			verify(t)
		})
	}
}
