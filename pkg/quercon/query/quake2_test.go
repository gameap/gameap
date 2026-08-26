package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQuake2Response(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []byte
		wantResult *Result
		wantError  string
	}{
		{
			name:  "valid_response_with_players",
			input: buildQuake2Response("\\hostname\\Quake2 Server\\mapname\\q2dm1\\maxclients\\16", "10 50 \"Player1\"\n-5 100 \"Player2\""),
			wantResult: &Result{
				Name:          "Quake2 Server",
				Map:           "q2dm1",
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
			input: buildQuake2Response("\\hostname\\Empty Server\\mapname\\base1\\maxclients\\32", ""),
			wantResult: &Result{
				Name:          "Empty Server",
				Map:           "base1",
				MaxPlayersNum: 32,
				PlayersNum:    0,
				Players:       nil,
			},
		},
		{
			name:  "server_vars_only",
			input: []byte("\xFF\xFF\xFF\xFFprint\n\\hostname\\Minimal Server\\mapname\\test"),
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
			input: []byte("\xFF\xFF\xFF\xFFprint\n"),
			wantResult: &Result{
				Name:          "",
				Map:           "",
				PlayersNum:    0,
				MaxPlayersNum: 0,
			},
		},
		{
			name:  "soldier_of_fortune_style",
			input: buildQuake2Response("\\hostname\\SOF Server\\mapname\\sof_dm1\\sv_maxclients\\24", "25 15 \"Soldier\""),
			wantResult: &Result{
				Name:          "SOF Server",
				Map:           "sof_dm1",
				PlayersNum:    1,
				MaxPlayersNum: 24,
				Players: []ResultPlayer{
					{Name: "Soldier", Score: 25},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}
			err := parseQuake2Response(tt.input, result)

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

func buildQuake2Response(serverVars, players string) []byte {
	response := "\xFF\xFF\xFF\xFFprint\n" + serverVars
	if players != "" {
		response += "\n" + players
	}

	return []byte(response)
}
