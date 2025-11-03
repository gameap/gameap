package players

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// stat
// hostname:  GunGame 2.4 - HLDM.ORG
// version :  48/1.1.2.2/Stdio 3928 secure
// tcp/ip  :  192.0.2.10:27018
// map     :  bounce at: 0 x, 0 y, 0 z
// players :  5 active (24 max)
//
// #	name     	userid 	uniqueid            	frag	time    	ping	loss	adr
// 1	"Tolyan" 	4664   	STEAM_0:0:100001	300 	29:45   	43  	0   	192.0.2.101:27005
// 2	"Cep>I<aH	4693   	BOT                 	0   	89:13:09	0   	0
// 3	"Kis"    	4660   	STEAM_0:0:100002	0   	33:52   	83  	0   	192.0.2.102:27005
// 7	"jorkata"	4383   	STEAM_0:0:100003 	0   	6 :51:20	48  	0   	192.0.2.103:27005
// 9	"Olivka" 	4692   	STEAM_0:0:100004 	1   	00:47   	56  	0   	192.0.2.104:27005
// 10	"HLTV Pro	4684   	HLTV                	0   	00:05   	3   	0   	192.0.2.105:27120
// 6 users

// status
// hostname:  GunGame 2.4 - HLDM.ORG
// version :  48/1.1.2.2/Stdio 3928 secure  (70)
// tcp/ip  :  192.0.2.10:27018
// map     :  aecthdrl at: 0 x, 0 y, 0 z
// players :  5 active (24 max)
//
// #     name userid uniqueid frag time ping loss adr
// #1 "MeXaHuK" 4713 STEAM_0:1:100011   1 00:32   36    0 192.0.2.111:27005
// #3 "Olivka" 4707 STEAM_0:0:100004   0 02:33   53    0 192.0.2.112:27005
// #4 "KyPCaHT" 4712 BOT   0 89:20:54    0    0
// #7 "jorkata" 4383 STEAM_0:0:100003   0  6:59:06   47    0 192.0.2.103:27005
//#10 "HLTV Proxy" 4706 HLTV hltv:0/128 delay:30 02:53    6    0 192.0.2.105:27120
// 5 users

func TestValvePlayerManager_ParsePlayers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Player
	}{
		{
			name: "full_status_output_with_multiple_players",
			input: `hostname:  GunGame 2.4 - HLDM.ORG
version :  48/1.1.2.2/Stdio 3928 secure  (70)
tcp/ip  :  192.0.2.10:27018
map     :  4pillars_snow at: 0 x, 0 y, 0 z
players :  9 active (24 max)

#      name userid uniqueid frag time ping loss adr
# 1 "Tolyan" 4664 STEAM_0:0:100001 202 07:27   58    0 192.0.2.101:27005
# 2  "PAVEL" 4663 STEAM_0:0:100002 403 09:52   68    0 192.0.2.102:27005
# 3    "Kis" 4660 STEAM_0:0:100003 601 11:35  159    0 192.0.2.103:27005
# 4 "Barslan" 4662 STEAM_0:1:100004 802 10:42   79    0 192.0.2.104:27005
# 5   "GB47" 4648 STEAM_0:0:100005 701 25:49  182    0 192.0.2.105:27005
# 6 "KOT_MATROSKIN" 4669 STEAM_0:0:100006 600 02:06   16    0 192.0.2.106:27005
# 7 "jorkata" 4383 STEAM_0:0:100007   0  6:29:03   44    0 192.0.2.107:27005
# 8 "onaytimur6" 4667 STEAM_0:0:100008 203 04:00   83    0 192.0.2.108:33255
#10 "HLTV Proxy" 4671 HLTV hltv:0/128 delay:30 00:59  101    0 192.0.2.109:27120
9 users`,
			expected: []Player{
				{ID: "4664", Name: "Tolyan", UniqID: "STEAM_0:0:100001", Score: "202", Ping: "58", Addr: "192.0.2.101"},
				{ID: "4663", Name: "PAVEL", UniqID: "STEAM_0:0:100002", Score: "403", Ping: "68", Addr: "192.0.2.102"},
				{ID: "4660", Name: "Kis", UniqID: "STEAM_0:0:100003", Score: "601", Ping: "159", Addr: "192.0.2.103"},
				{ID: "4662", Name: "Barslan", UniqID: "STEAM_0:1:100004", Score: "802", Ping: "79", Addr: "192.0.2.104"},
				{ID: "4648", Name: "GB47", UniqID: "STEAM_0:0:100005", Score: "701", Ping: "182", Addr: "192.0.2.105"},
				{ID: "4669", Name: "KOT_MATROSKIN", UniqID: "STEAM_0:0:100006", Score: "600", Ping: "16", Addr: "192.0.2.106"},
				{ID: "4383", Name: "jorkata", UniqID: "STEAM_0:0:100007", Score: "0", Ping: "44", Addr: "192.0.2.107"},
				{ID: "4667", Name: "onaytimur6", UniqID: "STEAM_0:0:100008", Score: "203", Ping: "83", Addr: "192.0.2.108"},
				{ID: "4671", Name: "HLTV Proxy", UniqID: "HLTV", Score: "", Ping: "101", Addr: "192.0.2.109"},
			},
		},
		{
			name: "single_player",
			input: `hostname:  Test Server
version :  48/1.1.2.2/Stdio 3928 secure
tcp/ip  :  127.0.0.1:27015
map     :  de_dust2
players :  1 active (16 max)

#      name userid uniqueid frag time ping loss adr
# 1 "TestPlayer" 123 STEAM_0:1:12345678 10 05:30   25    0 192.168.1.100:27005`,
			expected: []Player{
				{ID: "123", Name: "TestPlayer", UniqID: "STEAM_0:1:12345678", Score: "10", Ping: "25", Addr: "192.168.1.100"},
			},
		},
		{
			name:  "player_with_spaces_in_name",
			input: `# 1 "Player Name With Spaces" 456 STEAM_0:0:99999999 50 10:15   30    0 10.0.0.1:27005`,
			expected: []Player{
				{ID: "456", Name: "Player Name With Spaces", UniqID: "STEAM_0:0:99999999", Score: "50", Ping: "30", Addr: "10.0.0.1"},
			},
		},
		{
			name:  "player_with_special_characters_in_name",
			input: `# 1 "[TAG] Player$123" 789 STEAM_0:1:11111111 75 15:45   40    0 172.16.0.1:27005`,
			expected: []Player{
				{ID: "789", Name: "[TAG] Player$123", UniqID: "STEAM_0:1:11111111", Score: "75", Ping: "40", Addr: "172.16.0.1"},
			},
		},
		{
			name: "player_without_address",
			input: `#      name userid uniqueid frag time ping loss
# 1 "NoAddress" 111 STEAM_0:0:55555555 20 02:30   50    0`,
			expected: []Player{
				{ID: "111", Name: "NoAddress", UniqID: "STEAM_0:0:55555555", Score: "20", Ping: "50", Addr: ""},
			},
		},
		{
			name:     "empty_input",
			input:    ``,
			expected: []Player{},
		},
		{
			name: "no_player_lines",
			input: `hostname:  Test Server
version :  48/1.1.2.2/Stdio 3928 secure
tcp/ip  :  127.0.0.1:27015
map     :  de_dust2
players :  0 active (16 max)`,
			expected: []Player{},
		},
		{
			name:     "header_line_only",
			input:    `#      name userid uniqueid frag time ping loss adr`,
			expected: []Player{},
		},
		{
			name: "mixed_valid_and_invalid_lines",
			input: `# 1 "ValidPlayer" 123 STEAM_0:1:12345678 10 05:30   25    0 192.168.1.100:27005
not a player line
# invalid line without enough fields
# 2 "AnotherValid" 456 STEAM_0:0:87654321 20 10:00   30    0 192.168.1.101:27005`,
			expected: []Player{
				{ID: "123", Name: "ValidPlayer", UniqID: "STEAM_0:1:12345678", Score: "10", Ping: "25", Addr: "192.168.1.100"},
				{ID: "456", Name: "AnotherValid", UniqID: "STEAM_0:0:87654321", Score: "20", Ping: "30", Addr: "192.168.1.101"},
			},
		},
		{
			name:  "player_with_bot_uniqueid",
			input: `# 1 "BOT Frank" 999 BOT 100 99:59   0    0 loopback`,
			expected: []Player{
				{ID: "999", Name: "BOT Frank", UniqID: "BOT", Score: "100", Ping: "0", Addr: "loopback"},
			},
		},
		{
			name: "players_with_varying_spacing",
			input: `# 1 "Player1" 100 STEAM_0:0:1 10 01:00   10    0 1.1.1.1:27005
#  2  "Player2"  200  STEAM_0:0:2  20  02:00    20     0  2.2.2.2:27005
#3 "Player3" 300 STEAM_0:0:3 30 03:00   30    0 3.3.3.3:27005`,
			expected: []Player{
				{ID: "100", Name: "Player1", UniqID: "STEAM_0:0:1", Score: "10", Ping: "10", Addr: "1.1.1.1"},
				{ID: "200", Name: "Player2", UniqID: "STEAM_0:0:2", Score: "20", Ping: "20", Addr: "2.2.2.2"},
				{ID: "300", Name: "Player3", UniqID: "STEAM_0:0:3", Score: "30", Ping: "30", Addr: "3.3.3.3"},
			},
		},
		{
			name:  "player_with_empty_name",
			input: `# 1 "" 123 STEAM_0:0:12345678 10 05:30   25    0 192.168.1.100:27005`,
			expected: []Player{
				{ID: "123", Name: "", UniqID: "STEAM_0:0:12345678", Score: "10", Ping: "25", Addr: "192.168.1.100"},
			},
		},
		{
			name:  "negative_scores",
			input: `# 1 "BadPlayer" 111 STEAM_0:0:11111111 -50 08:45   35    0 10.0.0.5:27005`,
			expected: []Player{
				{ID: "111", Name: "BadPlayer", UniqID: "STEAM_0:0:11111111", Score: "-50", Ping: "35", Addr: "10.0.0.5"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewValvePlayers()
			result, err := mgr.ParsePlayers(tt.input)

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expected), len(result), "Number of players should match")
			assert.Equal(t, tt.expected, result, "Parsed players should match expected")
		})
	}
}

func TestValvePlayerManager_parsePlayer(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *Player
		expectError bool
		errorType   error
	}{
		{
			name:  "valid_player_line",
			input: `# 1 "TestPlayer" 123 STEAM_0:1:12345678 10 05:30   25    0 192.168.1.100:27005`,
			expected: &Player{
				ID:     "123",
				Name:   "TestPlayer",
				UniqID: "STEAM_0:1:12345678",
				Score:  "10",
				Ping:   "25",
				Addr:   "192.168.1.100",
			},
			expectError: false,
		},
		{
			name:  "valid_player_line_without_hash_prefix",
			input: ` 2 "AnotherPlayer" 456 STEAM_0:0:87654321 20 10:00   30    0 192.168.1.101:27005`,
			expected: &Player{
				ID:     "456",
				Name:   "AnotherPlayer",
				UniqID: "STEAM_0:0:87654321",
				Score:  "20",
				Ping:   "30",
				Addr:   "192.168.1.101",
			},
			expectError: false,
		},
		{
			name:        "empty_line",
			input:       "# ",
			expected:    nil,
			expectError: true,
			errorType:   ErrLineIsEmpty,
		},
		{
			name:        "line_without_quotes",
			input:       "# 1 NoQuotes 123 STEAM_0:1:12345678 10 05:30   25    0 192.168.1.100:27005",
			expected:    nil,
			expectError: true,
			errorType:   ErrNicknameNotFound,
		},
		{
			name:        "line_with_single_quote",
			input:       `# 1 "OnlyOneQuote 123 STEAM_0:1:12345678 10 05:30   25    0 192.168.1.100:27005`,
			expected:    nil,
			expectError: true,
			errorType:   ErrNicknameNotFound,
		},
		{
			name:        "line_with_insufficient_fields",
			input:       `# 1 "Player" 123 STEAM_0:1:12345678`,
			expected:    nil,
			expectError: true,
			errorType:   ErrLineDoesNotMatch,
		},
		{
			name:  "player_without_address_field",
			input: `# 1 "NoAddr" 111 STEAM_0:0:55555555 20 02:30   50    0`,
			expected: &Player{
				ID:     "111",
				Name:   "NoAddr",
				UniqID: "STEAM_0:0:55555555",
				Score:  "20",
				Ping:   "50",
				Addr:   "",
			},
			expectError: false,
		},
		{
			name:  "player_with_complex_name",
			input: `# 5 "[CLAN] Player (AFK)" 999 STEAM_0:1:99999999 150 25:30   45    0 172.16.0.50:27005`,
			expected: &Player{
				ID:     "999",
				Name:   "[CLAN] Player (AFK)",
				UniqID: "STEAM_0:1:99999999",
				Score:  "150",
				Ping:   "45",
				Addr:   "172.16.0.50",
			},
			expectError: false,
		},
		{
			name:  "HLTV_proxy_player",
			input: `#10 "HLTV Proxy" 4671 HLTV hltv:0/128 delay:30 00:59  101    0 192.0.2.109:27120`,
			expected: &Player{
				ID:     "4671",
				Name:   "HLTV Proxy",
				UniqID: "HLTV",
				Score:  "",
				Ping:   "101",
				Addr:   "192.0.2.109",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &ValvePlayerManager{}
			result, err := mgr.parsePlayer(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
