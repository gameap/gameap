package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/tetratelabs/wazero"
)

// Fixed messages handed to the guest: the plugin can act on none of the
// underlying causes, and a raw driver error would describe the panel's
// database to it. The cause is logged instead.
const (
	hostGrantsFailureMessage = "failed to read plugin grants"
	hostConfigFailureMessage = "failed to read plugin configuration"
)

// HostInfo is what the panel tells every plugin about itself.
type HostInfo struct {
	PanelVersion     string
	PluginAPIVersion uint32
	InstanceID       string
}

// PluginStateSink is the slice of the plugin manager the gameap-host module
// writes to and reads from; satisfied by *pkgplugin.Manager.
type PluginStateSink interface {
	SetHealth(dbID uint64, report pkgplugin.HealthReport) bool
	HostModules(dbID uint64) ([]string, bool)
	ManifestConfigSchema(dbID uint64) (string, bool)
}

// HostServiceImpl implements gameap-host for one plugin. Every RPC is open
// to every plugin: nothing here changes panel state beyond the plugin's own
// health report. Transient loads (plugin ID 0) have no row and no index
// entry, so they get empty answers.
type HostServiceImpl struct {
	pluginID uint64
	repo     repositories.PluginRepository
	cipher   *secret.Cipher
	sink     PluginStateSink
	info     HostInfo
}

func NewHostService(
	pluginID uint64,
	repo repositories.PluginRepository,
	cipher *secret.Cipher,
	sink PluginStateSink,
	info HostInfo,
) *HostServiceImpl {
	return &HostServiceImpl{
		pluginID: pluginID,
		repo:     repo,
		cipher:   cipher,
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

func (s *HostServiceImpl) GetConfig(ctx context.Context, _ *host.GetConfigRequest) (*host.GetConfigResponse, error) {
	record, err := s.findRecord(ctx)
	if err != nil {
		s.logFailure(ctx, "failed to read plugin configuration", err)

		return &host.GetConfigResponse{Error: new(hostConfigFailureMessage)}, nil
	}

	if record == nil {
		return &host.GetConfigResponse{Found: false}, nil
	}

	s.adoptManifestSchema(record)

	values, err := pluginconfig.Effective(s.cipher, record)
	if err != nil {
		// The message names the key, never the value; the guest gets a
		// fixed text so a rotated ENCRYPTION_KEY is diagnosed in the panel
		// log, not in a plugin.
		s.logFailure(ctx, "failed to resolve plugin configuration", err)

		return &host.GetConfigResponse{Found: true, HasSchema: record.HasConfigSchema(),
			Error: new(hostConfigFailureMessage)}, nil
	}

	return &host.GetConfigResponse{
		Values:    values,
		Found:     true,
		HasSchema: record.HasConfigSchema(),
	}, nil
}

// adoptManifestSchema fills a row that carries no config_schema yet from
// the running module, so the first Initialize after an install reads the
// same defaults the loader persists once the load succeeded.
func (s *HostServiceImpl) adoptManifestSchema(record *domain.Plugin) {
	if record.HasConfigSchema() || s.sink == nil {
		return
	}

	schema, ok := s.sink.ManifestConfigSchema(s.pluginID)
	if !ok || schema == "" {
		return
	}

	record.ConfigSchema = &schema
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

func (s *HostServiceImpl) ReportStatus(
	_ context.Context,
	req *host.ReportStatusRequest,
) (*host.ReportStatusResponse, error) {
	if s.sink == nil || s.pluginID == 0 {
		return &host.ReportStatusResponse{Accepted: false}, nil
	}

	accepted := s.sink.SetHealth(s.pluginID, pkgplugin.HealthReport{
		Status:  healthStatusFromProto(req.Status),
		Message: req.Message,
		Details: req.Details,
	})

	return &host.ReportStatusResponse{Accepted: accepted}, nil
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

func healthStatusFromProto(status host.HealthStatus) pkgplugin.HealthStatus {
	switch status {
	case host.HealthStatus_HEALTH_STATUS_HEALTHY:
		return pkgplugin.HealthHealthy
	case host.HealthStatus_HEALTH_STATUS_DEGRADED:
		return pkgplugin.HealthDegraded
	case host.HealthStatus_HEALTH_STATUS_UNHEALTHY:
		return pkgplugin.HealthUnhealthy
	case host.HealthStatus_HEALTH_STATUS_UNSPECIFIED:
		return pkgplugin.HealthUnknown
	default:
		return pkgplugin.HealthUnknown
	}
}

type HostHostLibrary struct {
	impl *HostServiceImpl
}

func (l *HostHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return host.Instantiate(ctx, r, l.impl)
}

// HostHostLibraryFactory builds the per-plugin gameap-host module.
type HostHostLibraryFactory struct {
	repo   repositories.PluginRepository
	cipher *secret.Cipher
	sink   PluginStateSink
	info   HostInfo
}

func NewHostHostLibraryFactory(
	repo repositories.PluginRepository,
	cipher *secret.Cipher,
	sink PluginStateSink,
	info HostInfo,
) *HostHostLibraryFactory {
	return &HostHostLibraryFactory{
		repo:   repo,
		cipher: cipher,
		sink:   sink,
		info:   info,
	}
}

func (f *HostHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &HostHostLibrary{impl: NewHostService(pluginID, f.repo, f.cipher, f.sink, f.info)}
}
