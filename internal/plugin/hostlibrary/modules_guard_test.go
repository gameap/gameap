// Security tests for the grant gates of the host libraries that were open to
// every plugin before: OWASP API5:2023 Broken Function Level Authorization
// (server control, daemon tasks, server and setting writes, node commands)
// and API1:2023 Broken Object Level Authorization (the cache namespace of one
// plugin is invisible to another).
package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	intcache "github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/cache"
	"github.com/gameap/gameap/pkg/plugin/sdk/daemontasks"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/gameap/gameap/pkg/plugin/sdk/servercontrol"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/gameap/gameap/pkg/plugin/sdk/serversettings"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerControlService_requires_manage_servers(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.Server{Name: "cs"}))

	recorder := &auditRecorder{}
	denied := NewGuard(grantSet{}, WithGuardAudit(recorder)).For(testPluginID)
	svc := NewServerControlService(repo, &mockServerController{}, denied)

	calls := map[string]func(context.Context, *servercontrol.ServerControlRequest) (*servercontrol.ServerControlResponse, error){
		"start":     svc.StartServer,
		"stop":      svc.StopServer,
		"restart":   svc.RestartServer,
		"update":    svc.UpdateServer,
		"install":   svc.InstallServer,
		"reinstall": svc.ReinstallServer,
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resp, err := call(context.Background(), &servercontrol.ServerControlRequest{ServerId: 1})
			require.NoError(t, err)
			assert.False(t, resp.Success)
			require.NotNil(t, resp.Error)
			assert.Equal(t, "plugin permission manage_servers required", *resp.Error)
		})
	}
}

func TestServerControlService_audits_control_with_plugin_actor(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.Server{Name: "cs", DSID: 4}))

	recorder := &auditRecorder{}
	guard := NewGuard(grantSet{domain.PluginPermissionManageServers: true}, WithGuardAudit(recorder)).For(testPluginID)
	ctrl := &mockServerController{
		startFunc: func(context.Context, *domain.Server) (uint, error) { return 55, nil },
	}

	resp, err := NewServerControlService(repo, ctrl, guard).StartServer(context.Background(),
		&servercontrol.ServerControlRequest{ServerId: 1})
	require.NoError(t, err)
	require.True(t, resp.Success)
	assert.Equal(t, uint64(55), *resp.TaskId)

	events := recorder.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventPluginServerControl, events[0].Type)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	assert.Equal(t, audit.AuthMethodPlugin, events[0].AuthMethod)
	assert.Equal(t, uint(testPluginID), events[0].ActorID)
	assert.Equal(t, "server", events[0].ResourceType)
	assert.Equal(t, "1", events[0].ResourceID)
	assert.Equal(t, "start", events[0].Action)
	assert.Equal(t, "4", attrValue(events[0], "node_id"))
}

func TestNodeCmdService_requires_node_commands(t *testing.T) {
	t.Parallel()

	repo := setupNodeFSRepo(seedTestNode)
	recorder := &auditRecorder{}

	tests := []struct {
		name      string
		grants    grantSet
		wantError string
	}{
		{
			name:      "files_grant_is_not_enough",
			grants:    grantSet{domain.PluginPermissionFiles: true, domain.PluginPermissionManageServers: true},
			wantError: "plugin permission node_commands required",
		},
		{
			name:   "node_commands_grant",
			grants: grantSet{domain.PluginPermissionNodeCommands: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guard := NewGuard(tt.grants, WithGuardAudit(recorder)).For(testPluginID)
			svc := NewNodeCmdService(testPluginID, &mockCommandService{}, repo, guard)

			resp, err := svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
				NodeId: 1, Command: "uptime",
			})
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Equal(t, tt.wantError, *resp.Error)

				return
			}

			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeCmdService_audit_never_carries_the_command(t *testing.T) {
	t.Parallel()

	repo := setupNodeFSRepo(seedTestNode)
	recorder := &auditRecorder{}
	guard := NewGuard(grantSet{domain.PluginPermissionNodeCommands: true}, WithGuardAudit(recorder)).For(testPluginID)
	svc := NewNodeCmdService(testPluginID, &mockCommandService{}, repo, guard)

	resp, err := svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
		NodeId: 1, Command: "mysql -psecret-password", WorkDir: new("/srv"),
	})
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	events := recorder.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventPluginNodeCommand, events[0].Type)
	assert.Equal(t, "node", events[0].ResourceType)
	assert.Equal(t, "1", events[0].ResourceID)
	assert.Equal(t, "/srv", attrValue(events[0], "work_dir"))
	assert.Equal(t, "0", attrValue(events[0], "exit_code"))

	for _, attr := range events[0].Extra {
		assert.NotContains(t, attr.Value.String(), "secret-password")
	}
}

func TestDaemonTasksService_CreateDaemonTask_grants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		grants    grantSet
		taskType  proto.DaemonTaskType
		wantError string
	}{
		{
			name:      "no_grant",
			grants:    grantSet{},
			taskType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START,
			wantError: "plugin permission manage_servers required",
		},
		{
			name:     "manage_servers_allows_server_tasks",
			grants:   grantSet{domain.PluginPermissionManageServers: true},
			taskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START,
		},
		{
			name:      "cmdexec_additionally_needs_node_commands",
			grants:    grantSet{domain.PluginPermissionManageServers: true},
			taskType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
			wantError: "plugin permission node_commands required",
		},
		{
			name:      "node_commands_alone_is_not_enough_for_cmdexec",
			grants:    grantSet{domain.PluginPermissionNodeCommands: true},
			taskType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
			wantError: "plugin permission manage_servers required",
		},
		{
			name: "cmdexec_with_both_grants",
			grants: grantSet{
				domain.PluginPermissionManageServers: true,
				domain.PluginPermissionNodeCommands:  true,
			},
			taskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := &auditRecorder{}
			guard := NewGuard(tt.grants, WithGuardAudit(recorder)).For(testPluginID)
			svc := NewDaemonTasksService(inmemory.NewDaemonTaskRepository(), nil, guard)

			resp, err := svc.CreateDaemonTask(context.Background(), &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				TaskType: tt.taskType,
				Cmd:      new("ls"),
			})
			require.NoError(t, err)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Equal(t, tt.wantError, *resp.Error)

				return
			}

			assert.True(t, resp.Success)
			assert.NotZero(t, resp.TaskId)

			events := recorder.all()
			require.Len(t, events, 1)
			assert.Equal(t, audit.EventPluginTaskCreate, events[0].Type)
			assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
			assert.Equal(t, "node", events[0].ResourceType)
			assert.Equal(t, "1", events[0].ResourceID)
		})
	}
}

func TestServersService_writes_require_manage_servers(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerRepository()
	denied := NewGuard(grantSet{domain.PluginPermissionFiles: true}).For(testPluginID)
	svc := NewServersService(repo, denied)

	saveResp, err := svc.SaveServer(context.Background(), &servers.SaveServerRequest{
		Server: &proto.Server{Name: "new"},
	})
	require.NoError(t, err)
	assert.False(t, saveResp.Success)
	assert.Equal(t, "plugin permission manage_servers required", *saveResp.Error)

	deleteResp, err := svc.DeleteServer(context.Background(), &servers.DeleteServerRequest{Id: 1})
	require.NoError(t, err)
	assert.False(t, deleteResp.Success)
	assert.Equal(t, "plugin permission manage_servers required", *deleteResp.Error)

	// Reads stay open.
	findResp, err := svc.FindServers(context.Background(), &servers.FindServersRequest{})
	require.NoError(t, err)
	assert.NotNil(t, findResp)
}

func TestServersService_save_audits_create_and_update(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerRepository()
	recorder := &auditRecorder{}
	guard := NewGuard(grantSet{domain.PluginPermissionManageServers: true}, WithGuardAudit(recorder)).For(testPluginID)
	svc := NewServersService(repo, guard)

	created, err := svc.SaveServer(context.Background(), &servers.SaveServerRequest{
		Server: &proto.Server{Name: "new", DsId: 2},
	})
	require.NoError(t, err)
	require.True(t, created.Success)

	updated, err := svc.SaveServer(context.Background(), &servers.SaveServerRequest{
		Server: &proto.Server{Id: created.Id, Name: "renamed", DsId: 2},
	})
	require.NoError(t, err)
	require.True(t, updated.Success)

	events := recorder.all()
	require.Len(t, events, 2)
	assert.Equal(t, audit.EventPluginServerSave, events[0].Type)
	assert.Equal(t, "create", events[0].Action)
	assert.Equal(t, "update", events[1].Action)
	assert.Equal(t, "server", events[1].ResourceType)
}

func TestServerSettingsService_save_requires_manage_servers_and_hides_value(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerSettingRepository()

	denied := NewServerSettingsService(repo, NewGuard(grantSet{}).For(testPluginID))
	resp, err := denied.SaveServerSetting(context.Background(), &serversettings.SaveServerSettingRequest{
		ServerId: 1, Name: "rcon_password", Value: "hunter2",
	})
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "plugin permission manage_servers required", *resp.Error)

	recorder := &auditRecorder{}
	allowed := NewServerSettingsService(repo,
		NewGuard(grantSet{domain.PluginPermissionManageServers: true}, WithGuardAudit(recorder)).For(testPluginID))
	resp, err = allowed.SaveServerSetting(context.Background(), &serversettings.SaveServerSettingRequest{
		ServerId: 1, Name: "rcon_password", Value: "hunter2",
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	events := recorder.all()
	require.Len(t, events, 1)
	assert.Equal(t, audit.EventPluginServerSetting, events[0].Type)
	assert.Equal(t, "rcon_password", attrValue(events[0], "setting"))

	for _, attr := range events[0].Extra {
		assert.NotContains(t, attr.Value.String(), "hunter2", "setting values may be credentials")
	}
}

func TestCacheHostLibraryFactory_isolates_plugins(t *testing.T) {
	t.Parallel()

	backend := intcache.NewInMemory()
	factory := NewCacheHostLibraryFactory(backend, "plugin:")

	first, ok := factory.Create(1).(*CacheHostLibrary)
	require.True(t, ok)
	second, ok := factory.Create(2).(*CacheHostLibrary)
	require.True(t, ok)

	ctx := context.Background()

	setResp, err := first.impl.Set(ctx, &cache.CacheSetRequest{Key: "token", Value: []byte("first")})
	require.NoError(t, err)
	require.True(t, setResp.Success)

	got, err := second.impl.Get(ctx, &cache.CacheGetRequest{Key: "token"})
	require.NoError(t, err)
	assert.False(t, got.Found, "another plugin must not see the entry")

	delResp, err := second.impl.Delete(ctx, &cache.CacheDeleteRequest{Key: "token"})
	require.NoError(t, err)
	assert.True(t, delResp.Success)

	got, err = first.impl.Get(ctx, &cache.CacheGetRequest{Key: "token"})
	require.NoError(t, err)
	assert.True(t, got.Found, "another plugin's delete must not reach the entry")
	assert.Equal(t, []byte("first"), got.Value)

	assert.Equal(t, "plugin:1:", first.impl.keyPrefix)
	assert.Equal(t, "plugin:2:", second.impl.keyPrefix)
}

func TestCacheService_value_size_cap(t *testing.T) {
	t.Parallel()

	svc := NewCacheService(intcache.NewInMemory(), "plugin:1:", WithCacheMaxValueBytes(4))

	resp, err := svc.Set(context.Background(), &cache.CacheSetRequest{Key: "k", Value: []byte("12345")})
	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "value too large: 5 bytes exceeds the cache value limit of 4 bytes")

	resp, err = svc.Set(context.Background(), &cache.CacheSetRequest{Key: "k", Value: []byte("1234")})
	require.NoError(t, err)
	assert.True(t, resp.Success)
}
