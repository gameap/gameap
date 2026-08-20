package query

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRakNetResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
		wantError  string
	}{
		{
			name:  "valid_response",
			input: buildRakNetResponse("MCPE;My Server;486;1.19.0;5;20;12345;Overworld;Survival"),
			wantResult: &Result{
				Name:          "My Server",
				Map:           "Survival",
				PlayersNum:    5,
				MaxPlayersNum: 20,
			},
		},
		{
			name:  "minimal_payload",
			input: buildRakNetResponse("MCBE;Server;486;1.19;0;10"),
			wantResult: &Result{
				Name:          "Server",
				PlayersNum:    0,
				MaxPlayersNum: 10,
			},
		},
		{
			name:      "response_too_short",
			input:     make([]byte, 30),
			wantError: "response too short",
		},
		{
			name:      "invalid_packet_type",
			input:     buildRakNetResponseWithType(0x00, "MCPE;Server;486;1.19;0;10"),
			wantError: "invalid packet type",
		},
		{
			name:      "invalid_magic",
			input:     buildRakNetResponseWithInvalidMagic("MCPE;Server;486;1.19;0;10"),
			wantError: "invalid magic bytes",
		},
		{
			name:      "short_payload",
			input:     buildRakNetResponse("MCPE;S"),
			wantError: "expected at least 6 parts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseRakNetResponse(tt.input, result)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResult.Name, result.Name)
			assert.Equal(t, tt.wantResult.Map, result.Map)
			assert.Equal(t, tt.wantResult.PlayersNum, result.PlayersNum)
			assert.Equal(t, tt.wantResult.MaxPlayersNum, result.MaxPlayersNum)
		})
	}
}

func TestParseRakNetPayload(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		payload    string
		wantResult *Result
		wantError  string
	}{
		{
			name:    "full_payload",
			payload: "MCPE;Awesome Server;486;1.19.50;10;100;13579;World;Adventure;0;19132",
			wantResult: &Result{
				Name:          "Awesome Server",
				Map:           "Adventure",
				PlayersNum:    10,
				MaxPlayersNum: 100,
			},
		},
		{
			name:    "empty_server_name",
			payload: "MCBE;;486;1.19;5;50",
			wantResult: &Result{
				Name:          "",
				PlayersNum:    5,
				MaxPlayersNum: 50,
			},
		},
		{
			name:    "non_numeric_players",
			payload: "MCPE;Server;486;1.19;abc;xyz",
			wantResult: &Result{
				Name:          "Server",
				PlayersNum:    0,
				MaxPlayersNum: 0,
			},
		},
		{
			name:      "too_few_parts",
			payload:   "MCPE;Server;486",
			wantError: "expected at least 6 parts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseRakNetPayload(tt.payload, result)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResult.Name, result.Name)
			assert.Equal(t, tt.wantResult.Map, result.Map)
			assert.Equal(t, tt.wantResult.PlayersNum, result.PlayersNum)
			assert.Equal(t, tt.wantResult.MaxPlayersNum, result.MaxPlayersNum)
		})
	}
}

func TestBuildRakNetPingPacket(t *testing.T) {
	t.Parallel()
	packet := buildRakNetPingPacket()

	assert.Len(t, packet, 25)
	assert.Equal(t, byte(raknetUnconnectedPing), packet[0])
	assert.Equal(t, raknetMagic, packet[9:25])
}

func buildRakNetResponse(payload string) []byte {
	return buildRakNetResponseWithType(raknetUnconnectedPong, payload)
}

func buildRakNetResponseWithType(packetType byte, payload string) []byte {
	// Structure: type(1) + timestamp(8) + guid(8) + magic(16) + length(2) + payload
	response := make([]byte, 35+len(payload))

	response[0] = packetType

	binary.BigEndian.PutUint64(response[1:9], 12345)

	binary.BigEndian.PutUint64(response[9:17], 67890)

	copy(response[17:33], raknetMagic)

	binary.BigEndian.PutUint16(response[33:35], uint16(len(payload)))

	copy(response[35:], payload)

	return response
}

func buildRakNetResponseWithInvalidMagic(payload string) []byte {
	response := buildRakNetResponse(payload)

	for i := 17; i < 33; i++ {
		response[i] = 0xFF
	}

	return response
}
