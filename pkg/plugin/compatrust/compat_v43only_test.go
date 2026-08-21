package compatrust

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// CI copies this file only onto v4.3.5 checkouts. Panel v4.3.5 has no
// gameap-authz / gameap-rbac / gameap-scheduler / gameap-net host modules,
// so the fixtures that import them must fail to load.

func TestRustPluginCompatV43_UnsupportedFixtures(t *testing.T) {
	for _, name := range []string{"authz-rbac", "scheduler-protocol"} {
		t.Run(name, func(t *testing.T) {
			// ARRANGE
			wasmBytes := readFixtureWASM(t, name)
			manager := plugin.NewManager(plugin.ManagerConfig{Libraries: newV435HostLibraries()})

			// ACT
			loadedPlugin, err := manager.Load(context.Background(), wasmBytes, map[string]string{}, 0)

			// ASSERT
			require.Error(t, err, "fixture %s imports host modules that panel v4.3.5 does not provide", name)
			assert.Nil(t, loadedPlugin)
			assert.Contains(t, err.Error(), "gameap-", "the error must name the missing host module")
			assert.Contains(t, err.Error(), "instantiate", "the error must be a module instantiation failure")
		})
	}
}

// newV435HostLibraries instantiates every host library that exists on panel
// v4.3.5, so the load failure is caused specifically by the 4.4+ imports.
func newV435HostLibraries() []plugin.HostLibrary {
	return []plugin.HostLibrary{
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return log.Instantiate(ctx, r, &stubLogService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return cache.Instantiate(ctx, r, &stubCacheService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return crypto.Instantiate(ctx, r, &stubCryptoService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return http.Instantiate(ctx, r, &stubHTTPService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return storage.Instantiate(ctx, r, &stubStorageService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return servers.Instantiate(ctx, r, &stubServersService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return users.Instantiate(ctx, r, &stubUsersService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return games.Instantiate(ctx, r, &stubGamesService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return gamemods.Instantiate(ctx, r, &stubGameModsService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return nodes.Instantiate(ctx, r, &stubNodesService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return serversettings.Instantiate(ctx, r, &stubServerSettingsService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return daemontasks.Instantiate(ctx, r, &stubDaemonTasksService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return servercontrol.Instantiate(ctx, r, &stubServerControlService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return nodefs.Instantiate(ctx, r, &stubNodeFSService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return nodecmd.Instantiate(ctx, r, &stubNodeCmdService{})
		}),
	}
}
