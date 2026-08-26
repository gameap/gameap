package players

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuakePlayerManager_ParsePlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected []Player
	}{
		{
			name: "quake3_status_with_colour_codes",
			input: `map: q3dm17
num score ping name            lastmsg address               qport rate
--- ----- ---- --------------- ------- --------------------- ----- -----
  0    15   45 ^1Red^7Player^7        50 192.0.2.10:27960      12345 25000
  1    -3   72 Space Name^7           10 192.0.2.11:27960      12346 25000
`,
			expected: []Player{
				{ID: "0", Name: "RedPlayer", Score: "15", Ping: "45", Addr: "192.0.2.10", UniqID: "0"},
				{ID: "1", Name: "Space Name", Score: "-3", Ping: "72", Addr: "192.0.2.11", UniqID: "1"},
			},
		},
		{
			name: "call_of_duty_status_with_guid_column",
			input: `num score ping guid                             name            lastmsg address               qport rate
--- ----- ---- -------------------------------- --------------- ------- --------------------- ----- -----
  0     7   58 0123456789abcdef0123456789abcdef Sniper^7               0 192.0.2.20:28960      12345 25000
`,
			expected: []Player{
				{
					ID: "0", Name: "Sniper", Score: "7", Ping: "58",
					Addr: "192.0.2.20", UniqID: "0123456789abcdef0123456789abcdef",
				},
			},
		},
		{
			name: "quake2_status_without_rate_column",
			input: `map              : q2dm1
num score ping name            lastmsg address               qport
--- ----- ---- --------------- ------- --------------------- ------
  0    12   50 Ranger                0 192.0.2.30:27901       1234
`,
			expected: []Player{
				{ID: "0", Name: "Ranger", Score: "12", Ping: "50", Addr: "192.0.2.30", UniqID: "0"},
			},
		},
		{
			name: "connecting_and_zombie_clients_keep_their_ping_marker",
			input: `num score ping name            lastmsg address               qport
  0    12 CNCT Joining              0 192.0.2.30:27901       1234
  1     0 ZMBI Leaving              0 192.0.2.31:27901       1235
`,
			expected: []Player{
				{ID: "0", Name: "Joining", Score: "12", Ping: "CNCT", Addr: "192.0.2.30", UniqID: "0"},
				{ID: "1", Name: "Leaving", Score: "0", Ping: "ZMBI", Addr: "192.0.2.31", UniqID: "1"},
			},
		},
		{
			name: "bot_and_loopback_addresses_are_kept_verbatim",
			input: `  0    15   45 BotPlayer^7           50 bot                   12345 25000
  1    20    0 LocalHost^7            0 loopback              12346 25000
`,
			expected: []Player{
				{ID: "0", Name: "BotPlayer", Score: "15", Ping: "45", Addr: "bot", UniqID: "0"},
				{ID: "1", Name: "LocalHost", Score: "20", Ping: "0", Addr: "loopback", UniqID: "1"},
			},
		},
		{
			name:     "header_only_output_yields_no_players",
			input:    "map: q3dm17\nnum score ping name            lastmsg address               qport rate\n--- ----- ---- --------------- ------- --------------------- ----- -----\n",
			expected: []Player{},
		},
		{
			name:     "server_message_is_not_a_player",
			input:    "Server is not running.\n",
			expected: []Player{},
		},
		{
			name:     "empty_output",
			input:    "",
			expected: []Player{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewQuakePlayers()

			got, err := mgr.ParsePlayers(tt.input)

			require.NoError(t, err)
			require.Len(t, got, len(tt.expected))
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestQuakePlayerManager_PlayersCommand(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "status", NewQuakePlayers().PlayersCommand())
}

func TestQuakePlayerManager_ModerationIsNotSupported(t *testing.T) {
	t.Parallel()
	mgr := NewQuakePlayers()
	player := Player{ID: "0", Name: "Player", UniqID: "0"}

	kick, kickErr := mgr.KickCommand(player, "cheating")
	ban, banErr := mgr.BanCommand(player, "cheating", time.Minute)

	assert.ErrorIs(t, kickErr, ErrPlayerActionNotSupported)
	assert.Empty(t, kick)
	assert.ErrorIs(t, banErr, ErrPlayerActionNotSupported)
	assert.Empty(t, ban)
}

func TestQuakePlayerManager_DecodesLatin1Nickname(t *testing.T) {
	t.Parallel()
	// A nickname sent as raw ISO 8859-1 bytes: "Jos\xe9".
	input := "  0    15   45 Jos\xe9^7           50 192.0.2.10:27960      12345 25000\n"

	got, err := NewQuakePlayers().ParsePlayers(input)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "José", got[0].Name)
}

func TestQuakePlayerManager_KeepsValidUTF8Nickname(t *testing.T) {
	t.Parallel()
	input := "  0    15   45 Пётр^7           50 192.0.2.10:27960      12345 25000\n"

	got, err := NewQuakePlayers().ParsePlayers(input)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Пётр", got[0].Name, "valid UTF-8 must not be re-decoded into mojibake")
}
