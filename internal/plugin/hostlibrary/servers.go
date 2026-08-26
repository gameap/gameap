package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

// ServersServiceImpl is per plugin: reads are open to every plugin, writes
// (save, delete) are gated on manage_servers, rate limited and audited.
type ServersServiceImpl struct {
	serverRepo repositories.ServerRepository
	guard      *PluginGuard
}

func NewServersService(serverRepo repositories.ServerRepository, guard *PluginGuard) *ServersServiceImpl {
	return &ServersServiceImpl{
		serverRepo: serverRepo,
		guard:      guard,
	}
}

func (s *ServersServiceImpl) FindServers(
	ctx context.Context,
	req *servers.FindServersRequest,
) (*servers.FindServersResponse, error) {
	var filter *filters.FindServer
	if req.Filter != nil {
		filter = &filters.FindServer{
			IDs:     uintsFromUint64s(req.Filter.Ids),
			DSIDs:   uintsFromUint64s(req.Filter.NodeIds),
			GameIDs: req.Filter.GameIds,
			Enabled: req.Filter.Enabled,
		}
	}

	var pagination *filters.Pagination
	if req.Pagination != nil {
		pagination = &filters.Pagination{
			Limit:  uint64(req.Pagination.Limit),
			Offset: uint64(req.Pagination.Offset),
		}
	}

	sorting := convertSorting(req.Sorting)

	result, err := s.serverRepo.Find(ctx, filter, sorting, pagination)
	if err != nil {
		return nil, err
	}

	return &servers.FindServersResponse{
		Servers: convertServersToProto(result),
		Total:   int32(len(result)), //nolint:gosec
	}, nil
}

func (s *ServersServiceImpl) GetServer(
	ctx context.Context,
	req *servers.GetServerRequest,
) (*servers.GetServerResponse, error) {
	result, err := s.serverRepo.Find(
		ctx,
		filters.FindServerByIDs(uint(req.Id)),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return &servers.GetServerResponse{Found: false}, nil
	}

	return &servers.GetServerResponse{
		Server: convertServerToProto(&result[0]),
		Found:  true,
	}, nil
}

func (s *ServersServiceImpl) SaveServer(
	ctx context.Context,
	req *servers.SaveServerRequest,
) (*servers.SaveServerResponse, error) {
	if msg := s.guard.Check(ctx, ModuleServers, "save_server"); msg != "" {
		return &servers.SaveServerResponse{Success: false, Error: new(msg)}, nil
	}

	if req.Server == nil {
		return &servers.SaveServerResponse{
			Success: false,
			Error:   new("server is required"),
		}, nil
	}

	server := convertServerFromProto(req.Server)
	err := s.serverRepo.Save(ctx, server)

	action := "update"
	if req.Server.Id == 0 {
		action = "create"
	}

	s.guard.Audit(ctx, audit.EventPluginServerSave, action, "server", serverResourceID(uint64(server.ID)), err,
		slog.Uint64("node_id", uint64(server.DSID)))

	if err != nil {
		return &servers.SaveServerResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &servers.SaveServerResponse{
		Success: true,
		Id:      uint64(server.ID),
	}, nil
}

func (s *ServersServiceImpl) DeleteServer(
	ctx context.Context,
	req *servers.DeleteServerRequest,
) (*servers.DeleteServerResponse, error) {
	if msg := s.guard.Check(ctx, ModuleServers, "delete_server"); msg != "" {
		return &servers.DeleteServerResponse{Success: false, Error: new(msg)}, nil
	}

	err := s.serverRepo.Delete(ctx, uint(req.Id))
	s.guard.Audit(ctx, audit.EventPluginServerDelete, "delete", "server", serverResourceID(req.Id), err)

	if err != nil {
		return &servers.DeleteServerResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &servers.DeleteServerResponse{Success: true}, nil
}

func convertServersToProto(srvs []domain.Server) []*proto.Server {
	return lo.Map(srvs, func(s domain.Server, _ int) *proto.Server {
		return convertServerToProto(&s)
	})
}

func convertServerToProto(s *domain.Server) *proto.Server {
	var queryPort, rconPort *int32
	if s.QueryPort != nil {
		queryPort = new(int32(*s.QueryPort)) //nolint:gosec
	}

	if s.RconPort != nil {
		rconPort = new(int32(*s.RconPort)) //nolint:gosec
	}

	var suUser, startCommand *string
	if s.SuUser != nil {
		suUser = s.SuUser
	}
	if s.StartCommand != nil {
		startCommand = s.StartCommand
	}

	return &proto.Server{
		Id:            uint64(s.ID),
		Uuid:          s.UUID.String(),
		UuidShort:     s.UUIDShort,
		Enabled:       s.Enabled,
		Installed:     proto.ServerInstalledStatus(s.Installed), //nolint:gosec
		Blocked:       s.Blocked,
		Name:          s.Name,
		GameId:        s.GameID,
		DsId:          uint64(s.DSID),
		GameModId:     uint64(s.GameModID),
		ServerIp:      s.ServerIP,
		ServerPort:    int32(s.ServerPort), //nolint:gosec
		QueryPort:     queryPort,
		RconPort:      rconPort,
		Dir:           s.Dir,
		SuUser:        suUser,
		StartCommand:  startCommand,
		ProcessActive: s.ProcessActive,
		Metadata:      domainMetadataToProto(s.Metadata),
	}
}

func convertServerFromProto(s *proto.Server) *domain.Server {
	var queryPort, rconPort *int
	if s.QueryPort != nil {
		queryPort = new(int(*s.QueryPort))
	}

	if s.RconPort != nil {
		rconPort = new(int(*s.RconPort))
	}

	var suUser, startCommand *string
	if s.SuUser != nil {
		suUser = s.SuUser
	}

	if s.StartCommand != nil {
		startCommand = s.StartCommand
	}

	return &domain.Server{
		ID:            uint(s.Id),
		Enabled:       s.Enabled,
		ProcessActive: s.ProcessActive,
		Blocked:       s.Blocked,
		Name:          s.Name,
		GameID:        s.GameId,
		GameModID:     uint(s.GameModId),
		DSID:          uint(s.DsId),
		ServerIP:      s.ServerIp,
		ServerPort:    int(s.ServerPort),
		QueryPort:     queryPort,
		RconPort:      rconPort,
		Dir:           s.Dir,
		SuUser:        suUser,
		StartCommand:  startCommand,
		Installed:     domain.ServerInstalledStatus(s.Installed),
	}
}

type ServersHostLibrary struct {
	impl *ServersServiceImpl
}

func (l *ServersHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return servers.Instantiate(ctx, r, l.impl)
}

// ServersHostLibraryFactory builds a per-plugin servers module bound to the
// plugin's guard.
type ServersHostLibraryFactory struct {
	serverRepo repositories.ServerRepository
	guard      *Guard
}

func NewServersHostLibraryFactory(serverRepo repositories.ServerRepository, guard *Guard) *ServersHostLibraryFactory {
	return &ServersHostLibraryFactory{serverRepo: serverRepo, guard: guard}
}

func (f *ServersHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &ServersHostLibrary{impl: NewServersService(f.serverRepo, f.guard.For(pluginID))}
}
