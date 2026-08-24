package application

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	trmsql "github.com/avito-tech/go-transaction-manager/drivers/sql/v2"
	trmcontext "github.com/avito-tech/go-transaction-manager/trm/v2/context"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/gameap/gameap/internal/acme"
	acmedns "github.com/gameap/gameap/internal/acme/dnsprovider"
	acmelocker "github.com/gameap/gameap/internal/acme/locker"
	acmestorage "github.com/gameap/gameap/internal/acme/storage"
	internalapi "github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagermime"
	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/application/defaults"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/files"
	internalgrpc "github.com/gameap/gameap/internal/grpc"
	"github.com/gameap/gameap/internal/grpc/filetransfer"
	"github.com/gameap/gameap/internal/grpc/gateway"
	"github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/i18n"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/metrics"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	pubsubintegration "github.com/gameap/gameap/internal/pubsub/integration"
	pubsubmemory "github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	pubsubpg "github.com/gameap/gameap/internal/pubsub/postgres"
	pubsubredis "github.com/gameap/gameap/internal/pubsub/redis"
	"github.com/gameap/gameap/internal/pubsub/retry"
	"github.com/gameap/gameap/internal/quercon"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/cached"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/repositories/mysql"
	"github.com/gameap/gameap/internal/repositories/postgres"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/captcha"
	"github.com/gameap/gameap/internal/services/filemanager/archiver"
	"github.com/gameap/gameap/internal/services/gameapimporter"
	"github.com/gameap/gameap/internal/services/gameexporter"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/internal/services/pelicaneggimporter"
	"github.com/gameap/gameap/internal/services/pluginarchive"
	"github.com/gameap/gameap/internal/services/pluginscheduler"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/internal/services/serverconfigpush"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/internal/services/servertaskdispatcher"
	"github.com/gameap/gameap/internal/services/taskdispatcher"
	"github.com/gameap/gameap/internal/services/taskreaper"
	"github.com/gameap/gameap/internal/telemetry"
	"github.com/gameap/gameap/internal/transfers"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/mergefs"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/gameap/gameap/pkg/tlsutil"
	"github.com/gameap/gameap/pkg/twofactor"
	webstatic "github.com/gameap/gameap/web/static"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	databaseDriverMySQL    = "mysql"
	databaseDriverPostgres = "postgres"
	databaseDriverPGX      = "pgx"
	databaseDriverSQLite   = "sqlite"
	databaseDriverInMemory = "inmemory"
)

const (
	cacheDriverInmemory = "inmemory"
	cacheDriverMemory   = "memory"
	cacheDriverMySQL    = "mysql"
	cacheDriverRedis    = "redis"
)

const (
	pubsubDriverMemory   = "memory"
	pubsubDriverRedis    = "redis"
	pubsubDriverPostgres = "postgres"
)

const (
	filesDriverLocal = "local"
)

const (
	sessionDrainTimeout = 2 * time.Second
	httpShutdownTimeout = 10 * time.Second
	grpcGraceTimeout    = 5 * time.Second
)

const (
	httpServerWriteTimeout = 30 * time.Second
	httpServerReadTimeout  = 15 * time.Second
	httpServerIdleTimeout  = 60 * time.Second
	defaultInstanceID      = "default"
)

type Container struct {
	config *config.Config

	context context.Context

	db                 *sql.DB
	transactionalDB    base.DB
	transactionManager *manager.Manager

	// Repositories
	gameRepository                repositories.GameRepository
	gameModRepository             repositories.GameModRepository
	serverRepository              repositories.ServerRepository
	userRepository                repositories.UserRepository
	rbacRepository                repositories.RBACRepository
	personalAccessTokenRepository repositories.PersonalAccessTokenRepository
	daemonTasksRepository         repositories.DaemonTaskRepository
	serverTaskRepository          repositories.ServerTaskRepository
	serverTaskExecutionRepository repositories.ServerTaskExecutionRepository
	serverSettingRepository       repositories.ServerSettingRepository
	nodeRepository                repositories.NodeRepository
	clientCertificateRepository   repositories.ClientCertificateRepository
	pluginStorageRepository       repositories.PluginStorageRepository
	pluginScheduledTaskRepository repositories.PluginScheduledTaskRepository
	pluginSecretRepository        repositories.PluginSecretRepository
	dlqRepository                 repositories.DLQRepository

	// Services
	authService          auth.Service
	twoFactorManager     *twofactor.Manager
	userService          *services.UserService
	serverControlService *servercontrol.Service
	taskDispatcher       *taskdispatcher.Dispatcher
	serverTaskDispatcher *servertaskdispatcher.Dispatcher
	serverConfigPusher   *serverconfigpush.Pusher
	globalAPIService     *services.GlobalAPIService
	cdnGamesService      *services.CDNGamesService
	pluginStoreService   *pluginstore.Service
	captchaVerifier      *captcha.Service
	gameUpgrader         *services.GameUpgradeService
	pelicanEggImporter   *pelicaneggimporter.Importer
	gameAPImporter       *gameapimporter.Importer
	gameExporter         *gameexporter.Exporter
	mfaNudgeService      *mfanudge.Service
	rbac                 *rbac.RBAC
	cache                cache.Cache
	fileManager          files.FileManager
	certificatesService  *certificates.Service

	// Enrollment
	enrollmentService *enrollment.Service

	secretCipher *secret.Cipher

	passwordBlocklist     auth.Blocklist
	passwordBlocklistOnce sync.Once

	// Daemon Services
	daemonStatus         *daemon.StatusService
	daemonFiles          *daemon.FileService
	daemonArchive        *daemon.ArchiveService
	fileDispatcher       daemon.FileDispatcher
	commandDispatcher    daemon.CommandDispatcher
	statusDispatcher     daemon.StatusDispatcher
	consoleLogDispatcher daemon.ConsoleLogDispatcher
	httpProxyDispatcher  daemon.HTTPProxyDispatcher
	daemonCommands       *daemon.CommandService
	daemonConsoleLog     *daemon.ConsoleLogService
	daemonHTTPProxy      *daemon.HTTPProxyService

	// Upload sessions
	uploadSessionService *upload.Service
	uploadJanitor        *upload.Janitor

	fileManagerArchiver     *archiver.Archiver
	fileManagerArchiveGuard *archiver.InMemoryConcurrencyGuard

	// Plugins
	pluginManager         *pkgplugin.Manager
	pluginDispatcher      *pkgplugin.Dispatcher
	pluginGuard           *hostlibrary.Guard
	pluginPermissions     *hostlibrary.CachedPermissionChecker
	pluginSubscriptionsPS *pubsubintegration.PluginSubscriptionsNotifier
	telemetry             *telemetry.Registry
	pluginMetrics         *telemetry.PluginMetrics
	pluginRepository      repositories.PluginRepository
	pluginLoader          *internalplugin.Loader
	pluginRecovery        *internalplugin.Supervisor
	querconResolver       *quercon.Resolver
	netConnRegistry       *pkgplugin.ConnRegistry
	pluginScheduler       *pluginscheduler.Service
	schedulerLocker       locker.Locker

	pluginArchiveEvents *pluginarchive.Service

	// HTTP
	router                    *http.ServeMux
	httpServer                *http.Server
	httpsServer               *http.Server
	responder                 *api.Responder
	auditLogger               audit.Logger
	securityHeadersMiddleware *middlewares.SecurityHeadersMiddleware
	fileUploadMIMEChecker     *filemanagermime.Checker
	i18nFS                    fs.FS
	frontendFS                fs.FS

	// ACME
	acmeService *acme.Service

	// PubSub
	pubsub pubsub.PubSub

	// WebSocket
	wsHub    *ws.Hub
	wsBridge *ws.Bridge

	// gRPC
	sessionRegistry     *session.Registry
	gatewayService      *gateway.Service
	fileTransferService *filetransfer.Service
	transferRegistry    *transfers.Registry
	taskHandler         *handlers.TaskHandler
	commandHandler      *handlers.CommandHandler
	serverStatusHandler *handlers.ServerStatusHandler
	attachHandler       *handlers.AttachHandler
	metricsHandler      *handlers.MetricsHandler
	metricsHub          metrics.Hub
	taskReaper          *taskreaper.Reaper
	grpcServer          *grpc.Server
	grpcServerCertLeaf  *x509.Certificate
	multiplexedServer   *MultiplexedServer

	// Shutdown
	cancel            context.CancelFunc
	shotdownFuncs     []func() error
	lateShutdownFuncs []func() error
}

func NewContainer(config *config.Config) *Container {
	return &Container{
		config: config,
	}
}

func (c *Container) SetContext(ctx context.Context, cancel context.CancelFunc) {
	c.context = ctx
	c.cancel = cancel
}

func (c *Container) Shutdown() error {
	if c.sessionRegistry != nil {
		c.sessionRegistry.BroadcastShutdown(
			context.Background(),
			"server shutting down",
			30*time.Second,
		)
	}

	c.shutdownHTTPServers()

	if c.sessionRegistry != nil {
		c.sessionRegistry.WaitSessionsClosed(sessionDrainTimeout)
	}

	if c.cancel != nil {
		c.cancel()
	}

	if c.sessionRegistry != nil {
		c.sessionRegistry.CloseAllSessions()
	}

	c.shutdownGRPCServer()

	// Pending automatic plugin reloads must not race the runtime shutdown
	// below: stop the supervisor and wait for reloads in flight.
	if c.pluginRecovery != nil {
		c.pluginRecovery.Stop()
	}

	// The scheduler must join its in-flight runs before the plugin manager's
	// shutdown func (in shotdownFuncs below) closes the WASM runtimes; the
	// append order of shutdown funcs is accessor-order dependent, so the stop
	// is explicit here.
	if c.pluginScheduler != nil {
		c.pluginScheduler.Stop()
	}

	// Same contract for archive event deliveries: an in-flight guest
	// callback must finish before its runtime is closed.
	if c.pluginArchiveEvents != nil {
		c.pluginArchiveEvents.Stop()
	}

	for _, fn := range c.shotdownFuncs {
		if err := fn(); err != nil {
			slog.Error(
				"failed to execute shutdown function",
				slog.String("error", err.Error()),
			)
		}
	}

	for _, fn := range c.lateShutdownFuncs {
		if err := fn(); err != nil {
			slog.Error(
				"failed to execute late shutdown function",
				slog.String("error", err.Error()),
			)
		}
	}

	return nil
}

func (c *Container) shutdownHTTPServers() {
	if c.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		if err := c.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server shutdown error", slog.String("error", err.Error()))
		} else {
			slog.Info("http server shutdown succeeded")
		}
		cancel()
	}

	if c.httpsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		if err := c.httpsServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("https server shutdown error", slog.String("error", err.Error()))
		} else {
			slog.Info("https server shutdown succeeded")
		}
		cancel()
	}
}

func (c *Container) shutdownGRPCServer() {
	if c.grpcServer == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		c.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(grpcGraceTimeout):
		slog.Warn("gRPC server force stop due to timeout")
		c.grpcServer.Stop()
	}
}

func (c *Container) appendShutdownFunc(fn func() error) {
	c.shotdownFuncs = append(c.shotdownFuncs, fn)
}

func (c *Container) appendLateShutdownFunc(fn func() error) {
	c.lateShutdownFuncs = append(c.lateShutdownFuncs, fn)
}

func (c *Container) Config() *config.Config {
	return c.config
}

// SecretCipher returns the process-wide cipher used to encrypt reversible
// at-rest secrets (e.g. gdaemon_password). When ENCRYPTION_KEY is unset the
// cipher is disabled (plaintext passthrough) and a one-time warning is logged.
func (c *Container) SecretCipher() *secret.Cipher {
	if c.secretCipher != nil {
		return c.secretCipher
	}

	cipher, err := secret.NewCipher(c.config.EncryptionKey)
	if err != nil {
		slog.Error("failed to build secret cipher, falling back to disabled", slog.String("error", err.Error()))
		cipher = secret.Disabled()
	}

	if !cipher.Enabled() {
		slog.Warn("ENCRYPTION_KEY is not set: gdaemon_password is stored in plaintext")
	}

	c.secretCipher = cipher

	return c.secretCipher
}

// PasswordBlocklist returns the process-wide common-password blocklist
// consulted by auth.ValidatePassword to satisfy OWASP ASVS 4.0.3 §2.1.7.
//
// When AUTH_ALLOW_WEAK_PASSWORDS=true the loader is skipped entirely (no
// memory cost) and a startup slog.Warn is emitted. On a load failure the
// accessor falls back to auth.NoopBlocklist with a prominent slog.Error so
// a corrupt embedded asset does not block bootstrap of the admin user.
func (c *Container) PasswordBlocklist() auth.Blocklist {
	c.passwordBlocklistOnce.Do(func() {
		if c.config.Auth.AllowWeakPasswords {
			slog.Warn("AUTH_ALLOW_WEAK_PASSWORDS is enabled: common-password " +
				"blocklist is DISABLED; users may set weak passwords " +
				"(ASVS 2.1.7 not enforced)")

			c.passwordBlocklist = auth.NoopBlocklist{}

			return
		}

		bl, err := auth.LoadEmbeddedBlocklist()
		if err != nil {
			slog.Error(
				"password blocklist failed to load — common-password protection DISABLED for this process",
				slog.String("error", err.Error()),
			)

			c.passwordBlocklist = auth.NoopBlocklist{}

			return
		}

		slog.Info(
			"Password blocklist loaded",
			slog.Int("entries", bl.Len()),
			slog.String("source", "embedded"),
		)

		c.passwordBlocklist = bl
	})

	return c.passwordBlocklist
}

func (c *Container) DB() *sql.DB {
	if c.db == nil {
		db, err := c.createDB()
		if err != nil {
			panic(err)
		}

		c.db = db

		c.appendLateShutdownFunc(func() error {
			return c.db.Close()
		})
	}

	return c.db
}

func (c *Container) createDB() (*sql.DB, error) {
	db, err := sql.Open(c.config.DatabaseDriver, c.config.DatabaseURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to database")
	}

	err = c.pingDBWithRetry(db)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ping database")
	}

	return db, nil
}

const (
	dbPingRetryInitialDelay = 500 * time.Millisecond
	dbPingRetryMaxDelay     = 5 * time.Second
)

// Retries the initial ping within Config.DatabaseConnectTimeout so a database
// that is briefly unavailable (restarting alongside the panel, still booting)
// does not bring the process down. The window bounds blocked ping attempts and
// retry waits alike; a non-positive timeout means a single unbounded attempt.
func (c *Container) pingDBWithRetry(db *sql.DB) error {
	ctx := c.context
	if ctx == nil {
		ctx = context.Background()
	}

	if c.config.DatabaseConnectTimeout <= 0 {
		return db.PingContext(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.DatabaseConnectTimeout)
	defer cancel()

	delay := dbPingRetryInitialDelay

	for {
		err := db.PingContext(ctx)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return err
		}

		slog.Warn(
			"database not ready, retrying",
			slog.String("error", err.Error()),
			slog.Duration("retry_in", delay),
		)

		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}

		delay = min(delay*2, dbPingRetryMaxDelay)
	}
}

func (c *Container) TransactionalDB() base.DB {
	if c.transactionalDB == nil {
		c.transactionalDB = base.NewDBTxWrapper(c.DB(), trmsql.DefaultCtxGetter)

		if c.config.Logger.LogDBQueries {
			c.transactionalDB = base.NewDBLogWrapper(c.transactionalDB)
		}
	}

	return c.transactionalDB
}

func (c *Container) TransactionManager() base.TransactionManager {
	if c.transactionManager == nil {
		c.transactionManager = c.createTransactionManager()
	}

	return c.transactionManager
}

func (c *Container) createTransactionManager() *manager.Manager {
	return manager.Must(
		trmsql.NewDefaultFactory(c.DB()),
		manager.WithCtxManager(trmcontext.DefaultManager),
	)
}

func (c *Container) GameRepository() repositories.GameRepository {
	if c.gameRepository == nil {
		c.gameRepository = c.createGameRepository()
	}

	return c.gameRepository
}

func (c *Container) createGameRepository() repositories.GameRepository {
	var baseRepo repositories.GameRepository

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		baseRepo = mysql.NewGameRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		baseRepo = postgres.NewGameRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		baseRepo = sqlite.NewGameRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		baseRepo = inmemory.NewGameRepository()
	default:
		panic("Unknown database driver: " + c.config.DatabaseDriver)
	}

	// Wrap with cache if Redis is configured
	if c.config.Cache.Driver == cacheDriverRedis {
		ttl, err := time.ParseDuration(c.config.Cache.TTL.Games)
		if err != nil {
			ttl = 48 * time.Hour // Default to 48 hours
		}

		return cached.NewGameRepository(baseRepo, c.Cache(), ttl)
	}

	return baseRepo
}

func (c *Container) GameModRepository() repositories.GameModRepository {
	if c.gameModRepository == nil {
		c.gameModRepository = c.createGameModRepository()
	}

	return c.gameModRepository
}

func (c *Container) createGameModRepository() repositories.GameModRepository {
	var baseRepo repositories.GameModRepository

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		baseRepo = mysql.NewGameModRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		baseRepo = postgres.NewGameModRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		baseRepo = sqlite.NewGameModRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		baseRepo = inmemory.NewGameModRepository()
	default:
		// Use in-memory repository as fallback
		baseRepo = inmemory.NewGameModRepository()
	}

	// Wrap with cache if Redis is configured
	if c.config.Cache.Driver == cacheDriverRedis {
		ttl, err := time.ParseDuration(c.config.Cache.TTL.Games)
		if err != nil {
			ttl = 48 * time.Hour // Default to 48 hours (same as games)
		}

		return cached.NewGameModRepository(baseRepo, c.Cache(), ttl)
	}

	return baseRepo
}

func (c *Container) ServerRepository() repositories.ServerRepository {
	if c.serverRepository == nil {
		c.serverRepository = c.createServerRepository()
	}

	return c.serverRepository
}

func (c *Container) createServerRepository() repositories.ServerRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewServerRepository(c.TransactionalDB(), c.TransactionManager())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewServerRepository(c.TransactionalDB(), c.TransactionManager())
	case databaseDriverSQLite:
		return sqlite.NewServerRepository(c.TransactionalDB(), c.TransactionManager())
	case databaseDriverInMemory:
		return inmemory.NewServerRepository()
	default:
		// Use in-memory repository as fallback
		return inmemory.NewServerRepository()
	}
}

func (c *Container) HTTPServer() *http.Server {
	if c.httpServer == nil {
		c.httpServer = c.createHTTPServer()
	}

	return c.httpServer
}

func (c *Container) createHTTPServer() *http.Server {
	var handler http.Handler = c.Router()

	if c.config.TLSEnabled() && c.config.TLS.ForceHTTPS {
		handler = middlewares.HTTPSRedirectMiddleware(c.config.HTTPSPort)(handler)
	}

	// Wrapped outside HTTPSRedirect so the 301 carries HSTS too. Stays inside
	// audit because audit must remain the outermost middleware.
	handler = c.SecurityHeadersMiddleware().Middleware(handler)

	// Outermost: assign/propagate the correlation ID and capture per-request
	// metadata so every audit event of the request is joinable.
	handler = audit.NewRequestContextMiddleware(c.config.Audit.ClientIPHeader).Middleware(handler)

	return &http.Server{
		Addr:         net.JoinHostPort(c.config.HTTPBindIP, strconv.Itoa(int(c.config.HTTPPort))),
		Handler:      handler,
		WriteTimeout: httpServerWriteTimeout,
		ReadTimeout:  httpServerReadTimeout,
		IdleTimeout:  httpServerIdleTimeout,
	}
}

func (c *Container) HTTPSServer() *http.Server {
	if c.httpsServer == nil {
		c.httpsServer = c.createHTTPSServer()
	}

	return c.httpsServer
}

func (c *Container) createHTTPSServer() *http.Server {
	var handler http.Handler = c.Router()

	handler = c.SecurityHeadersMiddleware().Middleware(handler)
	handler = audit.NewRequestContextMiddleware(c.config.Audit.ClientIPHeader).Middleware(handler)

	return &http.Server{
		Addr:         net.JoinHostPort(c.config.HTTPBindIP, strconv.Itoa(int(c.config.HTTPSPort))),
		Handler:      handler,
		WriteTimeout: httpServerWriteTimeout,
		ReadTimeout:  httpServerReadTimeout,
		IdleTimeout:  httpServerIdleTimeout,
	}
}

func (c *Container) SecurityHeadersMiddleware() *middlewares.SecurityHeadersMiddleware {
	if c.securityHeadersMiddleware != nil {
		return c.securityHeadersMiddleware
	}

	static, err := webstatic.GetFS()
	if err != nil {
		panic("security headers: failed to get static files: " + err.Error())
	}

	m, err := middlewares.NewSecurityHeadersMiddleware(c.config, static)
	if err != nil {
		panic("security headers: " + err.Error())
	}

	c.securityHeadersMiddleware = m

	return c.securityHeadersMiddleware
}

// I18nFS returns the translation filesystem served at /lang/: enabled plugins'
// contributed translations layered above the built-in i18n files, so a plugin
// file shadows a core file of the same name and a new locale file is simply
// added. Layers are resolved per request, so plugins loaded at runtime are
// reflected without a restart.
func (c *Container) I18nFS() fs.FS {
	if c.i18nFS == nil {
		base := i18n.GetFS()
		c.i18nFS = mergefs.NewDynamic(func() []fs.FS {
			return c.pluginAssetLayers(pluginI18nFS, base)
		})
	}

	return c.i18nFS
}

// FrontendFS returns the SPA filesystem served at /: enabled plugins'
// contributed frontend files layered above the built-in bundle. The base build
// is resolved once here (a broken embed fails fast at startup); plugin layers
// are resolved per request. The security-headers middleware deliberately hashes
// the base build instead, so plugin files cannot alter the CSP.
func (c *Container) FrontendFS() fs.FS {
	if c.frontendFS == nil {
		base, err := webstatic.GetFS()
		if err != nil {
			panic("failed to get static files: " + err.Error())
		}

		c.frontendFS = mergefs.NewDynamic(func() []fs.FS {
			return c.pluginAssetLayers(pluginFrontendFS, base)
		})
	}

	return c.frontendFS
}

// pluginAssetLayers returns the ordered layers for a merged filesystem: each
// enabled plugin's contributed filesystem (ordered by plugin ID, above the
// base), followed by base. Two plugins shipping the same path therefore shadow
// each other deterministically. Plugins are consulted only when the manager
// already exists, so a disabled plugin subsystem leaves just the base layer.
func (c *Container) pluginAssetLayers(pick func(*pkgplugin.LoadedPlugin) fs.FS, base fs.FS) []fs.FS {
	var layers []fs.FS

	if c.pluginManager != nil {
		for _, p := range c.pluginManager.GetPlugins() {
			if !p.IsEnabled() {
				continue
			}

			if pluginFS := pick(p); pluginFS != nil {
				layers = append(layers, pluginFS)
			}
		}
	}

	return append(layers, base)
}

func pluginI18nFS(p *pkgplugin.LoadedPlugin) fs.FS { return p.I18nFS }

func pluginFrontendFS(p *pkgplugin.LoadedPlugin) fs.FS { return p.FrontendFS }

func (c *Container) ACMEService() *acme.Service {
	if c.acmeService == nil {
		c.acmeService = c.createACMEService()
	}

	return c.acmeService
}

func (c *Container) createACMEService() *acme.Service {
	storage := acmestorage.NewFileStorage(c.FileManager(), c.config.ACME.StoragePath)

	var locker acme.Locker

	if c.config.Cache.Driver == cacheDriverRedis {
		redisCache, ok := c.Cache().(*cache.Redis)
		if ok {
			locker = acmelocker.NewRedisLocker(redisCache.Client())
		}
	}

	if locker == nil {
		locker = acmelocker.NewInMemoryLocker()
	}

	registry := acmedns.NewBuiltinRegistry()

	return acme.NewService(acme.ServiceConfig{
		ChallengeType:        c.config.ACME.ChallengeType,
		Email:                c.config.ACME.Email,
		Domains:              c.config.ACME.Domains,
		DirectoryURL:         c.config.ACME.DirectoryURL,
		DNSProvider:          c.config.ACME.DNSProvider,
		RenewalThreshold:     c.config.ACME.RenewalThreshold,
		RenewalCheckInterval: c.config.ACME.RenewalCheckInterval,
		PropagationTimeout:   c.config.ACME.PropagationTimeout,
	}, storage, locker, registry, slog.Default())
}

func (c *Container) Router() *http.ServeMux {
	if c.router == nil {
		c.router = internalapi.CreateRouter(c)
	}

	return c.router
}

func (c *Container) Responder() *api.Responder {
	if c.responder == nil {
		c.responder = c.createResponder()
	}

	return c.responder
}

func (c *Container) createResponder() *api.Responder {
	return api.NewResponder()
}

// FileUploadMIMEChecker returns the configured MIME allowlist checker used
// by the file-upload handler. Built once at first access; the configuration
// is sourced from Files.Upload.{AllowedMIMEs,AllowArchives,AllowBinary}.
func (c *Container) FileUploadMIMEChecker() *filemanagermime.Checker {
	if c.fileUploadMIMEChecker == nil {
		c.fileUploadMIMEChecker = filemanagermime.NewChecker(filemanagermime.Config{
			AllowedMIMEs:  c.config.Files.Upload.AllowedMIMEs,
			AllowArchives: c.config.Files.Upload.AllowArchives,
			AllowBinary:   c.config.Files.Upload.AllowBinary,
		})
	}

	return c.fileUploadMIMEChecker
}

// AuditLogger returns the structured security audit logger. When audit
// logging is disabled in config it returns a no-op logger so call sites
// never need to guard.
func (c *Container) AuditLogger() audit.Logger {
	if c.auditLogger == nil {
		if c.config.Audit.Enabled {
			c.auditLogger = audit.NewLogger(slog.Default())
		} else {
			c.auditLogger = audit.NopLogger{}
		}
	}

	return c.auditLogger
}

func (c *Container) UserRepository() repositories.UserRepository {
	if c.userRepository == nil {
		c.userRepository = c.createUserRepository()
	}

	return c.userRepository
}

func (c *Container) createUserRepository() repositories.UserRepository {
	var baseRepo repositories.UserRepository

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		baseRepo = mysql.NewUserRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		baseRepo = postgres.NewUserRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		baseRepo = sqlite.NewUserRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		baseRepo = inmemory.NewUserRepository()
	default:
		// Use in-memory repository as fallback
		baseRepo = inmemory.NewUserRepository()
	}

	// Wrap with cache if Redis is configured
	if c.config.Cache.Driver == cacheDriverRedis {
		ttl, err := time.ParseDuration(c.config.Cache.TTL.Users)
		if err != nil {
			ttl = 6 * time.Hour // Default to 6 hours
		}

		return cached.NewUserRepository(baseRepo, c.Cache(), ttl)
	}

	return baseRepo
}

func (c *Container) ServerControlService() *servercontrol.Service {
	if c.serverControlService == nil {
		c.serverControlService = c.createServerControlService()
	}

	return c.serverControlService
}

func (c *Container) createServerControlService() *servercontrol.Service {
	var opts []servercontrol.ServiceOption
	if !c.config.Plugins.Disabled {
		opts = append(opts, servercontrol.WithPluginDispatcher(
			pkgplugin.NewServerControlAdapter(c.PluginDispatcher()),
		))
	}

	opts = append(opts, servercontrol.WithTaskDispatcher(c.TaskDispatcher()))

	return servercontrol.NewService(
		c.DaemonTaskRepository(),
		c.ServerSettingRepository(),
		c.TransactionManager(),
		opts...,
	)
}

func (c *Container) TaskDispatcher() *taskdispatcher.Dispatcher {
	if c.taskDispatcher == nil {
		c.taskDispatcher = taskdispatcher.NewDispatcher(
			c.SessionRegistry(),
			c.DaemonTaskRepository(),
			c.ServerRepository(),
			c.ServerSettingRepository(),
			c.GameRepository(),
			c.GameModRepository(),
			c.NodeRepository(),
			c.PubSub(),
			slog.Default(),
		)
		if !c.config.Plugins.Disabled {
			c.taskDispatcher.SetPluginEventDispatcher(&lazyPluginTaskEvents{container: c})
		}
	}

	return c.taskDispatcher
}

func (c *Container) ServerTaskDispatcher() *servertaskdispatcher.Dispatcher {
	if c.serverTaskDispatcher == nil {
		c.serverTaskDispatcher = servertaskdispatcher.NewDispatcher(
			c.SessionRegistry(),
			c.ServerTaskRepository(),
			c.ServerTaskExecutionRepository(),
			c.PubSub(),
			slog.Default(),
		)
	}

	return c.serverTaskDispatcher
}

func (c *Container) ServerConfigPusher() *serverconfigpush.Pusher {
	if c.serverConfigPusher == nil {
		c.serverConfigPusher = serverconfigpush.NewPusher(
			c.SessionRegistry(),
			c.ServerRepository(),
			c.ServerSettingRepository(),
			c.GameRepository(),
			c.GameModRepository(),
			c.NodeRepository(),
			slog.Default(),
		)
	}

	return c.serverConfigPusher
}

func (c *Container) AuthService() auth.Service {
	if c.authService == nil {
		c.authService = c.createAuthService()
	}

	return c.authService
}

func (c *Container) createAuthService() auth.Service {
	if c.config.AuthSecret == "" {
		panic("auth secret is not set")
	}

	authSecret := auth.DecodeWithPrefix([]byte(c.config.AuthSecret))

	switch strings.ToLower(c.config.AuthService) {
	case "jwt":
		return auth.NewJWTService(authSecret)
	case "paseto":
		authService, err := auth.NewPASETOService(authSecret)
		if err != nil {
			panic(errors.WithMessage(err, "failed to create auth service"))
		}

		return authService
	default:
		panic("invalid auth service: " + c.config.AuthService)
	}
}

// TwoFactorManager provides TOTP, secret-encryption and recovery-code
// primitives. The at-rest encryption key is HKDF-derived from EncryptionKey
// when set, otherwise from the (always-present) AuthSecret, so existing
// installs need no new configuration.
func (c *Container) TwoFactorManager() *twofactor.Manager {
	if c.twoFactorManager == nil {
		c.twoFactorManager = c.createTwoFactorManager()
	}

	return c.twoFactorManager
}

func (c *Container) createTwoFactorManager() *twofactor.Manager {
	appKey := c.config.EncryptionKey
	if appKey == "" {
		appKey = c.config.AuthSecret
	}
	if appKey == "" {
		panic("neither ENCRYPTION_KEY nor AUTH_SECRET is set; cannot initialise two-factor manager")
	}

	manager, err := twofactor.NewManager([]byte(appKey))
	if err != nil {
		panic(errors.WithMessage(err, "failed to create two-factor manager"))
	}

	return manager
}

func (c *Container) UserService() *services.UserService {
	if c.userService == nil {
		c.userService = c.createUserService()
	}

	return c.userService
}

func (c *Container) createUserService() *services.UserService {
	return services.NewUserService(c.UserRepository())
}

func (c *Container) MFANudgeService() *mfanudge.Service {
	if c.mfaNudgeService == nil {
		c.mfaNudgeService = mfanudge.New(*c.config, nil)
	}

	return c.mfaNudgeService
}

func (c *Container) RBACRepository() repositories.RBACRepository {
	if c.rbacRepository == nil {
		c.rbacRepository = c.createRBACRepository()
	}

	return c.rbacRepository
}

func (c *Container) createRBACRepository() repositories.RBACRepository {
	var baseRepo repositories.RBACRepository

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		baseRepo = mysql.NewRBACRepository(c.TransactionalDB(), c.TransactionManager())
	case databaseDriverPostgres, databaseDriverPGX:
		baseRepo = postgres.NewRBACRepository(c.TransactionalDB(), c.TransactionManager())
	case databaseDriverSQLite:
		baseRepo = sqlite.NewRBACRepository(c.TransactionalDB(), c.TransactionManager())
	case databaseDriverInMemory:
		baseRepo = inmemory.NewRBACRepository()
	default:
		// Use in-memory repository as fallback
		baseRepo = inmemory.NewRBACRepository()
	}

	// Wrap with cache if Redis is configured
	if c.config.Cache.Driver == cacheDriverRedis {
		ttl, err := time.ParseDuration(c.config.Cache.TTL.RBAC)
		if err != nil {
			ttl = 24 * time.Hour // Default to 24 hours
		}

		return cached.NewRBACRepository(baseRepo, c.Cache(), ttl)
	}

	return baseRepo
}

func (c *Container) PersonalAccessTokenRepository() repositories.PersonalAccessTokenRepository {
	if c.personalAccessTokenRepository == nil {
		c.personalAccessTokenRepository = c.createPersonalAccessTokenRepository()
	}

	return c.personalAccessTokenRepository
}

func (c *Container) createPersonalAccessTokenRepository() repositories.PersonalAccessTokenRepository {
	var baseRepo repositories.PersonalAccessTokenRepository

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		baseRepo = mysql.NewPersonalAccessTokenRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		baseRepo = postgres.NewPersonalAccessTokenRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		baseRepo = sqlite.NewPersonalAccessTokenRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		baseRepo = inmemory.NewPersonalAccessTokenRepository()
	default:
		// Use in-memory repository as fallback
		baseRepo = inmemory.NewPersonalAccessTokenRepository()
	}

	// Wrap with cache if Redis is configured
	if c.config.Cache.Driver == cacheDriverRedis {
		ttl, err := time.ParseDuration(c.config.Cache.TTL.PersonalTokens)
		if err != nil {
			ttl = 24 * time.Hour // Default to 24 hours
		}

		return cached.NewPersonalAccessTokenRepository(baseRepo, c.Cache(), ttl)
	}

	return baseRepo
}

func (c *Container) DaemonTaskRepository() repositories.DaemonTaskRepository {
	if c.daemonTasksRepository == nil {
		c.daemonTasksRepository = c.createDaemonTaskRepository()
	}

	return c.daemonTasksRepository
}

func (c *Container) createDaemonTaskRepository() repositories.DaemonTaskRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewDaemonTaskRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewDaemonTaskRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewDaemonTaskRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewDaemonTaskRepository()
	default:
		// Use in-memory repository as fallback
		return inmemory.NewDaemonTaskRepository()
	}
}

func (c *Container) ServerTaskRepository() repositories.ServerTaskRepository {
	if c.serverTaskRepository == nil {
		c.serverTaskRepository = c.createServerTaskRepository()
	}

	return c.serverTaskRepository
}

func (c *Container) createServerTaskRepository() repositories.ServerTaskRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewServerTaskRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewServerTaskRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewServerTaskRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewServerTaskRepository(c.ServerRepository())
	default:
		// Use in-memory repository as fallback
		return inmemory.NewServerTaskRepository(c.ServerRepository())
	}
}

func (c *Container) ServerTaskExecutionRepository() repositories.ServerTaskExecutionRepository {
	if c.serverTaskExecutionRepository == nil {
		c.serverTaskExecutionRepository = c.createServerTaskExecutionRepository()
	}

	return c.serverTaskExecutionRepository
}

func (c *Container) createServerTaskExecutionRepository() repositories.ServerTaskExecutionRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewServerTaskExecutionRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewServerTaskExecutionRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewServerTaskExecutionRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewServerTaskExecutionRepository()
	default:
		return inmemory.NewServerTaskExecutionRepository()
	}
}

func (c *Container) ServerSettingRepository() repositories.ServerSettingRepository {
	if c.serverSettingRepository == nil {
		c.serverSettingRepository = c.createServerSettingRepository()
	}

	return c.serverSettingRepository
}

func (c *Container) createServerSettingRepository() repositories.ServerSettingRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewServerSettingRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewServerSettingRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewServerSettingRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewServerSettingRepository()
	default:
		// Use in-memory repository as fallback
		return inmemory.NewServerSettingRepository()
	}
}

func (c *Container) NodeRepository() repositories.NodeRepository {
	if c.nodeRepository == nil {
		c.nodeRepository = c.createNodeRepository()
	}

	return c.nodeRepository
}

func (c *Container) createNodeRepository() repositories.NodeRepository {
	var baseRepo repositories.NodeRepository

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		baseRepo = mysql.NewNodeRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		baseRepo = postgres.NewNodeRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		baseRepo = sqlite.NewNodeRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		baseRepo = inmemory.NewNodeRepository()
	default:
		// Use in-memory repository as fallback
		baseRepo = inmemory.NewNodeRepository()
	}

	// Wrap with cache if Redis is configured
	if c.config.Cache.Driver == cacheDriverRedis {
		ttl, err := time.ParseDuration(c.config.Cache.TTL.Nodes)
		if err != nil {
			ttl = 24 * time.Hour // Default to 24 hours
		}

		return cached.NewNodeRepository(baseRepo, c.Cache(), ttl)
	}

	return baseRepo
}

func (c *Container) RBAC() *rbac.RBAC {
	if c.rbac == nil {
		cacheTTL, err := time.ParseDuration(c.config.RBAC.CacheTTL)
		if err != nil {
			panic(errors.WithMessage(err, "invalid RBAC cache TTL"))
		}

		c.rbac = rbac.NewRBAC(
			c.TransactionManager(),
			c.RBACRepository(),
			cacheTTL,
		)

		c.appendShutdownFunc(func() error {
			c.rbac.Close()

			return nil
		})
	}

	return c.rbac
}

func (c *Container) ClientCertificateRepository() repositories.ClientCertificateRepository {
	if c.clientCertificateRepository == nil {
		c.clientCertificateRepository = c.createClientCertificateRepository()
	}

	return c.clientCertificateRepository
}

func (c *Container) createClientCertificateRepository() repositories.ClientCertificateRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewClientCertificateRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewClientCertificateRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewClientCertificateRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewClientCertificateRepository()
	default:
		return inmemory.NewClientCertificateRepository()
	}
}

func (c *Container) PluginStorageRepository() repositories.PluginStorageRepository {
	if c.pluginStorageRepository == nil {
		c.pluginStorageRepository = c.createPluginStorageRepository()
	}

	return c.pluginStorageRepository
}

func (c *Container) createPluginStorageRepository() repositories.PluginStorageRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewPluginStorageRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewPluginStorageRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewPluginStorageRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewPluginStorageRepository()
	default:
		return inmemory.NewPluginStorageRepository()
	}
}

func (c *Container) PluginSecretRepository() repositories.PluginSecretRepository {
	if c.pluginSecretRepository == nil {
		c.pluginSecretRepository = c.createPluginSecretRepository()
	}

	return c.pluginSecretRepository
}

func (c *Container) createPluginSecretRepository() repositories.PluginSecretRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewPluginSecretRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewPluginSecretRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewPluginSecretRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewPluginSecretRepository()
	default:
		return inmemory.NewPluginSecretRepository()
	}
}

func (c *Container) PluginScheduledTaskRepository() repositories.PluginScheduledTaskRepository {
	if c.pluginScheduledTaskRepository == nil {
		c.pluginScheduledTaskRepository = c.createPluginScheduledTaskRepository()
	}

	return c.pluginScheduledTaskRepository
}

func (c *Container) createPluginScheduledTaskRepository() repositories.PluginScheduledTaskRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewPluginScheduledTaskRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewPluginScheduledTaskRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewPluginScheduledTaskRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewPluginScheduledTaskRepository()
	default:
		return inmemory.NewPluginScheduledTaskRepository()
	}
}

func (c *Container) DLQRepository() repositories.DLQRepository {
	if c.dlqRepository == nil {
		c.dlqRepository = c.createDLQRepository()
	}

	return c.dlqRepository
}

func (c *Container) createDLQRepository() repositories.DLQRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewDLQRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewDLQRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewDLQRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewDLQRepository(c.config.PubSub.DLQ.MaxSize)
	default:
		return inmemory.NewDLQRepository(c.config.PubSub.DLQ.MaxSize)
	}
}

func (c *Container) Cache() cache.Cache {
	if c.cache == nil {
		c.cache = c.createCache()
	}

	return c.cache
}

func (c *Container) createCache() cache.Cache {
	switch c.config.Cache.Driver {
	case cacheDriverMemory, cacheDriverInmemory:
		return cache.NewInMemory()

	case "database", "mysql": // Using MySQL cache for "database" driver for backward compatibility
		return cache.NewMySQL(c.DB())

	case "postgres", "postgresql", "pgsql", "pg":
		return cache.NewPostgreSQL(c.DB())

	case "redis":
		redisCache, err := cache.NewRedis(
			c.config.Cache.Redis.Addr,
			c.config.Cache.Redis.Password,
			c.config.Cache.Redis.DB,
		)
		if err != nil {
			panic(errors.WithMessage(err, "failed to create Redis cache"))
		}

		c.appendLateShutdownFunc(func() error {
			return redisCache.Close()
		})

		return redisCache

	default:
		panic("invalid cache driver: " + c.config.Cache.Driver)
	}
}

func (c *Container) PubSub() pubsub.PubSub {
	if c.pubsub == nil {
		c.pubsub = c.createPubSub()
	}

	return c.pubsub
}

func (c *Container) createPubSub() pubsub.PubSub {
	basePubSub := c.createBasePubSub()

	if c.config.PubSub.Retry.Enabled {
		retryCfg := c.buildRetryConfig()
		var opts []retry.Option

		if c.config.PubSub.DLQ.Enabled {
			dlqStore := c.createDLQStore()
			dlqHandler := dlq.NewHandler(dlqStore, basePubSub)
			opts = append(opts, retry.WithDLQ(dlqHandler))
		}

		retryPublisher := retry.NewPublisher(basePubSub, retryCfg, opts...)

		return &wrappedPubSub{
			publisher: retryPublisher,
			PubSub:    basePubSub,
		}
	}

	return basePubSub
}

func (c *Container) createBasePubSub() pubsub.PubSub {
	switch c.config.PubSub.Driver {
	case pubsubDriverMemory, "":
		return pubsubmemory.New()

	case pubsubDriverRedis:
		addr := c.config.PubSub.Redis.Addr
		if addr == "" {
			addr = c.config.Cache.Redis.Addr
		}

		password := c.config.PubSub.Redis.Password
		if password == "" {
			password = c.config.Cache.Redis.Password
		}

		ps, err := pubsubredis.New(pubsubredis.Config{
			Addr:       addr,
			Password:   password,
			DB:         c.config.PubSub.Redis.DB,
			InstanceID: c.config.PubSub.InstanceID,
		})
		if err != nil {
			panic(errors.WithMessage(err, "failed to create Redis pub-sub"))
		}

		c.appendLateShutdownFunc(func() error {
			return ps.Close()
		})

		return ps

	case pubsubDriverPostgres:
		ps, err := pubsubpg.New(pubsubpg.Config{
			ConnStr:    c.config.DatabaseURL,
			InstanceID: c.config.PubSub.InstanceID,
		})
		if err != nil {
			panic(errors.WithMessage(err, "failed to create PostgreSQL pub-sub"))
		}

		c.appendLateShutdownFunc(func() error {
			return ps.Close()
		})

		return ps

	default:
		panic("invalid pub-sub driver: " + c.config.PubSub.Driver)
	}
}

func (c *Container) buildRetryConfig() retry.Config {
	cfg := retry.DefaultConfig()
	cfg.MaxRetries = c.config.PubSub.Retry.MaxRetries
	cfg.Multiplier = c.config.PubSub.Retry.Multiplier

	if d, err := time.ParseDuration(c.config.PubSub.Retry.InitialDelay); err == nil {
		cfg.InitialDelay = d
	}
	if d, err := time.ParseDuration(c.config.PubSub.Retry.MaxDelay); err == nil {
		cfg.MaxDelay = d
	}

	return cfg
}

func (c *Container) createDLQStore() dlq.Store {
	switch c.config.PubSub.DLQ.Driver {
	case "database", "db":
		return c.DLQRepository()
	default:
		return dlq.NewMemoryStore(c.config.PubSub.DLQ.MaxSize)
	}
}

type wrappedPubSub struct {
	pubsub.PubSub

	publisher pubsub.Publisher
}

func (w *wrappedPubSub) Publish(ctx context.Context, channel string, msg *pubsub.Message) error {
	return w.publisher.Publish(ctx, channel, msg)
}

func (c *Container) FileManager() files.FileManager {
	if c.fileManager == nil {
		c.fileManager = c.createFileManager()
	}

	return c.fileManager
}

func (c *Container) createFileManager() files.FileManager {
	switch c.config.Files.Driver {
	case filesDriverLocal:
		basePath := c.config.Files.Local.BasePath
		if basePath == "" {
			basePath = defaults.StoragePath
		}

		if basePath == "" {
			panic("local files base path is not set")
		}

		return files.NewLocalFileManager(basePath)
	case "s3", "minio":
		if c.config.Files.S3.Endpoint == "" {
			panic("s3 endpoint is not set")
		}

		if c.config.Files.S3.AccessKeyID == "" {
			panic("s3 access key id is not set")
		}

		if c.config.Files.S3.SecretAccessKey == "" {
			panic("s3 secret access key is not set")
		}

		if c.config.Files.S3.Bucket == "" {
			panic("s3 bucket is not set")
		}

		s3Client, err := files.NewS3FileManager(
			c.config.Files.S3.Endpoint,
			c.config.Files.S3.AccessKeyID,
			c.config.Files.S3.SecretAccessKey,
			c.config.Files.S3.Bucket,
			c.config.Files.S3.UseSSL,
		)
		if err != nil {
			panic(errors.WithMessage(err, "failed to create S3 client"))
		}

		return s3Client
	default:
		panic("invalid files driver: " + c.config.Files.Driver)
	}
}

func (c *Container) StreamFileManager() files.StreamFileManager {
	fm := c.FileManager()

	sfm, ok := fm.(files.StreamFileManager)
	if !ok {
		panic("file manager does not implement StreamFileManager")
	}

	return sfm
}

func (c *Container) CertificatesService() *certificates.Service {
	if c.certificatesService == nil {
		c.certificatesService = certificates.NewService(c.FileManager())
	}

	return c.certificatesService
}

func (c *Container) EnrollmentService() *enrollment.Service {
	if c.enrollmentService == nil {
		keyManager := enrollment.NewSetupKeyManager(c.Cache(), c.config.DaemonSetupKey)
		c.enrollmentService = enrollment.NewService(
			keyManager,
			c.NodeRepository(),
			c.ClientCertificateRepository(),
			c.CertificatesService(),
		)
	}

	return c.enrollmentService
}

func (c *Container) GRPCPort() uint16 {
	return c.config.GRPC.Port
}

func (c *Container) GRPCExternalHost() string {
	return c.config.GRPC.ExternalHost
}

func (c *Container) GRPCExternalPort() uint16 {
	return c.config.GRPC.ExternalPort
}

// GRPCCertHostCovered reports whether the gRPC TLS server certificate of this
// instance covers host. Returns true when gRPC TLS is disabled or the
// certificate has not been loaded: there is nothing to warn about then.
func (c *Container) GRPCCertHostCovered(host string) bool {
	if c.grpcServerCertLeaf == nil {
		return true
	}

	return c.grpcServerCertLeaf.VerifyHostname(host) == nil
}

func (c *Container) GlobalAPIService() *services.GlobalAPIService {
	if c.globalAPIService == nil {
		c.globalAPIService = c.createGlobalAPIService()
	}

	return c.globalAPIService
}

func (c *Container) createGlobalAPIService() *services.GlobalAPIService {
	return services.NewGlobalAPIService(c.Config())
}

func (c *Container) CDNGamesService() *services.CDNGamesService {
	if c.cdnGamesService == nil {
		c.cdnGamesService = c.createCDNGamesService()
	}

	return c.cdnGamesService
}

func (c *Container) createCDNGamesService() *services.CDNGamesService {
	return services.NewCDNGamesService(c.Config())
}

// CaptchaVerifier returns the login captcha verifier. It is a no-op
// (Enabled() == false) until CAPTCHA_PROVIDER and CAPTCHA_SECRET_KEY are set.
func (c *Container) CaptchaVerifier() *captcha.Service {
	if c.captchaVerifier == nil {
		c.captchaVerifier = c.createCaptchaVerifier()
	}

	return c.captchaVerifier
}

func (c *Container) createCaptchaVerifier() *captcha.Service {
	cfg := c.Config().Captcha

	return captcha.NewService(captcha.Config{
		Provider:  captcha.Provider(cfg.Provider),
		SiteKey:   cfg.SiteKey,
		SecretKey: cfg.SecretKey,
		MinScore:  cfg.MinScore,
		FailOpen:  cfg.FailOpen,
		VerifyURL: cfg.VerifyURL,
	})
}

func (c *Container) PluginStoreService() *pluginstore.Service {
	if c.pluginStoreService == nil {
		c.pluginStoreService = pluginstore.NewService(
			c.config.PluginStore.URL,
			c.config.PluginStore.LicenseKey,
			c.Cache(),
		)
	}

	return c.pluginStoreService
}

func (c *Container) GameUpgradeService() *services.GameUpgradeService {
	if c.gameUpgrader == nil {
		c.gameUpgrader = c.createGameUpgradeService()
	}

	return c.gameUpgrader
}

func (c *Container) createGameUpgradeService() *services.GameUpgradeService {
	return services.NewGameUpgradeService(
		c.CDNGamesService(),
		c.GameRepository(),
		c.GameModRepository(),
		c.TransactionManager(),
	)
}

func (c *Container) PelicanEggImporter() *pelicaneggimporter.Importer {
	if c.pelicanEggImporter == nil {
		c.pelicanEggImporter = pelicaneggimporter.NewImporter(
			c.GameRepository(),
			c.GameModRepository(),
			c.TransactionManager(),
		)
	}

	return c.pelicanEggImporter
}

func (c *Container) GameAPImporter() *gameapimporter.Importer {
	if c.gameAPImporter == nil {
		c.gameAPImporter = gameapimporter.NewImporter(
			c.GameRepository(),
			c.GameModRepository(),
			c.TransactionManager(),
		)
	}

	return c.gameAPImporter
}

func (c *Container) GameExporter() *gameexporter.Exporter {
	if c.gameExporter == nil {
		c.gameExporter = gameexporter.NewExporter(
			c.GameRepository(),
			c.GameModRepository(),
			"",
		)
	}

	return c.gameExporter
}

func (c *Container) DaemonStatus() *daemon.StatusService {
	if c.daemonStatus == nil {
		c.daemonStatus = daemon.NewStatusService(
			c.GatewayService(),
			c.SessionRegistry(),
			c.StatusDispatcher(),
			slog.Default(),
		)
	}

	return c.daemonStatus
}

func (c *Container) DaemonFiles() *daemon.FileService {
	if c.daemonFiles == nil {
		c.daemonFiles = daemon.NewFileService(
			c.GatewayService(),
			c.SessionRegistry(),
			c.FileDispatcher(),
			c.StreamFileManager(),
			c.TransferRegistry(),
			slog.Default(),
		)
	}

	return c.daemonFiles
}

func (c *Container) DaemonArchive() *daemon.ArchiveService {
	if c.daemonArchive == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}

		c.daemonArchive = daemon.NewArchiveService(
			c.PubSub(),
			c.GatewayService(),
			c.SessionRegistry(),
			instanceID,
			daemon.ArchiveLimits{
				MaxTotalBytes: c.config.Files.Archive.MaxBytes.Uint64(),
				MaxFiles:      c.config.Files.Archive.MaxFiles,
			},
			slog.Default(),
		)
		c.GatewayService().SetArchiveProgressHandler(c.daemonArchive)
	}

	return c.daemonArchive
}

func (c *Container) UploadSessionService() *upload.Service {
	if c.uploadSessionService == nil {
		c.uploadSessionService = upload.NewService(
			c.StreamFileManager(),
			c.DaemonFiles(),
			upload.RealClock(),
			slog.Default(),
			upload.Config{
				ChunkSize:             c.config.Files.Upload.ChunkSize.Uint64(),
				SessionTTL:            c.config.Files.Upload.SessionTTL,
				MaxChunks:             c.config.Files.Upload.MaxChunks,
				DaemonDispatchTimeout: c.config.Files.Upload.DispatchTimeout,
			},
		)
	}

	return c.uploadSessionService
}

func (c *Container) UploadJanitor() *upload.Janitor {
	if c.uploadJanitor == nil {
		c.uploadJanitor = upload.NewJanitor(
			c.StreamFileManager(),
			upload.RealClock(),
			c.config.Files.Upload.JanitorInterval,
			slog.Default(),
		)
	}

	return c.uploadJanitor
}

func (c *Container) FileManagerArchiver() *archiver.Archiver {
	if c.fileManagerArchiver == nil {
		c.fileManagerArchiver = archiver.NewArchiver(
			c.DaemonFiles(),
			c.DaemonFiles(),
			slog.Default(),
		)
	}

	return c.fileManagerArchiver
}

func (c *Container) FileManagerArchiveGuard() *archiver.InMemoryConcurrencyGuard {
	if c.fileManagerArchiveGuard == nil {
		c.fileManagerArchiveGuard = archiver.NewInMemoryConcurrencyGuard(
			c.config.Files.Archive.ConcurrentPerServer,
		)
	}

	return c.fileManagerArchiveGuard
}

func (c *Container) FileDispatcher() daemon.FileDispatcher {
	if c.fileDispatcher == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}

		c.fileDispatcher = daemon.NewFileDispatcher(
			c.PubSub(),
			c.GatewayService(),
			c.SessionRegistry(),
			c.StreamFileManager(),
			instanceID,
			slog.Default(),
		)
	}

	return c.fileDispatcher
}

func (c *Container) DaemonCommands() *daemon.CommandService {
	if c.daemonCommands == nil {
		c.daemonCommands = daemon.NewCommandService(
			c.GatewayService(),
			c.SessionRegistry(),
			c.CommandDispatcher(),
			slog.Default(),
		)
	}

	return c.daemonCommands
}

func (c *Container) CommandDispatcher() daemon.CommandDispatcher {
	if c.commandDispatcher == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}

		c.commandDispatcher = daemon.NewCommandDispatcher(
			c.PubSub(),
			c.GatewayService(),
			c.SessionRegistry(),
			instanceID,
			slog.Default(),
		)
	}

	return c.commandDispatcher
}

func (c *Container) StatusDispatcher() daemon.StatusDispatcher {
	if c.statusDispatcher == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}

		c.statusDispatcher = daemon.NewStatusDispatcher(
			c.PubSub(),
			c.GatewayService(),
			c.SessionRegistry(),
			instanceID,
			slog.Default(),
		)
	}

	return c.statusDispatcher
}

func (c *Container) ConsoleLogDispatcher() daemon.ConsoleLogDispatcher {
	if c.consoleLogDispatcher == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}

		c.consoleLogDispatcher = daemon.NewConsoleLogDispatcher(
			c.PubSub(),
			c.GatewayService(),
			c.SessionRegistry(),
			instanceID,
			slog.Default(),
		)
	}

	return c.consoleLogDispatcher
}

func (c *Container) ConsoleLogService() *daemon.ConsoleLogService {
	if c.daemonConsoleLog == nil {
		c.daemonConsoleLog = daemon.NewConsoleLogService(
			c.GatewayService(),
			c.SessionRegistry(),
			c.ConsoleLogDispatcher(),
			slog.Default(),
		)
	}

	return c.daemonConsoleLog
}

func (c *Container) HTTPProxyDispatcher() daemon.HTTPProxyDispatcher {
	if c.httpProxyDispatcher == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}

		c.httpProxyDispatcher = daemon.NewHTTPProxyDispatcher(
			c.PubSub(),
			c.GatewayService(),
			c.SessionRegistry(),
			c.StreamFileManager(),
			instanceID,
			slog.Default(),
		)
	}

	return c.httpProxyDispatcher
}

func (c *Container) HTTPProxyService() *daemon.HTTPProxyService {
	if c.daemonHTTPProxy == nil {
		c.daemonHTTPProxy = daemon.NewHTTPProxyService(
			c.GatewayService(),
			c.SessionRegistry(),
			c.HTTPProxyDispatcher(),
			slog.Default(),
		)
	}

	return c.daemonHTTPProxy
}

func (c *Container) PluginManager() *pkgplugin.Manager {
	if c.pluginManager == nil {
		c.pluginManager = c.createPluginManager()

		c.appendShutdownFunc(func() error {
			return c.pluginManager.Shutdown(c.context)
		})
	}

	return c.pluginManager
}

func (c *Container) PluginArchiveEvents() *pluginarchive.Service {
	if c.pluginArchiveEvents == nil {
		c.pluginArchiveEvents = pluginarchive.New(
			c.PluginManager(),
			c.PluginLoader(),
			c.PubSub(),
			pluginarchive.Options{},
			slog.Default(),
		)
	}

	return c.pluginArchiveEvents
}

func (c *Container) PluginScheduler() *pluginscheduler.Service {
	if c.pluginScheduler == nil {
		c.pluginScheduler = pluginscheduler.New(
			c.PluginScheduledTaskRepository(),
			c.PluginManager(),
			c.PluginLoader(),
			c.SchedulerLocker(),
			pluginscheduler.Options{
				MinInterval:        c.config.Plugin.Scheduler.MinInterval,
				MaxTasksPerPlugin:  c.config.Plugin.Scheduler.MaxTasksPerPlugin,
				DefaultCallTimeout: c.config.Plugin.Scheduler.CallTimeout,
				MaxCallTimeout:     c.config.Plugin.Scheduler.MaxCallTimeout,
				MaxRetries:         c.config.Plugin.Scheduler.MaxRetries,
				MaxRetryDelay:      c.config.Plugin.Scheduler.MaxRetryDelay,
				MaxJitter:          c.config.Plugin.Scheduler.MaxJitter,
				RefreshInterval:    c.config.Plugin.Scheduler.RefreshInterval,
			},
			slog.Default(),
		)
	}

	return c.pluginScheduler
}

// SchedulerLocker picks the strongest available coordination backend: Redis
// when the cache runs on it, otherwise the shared database (kv_store table);
// the in-memory database has no shared medium, so the lock stays local.
func (c *Container) SchedulerLocker() locker.Locker {
	if c.schedulerLocker == nil {
		c.schedulerLocker = c.createSchedulerLocker()
	}

	return c.schedulerLocker
}

func (c *Container) createSchedulerLocker() locker.Locker {
	if c.config.Cache.Driver == cacheDriverRedis {
		if redisCache, ok := c.Cache().(*cache.Redis); ok {
			return locker.NewRedisLocker(redisCache.Client(), "gameap:lock:")
		}
	}

	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return locker.NewDBLocker(c.TransactionalDB(), locker.DBDialectMySQL)
	case databaseDriverPostgres, databaseDriverPGX:
		return locker.NewDBLocker(c.TransactionalDB(), locker.DBDialectPostgres)
	case databaseDriverSQLite:
		return locker.NewDBLocker(c.TransactionalDB(), locker.DBDialectSQLite)
	case databaseDriverInMemory:
		return locker.NewInMemoryLocker()
	default:
		return locker.NewInMemoryLocker()
	}
}

func (c *Container) connRegistry() *pkgplugin.ConnRegistry {
	if c.netConnRegistry == nil {
		c.netConnRegistry = pkgplugin.NewConnRegistry(c.config.Plugin.Net.MaxConnections)
	}

	return c.netConnRegistry
}

func (c *Container) createPluginManager() *pkgplugin.Manager {
	guard := c.PluginGuard()

	factories := []pkgplugin.HostLibraryFactory{
		hostlibrary.NewStorageHostLibraryFactory(
			c.PluginStorageRepository(),
			hostlibrary.WithStorageQuotas(hostlibrary.StorageConfig{
				MaxKeysPerPlugin: c.config.Plugin.Storage.MaxKeysPerPlugin,
				MaxValueBytes:    int(c.config.Plugin.Storage.MaxValue.Uint64()), //nolint:gosec
				MaxTotalBytes:    c.config.Plugin.Storage.MaxTotal.Uint64(),
			}),
		),
		hostlibrary.NewLogHostLibraryFactory(slog.Default()),
		// Per-plugin: the module is gated on the plugin's own
		// manage_rbac grant, so it needs to know which plugin it serves.
		hostlibrary.NewRBACHostLibraryFactory(c.RBAC(), c.RBACRepository(), guard),
		hostlibrary.NewSchedulerHostLibraryFactory(&lazyTaskScheduler{container: c}),
		// Per-plugin: secrets are scoped to the owning plugin and the module
		// is gated on its own secrets grant.
		hostlibrary.NewSecretsHostLibraryFactory(
			c.PluginSecretRepository(),
			c.SecretCipher(),
			guard,
			hostlibrary.SecretsConfig{
				MaxKeysPerPlugin:  c.config.Plugin.Secrets.MaxKeysPerPlugin,
				MaxValueBytes:     int(c.config.Plugin.Secrets.MaxValue.Uint64()), //nolint:gosec // a byte cap fits an int
				RequireEncryption: c.config.Plugin.Secrets.RequireEncryption,
			},
		),
		// Per-plugin: the module is gated on the plugin's own files grant
		// and archive callbacks must reach the initiating plugin.
		hostlibrary.NewNodeFSHostLibraryFactory(
			c.DaemonFiles(),
			c.NodeRepository(),
			c.DaemonArchive(),
			&lazyArchiveEvents{container: c},
			guard,
			hostlibrary.WithNodeFSMaxInlineBytes(c.config.Plugin.NodeFS.MaxInline.Uint64()),
		),
		// Per-plugin: writes are gated on manage_servers / node_commands,
		// rate limited and audited with the plugin as the actor.
		hostlibrary.NewServersHostLibraryFactory(c.ServerRepository(), guard),
		hostlibrary.NewDaemonTasksHostLibraryFactory(c.DaemonTaskRepository(), c.TaskDispatcher(), guard),
		hostlibrary.NewServerSettingsHostLibraryFactory(c.ServerSettingRepository(), guard),
		hostlibrary.NewServerControlHostLibraryFactory(
			c.ServerRepository(),
			&lazyServerController{container: c},
			guard,
		),
		hostlibrary.NewNodeCmdHostLibraryFactory(c.DaemonCommands(), c.NodeRepository(), guard),
		// Per-plugin: every plugin gets its own cache namespace.
		hostlibrary.NewCacheHostLibraryFactory(
			c.Cache(),
			"plugin:",
			hostlibrary.WithCacheMaxValueBytes(int(c.config.Plugin.Cache.MaxValue.Uint64())), //nolint:gosec
		),
		// Per-plugin: outbound requests are rate limited per plugin.
		hostlibrary.NewHTTPHostLibraryFactory(hostlibrary.HTTPConfig{
			BlockPrivateIPs:         c.config.Plugin.HTTP.BlockPrivateIPs,
			AllowedSchemes:          c.config.Plugin.HTTP.AllowedSchemes,
			AllowedHosts:            c.config.Plugin.HTTP.AllowedHosts,
			MaxTimeout:              c.config.Plugin.HTTP.MaxTimeout,
			MaxRedirects:            c.config.Plugin.HTTP.MaxRedirects,
			ResponseHeaderAllowlist: c.config.Plugin.HTTP.ResponseHeaderAllowlist,
		}, guard),
	}

	if c.config.Plugin.Net.Enabled {
		factories = append(factories, hostlibrary.NewNetHostLibraryFactory(
			c.connRegistry(),
			hostlibrary.NetConfig{
				MaxReadBytes: int(c.config.Plugin.Net.ReadBuffer.Uint64()), //nolint:gosec // a buffer size fits an int
				MaxTimeout:   c.config.Plugin.Net.MaxTimeout,
			},
		))
	}

	metrics := c.PluginMetrics()
	recovery := &lazyPluginRecovery{container: c}

	return pkgplugin.NewManager(pkgplugin.ManagerConfig{
		// Read-only modules need no plugin binding.
		Libraries: []pkgplugin.HostLibrary{
			hostlibrary.NewUsersHostLibrary(c.UserRepository()),
			hostlibrary.NewNodesHostLibrary(c.NodeRepository()),
			hostlibrary.NewGamesHostLibrary(c.GameRepository()),
			hostlibrary.NewGameModsHostLibrary(c.GameModRepository()),
			hostlibrary.NewCryptoHostLibrary(),
			hostlibrary.NewAuthzHostLibrary(c.RBAC()),
		},
		LibraryFactories: factories,

		MaxMemoryBytes: c.config.Plugin.Runtime.MaxMemory.Uint64(),
		//nolint:gosec // a module size cap fits an int
		MaxModuleBytes:          int(c.config.Plugin.Runtime.MaxModuleSize.Uint64()),
		CompilationCacheDir:     c.config.Plugins.Cache.Dir,
		DisableCompilationCache: !c.config.Plugins.Cache.Enabled,
		GuestLogger:             slog.Default(),
		Observer:                metrics,
		// Resolved at call time: the supervisor is created by PluginLoader(),
		// which depends on the manager built here.
		OnPluginDisabled: func(pluginID string, dbID uint64, reason string) {
			metrics.OnPluginDisabled(pluginID, dbID, reason)
			recovery.OnPluginDisabled(pluginID, dbID, reason)
		},
	})
}

// PluginGuard is the shared grant / rate-limit / audit enforcement in front
// of the privileged plugin host libraries.
func (c *Container) PluginGuard() *hostlibrary.Guard {
	if c.pluginGuard == nil {
		limits := c.config.Plugin.RateLimit

		c.pluginGuard = hostlibrary.NewGuard(
			c.PluginPermissionChecker(),
			hostlibrary.WithGuardRateLimits(map[hostlibrary.RateClass]hostlibrary.RateLimit{
				hostlibrary.RateClassNodeCmd:       {RPS: limits.NodeCmd.RPS, Burst: limits.NodeCmd.Burst},
				hostlibrary.RateClassServerControl: {RPS: limits.ServerControl.RPS, Burst: limits.ServerControl.Burst},
				hostlibrary.RateClassNodeFS:        {RPS: limits.NodeFS.RPS, Burst: limits.NodeFS.Burst},
				hostlibrary.RateClassHTTP:          {RPS: limits.HTTP.RPS, Burst: limits.HTTP.Burst},
				hostlibrary.RateClassRBAC:          {RPS: limits.RBAC.RPS, Burst: limits.RBAC.Burst},
			}),
			hostlibrary.WithGuardAudit(c.AuditLogger()),
			hostlibrary.WithGuardObserver(c.PluginMetrics()),
		)
	}

	return c.pluginGuard
}

// PluginPermissionChecker is the shared view of plugin grants behind the host
// libraries, the event delivery gate and file refs. One cache per instance, so
// an invalidation drops the answer for all three at once.
func (c *Container) PluginPermissionChecker() *hostlibrary.CachedPermissionChecker {
	if c.pluginPermissions == nil {
		c.pluginPermissions = hostlibrary.NewCachedPermissionChecker(
			hostlibrary.NewRepositoryPermissionChecker(c.PluginRepository()),
			c.config.Plugin.Permissions.CacheTTL,
		)
	}

	return c.pluginPermissions
}

// Telemetry is the panel's Prometheus registry.
func (c *Container) Telemetry() *telemetry.Registry {
	if c.telemetry == nil {
		c.telemetry = telemetry.New()
	}

	return c.telemetry
}

// PluginMetrics collects the plugin runtime metrics. The manager and the
// dispatcher are resolved lazily at scrape time: both depend on the metrics
// (as observer) while being built.
func (c *Container) PluginMetrics() *telemetry.PluginMetrics {
	if c.pluginMetrics == nil {
		c.pluginMetrics = telemetry.NewPluginMetrics(
			c.Telemetry(),
			&lazyPluginLister{container: c},
			&lazyPluginBacklog{container: c},
		)
	}

	return c.pluginMetrics
}

// lazyPluginLister resolves the plugin manager at scrape time.
type lazyPluginLister struct {
	container *Container
}

func (l *lazyPluginLister) GetPlugins() []*pkgplugin.LoadedPlugin {
	if l.container.pluginManager == nil {
		return nil
	}

	return l.container.pluginManager.GetPlugins()
}

// lazyPluginBacklog resolves the plugin dispatcher at scrape time.
type lazyPluginBacklog struct {
	container *Container
}

func (l *lazyPluginBacklog) AsyncBacklog() int {
	if l.container.pluginDispatcher == nil {
		return 0
	}

	return l.container.pluginDispatcher.AsyncBacklog()
}

// pluginMemoryLimitBytes converts the configured megabytes to the manager's
// byte cap; a non-positive value keeps the wazero default.
// lazyPluginRecovery forwards runtime disables to the recovery supervisor,
// which PluginLoader() creates after the manager; hooks only fire after
// LoadAll, so the supervisor exists by then.
type lazyPluginRecovery struct {
	container *Container
}

func (l *lazyPluginRecovery) OnPluginDisabled(pluginID string, dbID uint64, reason string) {
	if l.container.pluginRecovery == nil {
		slog.Warn("plugin disabled at runtime before the loader was built, not recorded",
			slog.String("plugin", pluginID),
			slog.Uint64("plugin_id", dbID),
			slog.String("reason", reason))

		return
	}

	l.container.pluginRecovery.OnPluginDisabled(pluginID, dbID, reason)
}

// lazyArchiveEvents defers PluginArchiveEvents resolution to call time: the
// dispatcher needs PluginManager to invoke callbacks, while the manager's
// host libraries need the dispatcher — resolving it during manager
// construction would recurse.
type lazyArchiveEvents struct {
	container *Container
}

func (l *lazyArchiveEvents) Register(pluginID uint64, operationID string, nodeID uint64, reportProgress bool) {
	l.container.PluginArchiveEvents().Register(pluginID, operationID, nodeID, reportProgress)
}

func (l *lazyArchiveEvents) NotifyCompleted(operationID string, result messages.ArchiveCompleteEventPayload) {
	l.container.PluginArchiveEvents().NotifyCompleted(operationID, result)
}

// lazyTaskScheduler defers PluginScheduler resolution to call time: the
// scheduler needs PluginManager to invoke handlers, while the manager's host
// libraries need the scheduler — resolving it during manager construction
// would recurse.
type lazyTaskScheduler struct {
	container *Container
}

func (l *lazyTaskScheduler) AddTask(ctx context.Context, task domain.PluginScheduledTask) error {
	return l.container.PluginScheduler().AddTask(ctx, task)
}

func (l *lazyTaskScheduler) RemoveTask(ctx context.Context, pluginID domain.Uint64ID, name string) error {
	return l.container.PluginScheduler().RemoveTask(ctx, pluginID, name)
}

func (l *lazyTaskScheduler) ListTasks(
	ctx context.Context,
	pluginID domain.Uint64ID,
) ([]domain.PluginScheduledTask, error) {
	return l.container.PluginScheduler().ListTasks(ctx, pluginID)
}

// lazyPluginTaskEvents defers PluginDispatcher resolution to call time:
// the plugin manager's host libraries depend on TaskDispatcher and the gRPC
// gateway (via TaskHandler), so resolving the dispatcher during their
// construction would recurse.
type lazyPluginTaskEvents struct {
	container *Container
}

func (l *lazyPluginTaskEvents) DispatchTaskEventAsync(
	ctx context.Context,
	eventType pluginproto.EventType,
	taskID, nodeID uint,
	serverID *uint,
	taskType, status string,
	extraData map[string]string,
) {
	l.container.PluginDispatcher().DispatchTaskEventAsync(
		ctx, eventType, taskID, nodeID, serverID, taskType, status, extraData,
	)
}

// lazyServerController is a wrapper that lazily resolves the ServerControlService to break circular deps.
type lazyServerController struct {
	container *Container
}

func (l *lazyServerController) Start(ctx context.Context, server *domain.Server) (uint, error) {
	return l.container.ServerControlService().Start(ctx, server)
}

func (l *lazyServerController) Stop(ctx context.Context, server *domain.Server) (uint, error) {
	return l.container.ServerControlService().Stop(ctx, server)
}

func (l *lazyServerController) Restart(ctx context.Context, server *domain.Server) (uint, error) {
	return l.container.ServerControlService().Restart(ctx, server)
}

func (l *lazyServerController) Update(ctx context.Context, server *domain.Server) (uint, error) {
	return l.container.ServerControlService().Update(ctx, server)
}

func (l *lazyServerController) Install(ctx context.Context, server *domain.Server) (uint, error) {
	return l.container.ServerControlService().Install(ctx, server)
}

func (l *lazyServerController) Reinstall(ctx context.Context, server *domain.Server) (uint, error) {
	return l.container.ServerControlService().Reinstall(ctx, server)
}

func (c *Container) PluginDispatcher() *pkgplugin.Dispatcher {
	if c.pluginDispatcher == nil {
		checker := c.PluginPermissionChecker()

		c.pluginDispatcher = pkgplugin.NewDispatcher(
			c.PluginManager(),
			slog.Default(),
			pkgplugin.WithDispatcherObserver(c.PluginMetrics()),
			// Event subscriptions are gated on the plugin's listen_events grant.
			pkgplugin.WithSubscriptionGate(func(ctx context.Context, plugin *pkgplugin.LoadedPlugin) bool {
				allowed, err := checker.Has(ctx, plugin.DBID, domain.PluginPermissionListenEvents)
				if err != nil {
					slog.ErrorContext(ctx, "failed to check plugin listen_events permission",
						slog.Uint64("plugin_id", plugin.DBID),
						slog.String("error", err.Error()))

					return false
				}

				return allowed
			}),
		)
	}

	return c.pluginDispatcher
}

// PluginSubscriptionsNotifier keeps the event subscriptions of every panel
// instance in step after a permission change.
func (c *Container) PluginSubscriptionsNotifier() *pubsubintegration.PluginSubscriptionsNotifier {
	if c.pluginSubscriptionsPS == nil {
		c.pluginSubscriptionsPS = pubsubintegration.NewPluginSubscriptionsNotifier(
			c.PubSub(),
			c.PluginDispatcher(),
			pubsubintegration.WithPermissionCache(c.PluginPermissionChecker()),
		)
	}

	return c.pluginSubscriptionsPS
}

func (c *Container) PluginRepository() repositories.PluginRepository {
	if c.pluginRepository == nil {
		c.pluginRepository = c.createPluginRepository()
	}

	return c.pluginRepository
}

func (c *Container) createPluginRepository() repositories.PluginRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewPluginRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewPluginRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewPluginRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewPluginRepository()
	default:
		return inmemory.NewPluginRepository()
	}
}

func (c *Container) PluginLoader() *internalplugin.Loader {
	if c.pluginLoader == nil {
		c.pluginLoader = internalplugin.NewLoader(
			c.PluginManager(),
			c.FileManager(),
			c.PluginRepository(),
			c.config.Plugins.AutoLoad,
			c.PluginsDir(),
			internalplugin.WithStrictLoad(c.config.Plugins.StrictLoad),
			internalplugin.WithSubscriptionRefresher(c.PluginDispatcher()),
		)

		// Always present: it records why a plugin was disabled even when
		// automatic reloads are switched off.
		c.pluginRecovery = internalplugin.NewSupervisor(
			c.pluginLoader,
			c.PluginRepository(),
			c.AuditLogger(),
			internalplugin.RecoveryOptions{
				InitialDelay:  c.config.Plugin.Recovery.InitialDelay,
				MaxDelay:      c.config.Plugin.Recovery.MaxDelay,
				MaxAttempts:   c.config.Plugin.Recovery.MaxAttempts,
				DisableReload: !c.config.Plugin.Recovery.Enabled,
			},
			slog.Default(),
		)
	}

	return c.pluginLoader
}

// PluginRecovery returns the supervisor that records runtime disables and,
// unless PLUGIN_RECOVERY_ENABLED is off, reloads the plugins.
func (c *Container) PluginRecovery() *internalplugin.Supervisor {
	c.PluginLoader()

	return c.pluginRecovery
}

func (c *Container) PluginsDir() string {
	return "plugins"
}

func (c *Container) WSHub() *ws.Hub {
	if c.wsHub == nil {
		c.wsHub = ws.NewHub(slog.Default())
		c.appendShutdownFunc(func() error {
			c.wsHub.Close()

			return nil
		})
	}

	return c.wsHub
}

func (c *Container) WSBridge() *ws.Bridge {
	if c.wsBridge == nil {
		c.wsBridge = ws.NewBridge(c.WSHub(), c.PubSub(), slog.Default())
	}

	return c.wsBridge
}

func (c *Container) SessionRegistry() *session.Registry {
	if c.sessionRegistry == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}
		c.sessionRegistry = session.NewRegistry(c.PubSub(), instanceID, slog.Default())
		c.sessionRegistry.SetMetricsWaiterRegistrar(c.MetricsHandler())
	}

	return c.sessionRegistry
}

func (c *Container) TaskHandler() *handlers.TaskHandler {
	if c.taskHandler == nil {
		var pluginEvents handlers.PluginEventDispatcher
		if !c.config.Plugins.Disabled {
			pluginEvents = &lazyPluginTaskEvents{container: c}
		}

		c.taskHandler = handlers.NewTaskHandler(
			c.DaemonTaskRepository(),
			c.ServerRepository(),
			c.PubSub(),
			pluginEvents,
			slog.Default(),
		)
	}

	return c.taskHandler
}

func (c *Container) CommandHandler() *handlers.CommandHandler {
	if c.commandHandler == nil {
		c.commandHandler = handlers.NewCommandHandler(c.PubSub(), slog.Default())
	}

	return c.commandHandler
}

func (c *Container) ServerStatusHandler() *handlers.ServerStatusHandler {
	if c.serverStatusHandler == nil {
		c.serverStatusHandler = handlers.NewServerStatusHandler(c.ServerRepository(), slog.Default())
	}

	return c.serverStatusHandler
}

func (c *Container) AttachHandler() *handlers.AttachHandler {
	if c.attachHandler == nil {
		c.attachHandler = handlers.NewAttachHandler(c.PubSub(), slog.Default())
	}

	return c.attachHandler
}

func (c *Container) MetricsHandler() *handlers.MetricsHandler {
	if c.metricsHandler == nil {
		c.metricsHandler = handlers.NewMetricsHandler(c.PubSub(), c.ServerRepository(), slog.Default())
	}

	return c.metricsHandler
}

func (c *Container) MetricsHub() metrics.Hub {
	if c.metricsHub == nil {
		instanceID := c.config.PubSub.InstanceID
		if instanceID == "" {
			instanceID = defaultInstanceID
		}
		c.metricsHub = metrics.NewHub(
			c.PubSub(),
			c.SessionRegistry(),
			c.MetricsHandler(),
			instanceID,
			slog.Default(),
			metrics.Options{},
		)

		c.appendShutdownFunc(func() error {
			c.metricsHub.Stop()

			return nil
		})
	}

	return c.metricsHub
}

func (c *Container) TaskReaper() *taskreaper.Reaper {
	if c.taskReaper == nil {
		c.taskReaper = taskreaper.NewReaper(
			c.DaemonTaskRepository(),
			c.SessionRegistry(),
			c.TaskHandler(),
			taskreaper.Options{
				Interval:       c.config.TaskReaper.Interval,
				StaleThreshold: c.config.TaskReaper.StaleThreshold,
			},
			slog.Default(),
		)
	}

	return c.taskReaper
}

func (c *Container) GatewayService() *gateway.Service {
	if c.gatewayService == nil {
		c.gatewayService = gateway.NewService(
			c.SessionRegistry(),
			c.NodeRepository(),
			c.ServerRepository(),
			c.ServerSettingRepository(),
			c.DaemonTaskRepository(),
			c.GameRepository(),
			c.GameModRepository(),
			nil,
			c.TaskHandler(),
			c.TaskDispatcher(),
			c.CommandHandler(),
			c.ServerStatusHandler(),
			c.AttachHandler(),
			c.MetricsHandler(),
			c.ServerTaskDispatcher(),
			c.EnrollmentService(),
			slog.Default(),
		)
		c.gatewayService.SetShutdownContext(c.context)
	}

	return c.gatewayService
}

func (c *Container) FileTransferService() *filetransfer.Service {
	if c.fileTransferService == nil {
		c.fileTransferService = filetransfer.NewService(
			c.StreamFileManager(),
			c.PubSub(),
			c.TransferRegistry(),
			slog.Default(),
		)
	}

	return c.fileTransferService
}

func (c *Container) TransferRegistry() *transfers.Registry {
	if c.transferRegistry == nil {
		c.transferRegistry = transfers.NewRegistry()
	}

	return c.transferRegistry
}

func (c *Container) grpcTLSConfig() (*tls.Config, error) {
	if !c.config.GRPC.TLSEnabled {
		slog.Warn("gRPC server is running without TLS. It is recommended to enable TLS for security")

		if c.config.GRPC.RequireMTLS {
			slog.Warn("GRPC_REQUIRE_MTLS is enabled but GRPC_TLS_ENABLED is false; mTLS will not work without TLS")
		}

		return nil, nil
	}

	tlsConfig, err := c.buildGRPCTLSConfig()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build gRPC TLS config")
	}

	slog.Info("gRPC TLS configured successfully")

	return tlsConfig, nil
}

func (c *Container) GRPCServer() (*grpc.Server, error) {
	if c.grpcServer == nil {
		tlsConfig, err := c.grpcTLSConfig()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to configure gRPC TLS")
		}

		c.grpcServer = internalgrpc.NewServer(
			&internalgrpc.ServerConfig{
				MaxRecvMsgSize:       c.config.GRPC.MaxRecvMsgSize,
				MaxSendMsgSize:       c.config.GRPC.MaxSendMsgSize,
				MaxConcurrentStreams: c.config.GRPC.MaxConcurrentStreams,
				RequireMTLS:          c.config.GRPC.RequireMTLS,
				FileTransferBasePath: c.config.GRPC.FileTransferBasePath,
				EnableReflection:     c.config.GRPC.EnableReflection,
				TLSConfig:            tlsConfig,
			},
			&internalgrpc.ServerDependencies{
				GatewayService:      c.GatewayService(),
				FileTransferService: c.FileTransferService(),
				NodeRepo:            c.NodeRepository(),
				Logger:              slog.Default(),
			},
		)
	}

	return c.grpcServer, nil
}

func (c *Container) buildGRPCTLSConfig() (*tls.Config, error) {
	ctx := c.context
	certSvc := c.CertificatesService()

	certPEM, keyPEM, err := certSvc.EnsureGenerated(ctx,
		certificates.ServerCertificatesPath+"/api-server.crt",
		certificates.ServerCertificatesPath+"/api-server.key",
		c.grpcCertSignOptions(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure gRPC server certificate")
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load gRPC server certificate")
	}

	if leaf, parseErr := x509.ParseCertificate(cert.Certificate[0]); parseErr == nil {
		c.grpcServerCertLeaf = leaf
		slog.Info("gRPC TLS server certificate loaded",
			"common_name", leaf.Subject.CommonName,
			"dns_names", leaf.DNSNames,
			"ip_addresses", ipAddressesToStrings(leaf.IPAddresses),
			"not_before", leaf.NotBefore.Format(time.RFC3339),
			"not_after", leaf.NotAfter.Format(time.RFC3339),
			"mtls_required", c.config.GRPC.RequireMTLS,
		)
	} else {
		slog.Warn("failed to parse gRPC TLS server certificate for logging",
			"error", parseErr.Error(),
		)
	}

	tlsCfg := tlsutil.HardenServerConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
		GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			remoteAddr := "unknown"
			if chi.Conn != nil && chi.Conn.RemoteAddr() != nil {
				remoteAddr = chi.Conn.RemoteAddr().String()
			}

			slog.Debug("gRPC TLS ClientHello received",
				"remote_addr", remoteAddr,
				"server_name", chi.ServerName,
				"supported_versions", chi.SupportedVersions,
				"supported_protos", chi.SupportedProtos,
			)

			return nil, nil //nolint:nilnil // returning nil config means "use the outer tlsCfg"
		},
		VerifyConnection: func(state tls.ConnectionState) error {
			attrs := []any{
				"tls_version", tlsVersionName(state.Version),
				"cipher_suite", tls.CipherSuiteName(state.CipherSuite),
				"server_name", state.ServerName,
				"negotiated_protocol", state.NegotiatedProtocol,
			}

			if len(state.PeerCertificates) > 0 {
				peerCert := state.PeerCertificates[0]
				attrs = append(attrs,
					"client_cn", peerCert.Subject.CommonName,
					"client_serial", peerCert.SerialNumber.String(),
					"client_not_after", peerCert.NotAfter.Format(time.RFC3339),
				)
			}

			slog.Info("gRPC TLS handshake completed", attrs...)

			return nil
		},
	})

	if c.config.GRPC.RequireMTLS {
		rootCAPEM, err := certSvc.Root(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load root CA for mTLS")
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM([]byte(rootCAPEM)) {
			return nil, errors.New("failed to add root CA to pool")
		}

		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		tlsCfg.ClientCAs = caPool
	}

	return tlsCfg, nil
}

func (c *Container) grpcCertSignOptions() *certificates.SignOptions {
	sanSources := resolveGRPCCertSANs(c.config.HTTPHost, c.config.HTTPBindIP, c.config.GRPC.ExternalHost)
	ipAddresses, dnsNames := splitSANs(sanSources)

	ipsLog, dnsLog := formatSANSourcesForLog(sanSources)
	slog.Info("gRPC TLS certificate SANs resolved",
		slog.Any("ips", ipsLog),
		slog.Any("dns_names", dnsLog),
	)

	return &certificates.SignOptions{
		CommonName:  "GameAP API Server",
		IPAddresses: ipAddresses,
		DNSNames:    dnsNames,
	}
}

func (c *Container) MultiplexedServer() (*MultiplexedServer, error) {
	if c.multiplexedServer != nil {
		return c.multiplexedServer, nil
	}

	tlsConfig, err := c.buildMultiplexerTLSConfig()
	if err != nil {
		return nil, err
	}

	addr := c.getMultiplexerAddress()

	grpcSrv, err := c.GRPCServer()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create gRPC server")
	}

	server, err := NewMultiplexedServer(c.context, &MultiplexerConfig{
		Address:    addr,
		TLSConfig:  tlsConfig,
		GRPCServer: grpcSrv,
		HTTPServer: c.HTTPServer(),
		Logger:     slog.Default(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "create multiplexed server")
	}

	c.multiplexedServer = server

	return c.multiplexedServer, nil
}

func (c *Container) buildMultiplexerTLSConfig() (*tls.Config, error) {
	if !c.config.TLSEnabled() {
		return nil, nil
	}

	cert, err := c.config.LoadTLSCertificate()
	if err != nil {
		return nil, errors.Wrap(err, "load TLS certificate")
	}

	return tlsutil.HardenServerConfig(&tls.Config{
		Certificates: []tls.Certificate{*cert},
	}), nil
}

func (c *Container) getMultiplexerAddress() string {
	if c.config.TLSEnabled() {
		return net.JoinHostPort(c.config.HTTPBindIP, strconv.Itoa(int(c.config.HTTPSPort)))
	}

	return net.JoinHostPort(c.config.HTTPBindIP, strconv.Itoa(int(c.config.HTTPPort)))
}
