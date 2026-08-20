package flexible

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBool_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected bool
	}{
		// Boolean values
		{
			name:     "true_boolean",
			jsonStr:  `true`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "false_boolean",
			jsonStr:  `false`,
			wantErr:  false,
			expected: false,
		},
		// String values
		{
			name:     "string_true",
			jsonStr:  `"true"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_false",
			jsonStr:  `"false"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_1",
			jsonStr:  `"1"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_0",
			jsonStr:  `"0"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_on",
			jsonStr:  `"on"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_On",
			jsonStr:  `"On"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_ON",
			jsonStr:  `"ON"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_yes",
			jsonStr:  `"yes"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_Yes",
			jsonStr:  `"Yes"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_YES",
			jsonStr:  `"YES"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_off",
			jsonStr:  `"off"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_no",
			jsonStr:  `"no"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_t",
			jsonStr:  `"t"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_f",
			jsonStr:  `"f"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_T",
			jsonStr:  `"T"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_F",
			jsonStr:  `"F"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "empty_string",
			jsonStr:  `""`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "random_string",
			jsonStr:  `"random"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_with_whitespace",
			jsonStr:  `"  true  "`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string_TRUE",
			jsonStr:  `"TRUE"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string_FALSE",
			jsonStr:  `"FALSE"`,
			wantErr:  false,
			expected: false,
		},
		// Integer values
		{
			name:     "integer_1",
			jsonStr:  `1`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "integer_0",
			jsonStr:  `0`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "positive_integer",
			jsonStr:  `42`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "negative_integer",
			jsonStr:  `-1`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "large_positive_integer",
			jsonStr:  `999999`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "large_negative_integer",
			jsonStr:  `-999999`,
			wantErr:  false,
			expected: true,
		},
		// Float values
		{
			name:     "float_1.0",
			jsonStr:  `1.0`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "float_0.0",
			jsonStr:  `0.0`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "positive_float",
			jsonStr:  `3.14`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "negative_float",
			jsonStr:  `-2.5`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "very_small_positive_float",
			jsonStr:  `0.00001`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "very_small_negative_float",
			jsonStr:  `-0.00001`,
			wantErr:  false,
			expected: true,
		},
		// Null value
		{
			name:     "null",
			jsonStr:  `null`,
			wantErr:  false,
			expected: false,
		},
		// Invalid JSON - error cases
		{
			name:    "invalid_JSON_syntax",
			jsonStr: `{invalid`,
			wantErr: true,
		},
		{
			name:    "unclosed_string",
			jsonStr: `"true`,
			wantErr: true,
		},
		{
			name:    "invalid_escape",
			jsonStr: `"\x"`,
			wantErr: true,
		},
		// Array (should not error but defaults to false)
		{
			name:     "array",
			jsonStr:  `[1, 2, 3]`,
			wantErr:  false,
			expected: false,
		},
		// Object (should not error but defaults to false)
		{
			name:     "object",
			jsonStr:  `{"value": true}`,
			wantErr:  false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var fb Bool
			err := json.Unmarshal([]byte(tt.jsonStr), &fb)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, bool(fb))
			}
		})
	}
}

func TestBool_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    Bool
		expected string
	}{
		{
			name:     "marshal true",
			value:    Bool(true),
			expected: `true`,
		},
		{
			name:     "marshal false",
			value:    Bool(false),
			expected: `false`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.value)
			if err != nil {
				t.Errorf("Bool.MarshalJSON() error = %v", err)

				return
			}

			if string(data) != tt.expected {
				t.Errorf("Bool.MarshalJSON() = %v, want %v", string(data), tt.expected)
			}
		})
	}
}

func TestBool_Bool(t *testing.T) {
	t.Parallel()

	trueVal := Bool(true)
	falseVal := Bool(false)

	tests := []struct {
		name     string
		value    *Bool
		expected bool
	}{
		{
			name:     "true value",
			value:    &trueVal,
			expected: true,
		},
		{
			name:     "false value",
			value:    &falseVal,
			expected: false,
		},
		{
			name:     "nil pointer",
			value:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.value.Bool()
			if result != tt.expected {
				t.Errorf("Bool.Bool() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBool_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "round_trip_true",
			input: `true`,
			want:  true,
		},
		{
			name:  "round_trip_false",
			input: `false`,
			want:  false,
		},
		{
			name:  "round_trip_string_1",
			input: `"1"`,
			want:  true,
		},
		{
			name:  "round_trip_integer_0",
			input: `0`,
			want:  false,
		},
		{
			name:  "round_trip_yes",
			input: `"yes"`,
			want:  true,
		},
		{
			name:  "round_trip_on",
			input: `"on"`,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var fb Bool
			err := json.Unmarshal([]byte(tt.input), &fb)
			assert.NoError(t, err)

			data, err := json.Marshal(fb)
			assert.NoError(t, err)

			var fb2 Bool
			err = json.Unmarshal(data, &fb2)
			assert.NoError(t, err)

			assert.Equal(t, tt.want, bool(fb2))
		})
	}
}

func TestBool_StructMarshaling(t *testing.T) {
	t.Parallel()

	type TestStruct struct {
		Enabled    Bool `json:"enabled"`
		Active     Bool `json:"active"`
		Deprecated Bool `json:"deprecated"`
	}

	t.Run("marshal_struct_with_flexible_bool", func(t *testing.T) {
		t.Parallel()

		ts := TestStruct{
			Enabled:    Bool(true),
			Active:     Bool(false),
			Deprecated: Bool(true),
		}

		data, err := json.Marshal(ts)
		assert.NoError(t, err)
		assert.Contains(t, string(data), `"enabled":true`)
		assert.Contains(t, string(data), `"active":false`)
		assert.Contains(t, string(data), `"deprecated":true`)
	})

	t.Run("unmarshal_struct_with_flexible_bool_mixed_formats", func(t *testing.T) {
		t.Parallel()

		jsonData := `{
			"enabled": "yes",
			"active": 0,
			"deprecated": true
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		assert.NoError(t, err)

		assert.Equal(t, true, bool(ts.Enabled))
		assert.Equal(t, false, bool(ts.Active))
		assert.Equal(t, true, bool(ts.Deprecated))
	})

	t.Run("unmarshal_struct_with_all_string_formats", func(t *testing.T) {
		t.Parallel()

		jsonData := `{
			"enabled": "on",
			"active": "1",
			"deprecated": "YES"
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		assert.NoError(t, err)

		assert.Equal(t, true, bool(ts.Enabled))
		assert.Equal(t, true, bool(ts.Active))
		assert.Equal(t, true, bool(ts.Deprecated))
	})
}

func TestBool_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("multiple_unmarshal_same_variable", func(t *testing.T) {
		t.Parallel()

		var fb Bool

		err := json.Unmarshal([]byte(`true`), &fb)
		assert.NoError(t, err)
		assert.Equal(t, true, bool(fb))

		err = json.Unmarshal([]byte(`false`), &fb)
		assert.NoError(t, err)
		assert.Equal(t, false, bool(fb))

		err = json.Unmarshal([]byte(`"yes"`), &fb)
		assert.NoError(t, err)
		assert.Equal(t, true, bool(fb))
	})

	t.Run("pointer_to_bool", func(t *testing.T) {
		t.Parallel()

		fb := Bool(true)
		ptr := &fb

		data, err := json.Marshal(ptr)
		assert.NoError(t, err)
		assert.Equal(t, `true`, string(data))

		var fb2 Bool
		err = json.Unmarshal(data, &fb2)
		assert.NoError(t, err)
		assert.Equal(t, true, bool(fb2))
	})

	t.Run("array_of_bools", func(t *testing.T) {
		t.Parallel()

		type BoolArray struct {
			Values []Bool `json:"values"`
		}

		jsonData := `{"values": [true, false, "yes", "on", 1, 0]}`
		var ba BoolArray
		err := json.Unmarshal([]byte(jsonData), &ba)
		assert.NoError(t, err)
		assert.Len(t, ba.Values, 6)
		assert.Equal(t, true, bool(ba.Values[0]))
		assert.Equal(t, false, bool(ba.Values[1]))
		assert.Equal(t, true, bool(ba.Values[2]))
		assert.Equal(t, true, bool(ba.Values[3]))
		assert.Equal(t, true, bool(ba.Values[4]))
		assert.Equal(t, false, bool(ba.Values[5]))
	})
}

func Test_anyToBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		// Boolean
		{name: "bool_true", input: true, expected: true},
		{name: "bool_false", input: false, expected: false},
		// String
		{name: "string_true", input: "true", expected: true},
		{name: "string_false", input: "false", expected: false},
		{name: "string_1", input: "1", expected: true},
		{name: "string_0", input: "0", expected: false},
		{name: "string_on", input: "on", expected: true},
		{name: "string_On", input: "On", expected: true},
		{name: "string_ON", input: "ON", expected: true},
		{name: "string_yes", input: "yes", expected: true},
		{name: "string_Yes", input: "Yes", expected: true},
		{name: "string_YES", input: "YES", expected: true},
		{name: "string_off", input: "off", expected: false},
		{name: "string_no", input: "no", expected: false},
		{name: "empty_string", input: "", expected: false},
		{name: "random_string", input: "random", expected: false},
		// Integer types - int
		{name: "int_0", input: int(0), expected: false},
		{name: "int_1", input: int(1), expected: true},
		{name: "int_positive", input: int(42), expected: true},
		{name: "int_negative", input: int(-1), expected: true},
		// int8
		{name: "int8_0", input: int8(0), expected: false},
		{name: "int8_1", input: int8(1), expected: true},
		{name: "int8_max", input: int8(127), expected: true},
		{name: "int8_min", input: int8(-128), expected: true},
		// int16
		{name: "int16_0", input: int16(0), expected: false},
		{name: "int16_1", input: int16(1), expected: true},
		// int32
		{name: "int32_0", input: int32(0), expected: false},
		{name: "int32_1", input: int32(1), expected: true},
		// int64
		{name: "int64_0", input: int64(0), expected: false},
		{name: "int64_1", input: int64(1), expected: true},
		// uint
		{name: "uint_0", input: uint(0), expected: false},
		{name: "uint_1", input: uint(1), expected: true},
		{name: "uint_large", input: uint(999), expected: true},
		// uint8
		{name: "uint8_0", input: uint8(0), expected: false},
		{name: "uint8_1", input: uint8(1), expected: true},
		{name: "uint8_max", input: uint8(255), expected: true},
		// uint16
		{name: "uint16_0", input: uint16(0), expected: false},
		{name: "uint16_1", input: uint16(1), expected: true},
		// uint32
		{name: "uint32_0", input: uint32(0), expected: false},
		{name: "uint32_1", input: uint32(1), expected: true},
		// uint64
		{name: "uint64_0", input: uint64(0), expected: false},
		{name: "uint64_1", input: uint64(1), expected: true},
		// Float32
		{name: "float32_0", input: float32(0.0), expected: false},
		{name: "float32_1", input: float32(1.0), expected: true},
		{name: "float32_positive", input: float32(3.14), expected: true},
		{name: "float32_negative", input: float32(-2.5), expected: true},
		{name: "float32_small", input: float32(0.00001), expected: true},
		// Float64
		{name: "float64_0", input: float64(0.0), expected: false},
		{name: "float64_1", input: float64(1.0), expected: true},
		{name: "float64_positive", input: float64(3.14), expected: true},
		{name: "float64_negative", input: float64(-2.5), expected: true},
		{name: "float64_very_small", input: float64(0.0000000001), expected: true},
		// Other types
		{name: "nil", input: nil, expected: false},
		{name: "slice", input: []int{1, 2, 3}, expected: false},
		{name: "map", input: map[string]int{"key": 1}, expected: false},
		{name: "struct", input: struct{ Value int }{Value: 1}, expected: false},
		{name: "pointer", input: new(int), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := anyToBool(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
