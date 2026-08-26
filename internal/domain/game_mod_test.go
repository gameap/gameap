package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameModFastRconList_Scan(t *testing.T) {
	t.Parallel()

	prefilled := GameModFastRconList{
		{Info: "preexisting", Command: "preexisting_cmd"},
	}

	tests := []struct {
		name     string
		receiver GameModFastRconList
		input    any
		expected GameModFastRconList
		wantErr  bool
	}{
		{
			name:     "nil_value_overrides_receiver_to_nil",
			receiver: prefilled,
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty_array",
			receiver: nil,
			input:    []byte("[]"),
			expected: GameModFastRconList{},
			wantErr:  false,
		},
		{
			name:     "empty_array_overwrites_prefilled",
			receiver: prefilled,
			input:    []byte("[]"),
			expected: GameModFastRconList{},
			wantErr:  false,
		},
		{
			name:     "valid_single_item",
			receiver: nil,
			input:    []byte(`[{"info":"Status","command":"status"}]`),
			expected: GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			wantErr: false,
		},
		{
			name:     "valid_single_item_overwrites_prefilled",
			receiver: prefilled,
			input:    []byte(`[{"info":"Status","command":"status"}]`),
			expected: GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			wantErr: false,
		},
		{
			name:     "valid_multiple_items",
			receiver: nil,
			input: []byte(`[
				{"info":"Status","command":"status"},
				{"info":"Players","command":"players"}
			]`),
			expected: GameModFastRconList{
				{Info: "Status", Command: "status"},
				{Info: "Players", Command: "players"},
			},
			wantErr: false,
		},
		{
			name:     "json_string_value_from_sqlite_driver",
			receiver: nil,
			input:    `[{"info":"Status","command":"status"}]`,
			expected: GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			wantErr: false,
		},
		{
			name:     "empty_string_resets_receiver",
			receiver: prefilled,
			input:    "",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "unsupported_value_type_resets_receiver",
			receiver: prefilled,
			input:    42,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "invalid_json",
			receiver: nil,
			input:    []byte(`{invalid json`),
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid_json_string",
			receiver: nil,
			input:    "string value",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			result := test.receiver

			// ACT
			err := result.Scan(test.input)

			// ASSERT
			if test.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expected, result, "fast rcon list mismatch")
		})
	}
}

func TestGameModFastRconList_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    GameModFastRconList
		expected driver.Value
		wantErr  bool
	}{
		{
			name:     "nil_list",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty_list",
			input:    GameModFastRconList{},
			expected: []byte("[]"),
			wantErr:  false,
		},
		{
			name: "single_item",
			input: GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			expected: []byte(`[{"info":"Status","command":"status"}]`),
			wantErr:  false,
		},
		{
			name: "multiple_items",
			input: GameModFastRconList{
				{Info: "Status", Command: "status"},
				{Info: "Players", Command: "players"},
			},
			expected: []byte(`[{"info":"Status","command":"status"},{"info":"Players","command":"players"}]`),
			wantErr:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.input.Value()

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if test.expected == nil {
					assert.Nil(t, result)
				} else {
					assert.JSONEq(t, string(test.expected.([]byte)), string(result.([]byte)))
				}
			}
		})
	}
}

func TestGameModVarDefault_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    GameModVarDefault
		expected string
	}{
		{
			name:     "empty_string",
			input:    GameModVarDefault(""),
			expected: `""`,
		},
		{
			name:     "simple_string",
			input:    GameModVarDefault("default_value"),
			expected: `"default_value"`,
		},
		{
			name:     "numeric_string",
			input:    GameModVarDefault("123"),
			expected: `"123"`,
		},
		{
			name:     "string_with_spaces",
			input:    GameModVarDefault("default value"),
			expected: `"default value"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := json.Marshal(test.input)
			require.NoError(t, err)
			assert.JSONEq(t, test.expected, string(result))
		})
	}
}

func TestGameModVarDefault_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected GameModVarDefault
	}{
		{
			name:     "string_value",
			input:    `"test_value"`,
			expected: GameModVarDefault("test_value"),
		},
		{
			name:     "empty_string",
			input:    `""`,
			expected: GameModVarDefault(""),
		},
		{
			name:     "numeric_string",
			input:    `"456"`,
			expected: GameModVarDefault("456"),
		},
		{
			name:     "integer_number",
			input:    `42`,
			expected: GameModVarDefault("42"),
		},
		{
			name:     "zero_number",
			input:    `0`,
			expected: GameModVarDefault("0"),
		},
		{
			name:     "large_number",
			input:    `65`,
			expected: GameModVarDefault("65"),
		},
		{
			name:     "negative_number",
			input:    `-1`,
			expected: GameModVarDefault("-1"),
		},
		{
			name:     "negative_large_number",
			input:    `-1000`,
			expected: GameModVarDefault("-1000"),
		},
		{
			name:     "number_beyond_rune_range",
			input:    `1114112`,
			expected: GameModVarDefault("1114112"),
		},
		{
			name:     "fractional_number",
			input:    `1.5`,
			expected: GameModVarDefault("1.5"),
		},
		{
			name:     "null_becomes_empty_string",
			input:    `null`,
			expected: GameModVarDefault(""),
		},
		{
			name:     "boolean_true",
			input:    `true`,
			expected: GameModVarDefault("true"),
		},
		{
			name:     "boolean_false",
			input:    `false`,
			expected: GameModVarDefault("false"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var result GameModVarDefault

			// ACT
			err := json.Unmarshal([]byte(test.input), &result)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, test.expected, result, "unmarshal result mismatch")
		})
	}
}

func TestGameModVarList_Scan(t *testing.T) {
	t.Parallel()

	prefilled := GameModVarList{
		{Var: "preexisting_var", Default: "preexisting", Info: "preexisting"},
	}

	tests := []struct {
		name     string
		receiver GameModVarList
		input    any
		expected GameModVarList
		wantErr  bool
	}{
		{
			name:     "nil_value_overrides_receiver_to_nil",
			receiver: prefilled,
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty_array",
			receiver: nil,
			input:    []byte("[]"),
			expected: GameModVarList{},
			wantErr:  false,
		},
		{
			name:     "empty_array_overwrites_prefilled",
			receiver: prefilled,
			input:    []byte("[]"),
			expected: GameModVarList{},
			wantErr:  false,
		},
		{
			name:     "valid_single_var",
			receiver: nil,
			input:    []byte(`[{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true}]`),
			expected: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
			},
			wantErr: false,
		},
		{
			name:     "valid_single_var_overwrites_prefilled",
			receiver: prefilled,
			input:    []byte(`[{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true}]`),
			expected: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
			},
			wantErr: false,
		},
		{
			name:     "valid_multiple_vars",
			receiver: nil,
			input: []byte(`[
				{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true},
				{"var":"hostname","default":"My Server","info":"Server name","admin_var":false}
			]`),
			expected: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
				{Var: "hostname", Default: "My Server", Info: "Server name", AdminVar: false},
			},
			wantErr: false,
		},
		{
			name:     "single_object_not_array",
			receiver: nil,
			input:    []byte(`{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true}`),
			expected: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
			},
			wantErr: false,
		},
		{
			name:     "json_string_value_from_sqlite_driver",
			receiver: nil,
			input:    `[{"var":"maxplayers","default":"32","info":"Max players"}]`,
			expected: GameModVarList{
				{Var: "maxplayers", Default: "32", Info: "Max players"},
			},
			wantErr: false,
		},
		{
			name:     "empty_string_resets_receiver",
			receiver: prefilled,
			input:    "",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "unsupported_value_type_resets_receiver",
			receiver: prefilled,
			input:    42,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "invalid_json_both_attempts",
			receiver: nil,
			input:    []byte(`{invalid json`),
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid_json_string",
			receiver: nil,
			input:    "string value",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			result := test.receiver

			// ACT
			err := result.Scan(test.input)

			// ASSERT
			if test.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expected, result, "var list mismatch")
		})
	}
}

func TestGameModVarList_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    GameModVarList
		expected driver.Value
		wantErr  bool
	}{
		{
			name:     "nil_list",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty_list",
			input:    GameModVarList{},
			expected: []byte("[]"),
			wantErr:  false,
		},
		{
			name: "single_var",
			input: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
			},
			expected: []byte(`[{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true}]`),
			wantErr:  false,
		},
		{
			name: "multiple_vars",
			input: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
				{Var: "hostname", Default: "My Server", Info: "Server name", AdminVar: false},
			},
			expected: []byte(`[{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true},{"var":"hostname","default":"My Server","info":"Server name","admin_var":false}]`),
			wantErr:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.input.Value()

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if test.expected == nil {
					assert.Nil(t, result)
				} else {
					assert.JSONEq(t, string(test.expected.([]byte)), string(result.([]byte)))
				}
			}
		})
	}
}

func TestGameModFastRconList_ScanAndValue_RoundTrip(t *testing.T) {
	t.Parallel()

	original := GameModFastRconList{
		{Info: "Status", Command: "status"},
		{Info: "Players", Command: "players"},
		{Info: "Maps", Command: "maps *"},
	}

	value, err := original.Value()
	require.NoError(t, err)

	var result GameModFastRconList
	err = result.Scan(value)
	require.NoError(t, err)

	assert.Equal(t, original, result)
}

func TestGameModVarList_ScanAndValue_RoundTrip(t *testing.T) {
	t.Parallel()

	original := GameModVarList{
		{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
		{Var: "hostname", Default: "My Server", Info: "Server name", AdminVar: false},
		{Var: "mp_timelimit", Default: "30", Info: "Time limit", AdminVar: true},
	}

	value, err := original.Value()
	require.NoError(t, err)

	var result GameModVarList
	err = result.Scan(value)
	require.NoError(t, err)

	assert.Equal(t, original, result)
}

func TestGameModVarDefault_MarshalUnmarshal_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input GameModVarDefault
	}{
		{
			name:  "simple_string",
			input: GameModVarDefault("test_value"),
		},
		{
			name:  "empty_string",
			input: GameModVarDefault(""),
		},
		{
			name:  "numeric_string",
			input: GameModVarDefault("12345"),
		},
		{
			name:  "string_with_special_chars",
			input: GameModVarDefault("test-value_123"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			marshaled, err := json.Marshal(test.input)
			require.NoError(t, err)

			var result GameModVarDefault
			err = json.Unmarshal(marshaled, &result)
			require.NoError(t, err)

			assert.Equal(t, test.input, result)
		})
	}
}

func TestGameMod_Merge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     *GameMod
		other    *GameMod
		expected *GameMod
	}{
		{
			name: "merge_all_nil_fields_with_values",
			base: &GameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Counter-Strike: GO",
			},
			other: &GameMod{
				RemoteRepositoryLinux:   new("linux-repo"),
				RemoteRepositoryWindows: new("windows-repo"),
				StartCmdLinux:           new("./start.sh"),
				StartCmdWindows:         new("start.bat"),
				KickCmd:                 new("kick {player}"),
				BanCmd:                  new("ban {player}"),
				ChnameCmd:               new("name {name}"),
				SrestartCmd:             new("restart"),
				ChmapCmd:                new("changelevel {map}"),
				SendmsgCmd:              new("say {message}"),
				PasswdCmd:               new("password {pass}"),
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
				},
				Vars: GameModVarList{
					{Var: "sv_cheats", Default: "0", Info: "Cheats", AdminVar: true},
				},
			},
			expected: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("linux-repo"),
				RemoteRepositoryWindows: new("windows-repo"),
				StartCmdLinux:           new("./start.sh"),
				StartCmdWindows:         new("start.bat"),
				KickCmd:                 new("kick {player}"),
				BanCmd:                  new("ban {player}"),
				ChnameCmd:               new("name {name}"),
				SrestartCmd:             new("restart"),
				ChmapCmd:                new("changelevel {map}"),
				SendmsgCmd:              new("say {message}"),
				PasswdCmd:               new("password {pass}"),
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
				},
				Vars: GameModVarList{
					{Var: "sv_cheats", Default: "0", Info: "Cheats", AdminVar: true},
				},
			},
		},
		{
			name: "override_existing_values",
			base: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("old-linux-repo"),
				RemoteRepositoryWindows: new("old-windows-repo"),
				StartCmdLinux:           new("./old-start.sh"),
				StartCmdWindows:         new("old-start.bat"),
				KickCmd:                 new("old kick"),
				FastRcon: GameModFastRconList{
					{Info: "Old Status", Command: "old status"},
				},
				Vars: GameModVarList{
					{Var: "old_var", Default: "old", Info: "Old", AdminVar: false},
				},
			},
			other: &GameMod{
				RemoteRepositoryLinux:   new("new-linux-repo"),
				RemoteRepositoryWindows: new("new-windows-repo"),
				StartCmdLinux:           new("./new-start.sh"),
				StartCmdWindows:         new("new-start.bat"),
				KickCmd:                 new("new kick"),
				FastRcon: GameModFastRconList{
					{Info: "New Status", Command: "new status"},
				},
				Vars: GameModVarList{
					{Var: "new_var", Default: "new", Info: "New", AdminVar: true},
				},
			},
			expected: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("new-linux-repo"),
				RemoteRepositoryWindows: new("new-windows-repo"),
				StartCmdLinux:           new("./new-start.sh"),
				StartCmdWindows:         new("new-start.bat"),
				KickCmd:                 new("new kick"),
				FastRcon: GameModFastRconList{
					{Info: "New Status", Command: "new status"},
					{Info: "Old Status", Command: "old status"},
				},
				Vars: GameModVarList{
					{Var: "new_var", Default: "new", Info: "New", AdminVar: true},
					{Var: "old_var", Default: "old", Info: "Old", AdminVar: false},
				},
			},
		},
		{
			name: "nil_fields_in_other_do_not_override",
			base: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("existing-linux-repo"),
				RemoteRepositoryWindows: new("existing-windows-repo"),
				StartCmdLinux:           new("./existing-start.sh"),
				StartCmdWindows:         new("existing-start.bat"),
				KickCmd:                 new("existing kick"),
				BanCmd:                  new("existing ban"),
				ChnameCmd:               new("existing chname"),
				SrestartCmd:             new("existing restart"),
				ChmapCmd:                new("existing chmap"),
				SendmsgCmd:              new("existing sendmsg"),
				PasswdCmd:               new("existing passwd"),
			},
			other: &GameMod{
				FastRcon: GameModFastRconList{
					{Info: "New Status", Command: "new status"},
				},
				Vars: GameModVarList{
					{Var: "new_var", Default: "new", Info: "New", AdminVar: true},
				},
			},
			expected: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("existing-linux-repo"),
				RemoteRepositoryWindows: new("existing-windows-repo"),
				StartCmdLinux:           new("./existing-start.sh"),
				StartCmdWindows:         new("existing-start.bat"),
				KickCmd:                 new("existing kick"),
				BanCmd:                  new("existing ban"),
				ChnameCmd:               new("existing chname"),
				SrestartCmd:             new("existing restart"),
				ChmapCmd:                new("existing chmap"),
				SendmsgCmd:              new("existing sendmsg"),
				PasswdCmd:               new("existing passwd"),
				FastRcon: GameModFastRconList{
					{Info: "New Status", Command: "new status"},
				},
				Vars: GameModVarList{
					{Var: "new_var", Default: "new", Info: "New", AdminVar: true},
				},
			},
		},
		{
			name: "partial_override",
			base: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("existing-linux-repo"),
				RemoteRepositoryWindows: new("existing-windows-repo"),
				StartCmdLinux:           new("./existing-start.sh"),
				KickCmd:                 new("existing kick"),
				BanCmd:                  new("existing ban"),
			},
			other: &GameMod{
				RemoteRepositoryWindows: new("new-windows-repo"),
				StartCmdWindows:         new("new-start.bat"),
				ChnameCmd:               new("new chname"),
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
				},
				Vars: GameModVarList{},
			},
			expected: &GameMod{
				ID:                      1,
				GameCode:                "csgo",
				Name:                    "Counter-Strike: GO",
				RemoteRepositoryLinux:   new("existing-linux-repo"),
				RemoteRepositoryWindows: new("new-windows-repo"),
				StartCmdLinux:           new("./existing-start.sh"),
				StartCmdWindows:         new("new-start.bat"),
				KickCmd:                 new("existing kick"),
				BanCmd:                  new("existing ban"),
				ChnameCmd:               new("new chname"),
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
				},
				Vars: nil,
			},
		},
		{
			name: "empty_other",
			base: &GameMod{
				ID:                    1,
				GameCode:              "csgo",
				Name:                  "Counter-Strike: GO",
				RemoteRepositoryLinux: new("existing-linux-repo"),
				FastRcon: GameModFastRconList{
					{Info: "Old Status", Command: "old status"},
				},
				Vars: GameModVarList{
					{Var: "old_var", Default: "old", Info: "Old", AdminVar: false},
				},
			},
			other: &GameMod{},
			expected: &GameMod{
				ID:                    1,
				GameCode:              "csgo",
				Name:                  "Counter-Strike: GO",
				RemoteRepositoryLinux: new("existing-linux-repo"),
				FastRcon: GameModFastRconList{
					{Info: "Old Status", Command: "old status"},
				},
				Vars: GameModVarList{
					{Var: "old_var", Default: "old", Info: "Old", AdminVar: false},
				},
			},
		},
		{
			name: "merge_fast_rcon_and_vars_keeps_local_additions",
			base: &GameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Counter-Strike: GO",
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
					{Info: "Players", Command: "players"},
				},
				Vars: GameModVarList{
					{Var: "sv_cheats", Default: "0", Info: "Cheats", AdminVar: true},
					{Var: "hostname", Default: "Server", Info: "Name", AdminVar: false},
				},
			},
			other: &GameMod{
				FastRcon: GameModFastRconList{
					{Info: "Maps", Command: "maps"},
				},
				Vars: GameModVarList{
					{Var: "mp_timelimit", Default: "30", Info: "Time", AdminVar: true},
				},
			},
			expected: &GameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Counter-Strike: GO",
				FastRcon: GameModFastRconList{
					{Info: "Maps", Command: "maps"},
					{Info: "Status", Command: "status"},
					{Info: "Players", Command: "players"},
				},
				Vars: GameModVarList{
					{Var: "mp_timelimit", Default: "30", Info: "Time", AdminVar: true},
					{Var: "sv_cheats", Default: "0", Info: "Cheats", AdminVar: true},
					{Var: "hostname", Default: "Server", Info: "Name", AdminVar: false},
				},
			},
		},
		{
			name: "merge_all_commands",
			base: &GameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Counter-Strike: GO",
			},
			other: &GameMod{
				KickCmd:     new("kick_cmd"),
				BanCmd:      new("ban_cmd"),
				ChnameCmd:   new("chname_cmd"),
				SrestartCmd: new("srestart_cmd"),
				ChmapCmd:    new("chmap_cmd"),
				SendmsgCmd:  new("sendmsg_cmd"),
				PasswdCmd:   new("passwd_cmd"),
				FastRcon:    GameModFastRconList{},
				Vars:        GameModVarList{},
			},
			expected: &GameMod{
				ID:          1,
				GameCode:    "csgo",
				Name:        "Counter-Strike: GO",
				KickCmd:     new("kick_cmd"),
				BanCmd:      new("ban_cmd"),
				ChnameCmd:   new("chname_cmd"),
				SrestartCmd: new("srestart_cmd"),
				ChmapCmd:    new("chmap_cmd"),
				SendmsgCmd:  new("sendmsg_cmd"),
				PasswdCmd:   new("passwd_cmd"),
				FastRcon:    nil,
				Vars:        nil,
			},
		},
		{
			name: "merge_replaces_the_catalog_variable_of_the_same_name",
			base: &GameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Counter-Strike: GO",
				Vars: GameModVarList{
					{Var: "maxplayers", Default: "16", Info: "Locally tuned", AdminVar: false},
					{Var: "custom", Default: "x", Info: "Added by an admin", AdminVar: false},
				},
			},
			other: &GameMod{
				Vars: GameModVarList{
					{
						Var: "maxplayers", Default: "32", Info: "Max players", AdminVar: true,
						Type: GameModVarTypeInt, Rules: &GameModVarRules{Max: new(64.0)},
					},
				},
			},
			expected: &GameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Counter-Strike: GO",
				Vars: GameModVarList{
					{
						Var: "maxplayers", Default: "32", Info: "Max players", AdminVar: true,
						Type: GameModVarTypeInt, Rules: &GameModVarRules{Max: new(64.0)},
					},
					{Var: "custom", Default: "x", Info: "Added by an admin", AdminVar: false},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.base.Merge(test.other)
			assert.Equal(t, test.expected, test.base)
		})
	}
}
