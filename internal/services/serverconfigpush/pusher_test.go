// Tests use a hand-written stub taskSender (Option (b) in the test plan): the
// Pusher only ever calls SendTask on its registry, so a small interface in
// contracts.go lets us avoid wiring a real *session.Registry, gRPC stream, and
// pubsub for every test case.

package serverconfigpush

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTaskSender struct {
	mu    sync.Mutex
	calls []sentTask
	err   error
}

type sentTask struct {
	nodeID uint64
	msg    *proto.GatewayMessage
}

func (s *stubTaskSender) SendTask(_ context.Context, nodeID uint64, msg *proto.GatewayMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, sentTask{nodeID: nodeID, msg: msg})

	return s.err
}

func (s *stubTaskSender) snapshot() []sentTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]sentTask, len(s.calls))
	copy(out, s.calls)

	return out
}

// discardLogger returns a logger whose output is discarded so tests stay quiet
// without coupling to slog.Default()'s configuration.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestPusher_PushServerConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		serverID      uint
		setupServer   func(*inmemory.ServerRepository)
		setupSettings func(*inmemory.ServerSettingRepository)
		setupGame     func(*inmemory.GameRepository)
		setupGameMod  func(*inmemory.GameModRepository)
		setupNode     func(*inmemory.NodeRepository)
		setupSender   func(*stubTaskSender)
		validate      func(t *testing.T, sender *stubTaskSender)
	}{
		{
			name:     "happy_path_pushes_full_config_for_linux_node",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      10,
					GameID:    "csgo",
					GameModID: 5,
					Name:      "happy-server",
					Enabled:   true,
				})
			},
			setupSettings: func(repo *inmemory.ServerSettingRepository) {
				_ = repo.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "autostart",
					Value:    domain.NewServerSettingValue(true),
				})
			},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code: "csgo",
					Name: "Counter-Strike: Global Offensive",
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:            5,
					GameCode:      "csgo",
					Name:          "Public Server",
					StartCmdLinux: new("./srcds_run -game csgo"),
				})
			},
			setupNode: func(repo *inmemory.NodeRepository) {
				_ = repo.Save(context.Background(), &domain.Node{
					ID:   10,
					Name: "linux-node",
					OS:   domain.NodeOSLinux,
				})
			},
			setupSender: func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1, "SendTask must be called exactly once")
				assert.Equal(t, uint64(10), calls[0].nodeID, "node id must come from server.DSID")

				msg := calls[0].msg
				require.NotNil(t, msg)
				assert.NotEmpty(t, msg.RequestId, "request id must be populated")

				update := msg.GetServerConfigUpdate()
				require.NotNil(t, update, "payload must be ServerConfigUpdate")

				require.NotNil(t, update.Server)
				assert.Equal(t, uint64(1), update.Server.Id)
				assert.Equal(t, "csgo", update.Server.GameId)
				assert.Equal(t, uint64(10), update.Server.DsId)
				assert.Equal(t, uint64(5), update.Server.GameModId)

				require.NotNil(t, update.Game, "game must be present when game exists")
				assert.Equal(t, "csgo", update.Game.Code)

				require.NotNil(t, update.GameMod, "game mod must be present when mod exists")
				assert.Equal(t, uint64(5), update.GameMod.Id)

				require.Len(t, update.Settings, 1, "exactly one setting must be carried over")
				assert.Equal(t, "autostart", update.Settings[0].Name)
				assert.Equal(t, uint64(1), update.Settings[0].ServerId)
			},
		},
		{
			name:     "server_not_found_skips_send",
			serverID: 999,
			setupServer: func(_ *inmemory.ServerRepository) {
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame:     func(_ *inmemory.GameRepository) {},
			setupGameMod:  func(_ *inmemory.GameModRepository) {},
			setupNode:     func(_ *inmemory.NodeRepository) {},
			setupSender:   func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				assert.Empty(t, sender.snapshot(), "no SendTask call when server is missing")
			},
		},
		{
			name:     "missing_game_mod_still_sends_with_nil_game_mod",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      10,
					GameID:    "csgo",
					GameModID: 999,
				})
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{Code: "csgo", Name: "CS:GO"})
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			setupNode: func(repo *inmemory.NodeRepository) {
				_ = repo.Save(context.Background(), &domain.Node{
					ID: 10,
					OS: domain.NodeOSLinux,
				})
			},
			setupSender: func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1)

				update := calls[0].msg.GetServerConfigUpdate()
				require.NotNil(t, update)
				assert.Nil(t, update.GameMod, "game mod must be nil when not found")
				require.NotNil(t, update.Game)
			},
		},
		{
			name:     "missing_game_still_sends_with_nil_game",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      10,
					GameID:    "unknown-game",
					GameModID: 5,
				})
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame:     func(_ *inmemory.GameRepository) {},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       5,
					GameCode: "unknown-game",
					Name:     "Mod",
				})
			},
			setupNode: func(repo *inmemory.NodeRepository) {
				_ = repo.Save(context.Background(), &domain.Node{
					ID: 10,
					OS: domain.NodeOSLinux,
				})
			},
			setupSender: func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1)

				update := calls[0].msg.GetServerConfigUpdate()
				require.NotNil(t, update)
				assert.Nil(t, update.Game, "game must be nil when not found")
				require.NotNil(t, update.GameMod)
			},
		},
		{
			name:     "missing_node_still_sends_with_default_os_in_proto",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      42,
					GameID:    "csgo",
					GameModID: 5,
				})
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{Code: "csgo", Name: "CS:GO"})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:              5,
					GameCode:        "csgo",
					Name:            "Mod",
					StartCmdLinux:   new("./linux-start"),
					StartCmdWindows: new("start.bat"),
				})
			},
			setupNode:   func(_ *inmemory.NodeRepository) {},
			setupSender: func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1, "send still happens even when node lookup yields nothing")
				assert.Equal(t, uint64(42), calls[0].nodeID)

				update := calls[0].msg.GetServerConfigUpdate()
				require.NotNil(t, update)
				require.NotNil(t, update.Server)
				if update.Server.StartCommand != nil {
					assert.Equal(t, "./linux-start", *update.Server.StartCommand,
						"missing node OS is treated as Linux by the converter")
				}
			},
		},
		{
			name:     "no_settings_yields_empty_settings_slice",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      10,
					GameID:    "csgo",
					GameModID: 5,
				})
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{Code: "csgo", Name: "CS:GO"})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{ID: 5, GameCode: "csgo", Name: "Mod"})
			},
			setupNode: func(repo *inmemory.NodeRepository) {
				_ = repo.Save(context.Background(), &domain.Node{ID: 10, OS: domain.NodeOSLinux})
			},
			setupSender: func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1)

				update := calls[0].msg.GetServerConfigUpdate()
				require.NotNil(t, update)
				assert.Empty(t, update.Settings, "settings must be empty when none are stored")
			},
		},
		{
			name:     "send_task_error_is_swallowed_no_panic",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      10,
					GameID:    "csgo",
					GameModID: 5,
				})
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{Code: "csgo", Name: "CS:GO"})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{ID: 5, GameCode: "csgo", Name: "Mod"})
			},
			setupNode: func(repo *inmemory.NodeRepository) {
				_ = repo.Save(context.Background(), &domain.Node{ID: 10, OS: domain.NodeOSLinux})
			},
			setupSender: func(s *stubTaskSender) {
				s.err = errors.WithMessage(errors.New("daemon offline"), "send")
			},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1, "method still calls SendTask even when it returns an error")
			},
		},
		{
			name:     "windows_node_uses_windows_start_command",
			serverID: 1,
			setupServer: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:        1,
					DSID:      77,
					GameID:    "csgo",
					GameModID: 5,
				})
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{Code: "csgo", Name: "CS:GO"})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:              5,
					GameCode:        "csgo",
					Name:            "Mod",
					StartCmdLinux:   new("./linux-start"),
					StartCmdWindows: new("start.bat"),
				})
			},
			setupNode: func(repo *inmemory.NodeRepository) {
				_ = repo.Save(context.Background(), &domain.Node{
					ID: 77,
					OS: domain.NodeOSWindows,
				})
			},
			setupSender: func(_ *stubTaskSender) {},
			validate: func(t *testing.T, sender *stubTaskSender) {
				t.Helper()

				calls := sender.snapshot()
				require.Len(t, calls, 1)
				assert.Equal(t, uint64(77), calls[0].nodeID)

				update := calls[0].msg.GetServerConfigUpdate()
				require.NotNil(t, update)
				require.NotNil(t, update.Server)
				require.NotNil(t, update.Server.StartCommand)
				assert.Equal(t, "start.bat", *update.Server.StartCommand,
					"windows node must surface windows start command via converter")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			settingRepo := inmemory.NewServerSettingRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			nodeRepo := inmemory.NewNodeRepository()
			sender := &stubTaskSender{}

			tt.setupServer(serverRepo)
			tt.setupSettings(settingRepo)
			tt.setupGame(gameRepo)
			tt.setupGameMod(gameModRepo)
			tt.setupNode(nodeRepo)
			tt.setupSender(sender)

			pusher := NewPusher(
				sender,
				serverRepo,
				settingRepo,
				gameRepo,
				gameModRepo,
				nodeRepo,
				discardLogger(),
			)

			// ACT
			pusher.PushServerConfig(context.Background(), tt.serverID)

			// ASSERT
			tt.validate(t, sender)
		})
	}
}

func TestPusher_PushServerConfig_server_lookup_error_skips_send(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := &erroringServerRepo{
		ServerRepository: inmemory.NewServerRepository(),
		err:              errors.New("server storage offline"),
	}
	settingRepo := inmemory.NewServerSettingRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	nodeRepo := inmemory.NewNodeRepository()
	sender := &stubTaskSender{}

	pusher := NewPusher(
		sender,
		serverRepo,
		settingRepo,
		gameRepo,
		gameModRepo,
		nodeRepo,
		discardLogger(),
	)

	// ACT
	require.NotPanics(t, func() {
		pusher.PushServerConfig(context.Background(), 1)
	})

	// ASSERT
	assert.Empty(t, sender.snapshot(), "no SendTask call when the server lookup itself errors")
}

func TestNewPusher_nil_logger_defaults_to_slog_default(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	settingRepo := inmemory.NewServerSettingRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	nodeRepo := inmemory.NewNodeRepository()
	sender := &stubTaskSender{}

	// ACT
	pusher := NewPusher(sender, serverRepo, settingRepo, gameRepo, gameModRepo, nodeRepo, nil)

	// ASSERT
	require.NotNil(t, pusher, "constructor must return a non-nil pusher")
	require.NotNil(t, pusher.logger, "nil logger argument must be replaced with a default logger")

	// ACT (sanity): a subsequent call must not panic with the default logger.
	require.NotPanics(t, func() {
		pusher.PushServerConfig(context.Background(), 999)
	})
}

func TestPusher_PushServerConfig_repository_errors_swallowed(t *testing.T) {
	t.Parallel()

	// ARRANGE — stub repositories that return errors verify the method
	// completes without panicking and without invoking SendTask twice.
	serverRepo := inmemory.NewServerRepository()
	settingRepo := &erroringServerSettingRepo{
		ServerSettingRepository: inmemory.NewServerSettingRepository(),
		err:                     errors.New("settings storage offline"),
	}
	gameRepo := &erroringGameRepo{
		GameRepository: inmemory.NewGameRepository(),
		err:            errors.New("game storage offline"),
	}
	gameModRepo := &erroringGameModRepo{
		GameModRepository: inmemory.NewGameModRepository(),
		err:               errors.New("gamemod storage offline"),
	}
	nodeRepo := &erroringNodeRepo{
		NodeRepository: inmemory.NewNodeRepository(),
		err:            errors.New("node storage offline"),
	}
	sender := &stubTaskSender{}

	_ = serverRepo.Save(context.Background(), &domain.Server{
		ID:        1,
		DSID:      10,
		GameID:    "csgo",
		GameModID: 5,
	})

	pusher := NewPusher(
		sender,
		serverRepo,
		settingRepo,
		gameRepo,
		gameModRepo,
		nodeRepo,
		discardLogger(),
	)

	// ACT
	require.NotPanics(t, func() {
		pusher.PushServerConfig(context.Background(), 1)
	})

	// ASSERT — server was loadable, so SendTask still fires; the failing
	// auxiliary lookups must not block the dispatch.
	calls := sender.snapshot()
	require.Len(t, calls, 1, "send must still happen when only auxiliary lookups fail")
	assert.Equal(t, uint64(10), calls[0].nodeID)

	update := calls[0].msg.GetServerConfigUpdate()
	require.NotNil(t, update)
	assert.Nil(t, update.Game, "game lookup error yields nil game")
	assert.Nil(t, update.GameMod, "game mod lookup error yields nil game mod")
	assert.Empty(t, update.Settings, "settings lookup error yields empty settings")
}

// erroringServerRepo, erroringServerSettingRepo, erroringGameRepo,
// erroringGameModRepo and erroringNodeRepo let tests verify that
// PushServerConfig handles repository errors gracefully. Only Find is
// overridden because the SUT only invokes Find on these collaborators.
type erroringServerRepo struct {
	*inmemory.ServerRepository

	err error
}

func (r *erroringServerRepo) Find(
	_ context.Context,
	_ *filters.FindServer,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Server, error) {
	return nil, r.err
}

type erroringServerSettingRepo struct {
	*inmemory.ServerSettingRepository

	err error
}

func (r *erroringServerSettingRepo) Find(
	_ context.Context,
	_ *filters.FindServerSetting,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.ServerSetting, error) {
	return nil, r.err
}

type erroringGameRepo struct {
	*inmemory.GameRepository

	err error
}

func (r *erroringGameRepo) Find(
	_ context.Context,
	_ *filters.FindGame,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Game, error) {
	return nil, r.err
}

type erroringGameModRepo struct {
	*inmemory.GameModRepository

	err error
}

func (r *erroringGameModRepo) Find(
	_ context.Context,
	_ *filters.FindGameMod,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.GameMod, error) {
	return nil, r.err
}

type erroringNodeRepo struct {
	*inmemory.NodeRepository

	err error
}

func (r *erroringNodeRepo) Find(
	_ context.Context,
	_ *filters.FindNode,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Node, error) {
	return nil, r.err
}
