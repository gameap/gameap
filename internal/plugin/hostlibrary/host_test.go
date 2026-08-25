package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStateSink struct {
	modules map[uint64][]string
}

func (f *fakeStateSink) HostModules(dbID uint64) ([]string, bool) {
	modules, ok := f.modules[dbID]

	return modules, ok
}

func hostTestRecord(t *testing.T) *inmemory.PluginRepository {
	t.Helper()

	repo := inmemory.NewPluginRepository()
	record := &domain.Plugin{
		ID:                 domain.Uint64ID(testPluginID),
		Name:               "introspective",
		Version:            "1.0.0",
		Status:             domain.PluginStatusActive,
		AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionFiles, domain.PluginPermissionSecrets},
	}
	require.NoError(t, repo.Save(context.Background(), record))

	return repo
}

func TestHostService_GetGrants(t *testing.T) {
	t.Parallel()

	repo := hostTestRecord(t)
	svc := NewHostService(testPluginID, repo, &fakeStateSink{}, HostInfo{})

	resp, err := svc.GetGrants(context.Background(), &host.GetGrantsRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"files", "secrets"}, resp.Permissions)

	transient := NewHostService(0, repo, &fakeStateSink{}, HostInfo{})
	resp, err = transient.GetGrants(context.Background(), &host.GetGrantsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Permissions)

	unknown := NewHostService(99, repo, &fakeStateSink{}, HostInfo{})
	resp, err = unknown.GetGrants(context.Background(), &host.GetGrantsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Permissions)
}

func TestHostService_GetHostInfo(t *testing.T) {
	t.Parallel()

	sink := &fakeStateSink{modules: map[uint64][]string{testPluginID: {"gameap-host", "gameap-log"}}}
	info := HostInfo{PanelVersion: "4.5.0", PluginAPIVersion: 1, InstanceID: "panel-a"}

	resp, err := NewHostService(testPluginID, inmemory.NewPluginRepository(), sink, info).
		GetHostInfo(context.Background(), &host.GetHostInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, "4.5.0", resp.PanelVersion)
	assert.Equal(t, uint32(1), resp.PluginApiVersion)
	assert.Equal(t, "panel-a", resp.InstanceId)
	assert.Equal(t, []string{"gameap-host", "gameap-log"}, resp.Modules)

	resp, err = NewHostService(0, inmemory.NewPluginRepository(), sink, info).
		GetHostInfo(context.Background(), &host.GetHostInfoRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Modules, "a transient load is not indexed")
	assert.Equal(t, "4.5.0", resp.PanelVersion)
}

func TestHostHostLibraryFactory_binds_the_plugin_id(t *testing.T) {
	t.Parallel()

	factory := NewHostHostLibraryFactory(inmemory.NewPluginRepository(), &fakeStateSink{}, HostInfo{})
	library, ok := factory.Create(testPluginID).(*HostHostLibrary)
	require.True(t, ok)
	assert.Equal(t, testPluginID, library.impl.pluginID)
}
