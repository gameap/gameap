package flexible

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUint_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected uint
	}{
		{
			name:     "integer_zero",
			jsonStr:  `0`,
			wantErr:  false,
			expected: 0,
		},
		{
			name:     "integer_positive",
			jsonStr:  `42`,
			wantErr:  false,
			expected: 42,
		},
		{
			name:     "integer_large",
			jsonStr:  `999999`,
			wantErr:  false,
			expected: 999999,
		},
		{
			name:     "string_zero",
			jsonStr:  `"0"`,
			wantErr:  false,
			expected: 0,
		},
		{
			name:     "string_positive",
			jsonStr:  `"42"`,
			wantErr:  false,
			expected: 42,
		},
		{
			name:     "string_large",
			jsonStr:  `"999999"`,
			wantErr:  false,
			expected: 999999,
		},
		{
			name:     "empty_string",
			jsonStr:  `""`,
			wantErr:  false,
			expected: 0,
		},
		{
			name:     "float_zero",
			jsonStr:  `0.0`,
			wantErr:  false,
			expected: 0,
		},
		{
			name:     "float_positive",
			jsonStr:  `42.5`,
			wantErr:  false,
			expected: 42,
		},
		{
			name:     "null",
			jsonStr:  `null`,
			wantErr:  false,
			expected: 0,
		},
		{
			name:    "negative_integer",
			jsonStr: `-1`,
			wantErr: true,
		},
		{
			name:    "string_negative",
			jsonStr: `"-42"`,
			wantErr: true,
		},
		{
			name:    "string_invalid",
			jsonStr: `"abc"`,
			wantErr: true,
		},
		{
			name:    "string_with_spaces",
			jsonStr: `"  42  "`,
			wantErr: true,
		},
		{
			name:    "string_decimal",
			jsonStr: `"42.5"`,
			wantErr: true,
		},
		{
			name:    "negative_float",
			jsonStr: `-42.5`,
			wantErr: true,
		},
		{
			name:    "invalid_JSON_syntax",
			jsonStr: `{invalid`,
			wantErr: true,
		},
		{
			name:    "array",
			jsonStr: `[1, 2, 3]`,
			wantErr: true,
		},
		{
			name:    "object",
			jsonStr: `{"value": 42}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u Uint
			err := json.Unmarshal([]byte(tt.jsonStr), &u)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, uint(u))
			}
		})
	}
}

func TestUint_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    Uint
		expected string
	}{
		{
			name:     "zero",
			value:    Uint(0),
			expected: `0`,
		},
		{
			name:     "positive",
			value:    Uint(42),
			expected: `42`,
		},
		{
			name:     "large",
			value:    Uint(999999),
			expected: `999999`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestUint_Uint(t *testing.T) {
	zeroVal := Uint(0)
	positiveVal := Uint(42)

	tests := []struct {
		name     string
		value    *Uint
		expected uint
	}{
		{
			name:     "zero_value",
			value:    &zeroVal,
			expected: 0,
		},
		{
			name:     "positive_value",
			value:    &positiveVal,
			expected: 42,
		},
		{
			name:     "nil_pointer",
			value:    nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.value.Uint()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUint_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  uint
	}{
		{
			name:  "round_trip_zero",
			input: `0`,
			want:  0,
		},
		{
			name:  "round_trip_positive",
			input: `42`,
			want:  42,
		},
		{
			name:  "round_trip_string",
			input: `"123"`,
			want:  123,
		},
		{
			name:  "round_trip_float",
			input: `99.8`,
			want:  99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u Uint
			err := json.Unmarshal([]byte(tt.input), &u)
			require.NoError(t, err)

			data, err := json.Marshal(u)
			require.NoError(t, err)

			var u2 Uint
			err = json.Unmarshal(data, &u2)
			require.NoError(t, err)

			assert.Equal(t, tt.want, uint(u2))
		})
	}
}

func TestUint_StructMarshaling(t *testing.T) {
	type TestStruct struct {
		SteamAppIDLinux   *Uint `json:"steam_app_id_linux,omitempty"`
		SteamAppIDWindows *Uint `json:"steam_app_id_windows,omitempty"`
		Count             Uint  `json:"count"`
	}

	t.Run("marshal_struct_with_flexible_uint", func(t *testing.T) {
		linux := Uint(730)
		windows := Uint(440)
		ts := TestStruct{
			SteamAppIDLinux:   &linux,
			SteamAppIDWindows: &windows,
			Count:             Uint(5),
		}

		data, err := json.Marshal(ts)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"steam_app_id_linux":730`)
		assert.Contains(t, string(data), `"steam_app_id_windows":440`)
		assert.Contains(t, string(data), `"count":5`)
	})

	t.Run("unmarshal_struct_with_flexible_uint_mixed_formats", func(t *testing.T) {
		jsonData := `{
			"steam_app_id_linux": "730",
			"steam_app_id_windows": 440,
			"count": 5
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Equal(t, uint(730), ts.SteamAppIDLinux.Uint())
		assert.Equal(t, uint(440), ts.SteamAppIDWindows.Uint())
		assert.Equal(t, uint(5), uint(ts.Count))
	})

	t.Run("unmarshal_struct_with_all_string_formats", func(t *testing.T) {
		jsonData := `{
			"steam_app_id_linux": "730",
			"steam_app_id_windows": "440",
			"count": "5"
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Equal(t, uint(730), ts.SteamAppIDLinux.Uint())
		assert.Equal(t, uint(440), ts.SteamAppIDWindows.Uint())
		assert.Equal(t, uint(5), uint(ts.Count))
	})

	t.Run("unmarshal_struct_with_omitted_fields", func(t *testing.T) {
		jsonData := `{
			"count": 10
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Nil(t, ts.SteamAppIDLinux)
		assert.Nil(t, ts.SteamAppIDWindows)
		assert.Equal(t, uint(10), uint(ts.Count))
	})
}

func TestUint_EdgeCases(t *testing.T) {
	t.Run("multiple_unmarshal_same_variable", func(t *testing.T) {
		var u Uint

		err := json.Unmarshal([]byte(`42`), &u)
		require.NoError(t, err)
		assert.Equal(t, uint(42), uint(u))

		err = json.Unmarshal([]byte(`"100"`), &u)
		require.NoError(t, err)
		assert.Equal(t, uint(100), uint(u))

		err = json.Unmarshal([]byte(`0`), &u)
		require.NoError(t, err)
		assert.Equal(t, uint(0), uint(u))
	})

	t.Run("pointer_to_uint", func(t *testing.T) {
		u := Uint(42)
		ptr := &u

		data, err := json.Marshal(ptr)
		require.NoError(t, err)
		assert.Equal(t, `42`, string(data))

		var u2 Uint
		err = json.Unmarshal(data, &u2)
		require.NoError(t, err)
		assert.Equal(t, uint(42), uint(u2))
	})

	t.Run("array_of_uints", func(t *testing.T) {
		type UintArray struct {
			Values []Uint `json:"values"`
		}

		jsonData := `{"values": [1, "2", 3.0, "100", 0]}`
		var ua UintArray
		err := json.Unmarshal([]byte(jsonData), &ua)
		require.NoError(t, err)
		require.Len(t, ua.Values, 5)
		assert.Equal(t, uint(1), uint(ua.Values[0]))
		assert.Equal(t, uint(2), uint(ua.Values[1]))
		assert.Equal(t, uint(3), uint(ua.Values[2]))
		assert.Equal(t, uint(100), uint(ua.Values[3]))
		assert.Equal(t, uint(0), uint(ua.Values[4]))
	})
}

func Test_anyToUint(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected uint
		wantErr  bool
	}{
		{name: "string_zero", input: "0", expected: 0, wantErr: false},
		{name: "string_positive", input: "42", expected: 42, wantErr: false},
		{name: "string_large", input: "999999", expected: 999999, wantErr: false},
		{name: "string_empty", input: "", expected: 0, wantErr: false},
		{name: "string_negative", input: "-1", expected: 0, wantErr: true},
		{name: "string_invalid", input: "abc", expected: 0, wantErr: true},
		{name: "string_decimal", input: "42.5", expected: 0, wantErr: true},
		{name: "float64_zero", input: float64(0), expected: 0, wantErr: false},
		{name: "float64_positive", input: float64(42.8), expected: 42, wantErr: false},
		{name: "float64_negative", input: float64(-1), expected: 0, wantErr: true},
		{name: "int_zero", input: int(0), expected: 0, wantErr: false},
		{name: "int_positive", input: int(42), expected: 42, wantErr: false},
		{name: "int_negative", input: int(-1), expected: 0, wantErr: true},
		{name: "int8_positive", input: int8(42), expected: 42, wantErr: false},
		{name: "int8_negative", input: int8(-1), expected: 0, wantErr: true},
		{name: "int16_positive", input: int16(42), expected: 42, wantErr: false},
		{name: "int16_negative", input: int16(-1), expected: 0, wantErr: true},
		{name: "int32_positive", input: int32(42), expected: 42, wantErr: false},
		{name: "int32_negative", input: int32(-1), expected: 0, wantErr: true},
		{name: "int64_positive", input: int64(42), expected: 42, wantErr: false},
		{name: "int64_negative", input: int64(-1), expected: 0, wantErr: true},
		{name: "uint_zero", input: uint(0), expected: 0, wantErr: false},
		{name: "uint_positive", input: uint(42), expected: 42, wantErr: false},
		{name: "uint8_positive", input: uint8(42), expected: 42, wantErr: false},
		{name: "uint8_max", input: uint8(255), expected: 255, wantErr: false},
		{name: "uint16_positive", input: uint16(42), expected: 42, wantErr: false},
		{name: "uint32_positive", input: uint32(42), expected: 42, wantErr: false},
		{name: "uint64_positive", input: uint64(42), expected: 42, wantErr: false},
		{name: "float32_zero", input: float32(0), expected: 0, wantErr: false},
		{name: "float32_positive", input: float32(42.5), expected: 42, wantErr: false},
		{name: "float32_negative", input: float32(-1), expected: 0, wantErr: true},
		{name: "nil", input: nil, expected: 0, wantErr: false},
		{name: "slice", input: []int{1, 2, 3}, expected: 0, wantErr: true},
		{name: "map", input: map[string]int{"key": 1}, expected: 0, wantErr: true},
		{name: "struct", input: struct{ Value int }{Value: 1}, expected: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := anyToUint(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
