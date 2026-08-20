package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQuake3Response(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
		wantError  string
	}{
		{
			name:  "valid_response_with_players",
			input: buildQuake3Response("\\sv_hostname\\Test Server\\mapname\\dm6\\sv_maxclients\\16", "10 50 \"Player1\"\n-5 100 \"Player2\""),
			wantResult: &Result{
				Name:          "Test Server",
				Map:           "dm6",
				PlayersNum:    2,
				MaxPlayersNum: 16,
				Players: []ResultPlayer{
					{Name: "Player1", Score: 10},
					{Name: "Player2", Score: -5},
				},
			},
		},
		{
			name:  "valid_response_no_players",
			input: buildQuake3Response("\\hostname\\Empty Server\\mapname\\q3dm1\\maxclients\\32", ""),
			wantResult: &Result{
				Name:          "Empty Server",
				Map:           "q3dm1",
				MaxPlayersNum: 32,
				PlayersNum:    0,
				Players:       nil,
			},
		},
		{
			name:  "server_vars_only",
			input: []byte("\xFF\xFF\xFF\xFFstatusResponse\n\\sv_hostname\\Minimal Server\\mapname\\test"),
			wantResult: &Result{
				Name: "Minimal Server",
				Map:  "test",
			},
		},
		{
			name:      "response_too_short",
			input:     []byte("\xFF\xFF\xFF\xFF"),
			wantError: "response too short",
		},
		{
			name:      "invalid_header",
			input:     []byte("\x00\x00\x00\x00invalidResponse\n\\key\\value"),
			wantError: "invalid response header",
		},
		{
			name:  "empty_server_vars",
			input: []byte("\xFF\xFF\xFF\xFFstatusResponse\n"),
			wantResult: &Result{
				Name:          "",
				Map:           "",
				PlayersNum:    0,
				MaxPlayersNum: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseQuake3Response(tt.input, result)

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
					assert.Equal(t, p.Score, result.Players[i].Score)
				}
			}
		})
	}
}

func TestParseQuake3ServerVars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
	}{
		{
			name:  "all_fields",
			input: []byte("\\sv_hostname\\My Server\\mapname\\ctf1\\sv_maxclients\\24"),
			wantResult: &Result{
				Name:          "My Server",
				Map:           "ctf1",
				MaxPlayersNum: 24,
			},
		},
		{
			name:  "alternative_keys",
			input: []byte("\\hostname\\Alt Server\\mapname\\dm1\\maxclients\\8"),
			wantResult: &Result{
				Name:          "Alt Server",
				Map:           "dm1",
				MaxPlayersNum: 8,
			},
		},
		{
			name:  "case_insensitive_keys",
			input: []byte("\\SV_HOSTNAME\\Upper Server\\MAPNAME\\upper_map\\SV_MAXCLIENTS\\10"),
			wantResult: &Result{
				Name:          "Upper Server",
				Map:           "upper_map",
				MaxPlayersNum: 10,
			},
		},
		{
			name:  "empty_input",
			input: []byte(""),
			wantResult: &Result{
				Name:          "",
				Map:           "",
				MaxPlayersNum: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			parseQuake3ServerVars(tt.input, result)

			assert.Equal(t, tt.wantResult.Name, result.Name)
			assert.Equal(t, tt.wantResult.Map, result.Map)
			assert.Equal(t, tt.wantResult.MaxPlayersNum, result.MaxPlayersNum)
		})
	}
}

func TestParseQuake3Players(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      [][]byte
		wantResult *Result
	}{
		{
			name: "multiple_players",
			input: [][]byte{
				[]byte("100 20 \"ProPlayer\""),
				[]byte("50 30 \"Noob\""),
				[]byte("-10 999 \"Camper\""),
			},
			wantResult: &Result{
				PlayersNum: 3,
				Players: []ResultPlayer{
					{Name: "ProPlayer", Score: 100},
					{Name: "Noob", Score: 50},
					{Name: "Camper", Score: -10},
				},
			},
		},
		{
			name:  "single_player",
			input: [][]byte{[]byte("0 50 \"Solo\"")},
			wantResult: &Result{
				PlayersNum: 1,
				Players:    []ResultPlayer{{Name: "Solo", Score: 0}},
			},
		},
		{
			name:  "no_players",
			input: [][]byte{},
			wantResult: &Result{
				PlayersNum: 0,
				Players:    []ResultPlayer{},
			},
		},
		{
			name:  "empty_lines_filtered",
			input: [][]byte{[]byte(""), []byte("25 10 \"ValidPlayer\""), []byte("")},
			wantResult: &Result{
				PlayersNum: 1,
				Players:    []ResultPlayer{{Name: "ValidPlayer", Score: 25}},
			},
		},
		{
			name:  "invalid_format_skipped",
			input: [][]byte{[]byte("invalid line"), []byte("30 20 \"Valid\"")},
			wantResult: &Result{
				PlayersNum: 1,
				Players:    []ResultPlayer{{Name: "Valid", Score: 30}},
			},
		},
		{
			name:  "player_with_spaces_in_name",
			input: [][]byte{[]byte("15 25 \"Player With Spaces\"")},
			wantResult: &Result{
				PlayersNum: 1,
				Players:    []ResultPlayer{{Name: "Player With Spaces", Score: 15}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			parseQuake3Players(tt.input, result)

			assert.Equal(t, tt.wantResult.PlayersNum, result.PlayersNum)

			require.Len(t, result.Players, len(tt.wantResult.Players))
			for i, p := range tt.wantResult.Players {
				assert.Equal(t, p.Name, result.Players[i].Name)
				assert.Equal(t, p.Score, result.Players[i].Score)
			}
		})
	}
}

func buildQuake3Response(serverVars, players string) []byte {
	response := "\xFF\xFF\xFF\xFFstatusResponse\n" + serverVars
	if players != "" {
		response += "\n" + players
	}

	return []byte(response)
}
