package query

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGameSpy3Challenge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []byte
		want      []byte
		wantError string
	}{
		{
			name:  "normal_challenge",
			input: []byte{0x09, 0x10, 0x20, 0x30, 0x40, '1', '2', '3', '4', '5', 0x00},
			want:  []byte{0x00, 0x00, 0x30, 0x39}, // 12345 in BE
		},
		{
			name:  "zero_challenge",
			input: []byte{0x09, 0x10, 0x20, 0x30, 0x40, '0', 0x00},
			want:  []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "empty_challenge",
			input: []byte{0x09, 0x10, 0x20, 0x30, 0x40, 0x00},
			want:  []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "negative_challenge",
			input: []byte{0x09, 0x10, 0x20, 0x30, 0x40, '-', '1', '0', '0', 0x00},
			want:  []byte{0xFF, 0xFF, 0xFF, 0x9C}, // -100 as uint32 in BE
		},
		{
			name:      "response_too_short",
			input:     []byte{0x09, 0x10},
			wantError: "challenge response too short",
		},
		{
			name:      "invalid_number",
			input:     []byte{0x09, 0x10, 0x20, 0x30, 0x40, 'a', 'b', 'c', 0x00},
			wantError: "failed to parse challenge number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseGameSpy3Challenge(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestParseGameSpy3PacketHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []byte
		wantID    int
		wantData  []byte
		wantSplit bool
		wantError string
	}{
		{
			name:      "non_split_packet",
			input:     append(make([]byte, 16), []byte("key\x00value\x00")...),
			wantID:    0,
			wantData:  []byte("key\x00value\x00"),
			wantSplit: false,
		},
		{
			name:      "split_packet",
			input:     buildSplitPacketHeader(2, []byte("data")),
			wantID:    2,
			wantData:  []byte("data"),
			wantSplit: true,
		},
		{
			name:      "packet_too_short",
			input:     []byte{0x00, 0x01},
			wantError: "packet too short",
		},
		{
			name:      "invalid_packet_type",
			input:     []byte{0x01, 0x00, 0x00, 0x00, 0x00},
			wantError: "invalid packet type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseGameSpy3PacketHeader(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, result.packetID)
			assert.Equal(t, tt.wantData, result.data)
			assert.Equal(t, tt.wantSplit, result.isSplit)
		})
	}
}

func TestCleanGameSpy3Packets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		packets []gamespy3Packet
		want    []byte
	}{
		{
			name:    "single_packet",
			packets: []gamespy3Packet{{packetID: 0, data: []byte("hello")}},
			want:    []byte("hello"),
		},
		{
			name: "multiple_packets_ordered",
			packets: []gamespy3Packet{
				{packetID: 0, data: []byte("first")},
				{packetID: 1, data: []byte("second")},
				{packetID: 2, data: []byte("third")},
			},
			want: []byte("firstsecondthird"),
		},
		{
			name: "multiple_packets_unordered",
			packets: []gamespy3Packet{
				{packetID: 2, data: []byte("third")},
				{packetID: 0, data: []byte("first")},
				{packetID: 1, data: []byte("second")},
			},
			want: []byte("firstsecondthird"),
		},
		{
			name: "packets_with_overlap",
			packets: []gamespy3Packet{
				{packetID: 0, data: []byte("hello world")},
				{packetID: 1, data: []byte("world end")},
			},
			want: []byte("hello world end"),
		},
		{
			name:    "empty_packets",
			packets: []gamespy3Packet{},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := cleanGameSpy3Packets(tt.packets)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFindOverlap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ending    []byte
		beginning []byte
		want      int
	}{
		{
			name:      "full_overlap",
			ending:    []byte("hello"),
			beginning: []byte("hello world"),
			want:      5,
		},
		{
			name:      "partial_overlap",
			ending:    []byte("abc123"),
			beginning: []byte("123xyz"),
			want:      3,
		},
		{
			name:      "no_overlap",
			ending:    []byte("abc"),
			beginning: []byte("xyz"),
			want:      0,
		},
		{
			name:      "single_byte_overlap",
			ending:    []byte("abc"),
			beginning: []byte("cde"),
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := findOverlap(tt.ending, tt.beginning)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestParseGameSpy3Response(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
		wantError  string
	}{
		{
			name:  "basic_server_info",
			input: buildGameSpy3ServerInfo("Test Server", "de_dust2", 10, 24),
			wantResult: &Result{
				Name:          "Test Server",
				Map:           "de_dust2",
				PlayersNum:    10,
				MaxPlayersNum: 24,
			},
		},
		{
			name:  "server_with_players",
			input: buildGameSpy3ResponseWithPlayers("BF2 Server", "strike_at_karkand", 5, 64, []string{"Player1", "Player2"}),
			wantResult: &Result{
				Name:          "BF2 Server",
				Map:           "strike_at_karkand",
				PlayersNum:    5,
				MaxPlayersNum: 64,
				Players: []ResultPlayer{
					{Name: "Player1", Score: 0},
					{Name: "Player2", Score: 0},
				},
			},
		},
		{
			name:      "empty_response",
			input:     []byte{},
			wantError: "empty response data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseGameSpy3Response(tt.input, result)

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

			if tt.wantResult.Players != nil {
				require.Len(t, result.Players, len(tt.wantResult.Players))
				for i, p := range tt.wantResult.Players {
					assert.Equal(t, p.Name, result.Players[i].Name)
				}
			}
		})
	}
}

func TestParseGameSpy3ServerDetails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
	}{
		{
			name:  "all_fields",
			input: buildKeyValuePairs(map[string]string{"hostname": "Server", "mapname": "map1", "numplayers": "5", "maxplayers": "32"}),
			wantResult: &Result{
				Name:          "Server",
				Map:           "map1",
				PlayersNum:    5,
				MaxPlayersNum: 32,
			},
		},
		{
			name:  "alternative_keys",
			input: buildKeyValuePairs(map[string]string{"sv_hostname": "Alt Server", "map": "alt_map"}),
			wantResult: &Result{
				Name: "Alt Server",
				Map:  "alt_map",
			},
		},
		{
			name:       "empty_input",
			input:      []byte{},
			wantResult: &Result{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			parseGameSpy3ServerDetails(tt.input, result)

			assert.Equal(t, tt.wantResult.Name, result.Name)
			assert.Equal(t, tt.wantResult.Map, result.Map)
			assert.Equal(t, tt.wantResult.PlayersNum, result.PlayersNum)
			assert.Equal(t, tt.wantResult.MaxPlayersNum, result.MaxPlayersNum)
		})
	}
}

func buildSplitPacketHeader(packetID int, data []byte) []byte {
	var buf bytes.Buffer

	buf.WriteByte(0x00)                       // type
	buf.Write([]byte{0x10, 0x20, 0x30, 0x40}) // session_id
	buf.WriteString(gamespy3SplitMarker)      // splitnum\x00
	buf.WriteByte(byte(packetID & 0x7F))      // packet_id
	buf.WriteByte(0x00)                       // unknown
	buf.Write(data)

	return buf.Bytes()
}

func buildGameSpy3ServerInfo(hostname, mapName string, numPlayers, maxPlayers int) []byte {
	return buildKeyValuePairs(map[string]string{
		"hostname":   hostname,
		"mapname":    mapName,
		"numplayers": intToString(numPlayers),
		"maxplayers": intToString(maxPlayers),
	})
}

func buildGameSpy3ResponseWithPlayers(hostname, mapName string, numPlayers, maxPlayers int, players []string) []byte {
	var buf bytes.Buffer

	serverInfo := buildGameSpy3ServerInfo(hostname, mapName, numPlayers, maxPlayers)
	buf.Write(serverInfo)

	buf.Write([]byte{0x00, 0x00, 0x01})

	buf.WriteString("player_")
	buf.WriteByte(0x00)
	for _, p := range players {
		buf.WriteString(p)
		buf.WriteByte(0x00)
	}
	buf.WriteByte(0x00)

	return buf.Bytes()
}

func buildKeyValuePairs(pairs map[string]string) []byte {
	var buf bytes.Buffer

	for k, v := range pairs {
		buf.WriteString(k)
		buf.WriteByte(0x00)
		buf.WriteString(v)
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

func intToString(n int) string {
	return strconv.Itoa(n)
}
