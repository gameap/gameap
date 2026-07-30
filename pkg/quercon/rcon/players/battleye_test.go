package players

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBattlEyePlayerManager_ParsePlayers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Player
	}{
		{
			name: "full_table",
			input: `Players on server:
[#] [IP Address]:[Port] [Ping] [GUID] [Name]
--------------------------------------------------
0   192.0.2.10:2304       45   80c1b8dbf9d9d3f9e1a2b3c4d5e6f708(OK) Nickname
1   192.0.2.11:2304       0    -                                    Other Name
(2 players in total)`,
			expected: []Player{
				{ID: "0", Name: "Nickname", Ping: "45", Addr: "192.0.2.10", UniqID: "80c1b8dbf9d9d3f9e1a2b3c4d5e6f708"},
				{ID: "1", Name: "Other Name", Ping: "0", Addr: "192.0.2.11", UniqID: ""},
			},
		},
		{
			name: "lobby_suffix_is_part_of_the_name",
			input: `Players on server:
[#] [IP Address]:[Port] [Ping] [GUID] [Name]
--------------------------------------------------
3   192.0.2.13:2304       88   aabbccddeeff00112233445566778899(OK) Joining Player (Lobby)
(1 players in total)`,
			expected: []Player{
				{
					ID: "3", Name: "Joining Player (Lobby)", Ping: "88",
					Addr: "192.0.2.13", UniqID: "aabbccddeeff00112233445566778899",
				},
			},
		},
		{
			name: "empty_server",
			input: `Players on server:
[#] [IP Address]:[Port] [Ping] [GUID] [Name]
--------------------------------------------------
(0 players in total)`,
			expected: []Player{},
		},
		{
			name:     "unrelated_console_output",
			input:    "Connected to BE Master\n",
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
			mgr := NewBattlEyePlayers()

			got, err := mgr.ParsePlayers(tt.input)

			require.NoError(t, err)
			require.Len(t, got, len(tt.expected))
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBattlEyePlayerManager_PlayersCommand(t *testing.T) {
	assert.Equal(t, "players", NewBattlEyePlayers().PlayersCommand())
}

func TestBattlEyePlayerManager_ModerationIsNotSupported(t *testing.T) {
	mgr := NewBattlEyePlayers()
	player := Player{ID: "0", Name: "Nickname", UniqID: "guid"}

	kick, kickErr := mgr.KickCommand(player, "cheating")
	ban, banErr := mgr.BanCommand(player, "cheating", time.Hour)

	assert.ErrorIs(t, kickErr, ErrPlayerActionNotSupported)
	assert.Empty(t, kick)
	assert.ErrorIs(t, banErr, ErrPlayerActionNotSupported)
	assert.Empty(t, ban)
}
