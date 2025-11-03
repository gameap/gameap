package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameModFastRconList_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected GameModFastRconList
		wantErr  bool
	}{
		{
			name:     "nil_value",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty_array",
			input:    []byte("[]"),
			expected: GameModFastRconList{},
			wantErr:  false,
		},
		{
			name:  "valid_single_item",
			input: []byte(`[{"info":"Status","command":"status"}]`),
			expected: GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			wantErr: false,
		},
		{
			name: "valid_multiple_items",
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
			name:     "non_byte_slice_value",
			input:    "string value",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "invalid_json",
			input:    []byte(`{invalid json`),
			expected: nil,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result GameModFastRconList
			err := result.Scan(test.input)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestGameModFastRconList_Value(t *testing.T) {
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
			result, err := json.Marshal(test.input)
			require.NoError(t, err)
			assert.JSONEq(t, test.expected, string(result))
		})
	}
}

func TestGameModVarDefault_UnmarshalJSON(t *testing.T) {
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
			expected: GameModVarDefault("*"),
		},
		{
			name:     "zero_number",
			input:    `0`,
			expected: GameModVarDefault("\x00"),
		},
		{
			name:     "large_number",
			input:    `65`,
			expected: GameModVarDefault("A"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result GameModVarDefault
			err := json.Unmarshal([]byte(test.input), &result)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestGameModVarList_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected GameModVarList
		wantErr  bool
	}{
		{
			name:     "nil_value",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "empty_array",
			input:    []byte("[]"),
			expected: GameModVarList{},
			wantErr:  false,
		},
		{
			name:  "valid_single_var",
			input: []byte(`[{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true}]`),
			expected: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
			},
			wantErr: false,
		},
		{
			name: "valid_multiple_vars",
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
			name:  "single_object_not_array",
			input: []byte(`{"var":"sv_cheats","default":"0","info":"Enable cheats","admin_var":true}`),
			expected: GameModVarList{
				{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
			},
			wantErr: false,
		},
		{
			name:     "non_byte_slice_value",
			input:    "string value",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "invalid_json_both_attempts",
			input:    []byte(`{invalid json`),
			expected: nil,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result GameModVarList
			err := result.Scan(test.input)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestGameModVarList_Value(t *testing.T) {
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
			marshaled, err := json.Marshal(test.input)
			require.NoError(t, err)

			var result GameModVarDefault
			err = json.Unmarshal(marshaled, &result)
			require.NoError(t, err)

			assert.Equal(t, test.input, result)
		})
	}
}
