package gateway

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_buildRegisterAck(t *testing.T) {
	t.Parallel()
	t.Run("empty_repos_yields_zero_length_collections", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, _ := newServiceWithDeps(t)

		// ACT
		ack, err := svc.buildRegisterAck(context.Background(), &proto.RegisterRequest{NodeId: 1})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, ack)
		assert.True(t, ack.Success)
		assert.Empty(t, ack.Servers, "no servers registered for node")
		assert.Empty(t, ack.Games, "no games saved")
		assert.Empty(t, ack.GameMods, "no mods saved")
		assert.Empty(t, ack.PendingTasks, "no pending tasks queued")
		assert.Empty(t, ack.ServerSettings, "no settings expected")
		require.NotNil(t, ack.HeartbeatInterval, "default heartbeat must be set")
		assert.Equal(t, defaultHeartbeatInterval, ack.HeartbeatInterval.AsDuration())
	})

	t.Run("filters_servers_to_node_and_includes_settings_and_mods", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		ctx := context.Background()

		require.NoError(t, deps.nodeRepo.Save(ctx, &domain.Node{
			Enabled: true,
			Name:    "node-linux",
			OS:      domain.NodeOSLinux,
		}))

		require.NoError(t, deps.gameModRepo.Save(ctx, &domain.GameMod{
			GameCode:      "cs",
			Name:          "Classic",
			StartCmdLinux: new("./cs.sh"),
		}))
		// game mod ID 1 should be the only one created
		gameModID := uint(1)

		require.NoError(t, deps.gameRepo.Save(ctx, &domain.Game{Code: "cs", Name: "CS", Enabled: 1}))
		require.NoError(t, deps.gameRepo.Save(ctx, &domain.Game{Code: "csgo", Name: "CSGO", Enabled: 0}))

		ourServer := &domain.Server{
			UID:        uuid.New(),
			Enabled:    true,
			Name:       "ours",
			GameID:     "cs",
			DSID:       1,
			GameModID:  gameModID,
			ServerIP:   "10.0.0.1",
			ServerPort: 27015,
			Dir:        "/srv/cs",
		}
		ourServer.Hydrate()
		require.NoError(t, deps.serverRepo.Save(ctx, ourServer))

		// server on a different node — must NOT appear in ack
		otherServer := &domain.Server{
			UID:        uuid.New(),
			Enabled:    true,
			Name:       "other",
			GameID:     "cs",
			DSID:       2,
			ServerIP:   "10.0.0.2",
			ServerPort: 27016,
			Dir:        "/srv/cs2",
		}
		otherServer.Hydrate()
		require.NoError(t, deps.serverRepo.Save(ctx, otherServer))

		// settings for our server only — must appear
		require.NoError(t, deps.serverSettingRepo.Save(ctx, &domain.ServerSetting{
			ServerID: ourServer.ID,
			Name:     "map",
			Value:    domain.NewServerSettingValue("de_dust2"),
		}))
		// settings for unrelated server — must NOT appear
		require.NoError(t, deps.serverSettingRepo.Save(ctx, &domain.ServerSetting{
			ServerID: otherServer.ID,
			Name:     "ignored",
			Value:    domain.NewServerSettingValue("noise"),
		}))

		// ACT
		ack, err := svc.buildRegisterAck(ctx, &proto.RegisterRequest{NodeId: 1})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, ack)
		assert.True(t, ack.Success)

		require.Len(t, ack.Servers, 1, "only the server for node 1 must be returned")
		assert.Equal(t, "ours", ack.Servers[0].Name)
		require.NotNil(t, ack.Servers[0].StartCommand, "linux start command must be inferred from game mod")
		assert.Equal(t, "./cs.sh", *ack.Servers[0].StartCommand)

		require.Len(t, ack.Games, 1, "only the enabled game must be returned")
		assert.Equal(t, "cs", ack.Games[0].Code)

		require.Len(t, ack.GameMods, 1, "the saved mod must be present")
		assert.Equal(t, "Classic", ack.GameMods[0].Name)

		require.Len(t, ack.ServerSettings, 1, "only settings for the node's servers must appear")
		assert.Equal(t, "map", ack.ServerSettings[0].Name)
		assert.Equal(t, "de_dust2", ack.ServerSettings[0].Value)
	})

	t.Run("pending_tasks_propagated_from_task_handler", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		deps.taskHandler.pendingTasks = []*proto.DaemonTask{
			{Id: 100, NodeId: 1},
			{Id: 200, NodeId: 1},
		}

		// ACT
		ack, err := svc.buildRegisterAck(context.Background(), &proto.RegisterRequest{NodeId: 1})

		// ASSERT
		require.NoError(t, err)
		require.Len(t, ack.PendingTasks, 2)
		assert.Equal(t, uint64(100), ack.PendingTasks[0].Id)
		assert.Equal(t, uint64(200), ack.PendingTasks[1].Id)
	})

	t.Run("pending_tasks_handler_error_does_not_fail_ack", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		deps.taskHandler.pendingErr = errSentinel

		// ACT
		ack, err := svc.buildRegisterAck(context.Background(), &proto.RegisterRequest{NodeId: 1})

		// ASSERT
		require.NoError(t, err, "ack must not fail when task handler errors")
		require.NotNil(t, ack)
		assert.Empty(t, ack.PendingTasks)
	})

	t.Run("nil_task_handler_yields_no_pending_tasks", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, _ := newServiceWithDeps(t)
		svc.taskHandler = nil

		// ACT
		ack, err := svc.buildRegisterAck(context.Background(), &proto.RegisterRequest{NodeId: 1})

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, ack.PendingTasks)
	})

	t.Run("windows_node_uses_windows_start_command", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, deps := newServiceWithDeps(t)
		ctx := context.Background()

		require.NoError(t, deps.nodeRepo.Save(ctx, &domain.Node{
			Enabled: true,
			Name:    "win-node",
			OS:      domain.NodeOSWindows,
		}))

		require.NoError(t, deps.gameModRepo.Save(ctx, &domain.GameMod{
			GameCode:        "cs",
			Name:            "Classic",
			StartCmdLinux:   new("./linux.sh"),
			StartCmdWindows: new("start.exe"),
		}))

		srv := &domain.Server{
			UID:        uuid.New(),
			Enabled:    true,
			Name:       "win-srv",
			GameID:     "cs",
			DSID:       1,
			GameModID:  1,
			ServerIP:   "10.0.0.5",
			ServerPort: 27015,
			Dir:        "C:\\srv",
		}
		srv.Hydrate()
		require.NoError(t, deps.serverRepo.Save(ctx, srv))

		// ACT
		ack, err := svc.buildRegisterAck(ctx, &proto.RegisterRequest{NodeId: 1})

		// ASSERT
		require.NoError(t, err)
		require.Len(t, ack.Servers, 1)
		require.NotNil(t, ack.Servers[0].StartCommand)
		assert.Equal(t, "start.exe", *ack.Servers[0].StartCommand,
			"windows nodes must inherit the windows start command")
	})
}
