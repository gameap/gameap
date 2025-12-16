package application

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	trmsql "github.com/avito-tech/go-transaction-manager/drivers/sql/v2"
	trmcontext "github.com/avito-tech/go-transaction-manager/trm/v2/context"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	internalapi "github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/cached"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/repositories/mysql"
	"github.com/gameap/gameap/internal/repositories/postgres"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
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
	cacheDriverMySQL    = "mysql"
	cacheDriverRedis    = "redis"
)

const (
	httpServerWriteTimeout = 15 * time.Second
	httpServerReadTimeout  = 15 * time.Second
	httpServerIdleTimeout  = 60 * time.Second
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
	serverTaskFailRepository      repositories.ServerTaskFailRepository
	serverSettingRepository       repositories.ServerSettingRepository
	nodeRepository                repositories.NodeRepository
	clientCertificateRepository   repositories.ClientCertificateRepository

	// Services
	authService          auth.Service
	userService          *services.UserService
	serverControlService *servercontrol.Service
	globalAPIService     *services.GlobalAPIService
	gameUpgrader         *services.GameUpgradeService
	rbac                 *rbac.RBAC
	cache                cache.Cache
	fileManager          files.FileManager
	certificatesService  *certificates.Service

	// Daemon Services
	daemonStatus   *daemon.StatusService
	daemonFiles    *daemon.FileService
	daemonCommands *daemon.CommandService

	// Plugins
	pluginManager    *pkgplugin.Manager
	pluginDispatcher *pkgplugin.Dispatcher
	pluginRepository repositories.PluginRepository
	pluginLoader     *internalplugin.Loader

	// HTTP
	router      *http.ServeMux
	httpServer  *http.Server
	httpsServer *http.Server
	responder   *api.Responder

	// Shutdown
	shotdownFuncs []func() error
}

func NewContainer(config *config.Config) *Container {
	return &Container{
		config: config,
	}
}

func (c *Container) SetContext(ctx context.Context) {
	c.context = ctx
}

func (c *Container) Shutdown() error {
	for _, fn := range c.shotdownFuncs {
		if err := fn(); err != nil {
			slog.Error(
				"failed to execute shutdown function",
				slog.String("error", err.Error()),
			)
		}
	}

	return nil
}

func (c *Container) appendShutdownFunc(fn func() error) {
	c.shotdownFuncs = append(c.shotdownFuncs, fn)
}

func (c *Container) Config() *config.Config {
	return c.config
}

func (c *Container) DB() *sql.DB {
	if c.db == nil {
		db, err := c.createDB()
		if err != nil {
			panic(err)
		}

		c.db = db

		c.appendShutdownFunc(func() error {
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

	err = db.PingContext(c.context)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ping database")
	}

	return db, nil
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

		c.appendShutdownFunc(func() error {
			err := c.httpServer.Shutdown(c.context)

			if err == nil {
				slog.InfoContext(c.context, "http server shutdown succeeded")
			}

			return err
		})
	}

	return c.httpServer
}

func (c *Container) createHTTPServer() *http.Server {
	var handler http.Handler = c.Router()

	if c.config.TLSEnabled() && c.config.TLS.ForceHTTPS {
		handler = middlewares.HTTPSRedirectMiddleware(c.config.HTTPSPort)(handler)
	}

	return &http.Server{
		Addr:         c.config.HTTPHost + ":" + strconv.Itoa(int(c.config.HTTPPort)),
		Handler:      handler,
		WriteTimeout: httpServerWriteTimeout,
		ReadTimeout:  httpServerReadTimeout,
		IdleTimeout:  httpServerIdleTimeout,
	}
}

func (c *Container) HTTPSServer() *http.Server {
	if c.httpsServer == nil {
		c.httpsServer = c.createHTTPSServer()

		c.appendShutdownFunc(func() error {
			err := c.httpsServer.Shutdown(c.context)

			if err == nil {
				slog.InfoContext(c.context, "https server shutdown succeeded")
			}

			return err
		})
	}

	return c.httpsServer
}

func (c *Container) createHTTPSServer() *http.Server {
	handler := c.Router()

	return &http.Server{
		Addr:         c.config.HTTPHost + ":" + strconv.Itoa(int(c.config.HTTPSPort)),
		Handler:      handler,
		WriteTimeout: httpServerWriteTimeout,
		ReadTimeout:  httpServerReadTimeout,
		IdleTimeout:  httpServerIdleTimeout,
	}
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

	return servercontrol.NewService(
		c.DaemonTaskRepository(),
		c.ServerSettingRepository(),
		c.TransactionManager(),
		opts...,
	)
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

func (c *Container) UserService() *services.UserService {
	if c.userService == nil {
		c.userService = c.createUserService()
	}

	return c.userService
}

func (c *Container) createUserService() *services.UserService {
	return services.NewUserService(c.UserRepository())
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

func (c *Container) ServerTaskFailRepository() repositories.ServerTaskFailRepository {
	if c.serverTaskFailRepository == nil {
		c.serverTaskFailRepository = c.createServerTaskFailRepository()
	}

	return c.serverTaskFailRepository
}

func (c *Container) createServerTaskFailRepository() repositories.ServerTaskFailRepository {
	switch c.config.DatabaseDriver {
	case databaseDriverMySQL:
		return mysql.NewServerTaskFailRepository(c.TransactionalDB())
	case databaseDriverPostgres, databaseDriverPGX:
		return postgres.NewServerTaskFailRepository(c.TransactionalDB())
	case databaseDriverSQLite:
		return sqlite.NewServerTaskFailRepository(c.TransactionalDB())
	case databaseDriverInMemory:
		return inmemory.NewServerTaskFailRepository()
	default:
		// Use in-memory repository as fallback
		return inmemory.NewServerTaskFailRepository()
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

func (c *Container) Cache() cache.Cache {
	if c.cache == nil {
		c.cache = c.createCache()
	}

	return c.cache
}

func (c *Container) createCache() cache.Cache {
	switch c.config.Cache.Driver {
	case "memory", "inmemory":
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

		c.appendShutdownFunc(func() error {
			if rc, ok := c.cache.(*cache.Redis); ok {
				return rc.Close()
			}

			return nil
		})

		return redisCache

	default:
		panic("invalid cache driver: " + c.config.Cache.Driver)
	}
}

func (c *Container) FileManager() files.FileManager {
	if c.fileManager == nil {
		c.fileManager = c.createFileManager()
	}

	return c.fileManager
}

func (c *Container) createFileManager() files.FileManager {
	switch c.config.Files.Driver {
	case "local":
		basePath := c.config.Files.Local.BasePath
		if basePath == "" {
			basePath = path.Join(c.config.Legacy.Path, "storage", "app")
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

func (c *Container) CertificatesService() *certificates.Service {
	if c.certificatesService == nil {
		c.certificatesService = certificates.NewService(c.FileManager())
	}

	return c.certificatesService
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

func (c *Container) GameUpgradeService() *services.GameUpgradeService {
	if c.gameUpgrader == nil {
		c.gameUpgrader = c.createGameUpgradeService()
	}

	return c.gameUpgrader
}

func (c *Container) createGameUpgradeService() *services.GameUpgradeService {
	return services.NewGameUpgradeService(
		c.GlobalAPIService(),
		c.GameRepository(),
		c.GameModRepository(),
		c.TransactionManager(),
	)
}

func (c *Container) DaemonStatus() *daemon.StatusService {
	if c.daemonStatus == nil {
		c.daemonStatus = daemon.NewStatusService(
			c.ClientCertificateRepository(),
			c.FileManager(),
		)
	}

	return c.daemonStatus
}

func (c *Container) DaemonFiles() *daemon.FileService {
	if c.daemonFiles == nil {
		c.daemonFiles = daemon.NewFileService(
			c.ClientCertificateRepository(),
			c.FileManager(),
		)
	}

	return c.daemonFiles
}

func (c *Container) DaemonCommands() *daemon.CommandService {
	if c.daemonCommands == nil {
		c.daemonCommands = daemon.NewCommandService(
			c.ClientCertificateRepository(),
			c.FileManager(),
		)
	}

	return c.daemonCommands
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

func (c *Container) createPluginManager() *pkgplugin.Manager {
	return pkgplugin.NewManager(pkgplugin.ManagerConfig{
		Libraries: []pkgplugin.HostLibrary{
			hostlibrary.NewServersHostLibrary(c.ServerRepository()),
			hostlibrary.NewUsersHostLibrary(c.UserRepository()),
			hostlibrary.NewNodesHostLibrary(c.NodeRepository()),
			hostlibrary.NewGamesHostLibrary(c.GameRepository()),
			hostlibrary.NewGameModsHostLibrary(c.GameModRepository()),
			hostlibrary.NewDaemonTasksHostLibrary(c.DaemonTaskRepository()),
			hostlibrary.NewServerSettingsHostLibrary(c.ServerSettingRepository()),
			hostlibrary.NewServerControlHostLibrary(
				c.ServerRepository(),
				&lazyServerController{container: c},
			),
			hostlibrary.NewCacheHostLibrary(c.Cache(), "plugin:"),
			hostlibrary.NewHTTPHostLibrary(),
			hostlibrary.NewLogHostLibrary(slog.Default()),
			hostlibrary.NewNodeFSHostLibrary(c.DaemonFiles(), c.NodeRepository()),
			hostlibrary.NewNodeCmdHostLibrary(c.DaemonCommands(), c.NodeRepository()),
		},
	})
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
		c.pluginDispatcher = pkgplugin.NewDispatcher(c.PluginManager(), slog.Default())
	}

	return c.pluginDispatcher
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
			"plugins",
		)
	}

	return c.pluginLoader
}
