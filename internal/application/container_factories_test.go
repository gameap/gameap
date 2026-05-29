// Broad lazy-singleton sweep + repository driver-switch coverage for the DI
// Container. Each "safe" accessor (one that builds a pure object graph and does
// not bind a port or dial the network at construction time) is built twice and
// asserted to be non-nil and memoised to the same instance.
//
// Deliberately NOT exercised here, and why:
//   - DB() / createDB()      — open + PingContext require a live database handle;
//     newWiredContainer injects c.db directly so these construction paths are
//     never reached, and the ping cannot succeed against a closed/in-memory DSN.
//   - Redis / Postgres cache + pub-sub branches — there is no miniredis or
//     Postgres in the test environment (only go-sqlmock + modernc.org/sqlite).
//   - ACMEService() / its Start — would dial a real ACME directory / open a
//     network listener; ACME wiring is covered separately in
//     buildgetcertificate_test.go with a stubbed lego factory.
//   - GRPCServer() / MultiplexedServer() Serve — the multiplexer is built and
//     closed (without Serve) as a smoke test in container_tls_test.go; the full
//     Serve loop is covered by multiplexer_test.go.
//   - Router() / HTTPServer() / HTTPSServer() / SecurityHeadersMiddleware()
//     build the object graph only (no listener); they are in the sweep because
//     they construct the embedded SPA + middleware chain in memory.

package application

import (
	"reflect"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authServicePaseto is the production-default auth service. newWiredContainer
// leaves AuthService unset, so any sweep that reaches createAuthService (directly
// or transitively via the HTTP router) must pin it to avoid the empty-value
// panic. loopbackBindIP is the loopback bind address used by the mux smoke test.
const (
	authServicePaseto = "paseto"
	loopbackBindIP    = "127.0.0.1"
)

// assertSameInstance asserts that two accessor results refer to the same
// underlying object. It tolerates both pointer returns and interface returns
// whose dynamic type is a pointer (the common shape for repositories and
// services in this container): pointer-like kinds are compared by their machine
// address, everything else by Go equality.
func assertSameInstance(t *testing.T, first, second any, msg string) {
	t.Helper()

	require.NotNil(t, first, msg+": first call must be non-nil")
	require.NotNil(t, second, msg+": second call must be non-nil")

	fv := reflect.ValueOf(first)
	sv := reflect.ValueOf(second)
	require.Equal(t, fv.Kind(), sv.Kind(), msg+": both calls must return the same kind")

	switch fv.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Func, reflect.Map, reflect.Chan, reflect.Slice:
		assert.Equal(t, fv.Pointer(), sv.Pointer(), msg+": both calls must return the same instance")
	default:
		assert.Equal(t, first, second, msg+": both calls must return the same instance")
	}
}

// TestContainerLazySingletonSweep is the core broad-coverage test: it builds the
// vast majority of the container's object graph through the public accessors and
// asserts each one is constructed exactly once (lazy singleton). Every accessor
// runs in its own subtest so a single construction panic or identity violation
// does not mask the rest, and so any accessor that turns out to bind/dial is
// trivially identifiable by name.
func TestContainerLazySingletonSweep(t *testing.T) {
	type accessor struct {
		name string
		get  func(c *Container) any
	}

	accessors := []accessor{
		// Repositories
		{"GameRepository", func(c *Container) any { return c.GameRepository() }},
		{"GameModRepository", func(c *Container) any { return c.GameModRepository() }},
		{"ServerRepository", func(c *Container) any { return c.ServerRepository() }},
		{"UserRepository", func(c *Container) any { return c.UserRepository() }},
		{"RBACRepository", func(c *Container) any { return c.RBACRepository() }},
		{"PersonalAccessTokenRepository", func(c *Container) any { return c.PersonalAccessTokenRepository() }},
		{"DaemonTaskRepository", func(c *Container) any { return c.DaemonTaskRepository() }},
		{"ServerTaskRepository", func(c *Container) any { return c.ServerTaskRepository() }},
		{"ServerTaskExecutionRepository", func(c *Container) any { return c.ServerTaskExecutionRepository() }},
		{"ServerSettingRepository", func(c *Container) any { return c.ServerSettingRepository() }},
		{"NodeRepository", func(c *Container) any { return c.NodeRepository() }},
		{"ClientCertificateRepository", func(c *Container) any { return c.ClientCertificateRepository() }},
		{"PluginStorageRepository", func(c *Container) any { return c.PluginStorageRepository() }},
		{"DLQRepository", func(c *Container) any { return c.DLQRepository() }},
		{"PluginRepository", func(c *Container) any { return c.PluginRepository() }},

		// Infrastructure
		{"Cache", func(c *Container) any { return c.Cache() }},
		{"PubSub", func(c *Container) any { return c.PubSub() }},
		{"TransactionalDB", func(c *Container) any { return c.TransactionalDB() }},
		{"TransactionManager", func(c *Container) any { return c.TransactionManager() }},
		{"Responder", func(c *Container) any { return c.Responder() }},
		{"FileManager", func(c *Container) any { return c.FileManager() }},
		{"StreamFileManager", func(c *Container) any { return c.StreamFileManager() }},
		{"FileUploadMIMEChecker", func(c *Container) any { return c.FileUploadMIMEChecker() }},
		{"AuditLogger", func(c *Container) any { return c.AuditLogger() }},

		// Auth / user / control services
		{"AuthService", func(c *Container) any { return c.AuthService() }},
		{"TwoFactorManager", func(c *Container) any { return c.TwoFactorManager() }},
		{"UserService", func(c *Container) any { return c.UserService() }},
		{"ServerControlService", func(c *Container) any { return c.ServerControlService() }},
		{"CertificatesService", func(c *Container) any { return c.CertificatesService() }},
		{"EnrollmentService", func(c *Container) any { return c.EnrollmentService() }},
		{"GlobalAPIService", func(c *Container) any { return c.GlobalAPIService() }},
		{"CaptchaVerifier", func(c *Container) any { return c.CaptchaVerifier() }},
		{"GameUpgradeService", func(c *Container) any { return c.GameUpgradeService() }},
		{"PelicanEggImporter", func(c *Container) any { return c.PelicanEggImporter() }},
		{"GameAPImporter", func(c *Container) any { return c.GameAPImporter() }},
		{"GameExporter", func(c *Container) any { return c.GameExporter() }},
		{"RBAC", func(c *Container) any { return c.RBAC() }},

		// Plugins
		{"PluginManager", func(c *Container) any { return c.PluginManager() }},
		{"PluginDispatcher", func(c *Container) any { return c.PluginDispatcher() }},
		{"PluginLoader", func(c *Container) any { return c.PluginLoader() }},
		{"PluginStoreService", func(c *Container) any { return c.PluginStoreService() }},

		// Dispatchers
		{"TaskDispatcher", func(c *Container) any { return c.TaskDispatcher() }},
		{"ServerTaskDispatcher", func(c *Container) any { return c.ServerTaskDispatcher() }},
		{"ServerConfigPusher", func(c *Container) any { return c.ServerConfigPusher() }},
		{"FileDispatcher", func(c *Container) any { return c.FileDispatcher() }},
		{"CommandDispatcher", func(c *Container) any { return c.CommandDispatcher() }},
		{"StatusDispatcher", func(c *Container) any { return c.StatusDispatcher() }},
		{"ConsoleLogDispatcher", func(c *Container) any { return c.ConsoleLogDispatcher() }},
		{"HTTPProxyDispatcher", func(c *Container) any { return c.HTTPProxyDispatcher() }},

		// Daemon-facing services
		{"DaemonStatus", func(c *Container) any { return c.DaemonStatus() }},
		{"DaemonFiles", func(c *Container) any { return c.DaemonFiles() }},
		{"DaemonCommands", func(c *Container) any { return c.DaemonCommands() }},
		{"ConsoleLogService", func(c *Container) any { return c.ConsoleLogService() }},
		{"HTTPProxyService", func(c *Container) any { return c.HTTPProxyService() }},
		{"UploadSessionService", func(c *Container) any { return c.UploadSessionService() }},
		{"UploadJanitor", func(c *Container) any { return c.UploadJanitor() }},
		{"FileManagerArchiver", func(c *Container) any { return c.FileManagerArchiver() }},
		{"FileManagerArchiveGuard", func(c *Container) any { return c.FileManagerArchiveGuard() }},

		// WebSocket + gRPC session graph
		{"WSHub", func(c *Container) any { return c.WSHub() }},
		{"WSBridge", func(c *Container) any { return c.WSBridge() }},
		{"SessionRegistry", func(c *Container) any { return c.SessionRegistry() }},
		{"TaskHandler", func(c *Container) any { return c.TaskHandler() }},
		{"CommandHandler", func(c *Container) any { return c.CommandHandler() }},
		{"ServerStatusHandler", func(c *Container) any { return c.ServerStatusHandler() }},
		{"AttachHandler", func(c *Container) any { return c.AttachHandler() }},
		{"MetricsHandler", func(c *Container) any { return c.MetricsHandler() }},
		{"MetricsHub", func(c *Container) any { return c.MetricsHub() }},
		{"TaskReaper", func(c *Container) any { return c.TaskReaper() }},
		{"GatewayService", func(c *Container) any { return c.GatewayService() }},
		{"FileTransferService", func(c *Container) any { return c.FileTransferService() }},
		{"TransferRegistry", func(c *Container) any { return c.TransferRegistry() }},

		// HTTP surface (object graph only, no listener)
		{"HTTPServer", func(c *Container) any { return c.HTTPServer() }},
		{"HTTPSServer", func(c *Container) any { return c.HTTPSServer() }},
		{"Router", func(c *Container) any { return c.Router() }},
		{"SecurityHeadersMiddleware", func(c *Container) any { return c.SecurityHeadersMiddleware() }},
	}

	for _, a := range accessors {
		t.Run(a.name, func(t *testing.T) {
			// ARRANGE
			// newWiredContainer leaves AuthService unset; createAuthService
			// panics on an empty value (the paseto default is only applied by
			// config.LoadConfig, which the harness bypasses). Pin paseto here so
			// AuthService and everything wired off the auth secret can build.
			c := newWiredContainer(t, func(cfg *config.Config) {
				cfg.AuthService = authServicePaseto
			})

			// ACT
			first := a.get(c)
			second := a.get(c)

			// ASSERT
			assertSameInstance(t, first, second, a.name)
		})
	}
}

// TestRepositoryFactoryInMemoryDriver pins the inmemory driver path for a couple
// of representative repositories. newMinimalContainer is used because the
// inmemory repositories never touch a database handle.
func TestRepositoryFactoryInMemoryDriver(t *testing.T) {
	t.Run("server_repository_uses_inmemory_implementation", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{DatabaseDriver: databaseDriverInMemory})

		// ACT
		repo := c.ServerRepository()

		// ASSERT
		require.NotNil(t, repo)
		assert.IsType(t, &inmemory.ServerRepository{}, repo,
			"inmemory driver must build the in-memory server repository")
	})

	t.Run("game_mod_repository_uses_inmemory_implementation", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{DatabaseDriver: databaseDriverInMemory})

		// ACT
		repo := c.GameModRepository()

		// ASSERT
		require.NotNil(t, repo)
		assert.IsType(t, inmemory.NewGameModRepository(), repo,
			"inmemory driver must build the in-memory game-mod repository")
	})
}

// TestRepositoryFactoryUnknownDriverBehaviour documents an existing, deliberate
// inconsistency between two repository factories: createGameRepository panics on
// an unrecognised driver, whereas createGameModRepository silently falls back to
// the in-memory implementation. These tests assert the code as it is — they do
// not endorse the inconsistency, only fence it so a change is noticed.
func TestRepositoryFactoryUnknownDriverBehaviour(t *testing.T) {
	t.Run("create_game_repository_panics_on_unknown_driver", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{DatabaseDriver: "totally-unknown"})

		// ACT / ASSERT
		assert.PanicsWithValue(t, "Unknown database driver: totally-unknown",
			func() { c.createGameRepository() },
			"createGameRepository has no fallback and must panic on an unknown driver")
	})

	t.Run("create_game_mod_repository_falls_back_to_inmemory_on_unknown_driver", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{DatabaseDriver: "totally-unknown"})

		// ACT
		repo := c.createGameModRepository()

		// ASSERT
		require.NotNil(t, repo, "createGameModRepository must not panic on an unknown driver")
		assert.IsType(t, inmemory.NewGameModRepository(), repo,
			"createGameModRepository must fall back to the in-memory implementation")
	})
}
