package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/servercontrol"
	"github.com/tetratelabs/wazero"
)

type ServerController interface {
	Start(ctx context.Context, server *domain.Server) (uint, error)
	Stop(ctx context.Context, server *domain.Server) (uint, error)
	Restart(ctx context.Context, server *domain.Server) (uint, error)
	Update(ctx context.Context, server *domain.Server) (uint, error)
	Install(ctx context.Context, server *domain.Server) (uint, error)
	Reinstall(ctx context.Context, server *domain.Server) (uint, error)
}

// serverControlAction names the operation in the audit record.
type serverControlAction string

const (
	serverControlStart     serverControlAction = "start"
	serverControlStop      serverControlAction = "stop"
	serverControlRestart   serverControlAction = "restart"
	serverControlUpdate    serverControlAction = "update"
	serverControlInstall   serverControlAction = "install"
	serverControlReinstall serverControlAction = "reinstall"
)

// serverControlExports maps actions to the host function names the guest
// imports, for the policy lookup.
var serverControlExports = map[serverControlAction]string{
	serverControlStart:     "start_server",
	serverControlStop:      "stop_server",
	serverControlRestart:   "restart_server",
	serverControlUpdate:    "update_server",
	serverControlInstall:   "install_server",
	serverControlReinstall: "reinstall_server",
}

// ServerControlServiceImpl is per plugin: every operation is gated on the
// plugin's manage_servers grant, rate limited and audited with the plugin as
// the actor.
type ServerControlServiceImpl struct {
	serverRepo       repositories.ServerRepository
	serverController ServerController
	guard            *PluginGuard
}

func NewServerControlService(
	serverRepo repositories.ServerRepository,
	serverController ServerController,
	guard *PluginGuard,
) *ServerControlServiceImpl {
	return &ServerControlServiceImpl{
		serverRepo:       serverRepo,
		serverController: serverController,
		guard:            guard,
	}
}

// control runs one operation through the guard: policy check, server lookup,
// the controller call, and the audit record of the outcome.
func (s *ServerControlServiceImpl) control(
	ctx context.Context,
	action serverControlAction,
	serverID uint64,
	run func(context.Context, *domain.Server) (uint, error),
) *servercontrol.ServerControlResponse {
	if msg := s.guard.Check(ctx, ModuleServerControl, serverControlExports[action]); msg != "" {
		return &servercontrol.ServerControlResponse{Success: false, Error: new(msg)}
	}

	server, err := s.getServer(ctx, serverID)
	if err != nil {
		return &servercontrol.ServerControlResponse{Success: false, Error: new(err.Error())}
	}

	if server == nil {
		return &servercontrol.ServerControlResponse{Success: false, Error: new("server not found")}
	}

	taskID, err := run(ctx, server)
	s.guard.Audit(ctx, audit.EventPluginServerControl, string(action), "server", serverResourceID(serverID), err,
		slog.Uint64("node_id", uint64(server.DSID)))

	if err != nil {
		return &servercontrol.ServerControlResponse{Success: false, Error: new(err.Error())}
	}

	return &servercontrol.ServerControlResponse{Success: true, TaskId: new(uint64(taskID))}
}

func (s *ServerControlServiceImpl) getServer(
	ctx context.Context,
	serverID uint64,
) (*domain.Server, error) {
	servers, err := s.serverRepo.Find(
		ctx,
		filters.FindServerByIDs(uint(serverID)),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	if len(servers) == 0 {
		return nil, nil
	}

	return &servers[0], nil
}

func (s *ServerControlServiceImpl) StartServer(
	ctx context.Context,
	req *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.control(ctx, serverControlStart, req.ServerId, s.serverController.Start), nil
}

func (s *ServerControlServiceImpl) StopServer(
	ctx context.Context,
	req *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.control(ctx, serverControlStop, req.ServerId, s.serverController.Stop), nil
}

func (s *ServerControlServiceImpl) RestartServer(
	ctx context.Context,
	req *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.control(ctx, serverControlRestart, req.ServerId, s.serverController.Restart), nil
}

func (s *ServerControlServiceImpl) UpdateServer(
	ctx context.Context,
	req *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.control(ctx, serverControlUpdate, req.ServerId, s.serverController.Update), nil
}

func (s *ServerControlServiceImpl) InstallServer(
	ctx context.Context,
	req *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.control(ctx, serverControlInstall, req.ServerId, s.serverController.Install), nil
}

func (s *ServerControlServiceImpl) ReinstallServer(
	ctx context.Context,
	req *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.control(ctx, serverControlReinstall, req.ServerId, s.serverController.Reinstall), nil
}

type ServerControlHostLibrary struct {
	impl *ServerControlServiceImpl
}

func (l *ServerControlHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return servercontrol.Instantiate(ctx, r, l.impl)
}

// ServerControlHostLibraryFactory builds a per-plugin servercontrol module
// bound to the plugin's guard.
type ServerControlHostLibraryFactory struct {
	serverRepo       repositories.ServerRepository
	serverController ServerController
	guard            *Guard
}

func NewServerControlHostLibraryFactory(
	serverRepo repositories.ServerRepository,
	serverController ServerController,
	guard *Guard,
) *ServerControlHostLibraryFactory {
	return &ServerControlHostLibraryFactory{
		serverRepo:       serverRepo,
		serverController: serverController,
		guard:            guard,
	}
}

func (f *ServerControlHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &ServerControlHostLibrary{
		impl: NewServerControlService(f.serverRepo, f.serverController, f.guard.For(pluginID)),
	}
}
