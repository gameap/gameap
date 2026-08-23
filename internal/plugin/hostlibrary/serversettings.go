package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/serversettings"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

// ServerSettingsServiceImpl is per plugin: reads are open, writes are gated
// on manage_servers, rate limited and audited.
type ServerSettingsServiceImpl struct {
	serverSettingRepo repositories.ServerSettingRepository
	guard             *PluginGuard
}

func NewServerSettingsService(
	serverSettingRepo repositories.ServerSettingRepository,
	guard *PluginGuard,
) *ServerSettingsServiceImpl {
	return &ServerSettingsServiceImpl{
		serverSettingRepo: serverSettingRepo,
		guard:             guard,
	}
}

func (s *ServerSettingsServiceImpl) FindServerSettings(
	ctx context.Context,
	req *serversettings.FindServerSettingsRequest,
) (*serversettings.FindServerSettingsResponse, error) {
	filter := &filters.FindServerSetting{
		ServerIDs: []uint{uint(req.ServerId)},
		Names:     req.Names,
	}

	settings, err := s.serverSettingRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		return nil, err
	}

	return &serversettings.FindServerSettingsResponse{
		Settings: convertServerSettingsToProto(settings),
	}, nil
}

func (s *ServerSettingsServiceImpl) SaveServerSetting(
	ctx context.Context,
	req *serversettings.SaveServerSettingRequest,
) (*serversettings.SaveServerSettingResponse, error) {
	if msg := s.guard.Check(ctx, ModuleServerSettings, "save_server_setting"); msg != "" {
		return &serversettings.SaveServerSettingResponse{Success: false, Error: new(msg)}, nil
	}

	setting := &domain.ServerSetting{
		ServerID: uint(req.ServerId),
		Name:     req.Name,
		Value:    domain.NewServerSettingValue(req.Value),
	}

	err := s.serverSettingRepo.Save(ctx, setting)
	// The setting name is recorded, its value is not: settings carry
	// credentials (rcon passwords, tokens).
	s.guard.Audit(ctx, audit.EventPluginServerSetting, "save", "server", serverResourceID(req.ServerId), err,
		slog.String("setting", req.Name))

	if err != nil {
		return &serversettings.SaveServerSettingResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &serversettings.SaveServerSettingResponse{Success: true}, nil
}

func convertServerSettingsToProto(settings []domain.ServerSetting) []*proto.ServerSetting {
	return lo.Map(settings, func(s domain.ServerSetting, _ int) *proto.ServerSetting {
		value, _ := s.Value.String()

		return &proto.ServerSetting{
			Id:       uint64(s.ID),
			ServerId: uint64(s.ServerID),
			Name:     s.Name,
			Value:    value,
		}
	})
}

type ServerSettingsHostLibrary struct {
	impl *ServerSettingsServiceImpl
}

func (l *ServerSettingsHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return serversettings.Instantiate(ctx, r, l.impl)
}

// ServerSettingsHostLibraryFactory builds a per-plugin serversettings module
// bound to the plugin's guard.
type ServerSettingsHostLibraryFactory struct {
	serverSettingRepo repositories.ServerSettingRepository
	guard             *Guard
}

func NewServerSettingsHostLibraryFactory(
	serverSettingRepo repositories.ServerSettingRepository,
	guard *Guard,
) *ServerSettingsHostLibraryFactory {
	return &ServerSettingsHostLibraryFactory{serverSettingRepo: serverSettingRepo, guard: guard}
}

func (f *ServerSettingsHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &ServerSettingsHostLibrary{impl: NewServerSettingsService(f.serverSettingRepo, f.guard.For(pluginID))}
}
