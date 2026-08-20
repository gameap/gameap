package players

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlayer_ValidateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		player      Player
		expectedErr error
	}{
		{
			name:        "valid_name",
			player:      Player{Name: "Steve"},
			expectedErr: nil,
		},
		{
			name:        "empty_name",
			player:      Player{Name: ""},
			expectedErr: ErrPlayerNameRequired,
		},
		{
			name:        "name_with_spaces",
			player:      Player{Name: "Player Name"},
			expectedErr: nil,
		},
		{
			name:        "name_with_special_characters",
			player:      Player{Name: "[CLAN] Player_123"},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.player.ValidateName()
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPlayer_ValidateUniqID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		player      Player
		expectedErr error
	}{
		{
			name:        "valid_uniq_id",
			player:      Player{UniqID: "STEAM_0:0:12345678"},
			expectedErr: nil,
		},
		{
			name:        "empty_uniq_id",
			player:      Player{UniqID: ""},
			expectedErr: ErrPlayerUniqIDRequired,
		},
		{
			name:        "bot_uniq_id",
			player:      Player{UniqID: "BOT"},
			expectedErr: nil,
		},
		{
			name:        "hltv_uniq_id",
			player:      Player{UniqID: "HLTV"},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.player.ValidateUniqID()
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
