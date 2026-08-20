package players

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinecraftPlayerManager_ParsePlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected []Player
	}{
		{
			name:  "multiple_players_online_with_uuids",
			input: "There are 3 of a max of 20 players online: Steve (550e8400-e29b-41d4-a716-446655440000), Alex (6ba7b810-9dad-11d1-80b4-00c04fd430c8), Notch (6ba7b811-9dad-11d1-80b4-00c04fd430c8)",
			expected: []Player{
				{Name: "Steve", ID: "550e8400-e29b-41d4-a716-446655440000", UniqID: "550e8400-e29b-41d4-a716-446655440000"},
				{Name: "Alex", ID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8", UniqID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
				{Name: "Notch", ID: "6ba7b811-9dad-11d1-80b4-00c04fd430c8", UniqID: "6ba7b811-9dad-11d1-80b4-00c04fd430c8"},
			},
		},
		{
			name:  "single_player_online_with_uuid",
			input: "There are 1 of a max of 20 players online: Steve (550e8400-e29b-41d4-a716-446655440000)",
			expected: []Player{
				{Name: "Steve", ID: "550e8400-e29b-41d4-a716-446655440000", UniqID: "550e8400-e29b-41d4-a716-446655440000"},
			},
		},
		{
			name:     "empty_server",
			input:    "There are 0 of a max of 20 players online:",
			expected: []Player{},
		},
		{
			name:  "player_with_special_characters_and_uuid",
			input: "There are 2 of a max of 20 players online: _xX_Player_Xx_ (550e8400-e29b-41d4-a716-446655440001), Dream123 (550e8400-e29b-41d4-a716-446655440002)",
			expected: []Player{
				{Name: "_xX_Player_Xx_", ID: "550e8400-e29b-41d4-a716-446655440001", UniqID: "550e8400-e29b-41d4-a716-446655440001"},
				{Name: "Dream123", ID: "550e8400-e29b-41d4-a716-446655440002", UniqID: "550e8400-e29b-41d4-a716-446655440002"},
			},
		},
		{
			name:  "players_with_extra_spaces",
			input: "There are 3 of a max of 20 players online:  Steve (550e8400-e29b-41d4-a716-446655440000) ,  Alex (6ba7b810-9dad-11d1-80b4-00c04fd430c8) , Notch (6ba7b811-9dad-11d1-80b4-00c04fd430c8) ",
			expected: []Player{
				{Name: "Steve", ID: "550e8400-e29b-41d4-a716-446655440000", UniqID: "550e8400-e29b-41d4-a716-446655440000"},
				{Name: "Alex", ID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8", UniqID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"},
				{Name: "Notch", ID: "6ba7b811-9dad-11d1-80b4-00c04fd430c8", UniqID: "6ba7b811-9dad-11d1-80b4-00c04fd430c8"},
			},
		},
		{
			name:     "malformed_response_no_colon",
			input:    "Some random text without colon",
			expected: []Player{},
		},
		{
			name:     "empty_input",
			input:    "",
			expected: []Player{},
		},
		{
			name:     "colon_only",
			input:    ":",
			expected: []Player{},
		},
		{
			name:     "colon_at_end",
			input:    "There are 0 players:",
			expected: []Player{},
		},
		{
			name:  "players_without_uuids_fallback",
			input: "There are 2 of a max of 20 players online: Player123, Test456",
			expected: []Player{
				{Name: "Player123"},
				{Name: "Test456"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewMinecraftPlayers()
			result, err := mgr.ParsePlayers(tt.input)

			assert.NoError(t, err)
			require.Len(t, result, len(tt.expected))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMinecraftPlayerManager_PlayersCommand(t *testing.T) {
	t.Parallel()
	mgr := NewMinecraftPlayers()
	assert.Equal(t, "list uuids", mgr.PlayersCommand())
}

func TestMinecraftPlayerManager_KickCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		player      Player
		reason      string
		expected    string
		expectedErr error
	}{
		{
			name:     "kick_with_reason",
			player:   Player{Name: "Steve"},
			reason:   "griefing",
			expected: "kick Steve griefing",
		},
		{
			name:     "kick_without_reason",
			player:   Player{Name: "Alex"},
			reason:   "",
			expected: "kick Alex",
		},
		{
			name:     "kick_with_long_reason",
			player:   Player{Name: "Notch"},
			reason:   "breaking server rules multiple times",
			expected: "kick Notch breaking server rules multiple times",
		},
		{
			name:        "kick_with_empty_name",
			player:      Player{Name: ""},
			reason:      "reason",
			expected:    "",
			expectedErr: ErrPlayerNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewMinecraftPlayers()
			result, err := mgr.KickCommand(tt.player, tt.reason)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMinecraftPlayerManager_BanCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		player      Player
		reason      string
		duration    time.Duration
		expected    string
		expectedErr error
	}{
		{
			name:     "ban_with_reason",
			player:   Player{Name: "Steve"},
			reason:   "hacking",
			duration: 24 * time.Hour,
			expected: "ban Steve hacking",
		},
		{
			name:     "ban_without_reason",
			player:   Player{Name: "Alex"},
			reason:   "",
			duration: time.Hour,
			expected: "ban Alex",
		},
		{
			name:     "ban_duration_ignored",
			player:   Player{Name: "Notch"},
			reason:   "spam",
			duration: 7 * 24 * time.Hour,
			expected: "ban Notch spam",
		},
		{
			name:        "ban_with_empty_name",
			player:      Player{Name: ""},
			reason:      "reason",
			duration:    time.Hour,
			expected:    "",
			expectedErr: ErrPlayerNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mgr := NewMinecraftPlayers()
			result, err := mgr.BanCommand(tt.player, tt.reason, tt.duration)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
