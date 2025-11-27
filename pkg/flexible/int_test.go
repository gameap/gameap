package flexible

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected int
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
			name:     "integer_negative",
			jsonStr:  `-42`,
			wantErr:  false,
			expected: -42,
		},
		{
			name:     "integer_large",
			jsonStr:  `999999`,
			wantErr:  false,
			expected: 999999,
		},
		{
			name:     "integer_large_negative",
			jsonStr:  `-999999`,
			wantErr:  false,
			expected: -999999,
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
			name:     "string_negative",
			jsonStr:  `"-42"`,
			wantErr:  false,
			expected: -42,
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
			name:     "float_negative",
			jsonStr:  `-42.5`,
			wantErr:  false,
			expected: -42,
		},
		{
			name:     "null",
			jsonStr:  `null`,
			wantErr:  false,
			expected: 0,
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
			var i Int
			err := json.Unmarshal([]byte(tt.jsonStr), &i)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, int(i))
			}
		})
	}
}

func TestInt_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    Int
		expected string
	}{
		{
			name:     "zero",
			value:    Int(0),
			expected: `0`,
		},
		{
			name:     "positive",
			value:    Int(42),
			expected: `42`,
		},
		{
			name:     "negative",
			value:    Int(-42),
			expected: `-42`,
		},
		{
			name:     "large",
			value:    Int(999999),
			expected: `999999`,
		},
		{
			name:     "large_negative",
			value:    Int(-999999),
			expected: `-999999`,
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

func TestInt_Int(t *testing.T) {
	zeroVal := Int(0)
	positiveVal := Int(42)
	negativeVal := Int(-42)

	tests := []struct {
		name     string
		value    *Int
		expected int
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
			name:     "negative_value",
			value:    &negativeVal,
			expected: -42,
		},
		{
			name:     "nil_pointer",
			value:    nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.value.Int()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInt_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
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
			name:  "round_trip_negative",
			input: `-42`,
			want:  -42,
		},
		{
			name:  "round_trip_string",
			input: `"123"`,
			want:  123,
		},
		{
			name:  "round_trip_string_negative",
			input: `"-123"`,
			want:  -123,
		},
		{
			name:  "round_trip_float",
			input: `99.8`,
			want:  99,
		},
		{
			name:  "round_trip_float_negative",
			input: `-99.8`,
			want:  -99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var i Int
			err := json.Unmarshal([]byte(tt.input), &i)
			require.NoError(t, err)

			data, err := json.Marshal(i)
			require.NoError(t, err)

			var i2 Int
			err = json.Unmarshal(data, &i2)
			require.NoError(t, err)

			assert.Equal(t, tt.want, int(i2))
		})
	}
}

func TestInt_StructMarshaling(t *testing.T) {
	type TestStruct struct {
		Priority *Int `json:"priority,omitempty"`
		Offset   *Int `json:"offset,omitempty"`
		Count    Int  `json:"count"`
	}

	t.Run("marshal_struct_with_flexible_int", func(t *testing.T) {
		priority := Int(10)
		offset := Int(-5)
		ts := TestStruct{
			Priority: &priority,
			Offset:   &offset,
			Count:    Int(100),
		}

		data, err := json.Marshal(ts)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"priority":10`)
		assert.Contains(t, string(data), `"offset":-5`)
		assert.Contains(t, string(data), `"count":100`)
	})

	t.Run("unmarshal_struct_with_flexible_int_mixed_formats", func(t *testing.T) {
		jsonData := `{
			"priority": "10",
			"offset": -5,
			"count": 100
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Equal(t, 10, ts.Priority.Int())
		assert.Equal(t, -5, ts.Offset.Int())
		assert.Equal(t, 100, int(ts.Count))
	})

	t.Run("unmarshal_struct_with_all_string_formats", func(t *testing.T) {
		jsonData := `{
			"priority": "10",
			"offset": "-5",
			"count": "100"
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Equal(t, 10, ts.Priority.Int())
		assert.Equal(t, -5, ts.Offset.Int())
		assert.Equal(t, 100, int(ts.Count))
	})

	t.Run("unmarshal_struct_with_omitted_fields", func(t *testing.T) {
		jsonData := `{
			"count": 10
		}`

		var ts TestStruct
		err := json.Unmarshal([]byte(jsonData), &ts)
		require.NoError(t, err)

		assert.Nil(t, ts.Priority)
		assert.Nil(t, ts.Offset)
		assert.Equal(t, 10, int(ts.Count))
	})
}

func TestInt_EdgeCases(t *testing.T) {
	t.Run("multiple_unmarshal_same_variable", func(t *testing.T) {
		var i Int

		err := json.Unmarshal([]byte(`42`), &i)
		require.NoError(t, err)
		assert.Equal(t, 42, int(i))

		err = json.Unmarshal([]byte(`"-100"`), &i)
		require.NoError(t, err)
		assert.Equal(t, -100, int(i))

		err = json.Unmarshal([]byte(`0`), &i)
		require.NoError(t, err)
		assert.Equal(t, 0, int(i))
	})

	t.Run("pointer_to_int", func(t *testing.T) {
		i := Int(-42)
		ptr := &i

		data, err := json.Marshal(ptr)
		require.NoError(t, err)
		assert.Equal(t, `-42`, string(data))

		var i2 Int
		err = json.Unmarshal(data, &i2)
		require.NoError(t, err)
		assert.Equal(t, -42, int(i2))
	})

	t.Run("array_of_ints", func(t *testing.T) {
		type IntArray struct {
			Values []Int `json:"values"`
		}

		jsonData := `{"values": [1, "-2", 3.0, "-100", 0]}`
		var ia IntArray
		err := json.Unmarshal([]byte(jsonData), &ia)
		require.NoError(t, err)
		require.Len(t, ia.Values, 5)
		assert.Equal(t, 1, int(ia.Values[0]))
		assert.Equal(t, -2, int(ia.Values[1]))
		assert.Equal(t, 3, int(ia.Values[2]))
		assert.Equal(t, -100, int(ia.Values[3]))
		assert.Equal(t, 0, int(ia.Values[4]))
	})
}

func Test_anyToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
		wantErr  bool
	}{
		{name: "string_zero", input: "0", expected: 0, wantErr: false},
		{name: "string_positive", input: "42", expected: 42, wantErr: false},
		{name: "string_negative", input: "-42", expected: -42, wantErr: false},
		{name: "string_large", input: "999999", expected: 999999, wantErr: false},
		{name: "string_empty", input: "", expected: 0, wantErr: false},
		{name: "string_invalid", input: "abc", expected: 0, wantErr: true},
		{name: "string_decimal", input: "42.5", expected: 0, wantErr: true},
		{name: "float64_zero", input: float64(0), expected: 0, wantErr: false},
		{name: "float64_positive", input: float64(42.8), expected: 42, wantErr: false},
		{name: "float64_negative", input: float64(-42.8), expected: -42, wantErr: false},
		{name: "int_zero", input: int(0), expected: 0, wantErr: false},
		{name: "int_positive", input: int(42), expected: 42, wantErr: false},
		{name: "int_negative", input: int(-42), expected: -42, wantErr: false},
		{name: "int8_positive", input: int8(42), expected: 42, wantErr: false},
		{name: "int8_negative", input: int8(-42), expected: -42, wantErr: false},
		{name: "int16_positive", input: int16(42), expected: 42, wantErr: false},
		{name: "int16_negative", input: int16(-42), expected: -42, wantErr: false},
		{name: "int32_positive", input: int32(42), expected: 42, wantErr: false},
		{name: "int32_negative", input: int32(-42), expected: -42, wantErr: false},
		{name: "int64_positive", input: int64(42), expected: 42, wantErr: false},
		{name: "int64_negative", input: int64(-42), expected: -42, wantErr: false},
		{name: "uint_zero", input: uint(0), expected: 0, wantErr: false},
		{name: "uint_positive", input: uint(42), expected: 42, wantErr: false},
		{name: "uint8_positive", input: uint8(42), expected: 42, wantErr: false},
		{name: "uint8_max", input: uint8(255), expected: 255, wantErr: false},
		{name: "uint16_positive", input: uint16(42), expected: 42, wantErr: false},
		{name: "uint32_positive", input: uint32(42), expected: 42, wantErr: false},
		{name: "uint64_positive", input: uint64(42), expected: 42, wantErr: false},
		{name: "float32_zero", input: float32(0), expected: 0, wantErr: false},
		{name: "float32_positive", input: float32(42.5), expected: 42, wantErr: false},
		{name: "float32_negative", input: float32(-42.5), expected: -42, wantErr: false},
		{name: "nil", input: nil, expected: 0, wantErr: false},
		{name: "slice", input: []int{1, 2, 3}, expected: 0, wantErr: true},
		{name: "map", input: map[string]int{"key": 1}, expected: 0, wantErr: true},
		{name: "struct", input: struct{ Value int }{Value: 1}, expected: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := anyToInt(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
