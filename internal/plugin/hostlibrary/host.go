package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	"github.com/tetratelabs/wazero"
)

// HostInfo is what the panel tells every plugin about itself.
type HostInfo struct {
	PanelVersion     string
	PluginAPIVersion uint32
	InstanceID       string
}

// PluginStateSink is the slice of the plugin manager the gameap-host module
// reads from; satisfied by *pkgplugin.Manager.
type PluginStateSink interface {
	HostModules(dbID uint64) ([]string, bool)
}

// HostServiceImpl implements gameap-host for one plugin. Every RPC is open
// to every plugin and read-only: nothing here changes panel state. Transient
// loads (plugin ID 0) have no row and no index entry, so they get empty
// answers.
type HostServiceImpl struct {
	pluginID uint64
	repo     repositories.PluginRepository
	sink     PluginStateSink
	info     HostInfo
}

func NewHostService(
	pluginID uint64,
	repo repositories.PluginRepository,
	sink PluginStateSink,
	info HostInfo,
) *HostServiceImpl {
	return &HostServiceImpl{
		pluginID: pluginID,
		repo:     repo,
		sink:     sink,
		info:     info,
	}
}

func (s *HostServiceImpl) GetGrants(ctx context.Context, _ *host.GetGrantsRequest) (*host.GetGrantsResponse, error) {
	record, err := s.findRecord(ctx)
	if err != nil {
		s.logFailure(ctx, "failed to read plugin grants", err)

		return &host.GetGrantsResponse{}, nil
	}

	if record == nil {
		return &host.GetGrantsResponse{}, nil
	}

	permissions := make([]string, 0, len(record.AllowedPermissions))
	for _, permission := range record.AllowedPermissions {
		permissions = append(permissions, string(permission))
	}

	return &host.GetGrantsResponse{Permissions: permissions}, nil
}

func (s *HostServiceImpl) GetHostInfo(
	_ context.Context,
	_ *host.GetHostInfoRequest,
) (*host.GetHostInfoResponse, error) {
	resp := &host.GetHostInfoResponse{
		PanelVersion:     s.info.PanelVersion,
		PluginApiVersion: s.info.PluginAPIVersion,
		InstanceId:       s.info.InstanceID,
	}

	if s.sink != nil && s.pluginID != 0 {
		if modules, ok := s.sink.HostModules(s.pluginID); ok {
			resp.Modules = modules
		}
	}

	return resp, nil
}

func (s *HostServiceImpl) findRecord(ctx context.Context) (*domain.Plugin, error) {
	if s.pluginID == 0 {
		return nil, nil
	}

	plugins, err := s.repo.Find(ctx, filters.FindPluginByIDs(domain.Uint64ID(s.pluginID)), nil,
		&filters.Pagination{Limit: 1})
	if err != nil {
		return nil, err
	}

	if len(plugins) == 0 {
		return nil, nil
	}

	return &plugins[0], nil
}

func (s *HostServiceImpl) logFailure(ctx context.Context, message string, err error) {
	slog.ErrorContext(ctx, message,
		slog.Uint64("plugin_id", s.pluginID),
		slog.String("error", err.Error()))
}

type HostHostLibrary struct {
	impl *HostServiceImpl
}

func (l *HostHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return host.Instantiate(ctx, r, l.impl)
}

// HostHostLibraryFactory builds the per-plugin gameap-host module.
type HostHostLibraryFactory struct {
	repo repositories.PluginRepository
	sink PluginStateSink
	info HostInfo
}

func NewHostHostLibraryFactory(
	repo repositories.PluginRepository,
	sink PluginStateSink,
	info HostInfo,
) *HostHostLibraryFactory {
	return &HostHostLibraryFactory{
		repo: repo,
		sink: sink,
		info: info,
	}
}

func (f *HostHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &HostHostLibrary{impl: NewHostService(pluginID, f.repo, f.sink, f.info)}
}
