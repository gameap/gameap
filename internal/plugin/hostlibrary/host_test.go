package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStateSink struct {
	modules  map[uint64][]string
	schemas  map[uint64]string
	reports  map[uint64]pkgplugin.HealthReport
	accepted bool
}

func (f *fakeStateSink) ManifestConfigSchema(dbID uint64) (string, bool) {
	schema, ok := f.schemas[dbID]

	return schema, ok
}

func (f *fakeStateSink) SetHealth(dbID uint64, report pkgplugin.HealthReport) bool {
	if f.reports == nil {
		f.reports = make(map[uint64]pkgplugin.HealthReport)
	}

	f.reports[dbID] = report

	return f.accepted
}

func (f *fakeStateSink) HostModules(dbID uint64) ([]string, bool) {
	modules, ok := f.modules[dbID]

	return modules, ok
}

func hostTestRecord(t *testing.T, cipher *secret.Cipher) *inmemory.PluginRepository {
	t.Helper()

	envelope, err := pluginconfig.EncryptSecret(cipher, testPluginID, "api_key", "s3cret")
	require.NoError(t, err)

	repo := inmemory.NewPluginRepository()
	record := &domain.Plugin{
		ID:                 domain.Uint64ID(testPluginID),
		Name:               "introspective",
		Version:            "1.0.0",
		Status:             domain.PluginStatusActive,
		AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionFiles, domain.PluginPermissionSecrets},
		Config:             map[string]any{"api_key": envelope, "port": float64(9000)},
		ConfigSchema:       new(`{"properties": {"port": {"type": "integer", "default": 80}, "region": {"type": "string", "default": "eu"}}}`),
	}
	require.NoError(t, repo.Save(context.Background(), record))

	return repo
}

func TestHostService_GetGrants(t *testing.T) {
	t.Parallel()

	repo := hostTestRecord(t, secret.Disabled())
	svc := NewHostService(testPluginID, repo, secret.Disabled(), &fakeStateSink{}, HostInfo{})

	resp, err := svc.GetGrants(context.Background(), &host.GetGrantsRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"files", "secrets"}, resp.Permissions)

	transient := NewHostService(0, repo, secret.Disabled(), &fakeStateSink{}, HostInfo{})
	resp, err = transient.GetGrants(context.Background(), &host.GetGrantsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Permissions)

	unknown := NewHostService(99, repo, secret.Disabled(), &fakeStateSink{}, HostInfo{})
	resp, err = unknown.GetGrants(context.Background(), &host.GetGrantsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Permissions)
}

func TestHostService_GetConfig_returns_effective_configuration(t *testing.T) {
	t.Parallel()

	cipher, err := secret.NewCipher("host-test-key-0123456789abcdef0123")
	require.NoError(t, err)

	repo := hostTestRecord(t, cipher)
	svc := NewHostService(testPluginID, repo, cipher, &fakeStateSink{}, HostInfo{})

	resp, err := svc.GetConfig(context.Background(), &host.GetConfigRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.True(t, resp.HasSchema)
	assert.Nil(t, resp.Error)
	assert.Equal(t, map[string]string{"api_key": "s3cret", "port": "9000", "region": "eu"}, resp.Values)

	transient := NewHostService(0, repo, cipher, &fakeStateSink{}, HostInfo{})
	resp, err = transient.GetConfig(context.Background(), &host.GetConfigRequest{})
	require.NoError(t, err)
	assert.False(t, resp.Found)
	assert.Empty(t, resp.Values)
}

func TestHostService_GetConfig_adopts_the_manifest_schema_on_first_load(t *testing.T) {
	t.Parallel()

	// The row of a freshly installed plugin carries no config_schema until
	// the loader persists the load outcome; inside the first Initialize the
	// running module's manifest is the only source of the defaults.
	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.Plugin{
		ID:     domain.Uint64ID(testPluginID),
		Name:   "fresh",
		Status: domain.PluginStatusActive,
		Config: map[string]any{"port": int64(9000)},
	}))

	sink := &fakeStateSink{schemas: map[uint64]string{
		testPluginID: `{"properties": {"port": {"type": "integer", "default": 80}, "region": {"type": "string", "default": "eu"}}}`,
	}}
	svc := NewHostService(testPluginID, repo, secret.Disabled(), sink, HostInfo{})

	resp, err := svc.GetConfig(context.Background(), &host.GetConfigRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.True(t, resp.HasSchema)
	assert.Equal(t, map[string]string{"port": "9000", "region": "eu"}, resp.Values)

	stored, err := repo.Find(context.Background(), filters.FindPluginByIDs(domain.Uint64ID(testPluginID)), nil, nil)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Nil(t, stored[0].ConfigSchema, "the row is left for the loader to update")

	noModule := NewHostService(testPluginID, repo, secret.Disabled(), &fakeStateSink{}, HostInfo{})
	resp, err = noModule.GetConfig(context.Background(), &host.GetConfigRequest{})
	require.NoError(t, err)
	assert.False(t, resp.HasSchema)
	assert.Equal(t, map[string]string{"port": "9000"}, resp.Values)
}

func TestHostService_GetConfig_decrypt_failure_is_reported_without_the_key_name(t *testing.T) {
	t.Parallel()

	cipher, err := secret.NewCipher("host-test-key-0123456789abcdef0123")
	require.NoError(t, err)

	repo := hostTestRecord(t, cipher)

	other, err := secret.NewCipher("another-key-0123456789abcdef012345")
	require.NoError(t, err)

	svc := NewHostService(testPluginID, repo, other, &fakeStateSink{}, HostInfo{})

	resp, err := svc.GetConfig(context.Background(), &host.GetConfigRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	require.NotNil(t, resp.Error)
	assert.Equal(t, hostConfigFailureMessage, *resp.Error)
	assert.Empty(t, resp.Values)
}

func TestHostService_GetHostInfo(t *testing.T) {
	t.Parallel()

	sink := &fakeStateSink{modules: map[uint64][]string{testPluginID: {"gameap-host", "gameap-log"}}}
	info := HostInfo{PanelVersion: "4.5.0", PluginAPIVersion: 1, InstanceID: "panel-a"}

	resp, err := NewHostService(testPluginID, inmemory.NewPluginRepository(), nil, sink, info).
		GetHostInfo(context.Background(), &host.GetHostInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, "4.5.0", resp.PanelVersion)
	assert.Equal(t, uint32(1), resp.PluginApiVersion)
	assert.Equal(t, "panel-a", resp.InstanceId)
	assert.Equal(t, []string{"gameap-host", "gameap-log"}, resp.Modules)

	resp, err = NewHostService(0, inmemory.NewPluginRepository(), nil, sink, info).
		GetHostInfo(context.Background(), &host.GetHostInfoRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Modules, "a transient load is not indexed")
	assert.Equal(t, "4.5.0", resp.PanelVersion)
}

func TestHostService_ReportStatus(t *testing.T) {
	t.Parallel()

	sink := &fakeStateSink{accepted: true}
	svc := NewHostService(testPluginID, inmemory.NewPluginRepository(), nil, sink, HostInfo{})

	resp, err := svc.ReportStatus(context.Background(), &host.ReportStatusRequest{
		Status:  host.HealthStatus_HEALTH_STATUS_DEGRADED,
		Message: "steam api unreachable",
		Details: map[string]string{"endpoint": "api.steampowered.com"},
	})
	require.NoError(t, err)
	assert.True(t, resp.Accepted)

	report := sink.reports[testPluginID]
	assert.Equal(t, pkgplugin.HealthDegraded, report.Status)
	assert.Equal(t, "steam api unreachable", report.Message)
	assert.Equal(t, map[string]string{"endpoint": "api.steampowered.com"}, report.Details)

	transient := NewHostService(0, inmemory.NewPluginRepository(), nil, sink, HostInfo{})
	resp, err = transient.ReportStatus(context.Background(), &host.ReportStatusRequest{Status: host.HealthStatus_HEALTH_STATUS_HEALTHY})
	require.NoError(t, err)
	assert.False(t, resp.Accepted)

	assert.Equal(t, pkgplugin.HealthHealthy, healthStatusFromProto(host.HealthStatus_HEALTH_STATUS_HEALTHY))
	assert.Equal(t, pkgplugin.HealthUnhealthy, healthStatusFromProto(host.HealthStatus_HEALTH_STATUS_UNHEALTHY))
	assert.Equal(t, pkgplugin.HealthUnknown, healthStatusFromProto(host.HealthStatus_HEALTH_STATUS_UNSPECIFIED))
	assert.Equal(t, pkgplugin.HealthUnknown, healthStatusFromProto(host.HealthStatus(42)))
}

func TestHostHostLibraryFactory_binds_the_plugin_id(t *testing.T) {
	t.Parallel()

	factory := NewHostHostLibraryFactory(inmemory.NewPluginRepository(), nil, &fakeStateSink{}, HostInfo{})
	library, ok := factory.Create(testPluginID).(*HostHostLibrary)
	require.True(t, ok)
	assert.Equal(t, testPluginID, library.impl.pluginID)
}
