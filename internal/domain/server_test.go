package domain

import (
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/idgen"
	"github.com/google/uuid"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func TestServer_ReplaceServerShortcodes(t *testing.T) {
	t.Parallel()

	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	queryPort := 27015
	rconPort := 27016
	suUser := "gameserver"

	server := &Server{
		ID:         42,
		UID:        testUUID,
		ServerIP:   "192.168.1.100",
		ServerPort: 27015,
		QueryPort:  &queryPort,
		RconPort:   &rconPort,
		SuUser:     &suUser,
		GameID:     "cs2",
		Dir:        "/var/games/server1",
	}
	server.Hydrate()

	node := &Node{
		WorkPath: "/var/gameap",
	}

	tests := []struct {
		name    string
		command string
		extra   map[string]string
		want    string
	}{
		{
			name:    "replace_basic_shortcodes",
			command: "connect {host}:{port}",
			extra:   nil,
			want:    "connect 192.168.1.100:27015",
		},
		{
			name:    "replace_node_paths",
			command: "{node_work_path}/scripts/start.sh",
			extra:   nil,
			want:    "/var/gameap/scripts/start.sh",
		},
		{
			name:    "replace_node_tools_path",
			command: "{node_tools_path}/steamcmd",
			extra:   nil,
			want:    "/var/gameap/tools/steamcmd",
		},
		{
			name:    "replace_server_id",
			command: "server_{id}",
			extra:   nil,
			want:    "server_42",
		},
		{
			name:    "replace_uuid",
			command: "UUID: {uuid}",
			extra:   nil,
			want:    "UUID: 550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:    "replace_uuid_short",
			command: "Short: {uuid_short}",
			extra:   nil,
			want:    "Short: 550e8400",
		},
		{
			name:    "replace_game_id",
			command: "Game: {game}",
			extra:   nil,
			want:    "Game: cs2",
		},
		{
			name:    "replace_dir",
			command: "{dir}/config.cfg",
			extra:   nil,
			want:    "/var/games/server1/config.cfg",
		},
		{
			name:    "replace_query_port",
			command: "Query port: {query_port}",
			extra:   nil,
			want:    "Query port: 27015",
		},
		{
			name:    "replace_rcon_port",
			command: "RCON port: {rcon_port}",
			extra:   nil,
			want:    "RCON port: 27016",
		},
		{
			name:    "replace_user",
			command: "su {user} -c 'start.sh'",
			extra:   nil,
			want:    "su gameserver -c 'start.sh'",
		},
		{
			name:    "replace_multiple_shortcodes",
			command: "{node_work_path}/{dir}/start.sh -ip {host} -port {port}",
			extra:   nil,
			want:    "/var/gameap//var/games/server1/start.sh -ip 192.168.1.100 -port 27015",
		},
		{
			name:    "replace_extra_data",
			command: "echo {custom_var}",
			extra: map[string]string{
				"custom_var": "test_value",
			},
			want: "echo test_value",
		},
		{
			name:    "replace_extra_and_standard_shortcodes",
			command: "{custom_path}/{dir} -host {host}",
			extra: map[string]string{
				"custom_path": "/custom",
			},
			want: "/custom//var/games/server1 -host 192.168.1.100",
		},
		{
			name:    "extra_data_takes_precedence",
			command: "{host}",
			extra: map[string]string{
				"host": "overridden.com",
			},
			want: "overridden.com",
		},
		{
			name:    "no_shortcodes_in_command",
			command: "simple command without placeholders",
			extra:   nil,
			want:    "simple command without placeholders",
		},
		{
			name:    "unknown_shortcode_not_replaced",
			command: "{unknown_shortcode}",
			extra:   nil,
			want:    "{unknown_shortcode}",
		},
		{
			name:    "empty_command",
			command: "",
			extra:   nil,
			want:    "",
		},
		{
			name:    "uppercase_host_shortcode_replaced",
			command: "connect {HOST}:{port}",
			extra:   nil,
			want:    "connect 192.168.1.100:27015",
		},
		{
			name:    "uppercase_port_shortcode_replaced",
			command: "connect {host}:{PORT}",
			extra:   nil,
			want:    "connect 192.168.1.100:27015",
		},
		{
			name:    "uppercase_id_shortcode_replaced",
			command: "server_{ID}",
			extra:   nil,
			want:    "server_42",
		},
		{
			name:    "mixed_case_shortcode_not_replaced",
			command: "connect {Host}:27015",
			extra:   nil,
			want:    "connect {Host}:27015",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			result := server.ReplaceServerShortcodes(node, test.command, test.extra)

			// ASSERT
			assert.Equal(t, test.want, result, "shortcode replacement mismatch")
		})
	}
}

func TestServer_ReplaceServerShortcodes_WithNilOptionalFields(t *testing.T) {
	t.Parallel()

	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	server := &Server{
		ID:         1,
		UID:        testUUID,
		ServerIP:   "10.0.0.1",
		ServerPort: 25565,
		QueryPort:  nil,
		RconPort:   nil,
		SuUser:     nil,
		GameID:     "minecraft",
		Dir:        "/minecraft",
	}
	server.Hydrate()

	node := &Node{
		WorkPath: "/gameap",
	}

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "query_port_empty_when_nil",
			command: "Query: {query_port}",
			want:    "Query: ",
		},
		{
			name:    "rcon_port_empty_when_nil",
			command: "RCON: {rcon_port}",
			want:    "RCON: ",
		},
		{
			name:    "user_empty_when_nil",
			command: "User: {user}",
			want:    "User: ",
		},
		{
			name:    "multiple_nil_fields",
			command: "su {user} query={query_port} rcon={rcon_port}",
			want:    "su  query= rcon=",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := server.ReplaceServerShortcodes(node, test.command, nil)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestServer_ReplaceServerShortcodes_MultipleReplacements(t *testing.T) {
	t.Parallel()

	testUUID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	queryPort := 27015

	server := &Server{
		ID:         100,
		UID:        testUUID,
		ServerIP:   "example.com",
		ServerPort: 7777,
		QueryPort:  &queryPort,
		GameID:     "game1",
		Dir:        "/games/server",
	}
	server.Hydrate()

	node := &Node{
		WorkPath: "/srv/gameap",
	}

	command := "{host} {host} {port} {port} {id} {id}"
	expected := "example.com example.com 7777 7777 100 100"

	result := server.ReplaceServerShortcodes(node, command, nil)
	assert.Equal(t, expected, result)
}

func TestServer_IsOnline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		processActive    bool
		lastProcessCheck *time.Time
		wantedOnline     bool
	}{
		{
			name:             "online_process_active_recent_check",
			processActive:    true,
			lastProcessCheck: new(time.Now().Add(-1 * time.Minute)),
			wantedOnline:     true,
		},
		{
			name:             "offline_process_active_old_check",
			processActive:    true,
			lastProcessCheck: new(time.Now().Add(-3 * time.Minute)),
			wantedOnline:     false,
		},
		{
			name:             "offline_process_inactive_recent_check",
			processActive:    false,
			lastProcessCheck: new(time.Now().Add(-1 * time.Minute)),
			wantedOnline:     false,
		},
		{
			name:             "offline_nil_last_check",
			processActive:    true,
			lastProcessCheck: nil,
			wantedOnline:     false,
		},
		{
			name:             "offline_zero_time_check",
			processActive:    true,
			lastProcessCheck: new(time.Time{}),
			wantedOnline:     false,
		},
		{
			name:             "online_exactly_at_threshold",
			processActive:    true,
			lastProcessCheck: new(time.Now().Add(-2 * time.Minute)),
			wantedOnline:     false,
		},
		{
			name:             "online_just_before_threshold",
			processActive:    true,
			lastProcessCheck: new(time.Now().Add(-119 * time.Second)),
			wantedOnline:     true,
		},
		{
			name:             "offline_both_false_and_nil",
			processActive:    false,
			lastProcessCheck: nil,
			wantedOnline:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := &Server{
				ProcessActive:    test.processActive,
				LastProcessCheck: test.lastProcessCheck,
			}

			result := server.IsOnline()
			assert.Equal(t, test.wantedOnline, result)
		})
	}
}

func TestServer_RconPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rcon *string
		want string
	}{
		{
			name: "nil_rcon_returns_empty_string",
			rcon: nil,
			want: "",
		},
		{
			name: "empty_rcon_returns_empty_string",
			rcon: new(""),
			want: "",
		},
		{
			name: "configured_rcon_returns_password",
			rcon: new("s3cr3tpass"),
			want: "s3cr3tpass",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			server := &Server{Rcon: test.rcon}

			// ACT
			got := server.RconPassword()

			// ASSERT
			assert.Equal(t, test.want, got)
		})
	}
}

func TestServerInstalledStatusConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ServerInstalledStatus(0), ServerInstalledStatusNotInstalled)
	assert.Equal(t, ServerInstalledStatus(1), ServerInstalledStatusInstalled)
	assert.Equal(t, ServerInstalledStatus(2), ServerInstalledStatusInstallationInProg)
}

func TestServer_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	queryPort := 27015
	rconPort := 27016
	rcon := "password123"
	suUser := "gameserver"
	cpuLimit := 100
	ramLimit := 2048
	netLimit := 1000
	startCmd := "./start.sh"
	stopCmd := "./stop.sh"
	forceStopCmd := "pkill -9 server"
	restartCmd := "./restart.sh"
	server := Server{
		ID:               42,
		UID:              testUUID,
		Enabled:          true,
		Installed:        ServerInstalledStatusInstalled,
		Blocked:          false,
		Name:             "Test Server",
		GameID:           "cs2",
		DSID:             10,
		GameModID:        5,
		Expires:          &now,
		ServerIP:         "192.168.1.100",
		ServerPort:       27015,
		QueryPort:        &queryPort,
		RconPort:         &rconPort,
		Rcon:             &rcon,
		Dir:              "/var/games/server1",
		SuUser:           &suUser,
		CPULimit:         &cpuLimit,
		RAMLimit:         &ramLimit,
		NetLimit:         &netLimit,
		StartCommand:     &startCmd,
		StopCommand:      &stopCmd,
		ForceStopCommand: &forceStopCmd,
		RestartCommand:   &restartCmd,
		ProcessActive:    true,
		LastProcessCheck: &now,
		Vars:             ServerVars{"key": "value"},
		CreatedAt:        &now,
		UpdatedAt:        &now,
		DeletedAt:        nil,
	}
	server.Hydrate()

	assert.Equal(t, uint(42), server.ID)
	assert.Equal(t, testUUID, server.UUID)
	assert.Equal(t, "550e8400", server.UUIDShort)
	assert.True(t, server.Enabled)
	assert.Equal(t, ServerInstalledStatusInstalled, server.Installed)
	assert.False(t, server.Blocked)
	assert.Equal(t, "Test Server", server.Name)
	assert.Equal(t, "cs2", server.GameID)
	assert.Equal(t, uint(10), server.DSID)
	assert.Equal(t, uint(5), server.GameModID)
	assert.Equal(t, &now, server.Expires)
	assert.Equal(t, "192.168.1.100", server.ServerIP)
	assert.Equal(t, 27015, server.ServerPort)
	assert.Equal(t, &queryPort, server.QueryPort)
	assert.Equal(t, &rconPort, server.RconPort)
	assert.Equal(t, &rcon, server.Rcon)
	assert.Equal(t, "/var/games/server1", server.Dir)
	assert.Equal(t, &suUser, server.SuUser)
	assert.Equal(t, &cpuLimit, server.CPULimit)
	assert.Equal(t, &ramLimit, server.RAMLimit)
	assert.Equal(t, &netLimit, server.NetLimit)
	assert.Equal(t, &startCmd, server.StartCommand)
	assert.Equal(t, &stopCmd, server.StopCommand)
	assert.Equal(t, &forceStopCmd, server.ForceStopCommand)
	assert.Equal(t, &restartCmd, server.RestartCommand)
	assert.True(t, server.ProcessActive)
	assert.Equal(t, &now, server.LastProcessCheck)
	assert.Equal(t, ServerVars{"key": "value"}, server.Vars)
	assert.Equal(t, &now, server.CreatedAt)
	assert.Equal(t, &now, server.UpdatedAt)
	assert.Nil(t, server.DeletedAt)
}

func TestServerInstalledStatus_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ServerInstalledStatus
		want   bool
	}{
		{name: "not_installed", status: ServerInstalledStatusNotInstalled, want: true},
		{name: "installed", status: ServerInstalledStatusInstalled, want: true},
		{name: "installation_in_progress", status: ServerInstalledStatusInstallationInProg, want: true},
		{name: "above_the_last_known_status", status: ServerInstalledStatus(3), want: false},
		{name: "byte_overflow_value", status: ServerInstalledStatus(255), want: false},
		{name: "negative_value", status: ServerInstalledStatus(-1), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			status := tt.status

			// ACT
			got := status.Valid()

			// ASSERT
			assert.Equal(t, tt.want, got, "validity of status %d", int(status))
		})
	}
}

func TestServer_XID(t *testing.T) {
	t.Parallel()

	t.Run("recovers_the_xid_the_uid_was_built_from", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		want := xid.New()
		server := &Server{UID: idgen.XIDToUUID(want)}

		// ACT
		got := server.XID()

		// ASSERT
		assert.Equal(t, want, got, "XID must round-trip a UID produced by idgen.XIDToUUID")
	})

	t.Run("matches_the_idgen_conversion_for_a_foreign_uuid", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		server := &Server{UID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")}

		// ACT
		got := server.XID()

		// ASSERT
		assert.Equal(t, idgen.UUIDToXID(server.UID), got)
		assert.Equal(t, got, server.XID(), "conversion must be deterministic")
	})

	t.Run("zero_uid_maps_to_the_nil_xid", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		server := &Server{}

		// ACT
		got := server.XID()

		// ASSERT
		assert.Equal(t, xid.NilID(), got)
		assert.Equal(t, idgen.UUIDToXID(uuid.Nil), got)
	})
}
