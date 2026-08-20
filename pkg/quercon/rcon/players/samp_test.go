package players

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSAMPPlayerManager_ParsePlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected []Player
	}{
		{
			name:  "tab_separated_table",
			input: "ID\tName\tPing\tIP\n0\tAlice\t42\t192.0.2.10\n1\tBob_1\t53\t192.0.2.11",
			expected: []Player{
				{ID: "0", Name: "Alice", Ping: "42", Addr: "192.0.2.10", UniqID: "0"},
				{ID: "1", Name: "Bob_1", Ping: "53", Addr: "192.0.2.11", UniqID: "1"},
			},
		},
		{
			name:  "space_padded_table",
			input: "ID    Name    Ping   IP\n0     Alice   42     192.0.2.10",
			expected: []Player{
				{ID: "0", Name: "Alice", Ping: "42", Addr: "192.0.2.10", UniqID: "0"},
			},
		},
		{
			name:  "row_without_ip",
			input: "ID\tName\tPing\tIP\n7\tCharlie\t99",
			expected: []Player{
				{ID: "7", Name: "Charlie", Ping: "99", UniqID: "7"},
			},
		},
		{
			name:     "empty_server_prints_nothing",
			input:    "",
			expected: []Player{},
		},
		{
			name:     "header_only",
			input:    "ID\tName\tPing\tIP",
			expected: []Player{},
		},
		{
			name:     "refused_password_message_is_not_a_player",
			input:    "Invalid RCON password.",
			expected: []Player{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewSAMPPlayers()

			got, err := mgr.ParsePlayers(tt.input)

			require.NoError(t, err)
			require.Len(t, got, len(tt.expected))
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSAMPPlayerManager_PlayersCommand(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "players", NewSAMPPlayers().PlayersCommand())
}

func TestSAMPPlayerManager_ModerationIsNotSupported(t *testing.T) {
	t.Parallel()
	mgr := NewSAMPPlayers()
	player := Player{ID: "0", Name: "Alice", UniqID: "0"}

	kick, kickErr := mgr.KickCommand(player, "cheating")
	ban, banErr := mgr.BanCommand(player, "cheating", time.Hour)

	assert.ErrorIs(t, kickErr, ErrPlayerActionNotSupported)
	assert.Empty(t, kick)
	assert.ErrorIs(t, banErr, ErrPlayerActionNotSupported)
	assert.Empty(t, ban)
}
