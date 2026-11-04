package players

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRustPlayerManager_ParsePlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		expected  []Player
		wantError string
	}{
		{
			name: "playerlist_with_two_players",
			input: `[
  {
    "SteamID": "76561198011856444",
    "OwnerSteamID": "0",
    "DisplayName": "NiK",
    "Ping": 34,
    "Address": "100.90.137.34:65039",
    "EntityId": 112233,
    "ConnectedSeconds": 120,
    "VoiationLevel": 0.0,
    "CurrentLevel": 0.0,
    "UnspentXp": 0.0,
    "Health": 100.0
  },
  {
    "SteamID": "76561197960265728",
    "DisplayName": "Player \"Two\"",
    "Ping": 0,
    "Address": "10.0.0.5:5000"
  }
]`,
			expected: []Player{
				{ID: "76561198011856444", UniqID: "76561198011856444", Name: "NiK", Ping: "34", Addr: "100.90.137.34:65039"},
				{ID: "76561197960265728", UniqID: "76561197960265728", Name: `Player "Two"`, Ping: "0", Addr: "10.0.0.5:5000"},
			},
		},
		{
			name:     "steam_id_as_number_keeps_all_digits",
			input:    `[{"SteamID": 76561198011856444, "DisplayName": "NiK", "Ping": 12}]`,
			expected: []Player{{ID: "76561198011856444", UniqID: "76561198011856444", Name: "NiK", Ping: "12"}},
		},
		{
			name:     "empty_server",
			input:    "[]",
			expected: []Player{},
		},
		{
			name:     "empty_response",
			input:    "  \n",
			expected: []Player{},
		},
		{
			name:      "not_a_playerlist_response",
			input:     "Command not found",
			wantError: "invalid playerlist response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := NewRustPlayers().ParsePlayers(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			require.Len(t, result, len(tt.expected))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRustPlayerManager_KickCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		player    Player
		reason    string
		expected  string
		wantError string
	}{
		{
			name:     "kick_by_steam_id_with_reason",
			player:   Player{UniqID: "76561198011856444", Name: "NiK"},
			reason:   "too loud",
			expected: `kick "76561198011856444" "too loud"`,
		},
		{
			name:     "kick_without_reason",
			player:   Player{UniqID: "76561198011856444"},
			expected: `kick "76561198011856444" ""`,
		},
		{
			name:     "quotes_in_reason_cannot_break_the_argument",
			player:   Player{UniqID: "76561198011856444"},
			reason:   `bye" "x`,
			expected: `kick "76561198011856444" "bye' 'x"`,
		},
		{
			name:      "player_without_steam_id",
			player:    Player{Name: "NiK"},
			wantError: ErrPlayerUniqIDRequired.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := NewRustPlayers().KickCommand(tt.player, tt.reason)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRustPlayerManager_BanCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		player    Player
		reason    string
		duration  time.Duration
		expected  string
		wantError string
	}{
		{
			name:     "permanent_ban",
			player:   Player{UniqID: "76561198011856444", Name: "NiK"},
			reason:   "cheating",
			expected: `banid "76561198011856444" "NiK" "cheating"`,
		},
		{
			name:     "temporary_ban_shorter_than_an_hour_stays_temporary",
			player:   Player{UniqID: "76561198011856444", Name: "NiK"},
			reason:   "spam",
			duration: 30 * time.Minute,
			expected: `banid "76561198011856444" "NiK" "spam" 1800s`,
		},
		{
			name:     "name_with_spaces_and_quotes",
			player:   Player{UniqID: "76561198011856444", Name: `The "Best" One`},
			duration: 24 * time.Hour,
			expected: `banid "76561198011856444" "The 'Best' One" "" 86400s`,
		},
		{
			name:      "player_without_steam_id",
			player:    Player{Name: "NiK"},
			wantError: ErrPlayerUniqIDRequired.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := NewRustPlayers().BanCommand(tt.player, tt.reason, tt.duration)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRustPlayerManager_CommandAndCapabilities(t *testing.T) {
	t.Parallel()

	mgr := NewRustPlayers()

	assert.Equal(t, "playerlist", mgr.PlayersCommand())
	assert.Equal(t, Capability{List: true, Kick: true, Ban: true}, mgr.Capabilities())
}
