package rcon

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_DispatchesByProtocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		protocol  Protocol
		wantType  string
		wantError string
	}{
		{
			name:     "source_protocol_returns_source_client",
			protocol: ProtocolSource,
			wantType: "*rcon.Source",
		},
		{
			name:     "goldsource_protocol_returns_goldsource_client",
			protocol: ProtocolGoldSrc,
			wantType: "*rcon.GoldSource",
		},
		{
			name:     "quake3_protocol_returns_quake_client",
			protocol: ProtocolQuake3,
			wantType: "*rcon.Quake",
		},
		{
			name:     "quake2_protocol_returns_quake_client",
			protocol: ProtocolQuake2,
			wantType: "*rcon.Quake",
		},
		{
			name:     "samp_protocol_returns_samp_client",
			protocol: ProtocolSAMP,
			wantType: "*rcon.SAMP",
		},
		{
			name:     "battleye_protocol_returns_battleye_client",
			protocol: ProtocolBattlEye,
			wantType: "*rcon.BattlEye",
		},
		{
			name:      "unknown_protocol_returns_error",
			protocol:  Protocol("rogue"),
			wantError: ErrUnsupportedProtocol.Error(),
		},
		{
			name:      "empty_protocol_returns_error",
			protocol:  Protocol(""),
			wantError: ErrUnsupportedProtocol.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			cfg := Config{
				Address:  "127.0.0.1:27015",
				Password: "x",
				Protocol: tt.protocol,
			}

			// ACT
			client, err := NewClient(cfg)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, client)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)

			assert.Equal(t, tt.wantType, fmt.Sprintf("%T", client))
		})
	}
}

// TestIsProtocolSupported_MatchesNewClient locks the two together. They used to be independent
// switch statements, and the plugin layer validates a declared built-in protocol name with
// IsProtocolSupported before NewClient ever runs.
func TestIsProtocolSupported_MatchesNewClient(t *testing.T) {
	t.Parallel()
	for protocol := range clientFactories {
		t.Run(string(protocol), func(t *testing.T) {
			t.Parallel()
			require.True(t, IsProtocolSupported(protocol))

			_, err := NewClient(Config{Address: "127.0.0.1:27015", Protocol: protocol})

			require.NoError(t, err)
		})
	}

	assert.False(t, IsProtocolSupported(Protocol("rogue")))
}

func TestIsProtocolSupported(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		protocol Protocol
		want     bool
	}{
		{name: "source_is_supported", protocol: ProtocolSource, want: true},
		{name: "goldsource_is_supported", protocol: ProtocolGoldSrc, want: true},
		{name: "quake3_is_supported", protocol: ProtocolQuake3, want: true},
		{name: "quake2_is_supported", protocol: ProtocolQuake2, want: true},
		{name: "samp_is_supported", protocol: ProtocolSAMP, want: true},
		{name: "battleye_is_supported", protocol: ProtocolBattlEye, want: true},
		{name: "unknown_is_not_supported", protocol: Protocol("rogue"), want: false},
		{name: "empty_is_not_supported", protocol: Protocol(""), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := IsProtocolSupported(tt.protocol)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPlayerManagementSupported(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		gameCode string
		want     bool
	}{
		{name: "cs_is_supported", gameCode: "cs", want: true},
		{name: "minecraft_is_supported", gameCode: "minecraft", want: true},
		{name: "valve_is_supported", gameCode: "valve", want: true},
		{name: "unknown_game_is_not_supported", gameCode: "rust", want: false},
		{name: "empty_game_code_is_not_supported", gameCode: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := IsPlayerManagementSupported(tt.gameCode)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
