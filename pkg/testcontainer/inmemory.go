package testcontainer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gameap/gameap/internal/acme"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/files"
	grpchandlers "github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/metrics"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/filemanager/archiver"
	"github.com/gameap/gameap/internal/services/gameapimporter"
	"github.com/gameap/gameap/internal/services/gameexporter"
	"github.com/gameap/gameap/internal/services/pelicaneggimporter"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/internal/services/serverconfigpush"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/internal/services/taskdispatcher"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/internal/ws"
	pkgapi "github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/samber/lo"
)

type InmemoryContainer struct {
	cfg                   *config.Config
	responder             *pkgapi.Responder
	gameRepo              repositories.GameRepository
	gameModRepo           repositories.GameModRepository
	serverRepo            repositories.ServerRepository
	userRepo              repositories.UserRepository
	authService           auth.Service
	userService           *services.UserService
	rbacRepo              repositories.RBACRepository
	tokenRepo             repositories.PersonalAccessTokenRepository
	daemonTaskRepo        repositories.DaemonTaskRepository
	serverTaskRepo        repositories.ServerTaskRepository
	serverTaskFailRepo    repositories.ServerTaskFailRepository
	serverSettingRepo     repositories.ServerSettingRepository
	nodeRepo              repositories.NodeRepository
	clientCertificateRepo repositories.ClientCertificateRepository
	rbacService           *rbac.RBAC
	serverControlService  *servercontrol.Service
	gameUpgradeService    *services.GameUpgradeService
	fileManager           files.FileManager
	cacheService          cache.Cache
	certificatesService   *certificates.Service
	globalAPIService      *services.GlobalAPIService
	daemonStatusService   *daemon.StatusService
	daemonFilesService    *daemon.FileService
	daemonCommandsService *daemon.CommandService
	uploadSessionService  *upload.Service
}

func (c *InmemoryContainer) Config() *config.Config                            { return c.cfg }
func (c *InmemoryContainer) DB() *sql.DB                                       { return nil }
func (c *InmemoryContainer) TransactionManager() base.TransactionManager       { return nil }
func (c *InmemoryContainer) Responder() *pkgapi.Responder                      { return c.responder }
func (c *InmemoryContainer) GameRepository() repositories.GameRepository       { return c.gameRepo }
func (c *InmemoryContainer) GameModRepository() repositories.GameModRepository { return c.gameModRepo }
func (c *InmemoryContainer) ServerRepository() repositories.ServerRepository   { return c.serverRepo }
func (c *InmemoryContainer) UserRepository() repositories.UserRepository       { return c.userRepo }
func (c *InmemoryContainer) AuthService() auth.Service                         { return c.authService }
func (c *InmemoryContainer) UserService() *services.UserService                { return c.userService }
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
func (c *InmemoryContainer) ServerTaskFailRepository() repositories.ServerTaskFailRepository {
	return c.serverTaskFailRepo
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
func (c *InmemoryContainer) DaemonStatus() *daemon.StatusService          { return c.daemonStatusService }
func (c *InmemoryContainer) DaemonFiles() *daemon.FileService             { return c.daemonFilesService }
func (c *InmemoryContainer) UploadSessionService() *upload.Service        { return c.uploadSessionService }
func (c *InmemoryContainer) FileManagerArchiver() *archiver.Archiver      { return nil }
func (c *InmemoryContainer) FileManagerArchiveGuard() *archiver.InMemoryConcurrencyGuard {
	return archiver.NewInMemoryConcurrencyGuard(2)
}
func (c *InmemoryContainer) DaemonCommands() *daemon.CommandService       { return c.daemonCommandsService }
func (c *InmemoryContainer) ConsoleLogService() *daemon.ConsoleLogService { return nil }
func (c *InmemoryContainer) PluginManager() *plugin.Manager               { return nil }
func (c *InmemoryContainer) PluginRepository() repositories.PluginRepository {
	return inmemory.NewPluginRepository()
}
func (c *InmemoryContainer) PluginLoader() *internalplugin.Loader             { return nil }
func (c *InmemoryContainer) PluginStoreService() *pluginstore.Service         { return nil }
func (c *InmemoryContainer) PluginsDir() string                               { return "plugins" }
func (c *InmemoryContainer) PelicanEggImporter() *pelicaneggimporter.Importer { return nil }
func (c *InmemoryContainer) GameAPImporter() *gameapimporter.Importer         { return nil }
func (c *InmemoryContainer) GameExporter() *gameexporter.Exporter             { return nil }
func (c *InmemoryContainer) TaskDispatcher() *taskdispatcher.Dispatcher       { return nil }
func (c *InmemoryContainer) ServerConfigPusher() *serverconfigpush.Pusher     { return nil }
func (c *InmemoryContainer) WSHub() *ws.Hub                                   { return ws.NewHub(nil) }
func (c *InmemoryContainer) SessionRegistry() *session.Registry               { return nil }
func (c *InmemoryContainer) CommandHandler() *grpchandlers.CommandHandler     { return nil }
func (c *InmemoryContainer) AttachHandler() *grpchandlers.AttachHandler       { return nil }
func (c *InmemoryContainer) MetricsHub() metrics.Hub                          { return nil }
func (c *InmemoryContainer) PubSub() pubsub.PubSub                            { return nil }
func (c *InmemoryContainer) ACMEService() *acme.Service                       { return nil }
func (c *InmemoryContainer) EnrollmentService() *enrollment.Service {
	keyManager := enrollment.NewSetupKeyManager(c.cacheService, "")

	return enrollment.NewService(
		keyManager,
		c.nodeRepo,
		c.clientCertificateRepo,
		c.certificatesService,
	)
}

func (c *InmemoryContainer) EnrollmentServiceOrNil() *enrollment.Service { return nil }
func (c *InmemoryContainer) GRPCPort() uint16                            { return 31718 }
func (c *InmemoryContainer) GRPCExternalHost() string                    { return "" }
func (c *InmemoryContainer) GRPCExternalPort() uint16                    { return 0 }

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

	c := &InmemoryContainer{
		cfg: &config.Config{
			AuthSecret:    "test-secret-key-for-testing",
			EncryptionKey: "test-encryption-key-testing",
		},
		responder:             pkgapi.NewResponder(),
		gameRepo:              inmemory.NewGameRepository(),
		gameModRepo:           inmemory.NewGameModRepository(),
		serverRepo:            serverRepo,
		userRepo:              userRepo,
		authService:           auth.NewJWTService([]byte("test-secret-key-for-testing")),
		userService:           services.NewUserService(userRepo),
		rbacRepo:              rbacRepo,
		tokenRepo:             inmemory.NewPersonalAccessTokenRepository(),
		daemonTaskRepo:        daemonTaskRepo,
		serverTaskRepo:        inmemory.NewServerTaskRepository(serverRepo),
		serverTaskFailRepo:    inmemory.NewServerTaskFailRepository(),
		serverSettingRepo:     serverSettingRepo,
		nodeRepo:              inmemory.NewNodeRepository(),
		clientCertificateRepo: inmemory.NewClientCertificateRepository(),
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
