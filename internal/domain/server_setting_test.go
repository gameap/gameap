package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerSettingValue(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantValue any
		wantType  serverSettingType
	}{
		{
			name:      "string_value",
			input:     "test string",
			wantValue: "test string",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "bool_true",
			input:     true,
			wantValue: true,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "bool_false",
			input:     false,
			wantValue: false,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "int_value",
			input:     42,
			wantValue: 42,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "int64_converted_to_int",
			input:     int64(100),
			wantValue: 100,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "float64_converted_to_int",
			input:     float64(123.456),
			wantValue: 123,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "nil_value",
			input:     nil,
			wantValue: nil,
			wantType:  serverSettingTypeUnknown,
		},
		{
			name:      "empty_string",
			input:     "",
			wantValue: "",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "zero_int",
			input:     0,
			wantValue: 0,
			wantType:  serverSettingTypeInt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := NewServerSettingValue(test.input)

			assert.Equal(t, test.wantValue, result.value)
			assert.Equal(t, test.wantType, result.tp)
		})
	}
}

func TestServerSettingValue_String(t *testing.T) {
	tests := []struct {
		name    string
		value   ServerSettingValue
		wantVal string
		wantOK  bool
	}{
		{
			name:    "valid_string",
			value:   NewServerSettingValue("hello"),
			wantVal: "hello",
			wantOK:  true,
		},
		{
			name:    "empty_string",
			value:   NewServerSettingValue(""),
			wantVal: "",
			wantOK:  true,
		},
		{
			name:    "bool_not_string",
			value:   NewServerSettingValue(true),
			wantVal: "true",
			wantOK:  true,
		},
		{
			name:    "int_not_string",
			value:   NewServerSettingValue(42),
			wantVal: "42",
			wantOK:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			val, ok := test.value.String()
			assert.Equal(t, test.wantVal, val)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestServerSettingValue_Bool(t *testing.T) {
	tests := []struct {
		name    string
		value   ServerSettingValue
		wantVal bool
		wantOK  bool
	}{
		{
			name:    "bool_true",
			value:   NewServerSettingValue(true),
			wantVal: true,
			wantOK:  true,
		},
		{
			name:    "bool_false",
			value:   NewServerSettingValue(false),
			wantVal: false,
			wantOK:  true,
		},
		{
			name:    "int_zero_as_false",
			value:   NewServerSettingValue(0),
			wantVal: false,
			wantOK:  true,
		},
		{
			name:    "int_nonzero_as_true",
			value:   NewServerSettingValue(1),
			wantVal: true,
			wantOK:  true,
		},
		{
			name:    "int_negative_as_true",
			value:   NewServerSettingValue(-1),
			wantVal: true,
			wantOK:  true,
		},
		{
			name:    "string_true",
			value:   ServerSettingValue{value: "true", tp: serverSettingTypeString},
			wantVal: true,
			wantOK:  true,
		},
		{
			name:    "string_false",
			value:   ServerSettingValue{value: "false", tp: serverSettingTypeString},
			wantVal: false,
			wantOK:  true,
		},
		{
			name:    "string_other_not_bool",
			value:   NewServerSettingValue("not a bool"),
			wantVal: false,
			wantOK:  false,
		},
		{
			name:    "unknown_type",
			value:   ServerSettingValue{value: nil, tp: serverSettingTypeUnknown},
			wantVal: false,
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			val, ok := test.value.Bool()
			assert.Equal(t, test.wantVal, val)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestServerSettingValue_Int(t *testing.T) {
	tests := []struct {
		name    string
		value   ServerSettingValue
		wantVal int
		wantOK  bool
	}{
		{
			name:    "valid_int",
			value:   NewServerSettingValue(42),
			wantVal: 42,
			wantOK:  true,
		},
		{
			name:    "zero_int",
			value:   NewServerSettingValue(0),
			wantVal: 0,
			wantOK:  true,
		},
		{
			name:    "negative_int",
			value:   NewServerSettingValue(-100),
			wantVal: -100,
			wantOK:  true,
		},
		{
			name:    "string_not_int",
			value:   NewServerSettingValue("123"),
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "bool_not_int",
			value:   NewServerSettingValue(true),
			wantVal: 0,
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			val, ok := test.value.Int()
			assert.Equal(t, test.wantVal, val)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestServerSettingValue_Any(t *testing.T) {
	tests := []struct {
		name  string
		value ServerSettingValue
		want  any
	}{
		{
			name:  "string_value",
			value: NewServerSettingValue("test"),
			want:  "test",
		},
		{
			name:  "bool_value",
			value: NewServerSettingValue(true),
			want:  true,
		},
		{
			name:  "int_value",
			value: NewServerSettingValue(42),
			want:  42,
		},
		{
			name:  "nil_value",
			value: NewServerSettingValue(nil),
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.value.Any()
			assert.Equal(t, test.want, result)
		})
	}
}

func TestServerSettingValue_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		value ServerSettingValue
		want  string
	}{
		{
			name:  "string_value",
			value: NewServerSettingValue("hello"),
			want:  `"hello"`,
		},
		{
			name:  "bool_true",
			value: NewServerSettingValue(true),
			want:  `true`,
		},
		{
			name:  "bool_false",
			value: NewServerSettingValue(false),
			want:  `false`,
		},
		{
			name:  "int_value",
			value: NewServerSettingValue(42),
			want:  `42`,
		},
		{
			name:  "int_zero",
			value: NewServerSettingValue(0),
			want:  `0`,
		},
		{
			name:  "nil_value",
			value: NewServerSettingValue(nil),
			want:  `null`,
		},
		{
			name:  "unknown_type",
			value: ServerSettingValue{value: "test", tp: serverSettingTypeUnknown},
			want:  `null`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := json.Marshal(test.value)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(result))
		})
	}
}

func TestServerSettingValue_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue any
	}{
		{
			name:      "string_value",
			input:     `"hello"`,
			wantValue: "hello",
		},
		{
			name:      "bool_true",
			input:     `true`,
			wantValue: true,
		},
		{
			name:      "bool_false",
			input:     `false`,
			wantValue: false,
		},
		{
			name:      "int_value",
			input:     `42`,
			wantValue: 42,
		},
		{
			name:      "int_zero",
			input:     `0`,
			wantValue: 0,
		},
		{
			name:      "empty_string",
			input:     `""`,
			wantValue: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result ServerSettingValue
			err := json.Unmarshal([]byte(test.input), &result)
			require.NoError(t, err)
			assert.Equal(t, test.wantValue, result.value)
		})
	}
}

func TestServerSettingValue_Scan(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantValue any
		wantType  serverSettingType
	}{
		{
			name:      "nil_value",
			input:     nil,
			wantValue: nil,
			wantType:  serverSettingTypeString,
		},
		{
			name:      "byte_slice_true",
			input:     []byte("true"),
			wantValue: true,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "byte_slice_false",
			input:     []byte("false"),
			wantValue: false,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "byte_slice_null",
			input:     []byte("null"),
			wantValue: nil,
			wantType:  serverSettingTypeUnknown,
		},
		{
			name:      "byte_slice_int",
			input:     []byte("42"),
			wantValue: 42,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_negative_int",
			input:     []byte("-100"),
			wantValue: -100,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_hex_int",
			input:     []byte("0x10"),
			wantValue: 16,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_string",
			input:     []byte("hello world"),
			wantValue: "hello world",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "direct_string",
			input:     "test",
			wantValue: "test",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "direct_bool",
			input:     true,
			wantValue: true,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "direct_int",
			input:     123,
			wantValue: 123,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "direct_int64",
			input:     int64(456),
			wantValue: 456,
			wantType:  serverSettingTypeInt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result ServerSettingValue
			err := result.Scan(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.wantValue, result.value)
			assert.Equal(t, test.wantType, result.tp)
		})
	}
}

func TestServerSettingValue_Value(t *testing.T) {
	tests := []struct {
		name  string
		value ServerSettingValue
		want  any
	}{
		{
			name:  "nil_value",
			value: NewServerSettingValue(nil),
			want:  nil,
		},
		{
			name:  "string_value",
			value: NewServerSettingValue("test"),
			want:  "test",
		},
		{
			name:  "bool_true",
			value: NewServerSettingValue(true),
			want:  true,
		},
		{
			name:  "bool_false",
			value: NewServerSettingValue(false),
			want:  false,
		},
		{
			name:  "int_value",
			value: NewServerSettingValue(42),
			want:  42,
		},
		{
			name:  "unknown_type",
			value: ServerSettingValue{value: "test", tp: serverSettingTypeUnknown},
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.value.Value()
			require.NoError(t, err)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestServerSettingValue_ScanAndValue_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{
			name:  "string_round_trip",
			input: []byte("test string"),
		},
		{
			name:  "bool_true_round_trip",
			input: []byte("true"),
		},
		{
			name:  "bool_false_round_trip",
			input: []byte("false"),
		},
		{
			name:  "int_round_trip",
			input: []byte("42"),
		},
		{
			name:  "direct_string_round_trip",
			input: "direct",
		},
		{
			name:  "direct_bool_round_trip",
			input: true,
		},
		{
			name:  "direct_int_round_trip",
			input: 123,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value ServerSettingValue
			err := value.Scan(test.input)
			require.NoError(t, err)

			result, err := value.Value()
			require.NoError(t, err)

			assert.NotNil(t, result)
		})
	}
}

func TestServerSettingValue_MarshalUnmarshal_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value ServerSettingValue
	}{
		{
			name:  "string_round_trip",
			value: NewServerSettingValue("test"),
		},
		{
			name:  "bool_true_round_trip",
			value: NewServerSettingValue(true),
		},
		{
			name:  "bool_false_round_trip",
			value: NewServerSettingValue(false),
		},
		{
			name:  "int_round_trip",
			value: NewServerSettingValue(42),
		},
		{
			name:  "zero_int_round_trip",
			value: NewServerSettingValue(0),
		},
		{
			name:  "empty_string_round_trip",
			value: NewServerSettingValue(""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			marshaled, err := json.Marshal(test.value)
			require.NoError(t, err)

			var result ServerSettingValue
			err = json.Unmarshal(marshaled, &result)
			require.NoError(t, err)

			assert.Equal(t, test.value.value, result.value)
		})
	}
}

func TestServerSetting_Fields(t *testing.T) {
	setting := ServerSetting{
		ID:       1,
		Name:     "max_players",
		ServerID: 42,
		Value:    NewServerSettingValue(16),
	}

	assert.Equal(t, uint(1), setting.ID)
	assert.Equal(t, "max_players", setting.Name)
	assert.Equal(t, uint(42), setting.ServerID)

	intVal, ok := setting.Value.Int()
	assert.True(t, ok)
	assert.Equal(t, 16, intVal)
}
