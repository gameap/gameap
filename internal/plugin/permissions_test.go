package plugin

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
)

func TestUsedPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		imports []pkgplugin.HostImport
		events  []proto.EventType
		want    []domain.PluginPermission
	}{
		{
			name: "nothing_gated",
			imports: []pkgplugin.HostImport{
				{Module: "gameap-log", Function: "log"},
				{Module: "gameap-servers", Function: "find_servers"},
				{Module: "gameap-http", Function: "fetch"},
			},
			want: []domain.PluginPermission{},
		},
		{
			name: "gated_imports_collapse_to_their_grants_in_panel_order",
			imports: []pkgplugin.HostImport{
				{Module: "gameap-nodecmd", Function: "execute_command"},
				{Module: "gameap-nodefs", Function: "read_dir"},
				{Module: "gameap-nodefs", Function: "upload"},
				{Module: "gameap-servercontrol", Function: "start_server"},
				{Module: "gameap-secrets", Function: "get"},
			},
			want: []domain.PluginPermission{
				domain.PluginPermissionManageServers,
				domain.PluginPermissionFiles,
				domain.PluginPermissionSecrets,
				domain.PluginPermissionNodeCommands,
			},
		},
		{
			name:   "subscriptions_imply_listen_events",
			events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
			want:   []domain.PluginPermission{domain.PluginPermissionListenEvents},
		},
		{
			name:    "unknown_module_is_ignored",
			imports: []pkgplugin.HostImport{{Module: "gameap-future", Function: "do"}},
			want:    []domain.PluginPermission{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, UsedPermissions(tt.imports, tt.events))
		})
	}
}

func TestMissingPermissions(t *testing.T) {
	t.Parallel()

	used := []domain.PluginPermission{
		domain.PluginPermissionFiles,
		domain.PluginPermissionNodeCommands,
		domain.PluginPermissionListenEvents,
	}

	assert.Equal(t, []domain.PluginPermission{domain.PluginPermissionNodeCommands},
		MissingPermissions(used, []domain.PluginPermission{
			domain.PluginPermissionFiles, domain.PluginPermissionListenEvents, domain.PluginPermissionManageRBAC,
		}))
	assert.Equal(t, used, MissingPermissions(used, nil))
	assert.Empty(t, MissingPermissions(nil, nil))
}

func TestPermissionNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"files", "secrets"}, PermissionNames([]domain.PluginPermission{
		domain.PluginPermissionFiles, domain.PluginPermissionSecrets,
	}))
	assert.Equal(t, []string{}, PermissionNames(nil), "JSON gets an empty list, not null")
}
