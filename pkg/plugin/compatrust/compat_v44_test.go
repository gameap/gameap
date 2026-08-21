package compatrust

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/authz"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/net"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/gameap/gameap/pkg/plugin/sdk/rbac"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// CI copies this file only onto v4.4.1 and master checkouts: it relies on the
// panel 4.4+ SDK modules (authz, rbac, scheduler, net) and on the 4.4+
// LoadedPlugin surface (Protocol, HasScheduledTaskHandler). The archive
// events handler exists only on master and is deliberately not used here.

// scheduledTaskHandler is the optional interface the plugin wrapper
// implements when the module exports a scheduled task handler.
type scheduledTaskHandler interface {
	HandleScheduledTask(context.Context, *scheduler.HandleScheduledTaskRequest) (*scheduler.HandleScheduledTaskResponse, error)
}

func TestRustPluginCompatV44_SchedulerProtocol(t *testing.T) {
	// ARRANGE
	schedulerStub := &stubSchedulerService{}
	netStub := &stubNetService{}
	logs := &stubLogService{}

	loadedPlugin := loadFixturePlugin(t, "scheduler-protocol", []plugin.HostLibrary{
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return scheduler.Instantiate(ctx, r, schedulerStub)
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return net.Instantiate(ctx, r, netStub)
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return log.Instantiate(ctx, r, logs)
		}),
	})
	assertLifecycle(t, loadedPlugin)

	// ASSERT: the fixture exports a scheduled task handler and registered its task
	require.True(t, loadedPlugin.HasScheduledTaskHandler(), "fixture must export the scheduled task handler")
	taskNames := schedulerStub.TaskNames()
	require.NotEmpty(t, taskNames, "plugin must register a task via the scheduler host library")

	handler, ok := loadedPlugin.Instance.(scheduledTaskHandler)
	require.True(t, ok, "plugin instance must expose HandleScheduledTask")

	// ACT: run the registered task once, like the panel scheduler does
	_, err := handler.HandleScheduledTask(context.Background(), &scheduler.HandleScheduledTaskRequest{
		TaskName:    taskNames[0],
		ScheduledAt: 1700000000000,
		Attempt:     1,
	})

	// ASSERT
	require.NoError(t, err, "the scheduled task handler must succeed")
	assert.True(t, schedulerStub.Called("AddTask"), "plugin must add a task via the scheduler host library")
	assert.True(t, schedulerStub.Called("ListTasks"), "plugin must list tasks via the scheduler host library")
	assert.True(t, schedulerStub.Called("RemoveTask"), "plugin must remove a task via the scheduler host library")

	// ACT: protocol registrations collected by Manager.Load
	require.NotNil(t, loadedPlugin.Protocol, "the wrapper must implement the protocol service")

	rconResp, err := loadedPlugin.Protocol.GetRconProtocols(context.Background(), &protocol.GetRconProtocolsRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, rconResp)
	assert.NotEmpty(t, rconResp.Protocols, "fixture must register at least one RCON protocol")

	// ACT
	queryResp, err := loadedPlugin.Protocol.GetQueryProtocols(context.Background(), &protocol.GetQueryProtocolsRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, queryResp)
	assert.NotEmpty(t, queryResp.Protocols, "fixture must register at least one Query protocol")
}

func TestRustPluginCompatV44_AuthzRBAC(t *testing.T) {
	// ARRANGE
	authzStub := &stubAuthzService{}
	rbacStub := &stubRBACService{}
	logs := &stubLogService{}

	loadedPlugin := loadFixturePlugin(t, "authz-rbac", []plugin.HostLibrary{
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return authz.Instantiate(ctx, r, authzStub)
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return rbac.Instantiate(ctx, r, rbacStub)
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return log.Instantiate(ctx, r, logs)
		}),
	})
	assertLifecycle(t, loadedPlugin)

	// ACT: the fixture handles the event only when the stub authz answer
	// (allowed) reaches it, and it saves a role through the rbac library.
	assertServerPostStartHandled(t, loadedPlugin)

	// ASSERT
	assert.True(t, authzStub.Called("Can"), "plugin must check abilities via the authz host library")
	assert.True(t, rbacStub.Called("SaveRole"), "plugin must save a role via the rbac host library")
}
