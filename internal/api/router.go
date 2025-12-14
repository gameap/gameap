package api

import (
	"database/sql"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gameap/gameap/internal/api/auth/login"
	"github.com/gameap/gameap/internal/api/clientcertificates/deleteclientcertificates"
	"github.com/gameap/gameap/internal/api/clientcertificates/getclientcertificates"
	"github.com/gameap/gameap/internal/api/clientcertificates/postclientcertificates"
	"github.com/gameap/gameap/internal/api/daemon/createnode"
	"github.com/gameap/gameap/internal/api/daemon/daemonsetup"
	"github.com/gameap/gameap/internal/api/daemonapi/getinitdata"
	daemonapiinit "github.com/gameap/gameap/internal/api/daemonapi/gettoken"
	daemonapigetserverid "github.com/gameap/gameap/internal/api/daemonapi/servers/getserverid"
	daemonapigetservers "github.com/gameap/gameap/internal/api/daemonapi/servers/getservers"
	daemonapipatchservers "github.com/gameap/gameap/internal/api/daemonapi/servers/patchservers"
	daemonapiputserver "github.com/gameap/gameap/internal/api/daemonapi/servers/putserver"
	daemonapifailservertask "github.com/gameap/gameap/internal/api/daemonapi/serverstasks/failservertask"
	daemonapiserverstasks "github.com/gameap/gameap/internal/api/daemonapi/serverstasks/getserverstasks"
	daemonapigetservertask "github.com/gameap/gameap/internal/api/daemonapi/serverstasks/getservertask"
	daemonapiupdateservertask "github.com/gameap/gameap/internal/api/daemonapi/serverstasks/updateservertask"
	daemonapiappendoutput "github.com/gameap/gameap/internal/api/daemonapi/tasks/appendoutput"
	daemonapitasks "github.com/gameap/gameap/internal/api/daemonapi/tasks/gettask"
	daemonapiupdatetask "github.com/gameap/gameap/internal/api/daemonapi/tasks/updatetask"
	"github.com/gameap/gameap/internal/api/daemontasks/getdaemontask"
	"github.com/gameap/gameap/internal/api/daemontasks/getdaemontasks"
	"github.com/gameap/gameap/internal/api/filemanager/content"
	filemanagercreatedirectory "github.com/gameap/gameap/internal/api/filemanager/createdirectory"
	filemanagercreatefile "github.com/gameap/gameap/internal/api/filemanager/createfile"
	filemanagerdelete "github.com/gameap/gameap/internal/api/filemanager/delete"
	filemanagerdownload "github.com/gameap/gameap/internal/api/filemanager/download"
	"github.com/gameap/gameap/internal/api/filemanager/initialize"
	filemanagerpaste "github.com/gameap/gameap/internal/api/filemanager/paste"
	filemanagerrename "github.com/gameap/gameap/internal/api/filemanager/rename"
	filemanagerstreamfile "github.com/gameap/gameap/internal/api/filemanager/streamfile"
	filemanagertree "github.com/gameap/gameap/internal/api/filemanager/tree"
	filemanagerupdatefile "github.com/gameap/gameap/internal/api/filemanager/updatefile"
	"github.com/gameap/gameap/internal/api/filemanager/upload"
	"github.com/gameap/gameap/internal/api/gamemods/deletegamemod"
	"github.com/gameap/gameap/internal/api/gamemods/getgamemod"
	"github.com/gameap/gameap/internal/api/gamemods/getgamemods"
	"github.com/gameap/gameap/internal/api/gamemods/getlistforgame"
	"github.com/gameap/gameap/internal/api/gamemods/postgamemod"
	"github.com/gameap/gameap/internal/api/gamemods/putgamemod"
	"github.com/gameap/gameap/internal/api/games/deletegame"
	"github.com/gameap/gameap/internal/api/games/getgame"
	gamesgetgamemods "github.com/gameap/gameap/internal/api/games/getgamemods"
	"github.com/gameap/gameap/internal/api/games/getgames"
	"github.com/gameap/gameap/internal/api/games/postgames"
	"github.com/gameap/gameap/internal/api/games/putgame"
	"github.com/gameap/gameap/internal/api/games/upgradegames"
	"github.com/gameap/gameap/internal/api/gethealth"
	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/api/nodes/deletenode"
	"github.com/gameap/gameap/internal/api/nodes/getbusyports"
	"github.com/gameap/gameap/internal/api/nodes/getcertificateszip"
	"github.com/gameap/gameap/internal/api/nodes/getdaemonstatus"
	"github.com/gameap/gameap/internal/api/nodes/getiplist"
	"github.com/gameap/gameap/internal/api/nodes/getlogszip"
	"github.com/gameap/gameap/internal/api/nodes/getnode"
	"github.com/gameap/gameap/internal/api/nodes/getnodes"
	nodesgetsummary "github.com/gameap/gameap/internal/api/nodes/getsummary"
	"github.com/gameap/gameap/internal/api/nodes/nodesetup"
	"github.com/gameap/gameap/internal/api/nodes/postnode"
	"github.com/gameap/gameap/internal/api/nodes/putnode"
	"github.com/gameap/gameap/internal/api/plugins/getfrontendplugins"
	"github.com/gameap/gameap/internal/api/profile/getprofile"
	"github.com/gameap/gameap/internal/api/profile/putprofile"
	"github.com/gameap/gameap/internal/api/servers/deleteserver"
	"github.com/gameap/gameap/internal/api/servers/getabilities"
	"github.com/gameap/gameap/internal/api/servers/getconsole"
	"github.com/gameap/gameap/internal/api/servers/getquery"
	"github.com/gameap/gameap/internal/api/servers/getserver"
	"github.com/gameap/gameap/internal/api/servers/getserverabilities"
	"github.com/gameap/gameap/internal/api/servers/getservers"
	"github.com/gameap/gameap/internal/api/servers/getstatus"
	"github.com/gameap/gameap/internal/api/servers/getsummary"
	"github.com/gameap/gameap/internal/api/servers/postcommand"
	"github.com/gameap/gameap/internal/api/servers/postconsole"
	"github.com/gameap/gameap/internal/api/servers/postserver"
	"github.com/gameap/gameap/internal/api/servers/putserver"
	"github.com/gameap/gameap/internal/api/servers/rcon/getfastrcon"
	rcongetplayers "github.com/gameap/gameap/internal/api/servers/rcon/getplayers"
	"github.com/gameap/gameap/internal/api/servers/rcon/getrconfeatures"
	rconkickplayer "github.com/gameap/gameap/internal/api/servers/rcon/kickplayer"
	rconpostcommand "github.com/gameap/gameap/internal/api/servers/rcon/postcommand"
	"github.com/gameap/gameap/internal/api/servers/searchservers"
	"github.com/gameap/gameap/internal/api/serversettings/getserversettings"
	"github.com/gameap/gameap/internal/api/serversettings/putserversettings"
	"github.com/gameap/gameap/internal/api/servertasks/deleteservertask"
	"github.com/gameap/gameap/internal/api/servertasks/getservertasks"
	"github.com/gameap/gameap/internal/api/servertasks/postservertask"
	"github.com/gameap/gameap/internal/api/servertasks/putservertask"
	"github.com/gameap/gameap/internal/api/tokens/deletetoken"
	tokensgetabilities "github.com/gameap/gameap/internal/api/tokens/getabilities"
	"github.com/gameap/gameap/internal/api/tokens/gettokens"
	"github.com/gameap/gameap/internal/api/tokens/posttoken"
	"github.com/gameap/gameap/internal/api/user/getuser"
	"github.com/gameap/gameap/internal/api/users/deleteuser"
	"github.com/gameap/gameap/internal/api/users/getserverperms"
	usersgetuser "github.com/gameap/gameap/internal/api/users/getuser"
	"github.com/gameap/gameap/internal/api/users/getusers"
	"github.com/gameap/gameap/internal/api/users/getuserservers"
	"github.com/gameap/gameap/internal/api/users/postusers"
	"github.com/gameap/gameap/internal/api/users/putserverperms"
	"github.com/gameap/gameap/internal/api/users/putuser"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/i18n"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin"
	webstatic "github.com/gameap/gameap/web/static"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type container interface {
	Config() *config.Config
	DB() *sql.DB
	TransactionManager() base.TransactionManager
	Responder() *api.Responder
	GameRepository() repositories.GameRepository
	GameModRepository() repositories.GameModRepository
	ServerRepository() repositories.ServerRepository
	UserRepository() repositories.UserRepository
	AuthService() auth.Service
	UserService() *services.UserService
	ServerControlService() *servercontrol.Service
	GameUpgradeService() *services.GameUpgradeService
	RBACRepository() repositories.RBACRepository
	PersonalAccessTokenRepository() repositories.PersonalAccessTokenRepository
	DaemonTaskRepository() repositories.DaemonTaskRepository
	ServerTaskRepository() repositories.ServerTaskRepository
	ServerTaskFailRepository() repositories.ServerTaskFailRepository
	ServerSettingRepository() repositories.ServerSettingRepository
	NodeRepository() repositories.NodeRepository
	ClientCertificateRepository() repositories.ClientCertificateRepository
	RBAC() *rbac.RBAC
	FileManager() files.FileManager
	Cache() cache.Cache
	CertificatesService() *certificates.Service
	GlobalAPIService() *services.GlobalAPIService
	DaemonStatus() *daemon.StatusService
	DaemonFiles() *daemon.FileService
	DaemonCommands() *daemon.CommandService
	PluginManager() *plugin.Manager
}

func CreateRouter(c container) *http.ServeMux {
	serverMux := http.NewServeMux()

	router := mux.NewRouter().StrictSlash(true)

	serverMux.Handle("/api/",
		handlers.HTTPMethodOverrideHandler(apiRoutes(c, router)),
	)
	serverMux.Handle("/gdaemon/", gdaemonSetupRoutes(c, router))
	serverMux.Handle("/gdaemon_api/", gdaemonAPIRoutes(c, router))

	static, err := webstatic.GetFS()
	if err != nil {
		panic("failed to get static files: " + err.Error())
	}

	serverMux.Handle("/", spaHandler(static))

	serverMux.Handle("/lang/", http.StripPrefix("/lang/", http.FileServer(http.FS(i18n.GetFS()))))

	serverMux.Handle("/plugins.js", frontendPluginsHandler(c))

	return serverMux
}

func frontendPluginsHandler(c container) http.Handler {
	authMiddleware := middlewares.NewAuthMiddleware(
		c.AuthService(),
		c.UserService(),
		c.PersonalAccessTokenRepository(),
		c.Responder(),
	)

	recoveryMiddleware := middlewares.NewRecoveryMiddleware(
		c.Responder(),
	)

	handler := getfrontendplugins.NewHandler(c.PluginManager())

	return recoveryMiddleware.Middleware(authMiddleware.Middleware(handler))
}

// spaHandler serves the frontend SPA, falling back to index.html for unknown routes.
func spaHandler(staticFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested file
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			// Remove leading slash for fs.FS
			path = path[1:]
		}

		f, err := staticFS.Open(path)
		if err == nil {
			// File exists, serve it
			closeErr := f.Close()
			if closeErr != nil {
				slog.Error("spaHandler: failed to close file", "error", closeErr)
			}

			http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)

			return
		}

		// File doesn't exist, serve index.html for SPA routing
		index, err := staticFS.Open("index.html")
		if err != nil {
			http.NotFound(w, r)

			return
		}
		defer func(index fs.File) {
			err := index.Close()
			if err != nil {
				slog.Error("spaHandler: failed to close index.html", "error", err)
			}
		}(index)

		stat, err := index.Stat()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)

			return
		}

		http.ServeContent(w, r, "index.html", stat.ModTime(), index.(io.ReadSeeker))
	})
}

//nolint:funlen
func apiRoutes(c container, router *mux.Router) *mux.Router {
	routes := []struct {
		Method            string
		Path              string
		Handler           http.Handler
		AllowGuestAccess  bool
		AdminOnly         bool
		CheckPATAbilities []domain.PATAbility
	}{
		{
			Method:           http.MethodGet,
			Path:             "/api/health",
			Handler:          gethealth.NewGetHealthHandler(c.DB(), c.Responder()),
			AllowGuestAccess: true,
		},

		// Auth
		{
			Method:           http.MethodPost,
			Path:             "/api/auth/login",
			Handler:          login.NewHandler(c.AuthService(), c.UserService(), c.Responder()),
			AllowGuestAccess: true,
		},

		// User
		{
			Method:  http.MethodGet,
			Path:    "/api/user",
			Handler: getuser.NewHandler(c.Responder()),
		},

		// Profile
		{
			Method: http.MethodGet,
			Path:   "/api/profile",
			Handler: getprofile.NewHandler(
				c.RBACRepository(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPut,
			Path:   "/api/profile",
			Handler: putprofile.NewHandler(
				c.UserService(),
				c.Responder(),
			),
		},

		// Tokens
		{
			Method: http.MethodGet,
			Path:   "/api/tokens",
			Handler: gettokens.NewHandler(
				c.PersonalAccessTokenRepository(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/tokens",
			Handler: posttoken.NewHandler(
				c.PersonalAccessTokenRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodDelete,
			Path:   "/api/tokens/{id}",
			Handler: deletetoken.NewHandler(
				c.PersonalAccessTokenRepository(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/tokens/abilities",
			Handler: tokensgetabilities.NewHandler(
				c.RBAC(),
				c.Responder(),
			),
		},

		// Servers
		{
			Method: http.MethodGet,
			Path:   "/api/servers",
			Handler: getservers.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerList,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers",
			Handler: postserver.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.GameModRepository(),
				c.DaemonTaskRepository(),
				c.Responder(),
			),
			AdminOnly: true,
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerCreate,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/summary",
			Handler: getsummary.NewHandler(
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/search",
			Handler: searchservers.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{id}",
			Handler: getserver.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerList,
			},
		},
		{
			Method: http.MethodDelete,
			Path:   "/api/servers/{id}",
			Handler: deleteserver.NewHandler(
				c.ServerRepository(),
				c.DaemonTaskRepository(),
				c.RBAC(),
				c.Responder(),
			),
			AdminOnly: true,
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerCreate,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/api/servers/{id}",
			Handler: putserver.NewHandler(
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			AdminOnly: true,
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerCreate,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/abilities",
			Handler: getserverabilities.NewHandler(
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/status",
			Handler: getstatus.NewHandler(
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/query",
			Handler: getquery.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/rcon/features",
			Handler: getrconfeatures.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRconConsole,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/rcon/fast_rcon",
			Handler: getfastrcon.NewHandler(
				c.ServerRepository(),
				c.GameModRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRconConsole,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/rcon",
			Handler: rconpostcommand.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRconConsole,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/rcon/players",
			Handler: rcongetplayers.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRconConsole,
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/servers/{server}/rcon/players/message",
			Handler: &notImplementedHandler{},
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRconConsole,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/rcon/players/{command}",
			Handler: rconkickplayer.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRconConsole,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/console",
			Handler: getconsole.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonCommands(),
				c.DaemonFiles(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerConsole,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/console",
			Handler: postconsole.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonCommands(),
				c.DaemonFiles(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerConsole,
			},
		},

		// File Manager
		{
			Method: http.MethodGet,
			Path:   "/api/file-manager/{server}/initialize",
			Handler: initialize.NewHandler(
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/file-manager/{server}/content",
			Handler: content.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/file-manager/{server}/tree",
			Handler: filemanagertree.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/delete",
			Handler: filemanagerdelete.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/upload",
			Handler: upload.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/update-file",
			Handler: filemanagerupdatefile.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/file-manager/{server}/download",
			Handler: filemanagerdownload.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/rename",
			Handler: filemanagerrename.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/create-directory",
			Handler: filemanagercreatedirectory.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/create-file",
			Handler: filemanagercreatefile.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/file-manager/{server}/stream-file",
			Handler: filemanagerstreamfile.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/api/file-manager/{server}/paste",
			Handler: filemanagerpaste.NewHandler(
				c.ServerRepository(),
				c.NodeRepository(),
				c.RBAC(),
				c.DaemonFiles(),
				c.Responder(),
			),
		},

		// Server Tasks
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/tasks",
			Handler: getservertasks.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerTasksManage,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/tasks",
			Handler: postservertask.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerTasksManage,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/api/servers/{server}/tasks/{id}",
			Handler: putservertask.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerTasksManage,
			},
		},
		{
			Method: http.MethodDelete,
			Path:   "/api/servers/{server}/tasks/{id}",
			Handler: deleteservertask.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerTasksManage,
			},
		},

		// Server Settings
		{
			Method: http.MethodGet,
			Path:   "/api/servers/{server}/settings",
			Handler: getserversettings.NewHandler(
				c.ServerSettingRepository(),
				c.ServerRepository(),
				c.GameModRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerSettingsManage,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/api/servers/{server}/settings",
			Handler: putserversettings.NewHandler(
				c.ServerSettingRepository(),
				c.ServerRepository(),
				c.GameModRepository(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerSettingsManage,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/start",
			Handler: postcommand.NewHandler(
				c.ServerRepository(),
				c.ServerControlService(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerStart,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/stop",
			Handler: postcommand.NewHandler(
				c.ServerRepository(),
				c.ServerControlService(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerStop,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/restart",
			Handler: postcommand.NewHandler(
				c.ServerRepository(),
				c.ServerControlService(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerRestart,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/update",
			Handler: postcommand.NewHandler(
				c.ServerRepository(),
				c.ServerControlService(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerUpdate,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/install",
			Handler: postcommand.NewHandler(
				c.ServerRepository(),
				c.ServerControlService(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerUpdate,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/api/servers/{server}/reinstall",
			Handler: postcommand.NewHandler(
				c.ServerRepository(),
				c.ServerControlService(),
				c.RBAC(),
				c.Responder(),
			),
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityServerUpdate,
			},
		},

		// Server Abilities
		{
			Method: http.MethodGet,
			Path:   "/api/user/servers_abilities",
			Handler: getabilities.NewHandler(
				c.UserService(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
		},

		//
		// Admin routes

		// Users
		{
			Method:    http.MethodGet,
			Path:      "/api/users",
			Handler:   getusers.NewHandler(c.UserService(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method: http.MethodPost,
			Path:   "/api/users",
			Handler: postusers.NewHandler(
				c.UserService(),
				c.ServerRepository(),
				c.RBAC(),
				c.TransactionManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/users/{id}",
			Handler: usersgetuser.NewHandler(
				c.UserService(),
				c.RBACRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodPut,
			Path:   "/api/users/{id}",
			Handler: putuser.NewHandler(
				c.UserService(),
				c.ServerRepository(),
				c.RBAC(),
				c.TransactionManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodDelete,
			Path:   "/api/users/{id}",
			Handler: deleteuser.NewHandler(
				c.UserService(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/users/{id}/servers",
			Handler: getuserservers.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.GameModRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/users/{id}/servers/{server}/permissions",
			Handler: getserverperms.NewHandler(
				c.UserRepository(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodPut,
			Path:   "/api/users/{id}/servers/{server}/permissions",
			Handler: putserverperms.NewHandler(
				c.UserRepository(),
				c.ServerRepository(),
				c.RBAC(),
				c.Responder(),
			),
			AdminOnly: true,
		},

		// Nodes / Dedicated Servers
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/certificates.zip",
			Handler: getcertificateszip.NewHandler(
				c.CertificatesService(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/nodes/certificates.zip",
			Handler: getcertificateszip.NewHandler(
				c.CertificatesService(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/setup",
			Handler: nodesetup.NewHandler(
				c.Cache(),
				c.Responder(),
				"",
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/setup
			Path: "/api/nodes/setup",
			Handler: nodesetup.NewHandler(
				c.Cache(),
				c.Responder(),
				"",
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers",
			Handler: getnodes.NewHandler(
				c.NodeRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodPost,
			Path:   "/api/dedicated_servers",
			Handler: postnode.NewHandler(
				c.NodeRepository(),
				c.FileManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers
			Path: "/api/nodes",
			Handler: getnodes.NewHandler(
				c.NodeRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/summary",
			Handler: nodesgetsummary.NewHandler(
				c.NodeRepository(),
				c.DaemonStatus(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/summary
			Path: "/api/nodes/summary",
			Handler: nodesgetsummary.NewHandler(
				c.NodeRepository(),
				c.DaemonStatus(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/{id}",
			Handler: getnode.NewHandler(
				c.NodeRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/{id}
			Path: "/api/nodes/{id}",
			Handler: getnode.NewHandler(
				c.NodeRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodPut,
			Path:   "/api/dedicated_servers/{id}",
			Handler: putnode.NewHandler(
				c.NodeRepository(),
				c.FileManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodPut,
			// alias for /api/dedicated_servers/{id}
			Path: "/api/nodes/{id}",
			Handler: putnode.NewHandler(
				c.NodeRepository(),
				c.FileManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodDelete,
			Path:   "/api/dedicated_servers/{id}",
			Handler: deletenode.NewHandler(
				c.NodeRepository(),
				c.ServerRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodDelete,
			// alias for /api/dedicated_servers/{id}
			Path: "/api/nodes/{id}",
			Handler: deletenode.NewHandler(
				c.NodeRepository(),
				c.ServerRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/{node}/busy_ports",
			Handler: getbusyports.NewHandler(
				c.ServerRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/{node}/busy_ports
			Path: "/api/nodes/{node}/busy_ports",
			Handler: getbusyports.NewHandler(
				c.ServerRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/{node}/ip_list",
			Handler: getiplist.NewHandler(
				c.NodeRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/{node}/ip_list
			Path: "/api/nodes/{node}/ip_list",
			Handler: getiplist.NewHandler(
				c.NodeRepository(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/{id}/daemon",
			Handler: getdaemonstatus.NewHandler(
				c.NodeRepository(),
				c.DaemonStatus(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/{id}/daemon
			Path: "/api/nodes/{id}/daemon",
			Handler: getdaemonstatus.NewHandler(
				c.NodeRepository(),
				c.DaemonStatus(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			Path:   "/api/dedicated_servers/{id}/logs.zip",
			Handler: getlogszip.NewHandler(
				c.NodeRepository(),
				c.DaemonFiles(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodGet,
			// alias for /api/dedicated_servers/{id}/logs.zip
			Path: "/api/nodes/{id}/logs.zip",
			Handler: getlogszip.NewHandler(
				c.NodeRepository(),
				c.DaemonFiles(),
				c.Responder(),
			),
			AdminOnly: true,
		},

		// Games
		{
			Method:    http.MethodGet,
			Path:      "/api/games",
			Handler:   getgames.NewHandler(c.GameRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodGet,
			Path:      "/api/games/{code}",
			Handler:   getgame.NewHandler(c.GameRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodPost,
			Path:      "/api/games",
			Handler:   postgames.NewHandler(c.GameRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodPut,
			Path:      "/api/games/{code}",
			Handler:   putgame.NewHandler(c.GameRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodDelete,
			Path:      "/api/games/{code}",
			Handler:   deletegame.NewHandler(c.GameRepository(), c.ServerRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodGet,
			Path:      "/api/games/{code}/mods",
			Handler:   gamesgetgamemods.NewHandler(c.GameModRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method: http.MethodPost,
			Path:   "/api/games/upgrade",
			Handler: upgradegames.NewHandler(
				c.GameUpgradeService(),
				c.Responder(),
			),
			AdminOnly: true,
		},

		// Daemon Tasks
		{
			Method:    http.MethodGet,
			Path:      "/api/gdaemon_tasks",
			Handler:   getdaemontasks.NewHandler(c.DaemonTaskRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodGet,
			Path:      "/api/gdaemon_tasks/{id}",
			Handler:   getdaemontask.NewHandler(c.DaemonTaskRepository(), c.Responder(), false),
			AdminOnly: true,
			CheckPATAbilities: []domain.PATAbility{
				domain.PATAbilityGDaemonTaskRead,
			},
		},
		{
			Method:    http.MethodGet,
			Path:      "/api/gdaemon_tasks/{id}/output",
			Handler:   getdaemontask.NewHandler(c.DaemonTaskRepository(), c.Responder(), true),
			AdminOnly: true,
		},

		// Game Mods
		{
			Method:    http.MethodGet,
			Path:      "/api/game_mods",
			Handler:   getgamemods.NewHandler(c.GameModRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodPost,
			Path:      "/api/game_mods",
			Handler:   postgamemod.NewHandler(c.GameModRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodGet,
			Path:      "/api/game_mods/get_list_for_game/{game}",
			Handler:   getlistforgame.NewHandler(c.GameModRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodGet,
			Path:      "/api/game_mods/{id}",
			Handler:   getgamemod.NewHandler(c.GameModRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodPut,
			Path:      "/api/game_mods/{id}",
			Handler:   putgamemod.NewHandler(c.GameModRepository(), c.Responder()),
			AdminOnly: true,
		},
		{
			Method:    http.MethodDelete,
			Path:      "/api/game_mods/{id}",
			Handler:   deletegamemod.NewHandler(c.GameModRepository(), c.ServerRepository(), c.Responder()),
			AdminOnly: true,
		},

		// Client Certificates
		{
			Method: http.MethodGet,
			Path:   "/api/client_certificates",
			Handler: getclientcertificates.NewHandler(
				c.ClientCertificateRepository(),
				c.FileManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodPost,
			Path:   "/api/client_certificates",
			Handler: postclientcertificates.NewHandler(
				c.ClientCertificateRepository(),
				c.FileManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
		{
			Method: http.MethodDelete,
			Path:   "/api/client_certificates/{id}",
			Handler: deleteclientcertificates.NewHandler(
				c.ClientCertificateRepository(),
				c.FileManager(),
				c.Responder(),
			),
			AdminOnly: true,
		},
	}

	authMiddleware := middlewares.NewAuthMiddleware(
		c.AuthService(),
		c.UserService(),
		c.PersonalAccessTokenRepository(),
		c.Responder(),
	)

	corsMiddleware := middlewares.NewCORSMiddleware(c.Config())

	patMiddleware := middlewares.NewPersonalAccessMiddleware(
		c.PersonalAccessTokenRepository(),
		c.Responder(),
	)

	isAdminMiddleware := middlewares.NewIsAdminMiddleware(
		c.RBAC(),
		c.Responder(),
	)

	recoveryMiddleware := middlewares.NewRecoveryMiddleware(
		c.Responder(),
	)

	for _, r := range routes {
		handler := r.Handler

		if len(r.CheckPATAbilities) > 0 {
			handler = patMiddleware.Middleware(handler, r.CheckPATAbilities)
		}

		handler = corsMiddleware.Middleware(handler)

		if r.AdminOnly {
			handler = isAdminMiddleware.Middleware(handler)
		}

		if !r.AllowGuestAccess {
			handler = authMiddleware.Middleware(handler)
		} else {
			handler = authMiddleware.OptionalMiddleware(handler)
		}

		// Recovery middleware wraps everything to catch panics
		handler = recoveryMiddleware.Middleware(handler)

		router.Handle(r.Path, handler).Methods(r.Method)
	}

	registerPluginRoutes(
		c,
		router,
		authMiddleware,
		isAdminMiddleware,
		corsMiddleware,
		recoveryMiddleware,
	)

	return router
}

func registerPluginRoutes(
	c container,
	router *mux.Router,
	authMiddleware *middlewares.AuthMiddleware,
	isAdminMiddleware *middlewares.IsAdminMiddleware,
	corsMiddleware *middlewares.CORSMiddleware,
	recoveryMiddleware *middlewares.RecoveryMiddleware,
) {
	pluginManager := c.PluginManager()
	if pluginManager == nil {
		return
	}

	pluginHandler := plugin.NewHTTPHandler(
		pluginManager,
		authMiddleware,
		isAdminMiddleware,
	)

	var handler http.Handler = pluginHandler
	handler = corsMiddleware.Middleware(handler)
	handler = authMiddleware.OptionalMiddleware(handler)
	handler = recoveryMiddleware.Middleware(handler)

	router.PathPrefix("/api/plugins/{plugin_id}/").Handler(handler)
	router.Handle("/api/plugins/{plugin_id}", handler)
}

func gdaemonSetupRoutes(c container, router *mux.Router) *mux.Router {
	recoveryMiddleware := middlewares.NewRecoveryMiddleware(
		c.Responder(),
	)

	routes := []struct {
		Method      string
		Path        string
		Handler     http.Handler
		Middlewares []mux.MiddlewareFunc
	}{
		{
			Method: http.MethodGet,
			Path:   "/gdaemon/setup/{token}",
			Handler: daemonsetup.NewHandler(
				c.Cache(),
				c.Responder(),
				"",
			),
		},
		{
			Method: http.MethodPost,
			Path:   "/gdaemon/create/{token}",
			Handler: createnode.NewHandler(
				c.Cache(),
				c.NodeRepository(),
				c.ClientCertificateRepository(),
				c.CertificatesService(),
				c.Responder(),
			),
		},
	}

	for _, r := range routes {
		handler := r.Handler

		for _, mw := range r.Middlewares {
			handler = mw(handler)
		}

		// Recovery middleware wraps everything to catch panics
		handler = recoveryMiddleware.Middleware(handler)

		if handler != nil {
			router.Handle(r.Path, handler).Methods(r.Method)
		}
	}

	return router
}

//nolint:funlen
func gdaemonAPIRoutes(c container, router *mux.Router) *mux.Router {
	daemonAuthMiddleware := middlewares.NewDaemonAuthMiddleware(
		c.NodeRepository(),
		c.Responder(),
	)

	recoveryMiddleware := middlewares.NewRecoveryMiddleware(
		c.Responder(),
	)

	routes := []struct {
		Method      string
		Path        string
		Handler     http.Handler
		Middlewares []mux.MiddlewareFunc
	}{
		{
			Method:  http.MethodGet,
			Path:    "/gdaemon_api/get_token",
			Handler: daemonapiinit.NewHandler(c.NodeRepository(), c.Responder()),
		},
		{
			Method:  http.MethodGet,
			Path:    "/gdaemon_api/dedicated_servers/get_init_data/{node}",
			Handler: getinitdata.NewHandler(c.Responder()),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/gdaemon_api/servers",
			Handler: daemonapigetservers.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.GameModRepository(),
				c.ServerSettingRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/gdaemon_api/servers/{server}",
			Handler: daemonapigetserverid.NewHandler(
				c.ServerRepository(),
				c.GameRepository(),
				c.GameModRepository(),
				c.ServerSettingRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/gdaemon_api/servers/{server}",
			Handler: daemonapiputserver.NewHandler(
				c.ServerRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodPatch,
			Path:   "/gdaemon_api/servers",
			Handler: daemonapipatchservers.NewHandler(
				c.ServerRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/gdaemon_api/tasks",
			Handler: daemonapitasks.NewHandler(
				c.DaemonTaskRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/gdaemon_api/tasks/{gdaemon_task}",
			Handler: daemonapiupdatetask.NewHandler(
				c.DaemonTaskRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/gdaemon_api/tasks/{gdaemon_task}/output",
			Handler: daemonapiappendoutput.NewHandler(
				c.DaemonTaskRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/gdaemon_api/servers_tasks",
			Handler: daemonapiserverstasks.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/gdaemon_api/servers_tasks/{server_task}",
			Handler: daemonapigetservertask.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodPut,
			Path:   "/gdaemon_api/servers_tasks/{server_task}",
			Handler: daemonapiupdateservertask.NewHandler(
				c.ServerTaskRepository(),
				c.ServerRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
		{
			Method: http.MethodPost,
			Path:   "/gdaemon_api/servers_tasks/{server_task}/fail",
			Handler: daemonapifailservertask.NewHandler(
				c.ServerTaskRepository(),
				c.ServerTaskFailRepository(),
				c.ServerRepository(),
				c.Responder(),
			),
			Middlewares: []mux.MiddlewareFunc{
				daemonAuthMiddleware.Middleware,
			},
		},
	}

	for _, r := range routes {
		handler := r.Handler

		for _, mw := range r.Middlewares {
			handler = mw(handler)
		}

		// Recovery middleware wraps everything to catch panics
		handler = recoveryMiddleware.Middleware(handler)

		if handler != nil {
			router.Handle(r.Path, handler).Methods(r.Method)
		}
	}

	return router
}

type notImplementedHandler struct{}

func (h *notImplementedHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
