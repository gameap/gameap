package testcontainer

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/gameap/gameap/internal/acme"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagermime"
	getqueryapi "github.com/gameap/gameap/internal/api/servers/getquery"
	rconbase "github.com/gameap/gameap/internal/api/servers/rcon/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/files"
	grpchandlers "github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/i18n"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/metrics"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
	"github.com/gameap/gameap/internal/pubsub"
	pubsubmemory "github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/quercon"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/inmemory"
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
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/gameap/gameap/internal/services/serverconfigpush"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/internal/services/servertaskdispatcher"
	"github.com/gameap/gameap/internal/services/taskdispatcher"
	"github.com/gameap/gameap/internal/telemetry"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/internal/ws"
	pkgapi "github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/secret"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/gameap/gameap/pkg/twofactor"
	webstatic "github.com/gameap/gameap/web/static"
	"github.com/samber/lo"
)

type InmemoryContainer struct {
	cfg                     *config.Config
	telemetry               *telemetry.Registry
	responder               *pkgapi.Responder
	gameRepo                repositories.GameRepository
	gameModRepo             repositories.GameModRepository
	serverRepo              repositories.ServerRepository
	userRepo                repositories.UserRepository
	authService             auth.Service
	twoFactorManager        *twofactor.Manager
	userService             *services.UserService
	mfaNudgeService         *mfanudge.Service
	rbacRepo                repositories.RBACRepository
	tokenRepo               repositories.PersonalAccessTokenRepository
	daemonTaskRepo          repositories.DaemonTaskRepository
	serverTaskRepo          repositories.ServerTaskRepository
	serverTaskExecutionRepo repositories.ServerTaskExecutionRepository
	serverSettingRepo       repositories.ServerSettingRepository
	nodeRepo                repositories.NodeRepository
	clientCertificateRepo   repositories.ClientCertificateRepository
	rbacService             *rbac.RBAC
	serverControlService    *servercontrol.Service
	gameUpgradeService      *services.GameUpgradeService
	fileManager             files.FileManager
	cacheService            cache.Cache
	certificatesService     *certificates.Service
	globalAPIService        *services.GlobalAPIService
	daemonStatusService     *daemon.StatusService
	daemonFilesService      *daemon.FileService
	daemonCommandsService   *daemon.CommandService
	uploadSessionService    *upload.Service
	auditLogger             audit.Logger
	pluginScheduler         *pluginscheduler.Service
	pluginArchiveEvents     *pluginarchive.Service
}

func (c *InmemoryContainer) Config() *config.Config                            { return c.cfg }
func (c *InmemoryContainer) SecretCipher() *secret.Cipher                      { return secret.Disabled() }
func (c *InmemoryContainer) DB() *sql.DB                                       { return nil }
func (c *InmemoryContainer) TransactionManager() base.TransactionManager       { return nil }
func (c *InmemoryContainer) Responder() *pkgapi.Responder                      { return c.responder }
func (c *InmemoryContainer) GameRepository() repositories.GameRepository       { return c.gameRepo }
func (c *InmemoryContainer) GameModRepository() repositories.GameModRepository { return c.gameModRepo }
func (c *InmemoryContainer) ServerRepository() repositories.ServerRepository   { return c.serverRepo }
func (c *InmemoryContainer) UserRepository() repositories.UserRepository       { return c.userRepo }
func (c *InmemoryContainer) AuthService() auth.Service                         { return c.authService }
func (c *InmemoryContainer) TwoFactorManager() *twofactor.Manager              { return c.twoFactorManager }
func (c *InmemoryContainer) UserService() *services.UserService                { return c.userService }
func (c *InmemoryContainer) MFANudgeService() *mfanudge.Service {
	if c.mfaNudgeService == nil {
		c.mfaNudgeService = mfanudge.New(*c.cfg, nil)
	}

	return c.mfaNudgeService
}
func (c *InmemoryContainer) ServerControlService() *servercontrol.Service {
	return c.serverControlService
}
func (c *InmemoryContainer) GameUpgradeService() *services.GameUpgradeService {
	return c.gameUpgradeService
}
func (c *InmemoryContainer) RBACRepository() repositories.RBACRepository { return c.rbacRepo }
func (c *InmemoryContainer) PersonalAccessTokenRepository() repositories.PersonalAccessTokenRepository {
	return c.tokenRepo
}
func (c *InmemoryContainer) DaemonTaskRepository() repositories.DaemonTaskRepository {
	return c.daemonTaskRepo
}
func (c *InmemoryContainer) ServerTaskRepository() repositories.ServerTaskRepository {
	return c.serverTaskRepo
}
func (c *InmemoryContainer) ServerTaskExecutionRepository() repositories.ServerTaskExecutionRepository {
	return c.serverTaskExecutionRepo
}
func (c *InmemoryContainer) ServerSettingRepository() repositories.ServerSettingRepository {
	return c.serverSettingRepo
}
func (c *InmemoryContainer) NodeRepository() repositories.NodeRepository { return c.nodeRepo }
func (c *InmemoryContainer) ClientCertificateRepository() repositories.ClientCertificateRepository {
	return c.clientCertificateRepo
}
func (c *InmemoryContainer) RBAC() *rbac.RBAC                             { return c.rbacService }
func (c *InmemoryContainer) FileManager() files.FileManager               { return c.fileManager }
func (c *InmemoryContainer) Cache() cache.Cache                           { return c.cacheService }
func (c *InmemoryContainer) CertificatesService() *certificates.Service   { return c.certificatesService }
func (c *InmemoryContainer) GlobalAPIService() *services.GlobalAPIService { return c.globalAPIService }
func (c *InmemoryContainer) CaptchaVerifier() *captcha.Service {
	return captcha.NewService(captcha.Config{})
}
func (c *InmemoryContainer) DaemonStatus() *daemon.StatusService     { return c.daemonStatusService }
func (c *InmemoryContainer) DaemonFiles() *daemon.FileService        { return c.daemonFilesService }
func (c *InmemoryContainer) DaemonArchive() *daemon.ArchiveService   { return nil }
func (c *InmemoryContainer) UploadSessionService() *upload.Service   { return c.uploadSessionService }
func (c *InmemoryContainer) FileManagerArchiver() *archiver.Archiver { return nil }
func (c *InmemoryContainer) FileManagerArchiveGuard() *archiver.InMemoryConcurrencyGuard {
	return archiver.NewInMemoryConcurrencyGuard(2)
}
func (c *InmemoryContainer) DaemonCommands() *daemon.CommandService       { return c.daemonCommandsService }
func (c *InmemoryContainer) ConsoleLogService() *daemon.ConsoleLogService { return nil }
func (c *InmemoryContainer) PluginManager() *plugin.Manager               { return nil }
func (c *InmemoryContainer) QuerconResolver() *quercon.Resolver {
	return quercon.New(quercon.Config{
		BuiltinRconProtocol:  rconbase.DetermineProtocol,
		BuiltinQueryProtocol: getqueryapi.QueryProtocolByEngine,
		BuiltinPlayerManager: rconbase.DeterminePlayerManager,
	})
}
func (c *InmemoryContainer) PluginDispatcher() *plugin.Dispatcher { return nil }

func (c *InmemoryContainer) I18nFS() fs.FS { return i18n.GetFS() }

func (c *InmemoryContainer) FrontendFS() fs.FS {
	fsys, err := webstatic.GetFS()
	if err != nil {
		panic("testcontainer: failed to get static files: " + err.Error())
	}

	return fsys
}

func (c *InmemoryContainer) PluginRepository() repositories.PluginRepository {
	return inmemory.NewPluginRepository()
}

func (c *InmemoryContainer) PluginStorageRepository() repositories.PluginStorageRepository {
	return inmemory.NewPluginStorageRepository()
}

func (c *InmemoryContainer) PluginSecretRepository() repositories.PluginSecretRepository {
	return inmemory.NewPluginSecretRepository()
}
func (c *InmemoryContainer) PluginLoader() *internalplugin.Loader { return nil }

func (c *InmemoryContainer) PluginPathPolicy() *hostlibrary.PathPolicy {
	return hostlibrary.DefaultPathPolicy()
}

func (c *InmemoryContainer) PluginSync() *pluginsync.Service { return nil }

// Telemetry is a fresh registry per container so tests never share metric
// state.
func (c *InmemoryContainer) Telemetry() *telemetry.Registry {
	if c.telemetry == nil {
		c.telemetry = telemetry.New()
	}

	return c.telemetry
}

// PluginScheduler is cached so every caller shares one task store: a handler
// registering tasks and one cleaning them up on uninstall must see the same
// registrations.
func (c *InmemoryContainer) PluginScheduler() *pluginscheduler.Service {
	if c.pluginScheduler == nil {
		c.pluginScheduler = pluginscheduler.New(
			inmemory.NewPluginScheduledTaskRepository(),
			nil,
			nil,
			locker.NewInMemoryLocker(),
			pluginscheduler.Options{},
			nil,
		)
	}

	return c.pluginScheduler
}
func (c *InmemoryContainer) PluginArchiveEvents() *pluginarchive.Service {
	if c.pluginArchiveEvents == nil {
		c.pluginArchiveEvents = pluginarchive.New(nil, nil, pubsubmemory.New(), pluginarchive.Options{}, nil)
	}

	return c.pluginArchiveEvents
}

func (c *InmemoryContainer) PluginStoreService() *pluginstore.Service         { return nil }
func (c *InmemoryContainer) PluginsDir() string                               { return "plugins" }
func (c *InmemoryContainer) PelicanEggImporter() *pelicaneggimporter.Importer { return nil }
func (c *InmemoryContainer) GameAPImporter() *gameapimporter.Importer         { return nil }
func (c *InmemoryContainer) GameExporter() *gameexporter.Exporter             { return nil }
func (c *InmemoryContainer) TaskDispatcher() *taskdispatcher.Dispatcher       { return nil }
func (c *InmemoryContainer) ServerTaskDispatcher() *servertaskdispatcher.Dispatcher {
	return nil
}
func (c *InmemoryContainer) ServerConfigPusher() *serverconfigpush.Pusher { return nil }
func (c *InmemoryContainer) WSHub() *ws.Hub                               { return ws.NewHub(nil) }
func (c *InmemoryContainer) SessionRegistry() *session.Registry           { return nil }
func (c *InmemoryContainer) CommandHandler() *grpchandlers.CommandHandler { return nil }
func (c *InmemoryContainer) AttachHandler() *grpchandlers.AttachHandler   { return nil }
func (c *InmemoryContainer) MetricsHub() metrics.Hub                      { return nil }
func (c *InmemoryContainer) PubSub() pubsub.PubSub                        { return nil }
func (c *InmemoryContainer) ACMEService() *acme.Service                   { return nil }

// AuditLogger returns the configured audit logger, defaulting to a no-op.
// Tests asserting on audit output inject a capturing logger via
// SetAuditLogger.
func (c *InmemoryContainer) AuditLogger() audit.Logger {
	if c.auditLogger == nil {
		c.auditLogger = audit.NopLogger{}
	}

	return c.auditLogger
}

// SetAuditLogger overrides the audit logger (e.g. with a capturing logger
// in audit-assertion tests).
func (c *InmemoryContainer) SetAuditLogger(l audit.Logger) {
	c.auditLogger = l
}

// FileUploadMIMEChecker returns a permissive MIME checker for tests so
// existing upload integrations (which use opaque random bodies) keep
// passing. Tests that exercise the C-8 rejection path construct their
// own Checker explicitly.
func (c *InmemoryContainer) FileUploadMIMEChecker() *filemanagermime.Checker {
	return filemanagermime.NewChecker(filemanagermime.Config{
		AllowArchives: true,
		AllowBinary:   true,
	})
}
func (c *InmemoryContainer) EnrollmentService() *enrollment.Service {
	keyManager := enrollment.NewSetupKeyManager(c.cacheService, "")

	return enrollment.NewService(
		keyManager,
		c.nodeRepo,
		c.clientCertificateRepo,
		c.certificatesService,
	)
}

func (c *InmemoryContainer) GRPCPort() uint16         { return 31718 }
func (c *InmemoryContainer) GRPCExternalHost() string { return "" }
func (c *InmemoryContainer) GRPCExternalPort() uint16 { return 0 }

func (c *InmemoryContainer) GRPCCertHostCovered(_ string) bool { return true }

type nopUploader struct{}

func (nopUploader) UploadStreamPrepared(
	context.Context, *domain.Node, string, string, string, uint64, daemon.OwnerOptions,
) error {
	return nil
}

func LoadInmemoryContainer() (*InmemoryContainer, error) {
	c := buildInmemoryTestContainer()

	return c, nil
}

func buildInmemoryTestContainer() *InmemoryContainer {
	userRepo := inmemory.NewUserRepository()
	rbacRepo := inmemory.NewRBACRepository()
	serverRepo := inmemory.NewServerRepository()

	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()
	tm := services.NewNilTransactionManager()

	twoFactorManager, tfErr := twofactor.NewManager([]byte("test-encryption-key-testing"))
	if tfErr != nil {
		panic(tfErr)
	}

	c := &InmemoryContainer{
		cfg: &config.Config{
			AuthSecret:    "test-secret-key-for-testing",
			EncryptionKey: "test-encryption-key-testing",
		},
		responder:               pkgapi.NewResponder(),
		gameRepo:                inmemory.NewGameRepository(),
		gameModRepo:             inmemory.NewGameModRepository(),
		serverRepo:              serverRepo,
		userRepo:                userRepo,
		authService:             auth.NewJWTService([]byte("test-secret-key-for-testing")),
		twoFactorManager:        twoFactorManager,
		userService:             services.NewUserService(userRepo),
		rbacRepo:                rbacRepo,
		tokenRepo:               inmemory.NewPersonalAccessTokenRepository(),
		daemonTaskRepo:          daemonTaskRepo,
		serverTaskRepo:          inmemory.NewServerTaskRepository(serverRepo),
		serverTaskExecutionRepo: inmemory.NewServerTaskExecutionRepository(),
		serverSettingRepo:       serverSettingRepo,
		nodeRepo:                inmemory.NewNodeRepository(),
		clientCertificateRepo:   inmemory.NewClientCertificateRepository(),
		// Use a very short cache TTL so that role/permission changes are observed
		// immediately by tests (e.g. revoking admin must remove access on the next request).
		rbacService:           rbac.NewRBAC(tm, rbacRepo, time.Millisecond),
		serverControlService:  servercontrol.NewService(daemonTaskRepo, serverSettingRepo, tm),
		gameUpgradeService:    nil,
		fileManager:           nil,
		cacheService:          cache.NewInMemory(),
		certificatesService:   nil,
		globalAPIService:      nil,
		daemonStatusService:   nil,
		daemonFilesService:    nil,
		daemonCommandsService: nil,
	}

	c.uploadSessionService = upload.NewService(
		files.NewInMemoryFileManager(),
		nopUploader{},
		upload.RealClock(),
		nil,
		upload.Config{
			ChunkSize:  1 << 20,
			SessionTTL: 24 * time.Hour,
			MaxChunks:  1000,
		},
	)

	ctx := context.Background()

	err := rbacRepo.SaveRole(ctx, &domain.Role{
		ID:   1,
		Name: "admin",
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create 'admin' role: %v", err))
	}

	err = rbacRepo.SaveRole(ctx, &domain.Role{
		ID:   2,
		Name: "user",
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create 'user' role: %v", err))
	}

	adminAbility := &domain.Ability{
		ID:   25,
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	err = rbacRepo.SaveAbility(ctx, adminAbility)
	if err != nil {
		panic(fmt.Sprintf("failed to create admin ability: %v", err))
	}

	adminPermission := &domain.Permission{
		AbilityID:  adminAbility.ID,
		EntityID:   new(uint(1)),
		EntityType: lo.ToPtr(domain.EntityTypeRole),
		Forbidden:  false,
	}
	err = rbacRepo.SavePermission(ctx, adminPermission)
	if err != nil {
		panic(fmt.Sprintf("failed to create admin permission: %v", err))
	}

	abilityID := uint(1)
	for _, abilityName := range domain.ServersAbilities {
		ability := &domain.Ability{
			ID:   abilityID,
			Name: abilityName,
		}
		err = rbacRepo.SaveAbility(ctx, ability)
		if err != nil {
			panic(fmt.Sprintf("failed to create ability %s: %v", abilityName, err))
		}

		permission := &domain.Permission{
			AbilityID:  ability.ID,
			EntityID:   new(uint(2)),
			EntityType: lo.ToPtr(domain.EntityTypeRole),
			Forbidden:  false,
		}
		err = rbacRepo.SavePermission(ctx, permission)
		if err != nil {
			panic(fmt.Sprintf("failed to create permission for ability %s: %v", abilityName, err))
		}

		abilityID++
	}

	return c
}

type TestFixtures struct {
	AdminUser   *domain.User
	RegularUser *domain.User
	Server1     *domain.Server
	Server2     *domain.Server
	Node1       *domain.Node
	Node2       *domain.Node
	// EnrollmentSetupKey is a valid daemon enrollment setup key stored in cache.
	EnrollmentSetupKey string
}

// Node tokens used in security tests for daemon X-Auth-Token authentication.
// These are the plaintext values daemons present in the X-Auth-Token header.
// The middleware hashes the presented token with SHA-256 and looks it up in the
// database, so SetupFixtures stores hash(node*GDaemonAPIToken) on the node row.
const (
	Node1GDaemonAPIToken = "test-daemon-token-node1"
	Node2GDaemonAPIToken = "test-daemon-token-node2"
)

func SetupFixtures(ctx context.Context, c *InmemoryContainer) (*TestFixtures, error) {
	adminUser := &domain.User{
		ID:    1,
		Login: "admin",
		Email: "admin@yousite.local",
		Name:  new("Administrator"),
	}
	err := c.userRepo.Save(ctx, adminUser)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	regularUser := &domain.User{
		ID:    2,
		Login: "user",
		Email: "test@gameap.com",
		Name:  new("User"),
	}
	err = c.userRepo.Save(ctx, regularUser)
	if err != nil {
		return nil, fmt.Errorf("failed to create regular user: %w", err)
	}

	err = c.rbacService.SetRolesToUser(ctx, adminUser.ID, []string{"admin", "user"})
	if err != nil {
		return nil, fmt.Errorf("failed to set admin roles: %w", err)
	}

	err = c.rbacService.SetRolesToUser(ctx, regularUser.ID, []string{"user"})
	if err != nil {
		return nil, fmt.Errorf("failed to set user roles: %w", err)
	}

	node1Token := pkgstrings.SHA256(Node1GDaemonAPIToken)
	node1 := &domain.Node{
		ID:              1,
		Enabled:         true,
		Name:            "Test Node 1",
		OS:              domain.NodeOSLinux,
		Location:        "Test",
		IPs:             domain.IPList{"127.0.0.1"},
		WorkPath:        "/srv/gameap",
		GdaemonHost:     "127.0.0.1",
		GdaemonPort:     31717,
		GdaemonAPIKey:   "test-api-key-node1",
		GdaemonAPIToken: &node1Token,
	}
	if err := c.nodeRepo.Save(ctx, node1); err != nil {
		return nil, fmt.Errorf("failed to create node 1: %w", err)
	}

	node2Token := pkgstrings.SHA256(Node2GDaemonAPIToken)
	node2 := &domain.Node{
		ID:              2,
		Enabled:         true,
		Name:            "Test Node 2",
		OS:              domain.NodeOSLinux,
		Location:        "Test",
		IPs:             domain.IPList{"127.0.0.2"},
		WorkPath:        "/srv/gameap",
		GdaemonHost:     "127.0.0.2",
		GdaemonPort:     31717,
		GdaemonAPIKey:   "test-api-key-node2",
		GdaemonAPIToken: &node2Token,
	}
	if err := c.nodeRepo.Save(ctx, node2); err != nil {
		return nil, fmt.Errorf("failed to create node 2: %w", err)
	}

	game := &domain.Game{
		Code:    "test",
		Name:    "Test Game",
		Engine:  "source",
		Enabled: 1,
	}
	err = c.gameRepo.Save(ctx, game)
	if err != nil {
		return nil, fmt.Errorf("failed to create game: %w", err)
	}

	server1 := &domain.Server{
		ID:             1,
		GameID:         game.Code,
		Name:           "Test Server 1",
		Dir:            "/path/to/server1",
		StartCommand:   new("start"),
		StopCommand:    new("stop"),
		RestartCommand: new("restart"),
	}
	err = c.serverRepo.Save(ctx, server1)
	if err != nil {
		return nil, fmt.Errorf("failed to create server 1: %w", err)
	}

	server2 := &domain.Server{
		ID:             2,
		GameID:         game.Code,
		Name:           "Test Server 2",
		Dir:            "/path/to/server2",
		StartCommand:   new("start"),
		StopCommand:    new("stop"),
		RestartCommand: new("restart"),
	}
	err = c.serverRepo.Save(ctx, server2)
	if err != nil {
		return nil, fmt.Errorf("failed to create server 2: %w", err)
	}

	err = c.serverRepo.SetUserServers(ctx, regularUser.ID, []uint{server1.ID})
	if err != nil {
		return nil, fmt.Errorf("failed to set user servers: %w", err)
	}

	rbacRepo := c.rbacRepo.(*inmemory.RBACRepository)

	abilityID := uint(50)
	for _, abilityName := range domain.ServersAbilities {
		ability := domain.CreateAbilityForEntity(abilityName, server1.ID, domain.EntityTypeServer)
		ability.ID = abilityID
		err = rbacRepo.SaveAbility(ctx, &ability)
		if err != nil {
			return nil, fmt.Errorf("failed to create server 1 ability %s: %w", abilityName, err)
		}

		permission := &domain.Permission{
			AbilityID:  ability.ID,
			EntityID:   new(uint(2)),
			EntityType: lo.ToPtr(domain.EntityTypeRole),
			Forbidden:  false,
		}
		err = rbacRepo.SavePermission(ctx, permission)
		if err != nil {
			return nil, fmt.Errorf("failed to create permission for server 1 ability %s: %w", abilityName, err)
		}

		abilityID++
	}

	abilityID = uint(77)
	for _, abilityName := range domain.ServersAbilities {
		ability := domain.CreateAbilityForEntity(abilityName, server2.ID, domain.EntityTypeServer)
		ability.ID = abilityID
		err = rbacRepo.SaveAbility(ctx, &ability)
		if err != nil {
			return nil, fmt.Errorf("failed to create server 2 ability %s: %w", abilityName, err)
		}

		permission := &domain.Permission{
			AbilityID:  ability.ID,
			EntityID:   new(uint(2)),
			EntityType: lo.ToPtr(domain.EntityTypeRole),
			Forbidden:  false,
		}
		err = rbacRepo.SavePermission(ctx, permission)
		if err != nil {
			return nil, fmt.Errorf("failed to create permission for server 2 ability %s: %w", abilityName, err)
		}

		abilityID++
	}

	const enrollmentSetupKey = "test-enrollment-setup-key"
	if c.cacheService != nil {
		if err := c.cacheService.Set(ctx, enrollment.SetupKeyCacheKey, enrollmentSetupKey); err != nil {
			return nil, fmt.Errorf("failed to seed enrollment setup key: %w", err)
		}
	}

	return &TestFixtures{
		AdminUser:          adminUser,
		RegularUser:        regularUser,
		Server1:            server1,
		Server2:            server2,
		Node1:              node1,
		Node2:              node2,
		EnrollmentSetupKey: enrollmentSetupKey,
	}, nil
}
