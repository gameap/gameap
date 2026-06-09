package gateway

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDomainServerToProtoWithGameMod(t *testing.T) {
	linuxCmd := "./start.sh -game cstrike"
	windowsCmd := "start.exe -game cstrike"

	tests := []struct {
		name             string
		serverStartCmd   *string
		gameMod          *domain.GameMod
		nodeOS           domain.NodeOS
		wantStartCommand *string
	}{
		{
			name:           "nil_server_command_uses_linux_game_mod_command",
			serverStartCmd: nil,
			gameMod: &domain.GameMod{
				StartCmdLinux:   &linuxCmd,
				StartCmdWindows: &windowsCmd,
			},
			nodeOS:           domain.NodeOSLinux,
			wantStartCommand: &linuxCmd,
		},
		{
			name:           "nil_server_command_uses_windows_game_mod_command",
			serverStartCmd: nil,
			gameMod: &domain.GameMod{
				StartCmdLinux:   &linuxCmd,
				StartCmdWindows: &windowsCmd,
			},
			nodeOS:           domain.NodeOSWindows,
			wantStartCommand: &windowsCmd,
		},
		{
			name:           "non_nil_server_command_preserved",
			serverStartCmd: &linuxCmd,
			gameMod: &domain.GameMod{
				StartCmdLinux: new("other-command"),
			},
			nodeOS:           domain.NodeOSLinux,
			wantStartCommand: &linuxCmd,
		},
		{
			name:           "empty_server_command_uses_game_mod_command",
			serverStartCmd: new(""),
			gameMod: &domain.GameMod{
				StartCmdLinux: &linuxCmd,
			},
			nodeOS:           domain.NodeOSLinux,
			wantStartCommand: &linuxCmd,
		},
		{
			name:             "nil_game_mod_returns_nil_command",
			serverStartCmd:   nil,
			gameMod:          nil,
			nodeOS:           domain.NodeOSLinux,
			wantStartCommand: nil,
		},
		{
			name:           "nil_server_command_and_nil_game_mod_command",
			serverStartCmd: nil,
			gameMod: &domain.GameMod{
				StartCmdLinux: nil,
			},
			nodeOS:           domain.NodeOSLinux,
			wantStartCommand: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &domain.Server{
				ID:           1,
				Name:         "test",
				ServerIP:     "127.0.0.1",
				ServerPort:   27015,
				Dir:          "/srv/server",
				StartCommand: tt.serverStartCmd,
			}

			result := DomainServerToProtoWithGameMod(server, tt.gameMod, tt.nodeOS)

			require.NotNil(t, result)

			if tt.wantStartCommand == nil {
				assert.Nil(t, result.StartCommand)
			} else {
				require.NotNil(t, result.StartCommand)
				assert.Equal(t, *tt.wantStartCommand, *result.StartCommand)
			}
		})
	}
}

func TestDomainServerToProto_minimalServer(t *testing.T) {
	// ARRANGE
	srv := &domain.Server{
		ID:         42,
		UUID:       uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		UUIDShort:  "11111111",
		Enabled:    true,
		Installed:  domain.ServerInstalledStatusInstalled,
		Blocked:    false,
		Name:       "minimal",
		GameID:     "csgo",
		DSID:       7,
		GameModID:  3,
		ServerIP:   "10.0.0.1",
		ServerPort: 27015,
		Dir:        "/srv/csgo",
	}

	// ACT
	got := DomainServerToProto(srv)

	// ASSERT
	require.NotNil(t, got)
	assert.Equal(t, uint64(42), got.Id, "id propagated")
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", got.Uuid)
	assert.Equal(t, "11111111", got.UuidShort)
	assert.True(t, got.Enabled, "enabled flag")
	assert.Equal(t, proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLED, got.Installed)
	assert.False(t, got.Blocked)
	assert.Equal(t, "minimal", got.Name)
	assert.Equal(t, "csgo", got.GameId)
	assert.Equal(t, uint64(7), got.DsId)
	assert.Equal(t, uint64(3), got.GameModId)
	assert.Equal(t, "10.0.0.1", got.ServerIp)
	assert.Equal(t, int32(27015), got.ServerPort)
	assert.Equal(t, "/srv/csgo", got.Dir)
	assert.Nil(t, got.QueryPort, "nil query port stays nil")
	assert.Nil(t, got.RconPort, "nil rcon port stays nil")
	assert.Nil(t, got.CpuLimit, "nil cpu limit stays nil")
	assert.Nil(t, got.RamLimit, "nil ram limit stays nil")
	assert.Nil(t, got.NetLimit, "nil net limit stays nil")
	assert.Nil(t, got.CreatedAt, "nil timestamp stays nil")
	assert.Nil(t, got.UpdatedAt)
	assert.Nil(t, got.DeletedAt)
	assert.Nil(t, got.Expires)
	assert.Nil(t, got.LastProcessCheck)
	assert.Nil(t, got.Vars, "nil server vars stays nil")
}

func TestDomainServerToProto_fullServer(t *testing.T) {
	// ARRANGE
	created := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2024, 1, 3, 3, 4, 5, 0, time.UTC)
	deleted := time.Date(2024, 1, 4, 3, 4, 5, 0, time.UTC)
	expires := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	processCheck := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	srv := &domain.Server{
		ID:               99,
		UUID:             uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		UUIDShort:        "aaaaaaaa",
		Enabled:          true,
		Installed:        domain.ServerInstalledStatusInstallationInProg,
		Blocked:          true,
		Name:             "full server",
		GameID:           "csgo",
		DSID:             5,
		GameModID:        12,
		Expires:          &expires,
		ServerIP:         "192.168.1.1",
		ServerPort:       27015,
		QueryPort:        new(27016),
		RconPort:         new(27017),
		Rcon:             new("rcon-pass"),
		Dir:              "/srv/full",
		SuUser:           new("steam"),
		CPULimit:         new(80),
		RAMLimit:         new(4096),
		NetLimit:         new(1000),
		StartCommand:     new("./start.sh"),
		StopCommand:      new("./stop.sh"),
		ForceStopCommand: new("kill -9"),
		RestartCommand:   new("./restart.sh"),
		ProcessActive:    true,
		LastProcessCheck: &processCheck,
		Vars:             domain.ServerVars{"map": "de_dust2", "tickrate": "128"},
		Metadata:         domain.Metadata{"region": "eu"},
		CreatedAt:        &created,
		UpdatedAt:        &updated,
		DeletedAt:        &deleted,
	}

	// ACT
	got := DomainServerToProto(srv)

	// ASSERT
	require.NotNil(t, got)
	assert.Equal(t, uint64(99), got.Id)
	assert.True(t, got.Blocked)
	assert.Equal(t, proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLATION_IN_PROGRESS, got.Installed)

	require.NotNil(t, got.QueryPort)
	assert.Equal(t, int32(27016), *got.QueryPort)
	require.NotNil(t, got.RconPort)
	assert.Equal(t, int32(27017), *got.RconPort)
	require.NotNil(t, got.Rcon)
	assert.Equal(t, "rcon-pass", *got.Rcon)
	require.NotNil(t, got.SuUser)
	assert.Equal(t, "steam", *got.SuUser)
	require.NotNil(t, got.CpuLimit)
	assert.Equal(t, int32(80), *got.CpuLimit)
	require.NotNil(t, got.RamLimit)
	assert.Equal(t, int64(4096), *got.RamLimit)
	require.NotNil(t, got.NetLimit)
	assert.Equal(t, int32(1000), *got.NetLimit)
	require.NotNil(t, got.StartCommand)
	assert.Equal(t, "./start.sh", *got.StartCommand)
	assert.True(t, got.ProcessActive)

	require.NotNil(t, got.CreatedAt, "created_at propagated")
	assert.Equal(t, created, got.CreatedAt.AsTime())
	require.NotNil(t, got.UpdatedAt)
	assert.Equal(t, updated, got.UpdatedAt.AsTime())
	require.NotNil(t, got.DeletedAt)
	assert.Equal(t, deleted, got.DeletedAt.AsTime())
	require.NotNil(t, got.Expires)
	assert.Equal(t, expires, got.Expires.AsTime())
	require.NotNil(t, got.LastProcessCheck)
	assert.Equal(t, processCheck, got.LastProcessCheck.AsTime())

	require.NotNil(t, got.Vars, "vars must be encoded as JSON string pointer")
	var decoded map[string]string
	require.NoError(t, json.Unmarshal([]byte(*got.Vars), &decoded))
	assert.Equal(t, "de_dust2", decoded["map"])
	assert.Equal(t, "128", decoded["tickrate"])

	require.NotNil(t, got.Metadata)
	regionAny := got.Metadata["region"]
	require.NotNil(t, regionAny)
	wrapped := wrapperspb.String("")
	require.NoError(t, regionAny.UnmarshalTo(wrapped))
	assert.Equal(t, "eu", wrapped.GetValue())
}

func TestDomainServerToProto_ramLimitAboveInt32IsNotClamped(t *testing.T) {
	// ram_limit is stored in bytes; the proto field is int64, so values above the
	// int32/uint32 boundaries must pass through unchanged (the original 500 bug).
	tests := []struct {
		name     string
		ramLimit int
		want     int64
	}{
		{name: "at_int32_max", ramLimit: math.MaxInt32, want: math.MaxInt32},
		{name: "just_above_int32_max", ramLimit: math.MaxInt32 + 1, want: math.MaxInt32 + 1},
		{name: "four_gib_uint32_boundary", ramLimit: 4 * 1024 * 1024 * 1024, want: 4 * 1024 * 1024 * 1024},
		{name: "eight_gib_above_uint32", ramLimit: 8 * 1024 * 1024 * 1024, want: 8 * 1024 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			srv := &domain.Server{
				ID:         1,
				UUID:       uuid.MustParse("11111111-2222-3333-4444-555555555555"),
				UUIDShort:  "11111111",
				Name:       "big-ram",
				GameID:     "csgo",
				ServerIP:   "10.0.0.1",
				ServerPort: 27015,
				Dir:        "/srv/csgo",
				RAMLimit:   new(tt.ramLimit),
			}

			// ACT
			got := DomainServerToProto(srv)

			// ASSERT
			require.NotNil(t, got)
			require.NotNil(t, got.RamLimit)
			assert.Equal(t, tt.want, *got.RamLimit, "ram limit must pass through without int32 clamping")
		})
	}
}

func TestDomainServerToProto_cpuAndNetLimitsStillClampToInt32(t *testing.T) {
	// ARRANGE
	// cpu_limit and net_limit remain int32 on the wire, so values above int32 must
	// still be clamped — only ram_limit was widened to int64.
	aboveInt32 := math.MaxInt32 + 1

	srv := &domain.Server{
		ID:         1,
		UUID:       uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		UUIDShort:  "11111111",
		Name:       "clamp-limits",
		GameID:     "csgo",
		ServerIP:   "10.0.0.1",
		ServerPort: 27015,
		Dir:        "/srv/csgo",
		CPULimit:   new(aboveInt32),
		NetLimit:   new(aboveInt32),
		RAMLimit:   new(aboveInt32),
	}

	// ACT
	got := DomainServerToProto(srv)

	// ASSERT
	require.NotNil(t, got)
	require.NotNil(t, got.CpuLimit)
	assert.Equal(t, int32(math.MaxInt32), *got.CpuLimit, "cpu_limit must clamp to int32 max")
	require.NotNil(t, got.NetLimit)
	assert.Equal(t, int32(math.MaxInt32), *got.NetLimit, "net_limit must clamp to int32 max")
	require.NotNil(t, got.RamLimit)
	assert.Equal(t, int64(aboveInt32), *got.RamLimit, "ram_limit must NOT clamp — it is int64")
}

func TestDomainServerToProto_installedStatusMapping(t *testing.T) {
	tests := []struct {
		name string
		in   domain.ServerInstalledStatus
		want proto.ServerInstalledStatus
	}{
		{
			name: "not_installed_maps_to_not_installed",
			in:   domain.ServerInstalledStatusNotInstalled,
			want: proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_NOT_INSTALLED,
		},
		{
			name: "installed_maps_to_installed",
			in:   domain.ServerInstalledStatusInstalled,
			want: proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLED,
		},
		{
			name: "in_progress_maps_to_in_progress",
			in:   domain.ServerInstalledStatusInstallationInProg,
			want: proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_INSTALLATION_IN_PROGRESS,
		},
		{
			name: "unknown_value_falls_back_to_not_installed",
			in:   domain.ServerInstalledStatus(99),
			want: proto.ServerInstalledStatus_SERVER_INSTALLED_STATUS_NOT_INSTALLED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &domain.Server{ID: 1, Installed: tt.in, ServerIP: "1.1.1.1", ServerPort: 1}

			got := DomainServerToProto(srv)

			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.Installed)
		})
	}
}

func TestDomainGameToProto(t *testing.T) {
	tests := []struct {
		name      string
		in        *domain.Game
		assertion func(*testing.T, *proto.Game)
	}{
		{
			name: "minimal_game_with_zero_values",
			in: &domain.Game{
				Code:    "cs",
				Name:    "Counter-Strike",
				Engine:  "GoldSrc",
				Enabled: 1,
			},
			assertion: func(t *testing.T, p *proto.Game) {
				t.Helper()
				assert.Equal(t, "cs", p.Code)
				assert.Equal(t, "Counter-Strike", p.Name)
				assert.Equal(t, "GoldSrc", p.Engine)
				assert.True(t, p.Enabled, "Enabled=1 must produce true")
				assert.Nil(t, p.SteamAppIdLinux)
				assert.Nil(t, p.SteamAppIdWindows)
				assert.Nil(t, p.Metadata)
			},
		},
		{
			name: "disabled_game_propagates_false",
			in: &domain.Game{
				Code:    "cs",
				Enabled: 0,
			},
			assertion: func(t *testing.T, p *proto.Game) {
				t.Helper()
				assert.False(t, p.Enabled, "Enabled=0 must produce false")
			},
		},
		{
			name: "steam_app_ids_clamped_and_propagated",
			in: &domain.Game{
				Code:              "csgo",
				SteamAppIDLinux:   new(uint(740)),
				SteamAppIDWindows: new(uint(741)),
			},
			assertion: func(t *testing.T, p *proto.Game) {
				t.Helper()
				require.NotNil(t, p.SteamAppIdLinux)
				assert.Equal(t, uint32(740), *p.SteamAppIdLinux)
				require.NotNil(t, p.SteamAppIdWindows)
				assert.Equal(t, uint32(741), *p.SteamAppIdWindows)
			},
		},
		{
			name: "all_optional_fields_set",
			in: &domain.Game{
				Code:                    "csgo",
				Name:                    "CS:GO",
				Engine:                  "Source",
				EngineVersion:           "v2",
				SteamAppSetConfig:       new("config"),
				RemoteRepositoryLinux:   new("rrl"),
				RemoteRepositoryWindows: new("rrw"),
				LocalRepositoryLinux:    new("lrl"),
				LocalRepositoryWindows:  new("lrw"),
				Metadata:                domain.Metadata{"k": "v"},
			},
			assertion: func(t *testing.T, p *proto.Game) {
				t.Helper()
				require.NotNil(t, p.SteamAppSetConfig)
				assert.Equal(t, "config", *p.SteamAppSetConfig)
				require.NotNil(t, p.RemoteRepositoryLinux)
				assert.Equal(t, "rrl", *p.RemoteRepositoryLinux)
				require.NotNil(t, p.RemoteRepositoryWindows)
				assert.Equal(t, "rrw", *p.RemoteRepositoryWindows)
				require.NotNil(t, p.LocalRepositoryLinux)
				assert.Equal(t, "lrl", *p.LocalRepositoryLinux)
				require.NotNil(t, p.LocalRepositoryWindows)
				assert.Equal(t, "lrw", *p.LocalRepositoryWindows)
				require.NotNil(t, p.Metadata)
				require.Contains(t, p.Metadata, "k")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DomainGameToProto(tt.in)

			require.NotNil(t, got)
			tt.assertion(t, got)
		})
	}
}

func TestDomainGameModToProto(t *testing.T) {
	t.Run("minimal_game_mod_yields_empty_collections", func(t *testing.T) {
		// ARRANGE
		gm := &domain.GameMod{
			ID:       11,
			GameCode: "cs",
			Name:     "Classic",
		}

		// ACT
		got := DomainGameModToProto(gm)

		// ASSERT
		require.NotNil(t, got)
		assert.Equal(t, uint64(11), got.Id)
		assert.Equal(t, "cs", got.GameCode)
		assert.Equal(t, "Classic", got.Name)
		require.NotNil(t, got.FastRcon, "FastRcon must be non-nil empty slice")
		assert.Empty(t, got.FastRcon)
		require.NotNil(t, got.Vars, "Vars must be non-nil empty slice")
		assert.Empty(t, got.Vars)
		assert.Nil(t, got.StartCmdLinux)
	})

	t.Run("populated_game_mod_translates_all_fields", func(t *testing.T) {
		// ARRANGE
		gm := &domain.GameMod{
			ID:                      22,
			GameCode:                "cs",
			Name:                    "Mod",
			StartCmdLinux:           new("./start"),
			StartCmdWindows:         new("start.exe"),
			KickCmd:                 new("kick {user}"),
			BanCmd:                  new("ban {user}"),
			ChnameCmd:               new("name {x}"),
			SrestartCmd:             new("restart"),
			ChmapCmd:                new("map {map}"),
			SendmsgCmd:              new("msg {msg}"),
			PasswdCmd:               new("pass {p}"),
			RemoteRepositoryLinux:   new("rrl"),
			RemoteRepositoryWindows: new("rrw"),
			LocalRepositoryLinux:    new("lrl"),
			LocalRepositoryWindows:  new("lrw"),
			FastRcon: domain.GameModFastRconList{
				{Info: "kill all", Command: "killall"},
				{Info: "swap", Command: "swap"},
			},
			Vars: domain.GameModVarList{
				{Var: "tickrate", Default: domain.GameModVarDefault("128"), Info: "tick", AdminVar: false},
				{Var: "rcon_password", Default: domain.GameModVarDefault("changeme"), Info: "pwd", AdminVar: true},
			},
			Metadata: domain.Metadata{"key": "value"},
		}

		// ACT
		got := DomainGameModToProto(gm)

		// ASSERT
		require.NotNil(t, got)
		require.Len(t, got.FastRcon, 2, "FastRcon must keep order and length")
		assert.Equal(t, "killall", got.FastRcon[0].Command)
		assert.Equal(t, "swap", got.FastRcon[1].Command)

		require.Len(t, got.Vars, 2, "Vars must keep order and length")
		assert.Equal(t, "tickrate", got.Vars[0].Var)
		assert.Equal(t, "128", got.Vars[0].Default)
		assert.False(t, got.Vars[0].AdminVar)
		assert.Equal(t, "rcon_password", got.Vars[1].Var)
		assert.True(t, got.Vars[1].AdminVar)

		require.NotNil(t, got.StartCmdLinux)
		assert.Equal(t, "./start", *got.StartCmdLinux)
		require.NotNil(t, got.PasswdCmd)
		assert.Equal(t, "pass {p}", *got.PasswdCmd)

		require.NotNil(t, got.Metadata)
		require.Contains(t, got.Metadata, "key")
	})
}

func TestDomainMetadataToProto(t *testing.T) {
	t.Run("nil_metadata_returns_nil", func(t *testing.T) {
		assert.Nil(t, domainMetadataToProto(nil))
	})

	t.Run("scalars_stringified_into_anypb_wrappers", func(t *testing.T) {
		// ARRANGE
		m := domain.Metadata{
			"int_val":  42,
			"str_val":  "hello",
			"bool_val": true,
		}

		// ACT
		got := domainMetadataToProto(m)

		// ASSERT
		require.NotNil(t, got)
		require.Len(t, got, 3, "must contain three converted entries")

		for k, expected := range map[string]string{
			"int_val":  "42",
			"str_val":  "hello",
			"bool_val": "true",
		} {
			anyVal := got[k]
			require.NotNil(t, anyVal, "key %s must be present", k)
			wrapped := wrapperspb.String("")
			require.NoError(t, anyVal.UnmarshalTo(wrapped))
			assert.Equal(t, expected, wrapped.GetValue(), "value for %s must round-trip", k)
		}
	})
}

func TestDomainDaemonTaskToProto(t *testing.T) {
	created := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	serverID := uint(7)
	runAfterID := uint(3)
	cmd := "/usr/bin/start"
	data := `{"foo":"bar"}`
	output := "captured stdout"

	tests := []struct {
		name      string
		task      *domain.DaemonTask
		assertion func(*testing.T, *proto.DaemonTask)
	}{
		{
			name: "fully_populated_task_translates_all_fields",
			task: &domain.DaemonTask{
				ID:                10,
				RunAftID:          &runAfterID,
				CreatedAt:         &created,
				UpdatedAt:         &updated,
				DedicatedServerID: 5,
				ServerID:          &serverID,
				Task:              domain.DaemonTaskTypeServerInstall,
				Data:              &data,
				Cmd:               &cmd,
				Output:            &output,
				Status:            domain.DaemonTaskStatusWorking,
			},
			assertion: func(t *testing.T, p *proto.DaemonTask) {
				t.Helper()
				assert.Equal(t, uint64(10), p.Id)
				require.NotNil(t, p.RunAfterId)
				assert.Equal(t, uint64(3), *p.RunAfterId)
				assert.Equal(t, uint64(5), p.NodeId)
				require.NotNil(t, p.ServerId)
				assert.Equal(t, uint64(7), *p.ServerId)
				assert.Equal(t, proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL, p.TaskType)
				assert.Equal(t, proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING, p.Status)
				require.NotNil(t, p.Data)
				assert.Equal(t, data, *p.Data)
				require.NotNil(t, p.Cmd)
				assert.Equal(t, cmd, *p.Cmd)
				require.NotNil(t, p.Output)
				assert.Equal(t, output, *p.Output)
				require.NotNil(t, p.CreatedAt)
				assert.Equal(t, created, p.CreatedAt.AsTime())
				require.NotNil(t, p.UpdatedAt)
				assert.Equal(t, updated, p.UpdatedAt.AsTime())
			},
		},
		{
			name: "nil_optional_fields_stay_nil",
			task: &domain.DaemonTask{
				ID:                1,
				DedicatedServerID: 9,
				Task:              domain.DaemonTaskTypeServerStart,
				Status:            domain.DaemonTaskStatusWaiting,
			},
			assertion: func(t *testing.T, p *proto.DaemonTask) {
				t.Helper()
				assert.Nil(t, p.RunAfterId)
				assert.Nil(t, p.ServerId)
				assert.Nil(t, p.CreatedAt)
				assert.Nil(t, p.UpdatedAt)
				assert.Equal(t, proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START, p.TaskType)
				assert.Equal(t, proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING, p.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DomainDaemonTaskToProto(tt.task)

			require.NotNil(t, got)
			tt.assertion(t, got)
		})
	}
}

func TestDomainTaskTypeToProto(t *testing.T) {
	tests := []struct {
		name string
		in   domain.DaemonTaskType
		want proto.DaemonTaskType
	}{
		{name: "server_start", in: domain.DaemonTaskTypeServerStart, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START},
		{name: "server_stop", in: domain.DaemonTaskTypeServerStop, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_STOP},
		{name: "server_restart", in: domain.DaemonTaskTypeServerRestart, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_RESTART},
		{name: "server_update", in: domain.DaemonTaskTypeServerUpdate, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_UPDATE},
		{name: "server_install", in: domain.DaemonTaskTypeServerInstall, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL},
		{name: "server_delete", in: domain.DaemonTaskTypeServerDelete, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_DELETE},
		{name: "server_move", in: domain.DaemonTaskTypeServerMove, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_MOVE},
		{name: "cmd_exec", in: domain.DaemonTaskTypeCmdExec, want: proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC},
		{name: "unknown_falls_back_to_unspecified", in: domain.DaemonTaskType("garbage"), want: proto.DaemonTaskType_DAEMON_TASK_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainTaskTypeToProto(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainTaskStatusToProto(t *testing.T) {
	tests := []struct {
		name string
		in   domain.DaemonTaskStatus
		want proto.DaemonTaskStatus
	}{
		{name: "waiting", in: domain.DaemonTaskStatusWaiting, want: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING},
		{name: "working", in: domain.DaemonTaskStatusWorking, want: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING},
		{name: "error", in: domain.DaemonTaskStatusError, want: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR},
		{name: "success", in: domain.DaemonTaskStatusSuccess, want: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS},
		{name: "canceled", in: domain.DaemonTaskStatusCanceled, want: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED},
		{name: "unknown_falls_back_to_unspecified", in: domain.DaemonTaskStatus("nope"), want: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainTaskStatusToProto(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtoTaskStatusToDomain(t *testing.T) {
	tests := []struct {
		name string
		in   proto.DaemonTaskStatus
		want domain.DaemonTaskStatus
	}{
		{name: "waiting", in: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING, want: domain.DaemonTaskStatusWaiting},
		{name: "working", in: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING, want: domain.DaemonTaskStatusWorking},
		{name: "error", in: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR, want: domain.DaemonTaskStatusError},
		{name: "success", in: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS, want: domain.DaemonTaskStatusSuccess},
		{name: "canceled", in: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED, want: domain.DaemonTaskStatusCanceled},
		{name: "unspecified_falls_back_to_waiting", in: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_UNSPECIFIED, want: domain.DaemonTaskStatusWaiting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProtoTaskStatusToDomain(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainServerSettingsToProto(t *testing.T) {
	t.Run("empty_input_returns_empty_slice", func(t *testing.T) {
		got := DomainServerSettingsToProto(nil)
		require.NotNil(t, got, "must return non-nil even for nil input")
		assert.Empty(t, got)
	})

	t.Run("string_int_bool_values_serialised_via_String", func(t *testing.T) {
		// ARRANGE
		settings := []domain.ServerSetting{
			{ID: 1, ServerID: 10, Name: "map", Value: domain.NewServerSettingValue("de_dust2")},
			{ID: 2, ServerID: 10, Name: "tickrate", Value: domain.NewServerSettingValue(128)},
			{ID: 3, ServerID: 11, Name: "private", Value: domain.NewServerSettingValue(true)},
		}

		// ACT
		got := DomainServerSettingsToProto(settings)

		// ASSERT
		require.Len(t, got, 3)
		assert.Equal(t, uint64(1), got[0].Id)
		assert.Equal(t, uint64(10), got[0].ServerId)
		assert.Equal(t, "map", got[0].Name)
		assert.Equal(t, "de_dust2", got[0].Value)

		assert.Equal(t, "128", got[1].Value, "int values must be stringified")
		assert.Equal(t, "true", got[2].Value, "bool values must be stringified")
	})
}

func TestClampToInt32(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int32
	}{
		{name: "zero_passthrough", in: 0, want: 0},
		{name: "negative_passthrough", in: -1, want: -1},
		{name: "small_positive_passthrough", in: 27015, want: 27015},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampToInt32(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClampToUint32(t *testing.T) {
	tests := []struct {
		name string
		in   uint
		want uint32
	}{
		{name: "zero_passthrough", in: 0, want: 0},
		{name: "small_positive_passthrough", in: 740, want: 740},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampToUint32(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainServerTaskToProto(t *testing.T) {
	executeDate := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 13, 11, 30, 0, 0, time.UTC)
	repeatPeriod := 15 * time.Minute
	nodeID := uint(42)
	name := "nightly_restart"
	timezone := "Europe/Moscow"
	payload := `{"reason":"weekly"}`

	tests := []struct {
		name      string
		task      *domain.ServerTask
		assertion func(*testing.T, *proto.ServerTask)
	}{
		{
			name: "full_task_all_fields",
			task: &domain.ServerTask{
				ID:            10,
				ServerID:      7,
				NodeID:        &nodeID,
				Command:       domain.ServerTaskCommandRestart,
				ExecuteDate:   executeDate,
				RepeatPeriod:  repeatPeriod,
				Repeat:        5,
				Counter:       2,
				Payload:       &payload,
				Version:       99,
				OverlapPolicy: domain.ServerTaskOverlapPolicyQueue,
				CatchupPolicy: domain.ServerTaskCatchupPolicyRunOnce,
				Enabled:       true,
				Name:          &name,
				Timezone:      &timezone,
				UpdatedAt:     &updatedAt,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, uint64(10), p.Id)
				assert.Equal(t, uint64(7), p.ServerId)
				assert.Equal(t, uint64(42), p.NodeId)
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART, p.Command)
				require.NotNil(t, p.ExecuteDate)
				assert.Equal(t, executeDate, p.ExecuteDate.AsTime())
				require.NotNil(t, p.RepeatPeriod)
				assert.Equal(t, repeatPeriod, p.RepeatPeriod.AsDuration())
				assert.Equal(t, uint32(5), p.RepeatCount)
				assert.Equal(t, uint32(2), p.Counter)
				assert.Equal(t, payload, p.Payload)
				assert.Equal(t, uint64(99), p.Version)
				assert.Equal(t, proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_QUEUE, p.OverlapPolicy)
				assert.Equal(t, proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_RUN_ONCE, p.CatchupPolicy)
				assert.True(t, p.Enabled)
				assert.Equal(t, name, p.Name)
				assert.Equal(t, timezone, p.Timezone)
				require.NotNil(t, p.UpdatedAt)
				assert.Equal(t, updatedAt, p.UpdatedAt.AsTime())
			},
		},
		{
			name: "task_with_nil_optional_fields",
			task: &domain.ServerTask{
				ID:            1,
				ServerID:      2,
				NodeID:        nil,
				Command:       domain.ServerTaskCommandStart,
				ExecuteDate:   executeDate,
				RepeatPeriod:  0,
				Repeat:        0,
				Counter:       0,
				Payload:       nil,
				Version:       1,
				OverlapPolicy: domain.ServerTaskOverlapPolicySkip,
				CatchupPolicy: domain.ServerTaskCatchupPolicySkip,
				Enabled:       false,
				Name:          nil,
				Timezone:      nil,
				UpdatedAt:     nil,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, uint64(1), p.Id)
				assert.Equal(t, uint64(2), p.ServerId)
				assert.Equal(t, uint64(0), p.NodeId)
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_START, p.Command)
				assert.Empty(t, p.Name)
				assert.Empty(t, p.Timezone)
				assert.Empty(t, p.Payload)
				assert.False(t, p.Enabled)
				require.NotNil(t, p.ExecuteDate)
				require.NotNil(t, p.RepeatPeriod)
				assert.Equal(t, time.Duration(0), p.RepeatPeriod.AsDuration())
				assert.Nil(t, p.UpdatedAt, "nil UpdatedAt must remain nil on the proto side")
			},
		},
		{
			name: "command_mapping_start",
			task: &domain.ServerTask{
				ID:          1,
				ServerID:    1,
				Command:     domain.ServerTaskCommandStart,
				ExecuteDate: executeDate,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_START, p.Command)
			},
		},
		{
			name: "command_mapping_stop",
			task: &domain.ServerTask{
				ID:          1,
				ServerID:    1,
				Command:     domain.ServerTaskCommandStop,
				ExecuteDate: executeDate,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP, p.Command)
			},
		},
		{
			name: "command_mapping_restart",
			task: &domain.ServerTask{
				ID:          1,
				ServerID:    1,
				Command:     domain.ServerTaskCommandRestart,
				ExecuteDate: executeDate,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART, p.Command)
			},
		},
		{
			name: "command_mapping_update",
			task: &domain.ServerTask{
				ID:          1,
				ServerID:    1,
				Command:     domain.ServerTaskCommandUpdate,
				ExecuteDate: executeDate,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_UPDATE, p.Command)
			},
		},
		{
			name: "command_mapping_reinstall",
			task: &domain.ServerTask{
				ID:          1,
				ServerID:    1,
				Command:     domain.ServerTaskCommandReinstall,
				ExecuteDate: executeDate,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_REINSTALL, p.Command)
			},
		},
		{
			name: "overlap_policy_skip",
			task: &domain.ServerTask{
				ID:            1,
				ServerID:      1,
				Command:       domain.ServerTaskCommandStart,
				ExecuteDate:   executeDate,
				OverlapPolicy: domain.ServerTaskOverlapPolicySkip,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_SKIP, p.OverlapPolicy)
			},
		},
		{
			name: "overlap_policy_queue",
			task: &domain.ServerTask{
				ID:            1,
				ServerID:      1,
				Command:       domain.ServerTaskCommandStart,
				ExecuteDate:   executeDate,
				OverlapPolicy: domain.ServerTaskOverlapPolicyQueue,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_QUEUE, p.OverlapPolicy)
			},
		},
		{
			name: "catchup_policy_skip",
			task: &domain.ServerTask{
				ID:            1,
				ServerID:      1,
				Command:       domain.ServerTaskCommandStart,
				ExecuteDate:   executeDate,
				CatchupPolicy: domain.ServerTaskCatchupPolicySkip,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_SKIP, p.CatchupPolicy)
			},
		},
		{
			name: "catchup_policy_run_once",
			task: &domain.ServerTask{
				ID:            1,
				ServerID:      1,
				Command:       domain.ServerTaskCommandStart,
				ExecuteDate:   executeDate,
				CatchupPolicy: domain.ServerTaskCatchupPolicyRunOnce,
			},
			assertion: func(t *testing.T, p *proto.ServerTask) {
				t.Helper()
				assert.Equal(t, proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_RUN_ONCE, p.CatchupPolicy)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DomainServerTaskToProto(tt.task)

			require.NotNil(t, got)
			tt.assertion(t, got)
		})
	}
}

func TestDomainServerTasksToProtoList(t *testing.T) {
	executeDate := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)

	t.Run("empty_slice_returns_empty_not_nil", func(t *testing.T) {
		got := DomainServerTasksToProtoList([]domain.ServerTask{})

		require.NotNil(t, got, "result must be empty slice, not nil")
		require.Len(t, got, 0)
	})

	t.Run("nil_slice_returns_empty_not_nil", func(t *testing.T) {
		got := DomainServerTasksToProtoList(nil)

		require.NotNil(t, got, "converter must return an empty slice for nil input")
		require.Len(t, got, 0)
	})

	t.Run("preserves_order", func(t *testing.T) {
		tasks := []domain.ServerTask{
			{ID: 1, ServerID: 10, Command: domain.ServerTaskCommandStart, ExecuteDate: executeDate},
			{ID: 2, ServerID: 20, Command: domain.ServerTaskCommandStop, ExecuteDate: executeDate},
			{ID: 3, ServerID: 30, Command: domain.ServerTaskCommandRestart, ExecuteDate: executeDate},
		}

		got := DomainServerTasksToProtoList(tasks)

		require.Len(t, got, 3)
		assert.Equal(t, uint64(1), got[0].Id)
		assert.Equal(t, uint64(10), got[0].ServerId)
		assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_START, got[0].Command)
		assert.Equal(t, uint64(2), got[1].Id)
		assert.Equal(t, uint64(20), got[1].ServerId)
		assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP, got[1].Command)
		assert.Equal(t, uint64(3), got[2].Id)
		assert.Equal(t, uint64(30), got[2].ServerId)
		assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART, got[2].Command)
	})
}

func TestDomainServerTaskCommandToProto(t *testing.T) {
	tests := []struct {
		name string
		in   domain.ServerTaskCommand
		want proto.ServerTaskCommand
	}{
		{name: "start_to_proto_start", in: domain.ServerTaskCommandStart, want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_START},
		{name: "stop_to_proto_stop", in: domain.ServerTaskCommandStop, want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP},
		{name: "restart_to_proto_restart", in: domain.ServerTaskCommandRestart, want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART},
		{name: "update_to_proto_update", in: domain.ServerTaskCommandUpdate, want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_UPDATE},
		{name: "reinstall_to_proto_reinstall", in: domain.ServerTaskCommandReinstall, want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_REINSTALL},
		{name: "empty_domain_command_to_unspecified", in: "", want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_UNSPECIFIED},
		{name: "unknown_domain_command_to_unspecified", in: "garbage", want: proto.ServerTaskCommand_SERVER_TASK_COMMAND_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainServerTaskCommandToProto(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtoServerTaskCommandToDomain(t *testing.T) {
	tests := []struct {
		name string
		in   proto.ServerTaskCommand
		want domain.ServerTaskCommand
	}{
		{name: "proto_start_to_domain_start", in: proto.ServerTaskCommand_SERVER_TASK_COMMAND_START, want: domain.ServerTaskCommandStart},
		{name: "proto_stop_to_domain_stop", in: proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP, want: domain.ServerTaskCommandStop},
		{name: "proto_restart_to_domain_restart", in: proto.ServerTaskCommand_SERVER_TASK_COMMAND_RESTART, want: domain.ServerTaskCommandRestart},
		{name: "proto_update_to_domain_update", in: proto.ServerTaskCommand_SERVER_TASK_COMMAND_UPDATE, want: domain.ServerTaskCommandUpdate},
		{name: "proto_reinstall_to_domain_reinstall", in: proto.ServerTaskCommand_SERVER_TASK_COMMAND_REINSTALL, want: domain.ServerTaskCommandReinstall},
		{name: "unspecified_to_empty_string", in: proto.ServerTaskCommand_SERVER_TASK_COMMAND_UNSPECIFIED, want: ""},
		{name: "unknown_enum_value_to_empty_string", in: proto.ServerTaskCommand(9999), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProtoServerTaskCommandToDomain(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainServerTaskOverlapPolicyToProto(t *testing.T) {
	tests := []struct {
		name string
		in   domain.ServerTaskOverlapPolicy
		want proto.ServerTaskOverlapPolicy
	}{
		{
			name: "skip_to_proto_skip",
			in:   domain.ServerTaskOverlapPolicySkip,
			want: proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_SKIP,
		},
		{
			name: "queue_to_proto_queue",
			in:   domain.ServerTaskOverlapPolicyQueue,
			want: proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_QUEUE,
		},
		{
			name: "empty_domain_policy_defaults_to_proto_skip",
			in:   "",
			want: proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_SKIP,
		},
		{
			name: "unknown_domain_policy_defaults_to_proto_skip",
			in:   "garbage",
			want: proto.ServerTaskOverlapPolicy_SERVER_TASK_OVERLAP_POLICY_SKIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainServerTaskOverlapPolicyToProto(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainServerTaskCatchupPolicyToProto(t *testing.T) {
	tests := []struct {
		name string
		in   domain.ServerTaskCatchupPolicy
		want proto.ServerTaskCatchupPolicy
	}{
		{
			name: "skip_to_proto_skip",
			in:   domain.ServerTaskCatchupPolicySkip,
			want: proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_SKIP,
		},
		{
			name: "run_once_to_proto_run_once",
			in:   domain.ServerTaskCatchupPolicyRunOnce,
			want: proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_RUN_ONCE,
		},
		{
			name: "empty_domain_policy_defaults_to_proto_skip",
			in:   "",
			want: proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_SKIP,
		},
		{
			name: "unknown_domain_policy_defaults_to_proto_skip",
			in:   "garbage",
			want: proto.ServerTaskCatchupPolicy_SERVER_TASK_CATCHUP_POLICY_SKIP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainServerTaskCatchupPolicyToProto(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtoServerTaskExecutionStatusToDomain(t *testing.T) {
	tests := []struct {
		name string
		in   proto.ServerTaskExecutionStatus
		want domain.ServerTaskExecutionStatus
	}{
		{
			name: "running_to_domain_running",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_RUNNING,
			want: domain.ServerTaskExecutionStatusRunning,
		},
		{
			name: "success_to_domain_success",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			want: domain.ServerTaskExecutionStatusSuccess,
		},
		{
			name: "failed_to_domain_failed",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_FAILED,
			want: domain.ServerTaskExecutionStatusFailed,
		},
		{
			name: "canceled_to_domain_canceled",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_CANCELED,
			want: domain.ServerTaskExecutionStatusCanceled,
		},
		{
			name: "skipped_to_domain_skipped",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SKIPPED,
			want: domain.ServerTaskExecutionStatusSkipped,
		},
		{
			name: "timed_out_to_domain_timed_out",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_TIMED_OUT,
			want: domain.ServerTaskExecutionStatusTimedOut,
		},
		{
			name: "unspecified_to_empty_string",
			in:   proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_UNSPECIFIED,
			want: "",
		},
		{
			name: "unknown_enum_value_to_empty_string",
			in:   proto.ServerTaskExecutionStatus(9999),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProtoServerTaskExecutionStatusToDomain(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}
