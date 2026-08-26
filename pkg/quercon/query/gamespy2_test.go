package query

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGameSpy2Response(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
		wantError  string
	}{
		{
			name:  "valid_response_with_server_info",
			input: buildGameSpy2Response("Test Server", "de_dust2", 10, 24, nil),
			wantResult: &Result{
				Name:          "Test Server",
				Map:           "de_dust2",
				PlayersNum:    10,
				MaxPlayersNum: 24,
			},
		},
		{
			name:  "response_with_players",
			input: buildGameSpy2Response("BF1942 Server", "berlin", 5, 64, []string{"Player1", "Player2"}),
			wantResult: &Result{
				Name:          "BF1942 Server",
				Map:           "berlin",
				PlayersNum:    5,
				MaxPlayersNum: 64,
				Players: []ResultPlayer{
					{Name: "Player1", Score: 0},
					{Name: "Player2", Score: 0},
				},
			},
		},
		{
			name:      "response_too_short",
			input:     []byte{0x00, 0x10, 0x20},
			wantError: "response too short",
		},
		{
			name:      "invalid_response_type",
			input:     []byte{0x01, 0x10, 0x20, 0x30, 0x40},
			wantError: "invalid response type",
		},
		{
			name:  "empty_server_vars",
			input: []byte{0x00, 0x10, 0x20, 0x30, 0x40},
			wantResult: &Result{
				Name:          "",
				Map:           "",
				PlayersNum:    0,
				MaxPlayersNum: 0,
			},
		},
		{
			name:  "alternative_hostname_key",
			input: buildGameSpy2ResponseWithKeys(map[string]string{"servername": "Alt Server", "mapname": "map1", "numplayers": "3", "maxplayers": "16"}),
			wantResult: &Result{
				Name:          "Alt Server",
				Map:           "map1",
				PlayersNum:    3,
				MaxPlayersNum: 16,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseGameSpy2Response(tt.input, result)

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

func TestParseGameSpy2Players(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
	}{
		{
			name:  "multiple_players_with_scores",
			input: buildGameSpy2PlayerData([]string{"Alice", "Bob", "Charlie"}, []int{100, 50, 75}),
			wantResult: &Result{
				PlayersNum: 3,
				Players: []ResultPlayer{
					{Name: "Alice", Score: 100},
					{Name: "Bob", Score: 50},
					{Name: "Charlie", Score: 75},
				},
			},
		},
		{
			name:  "players_without_scores",
			input: buildGameSpy2PlayerDataNamesOnly([]string{"Player1", "Player2"}),
			wantResult: &Result{
				PlayersNum: 2,
				Players: []ResultPlayer{
					{Name: "Player1", Score: 0},
					{Name: "Player2", Score: 0},
				},
			},
		},
		{
			name:       "empty_player_data",
			input:      []byte{},
			wantResult: &Result{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			reader := bytes.NewReader(tt.input)
			parseGameSpy2Players(reader, result)

			if tt.wantResult.Players != nil {
				require.Len(t, result.Players, len(tt.wantResult.Players))
				for i, p := range tt.wantResult.Players {
					assert.Equal(t, p.Name, result.Players[i].Name)
					assert.Equal(t, p.Score, result.Players[i].Score)
				}
				assert.Equal(t, tt.wantResult.PlayersNum, result.PlayersNum)
			} else {
				assert.Empty(t, result.Players)
			}
		})
	}
}

func buildGameSpy2Response(hostname, mapName string, numPlayers, maxPlayers int, players []string) []byte {
	var buf bytes.Buffer

	buf.WriteByte(0x00)
	buf.Write([]byte{0x10, 0x20, 0x30, 0x40})

	buf.WriteString("hostname")
	buf.WriteByte(0x00)
	buf.WriteString(hostname)
	buf.WriteByte(0x00)

	buf.WriteString("mapname")
	buf.WriteByte(0x00)
	buf.WriteString(mapName)
	buf.WriteByte(0x00)

	buf.WriteString("numplayers")
	buf.WriteByte(0x00)
	buf.WriteString(intToString(numPlayers))
	buf.WriteByte(0x00)

	buf.WriteString("maxplayers")
	buf.WriteByte(0x00)
	buf.WriteString(intToString(maxPlayers))
	buf.WriteByte(0x00)

	buf.WriteByte(0x00)

	if len(players) > 0 {
		buf.WriteString("player_")
		buf.WriteByte(0x00)
		for _, p := range players {
			buf.WriteString(p)
			buf.WriteByte(0x00)
		}
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

func buildGameSpy2ResponseWithKeys(pairs map[string]string) []byte {
	var buf bytes.Buffer

	buf.WriteByte(0x00)
	buf.Write([]byte{0x10, 0x20, 0x30, 0x40})

	for k, v := range pairs {
		buf.WriteString(k)
		buf.WriteByte(0x00)
		buf.WriteString(v)
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

func buildGameSpy2PlayerData(names []string, scores []int) []byte {
	var buf bytes.Buffer

	buf.WriteString("player_")
	buf.WriteByte(0x00)
	for _, name := range names {
		buf.WriteString(name)
		buf.WriteByte(0x00)
	}
	buf.WriteByte(0x00)

	buf.WriteString("score_")
	buf.WriteByte(0x00)
	for _, score := range scores {
		buf.WriteString(intToString(score))
		buf.WriteByte(0x00)
	}
	buf.WriteByte(0x00)

	return buf.Bytes()
}

func buildGameSpy2PlayerDataNamesOnly(names []string) []byte {
	var buf bytes.Buffer

	buf.WriteString("player_")
	buf.WriteByte(0x00)
	for _, name := range names {
		buf.WriteString(name)
		buf.WriteByte(0x00)
	}
	buf.WriteByte(0x00)

	return buf.Bytes()
}
