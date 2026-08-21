package domain

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerSettingValue(t *testing.T) {
	t.Parallel()

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
			name:      "float64_value",
			input:     float64(123.456),
			wantValue: 123.456,
			wantType:  serverSettingTypeFloat,
		},
		{
			name:      "float64_whole_number",
			input:     float64(30),
			wantValue: float64(30),
			wantType:  serverSettingTypeFloat,
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
		{
			name:      "uint_value_falls_back_to_string",
			input:     uint(42),
			wantValue: "42",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "byte_slice_falls_back_to_string",
			input:     []byte("xy"),
			wantValue: "[120 121]",
			wantType:  serverSettingTypeString,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			result := NewServerSettingValue(test.input)

			// ASSERT
			assert.Equal(t, test.wantValue, result.value, "value mismatch")
			assert.Equal(t, test.wantType, result.tp, "type mismatch")
		})
	}
}

func TestServerSettingValue_String(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			val, ok := test.value.String()
			assert.Equal(t, test.wantVal, val)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestServerSettingValue_Bool(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			val, ok := test.value.Bool()
			assert.Equal(t, test.wantVal, val)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestServerSettingValue_Int(t *testing.T) {
	t.Parallel()

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
		{
			name:    "unknown_type_returns_zero_false",
			value:   ServerSettingValue{value: nil, tp: serverSettingTypeUnknown},
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:    "int_type_with_non_int_value_returns_zero_false",
			value:   ServerSettingValue{value: "not int", tp: serverSettingTypeInt},
			wantVal: 0,
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			val, ok := test.value.Int()

			// ASSERT
			assert.Equal(t, test.wantVal, val, "value mismatch")
			assert.Equal(t, test.wantOK, ok, "ok flag mismatch")
		})
	}
}

func TestServerSettingValue_Any(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			result := test.value.Any()
			assert.Equal(t, test.want, result)
		})
	}
}

func TestServerSettingValue_MarshalJSON(t *testing.T) {
	t.Parallel()

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
		{
			name:  "scanned_null_column",
			value: scanValue(nil),
			want:  `null`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := json.Marshal(test.value)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(result))
		})
	}
}

func TestServerSettingValue_UnmarshalJSON(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			var result ServerSettingValue
			err := json.Unmarshal([]byte(test.input), &result)
			require.NoError(t, err)
			assert.Equal(t, test.wantValue, result.value)
		})
	}
}

func TestServerSettingValue_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		receiver  ServerSettingValue
		input     any
		wantValue any
		wantType  serverSettingType
	}{
		{
			name:      "nil_value",
			receiver:  ServerSettingValue{},
			input:     nil,
			wantValue: nil,
			wantType:  serverSettingTypeUnknown,
		},
		{
			name:      "nil_value_overwrites_string",
			receiver:  NewServerSettingValue("preexisting_string"),
			input:     nil,
			wantValue: nil,
			wantType:  serverSettingTypeUnknown,
		},
		{
			name:      "byte_slice_true",
			receiver:  ServerSettingValue{},
			input:     []byte("true"),
			wantValue: true,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "byte_slice_true_overwrites_string",
			receiver:  NewServerSettingValue("preexisting_string"),
			input:     []byte("true"),
			wantValue: true,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "byte_slice_false",
			receiver:  ServerSettingValue{},
			input:     []byte("false"),
			wantValue: false,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "byte_slice_null",
			receiver:  ServerSettingValue{},
			input:     []byte("null"),
			wantValue: nil,
			wantType:  serverSettingTypeUnknown,
		},
		{
			name:      "null_bytes_overwrites_int",
			receiver:  NewServerSettingValue(42),
			input:     []byte("null"),
			wantValue: nil,
			wantType:  serverSettingTypeUnknown,
		},
		{
			name:      "byte_slice_int",
			receiver:  ServerSettingValue{},
			input:     []byte("42"),
			wantValue: 42,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_negative_int",
			receiver:  ServerSettingValue{},
			input:     []byte("-100"),
			wantValue: -100,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_hex_int",
			receiver:  ServerSettingValue{},
			input:     []byte("0x10"),
			wantValue: 16,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_max_int64",
			receiver:  ServerSettingValue{},
			input:     []byte("9223372036854775807"),
			wantValue: 9223372036854775807,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "byte_slice_string",
			receiver:  ServerSettingValue{},
			input:     []byte("hello world"),
			wantValue: "hello world",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "direct_string",
			receiver:  ServerSettingValue{},
			input:     "test",
			wantValue: "test",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "direct_string_overwrites_int",
			receiver:  NewServerSettingValue(99),
			input:     "test",
			wantValue: "test",
			wantType:  serverSettingTypeString,
		},
		{
			name:      "direct_bool",
			receiver:  ServerSettingValue{},
			input:     true,
			wantValue: true,
			wantType:  serverSettingTypeBool,
		},
		{
			name:      "direct_int",
			receiver:  ServerSettingValue{},
			input:     123,
			wantValue: 123,
			wantType:  serverSettingTypeInt,
		},
		{
			name:      "direct_int64",
			receiver:  ServerSettingValue{},
			input:     int64(456),
			wantValue: 456,
			wantType:  serverSettingTypeInt,
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
			require.NoError(t, err)
			assert.Equal(t, test.wantValue, result.value, "value mismatch")
			assert.Equal(t, test.wantType, result.tp, "type mismatch")
		})
	}
}

func TestServerSettingValue_Value(t *testing.T) {
	t.Parallel()

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
			want:  "true",
		},
		{
			name:  "bool_false",
			value: NewServerSettingValue(false),
			want:  "false",
		},
		{
			name:  "int_value",
			value: NewServerSettingValue(42),
			want:  "42",
		},
		{
			name:  "unknown_type",
			value: ServerSettingValue{value: "test", tp: serverSettingTypeUnknown},
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.value.Value()
			require.NoError(t, err)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestServerSettingValue_ScanAndValue_RoundTrip(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

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

func TestServerSettingValue_Raw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func() ServerSettingValue
		wantRaw string
		wantOK  bool
	}{
		{
			name:    "string_from_constructor",
			setup:   func() ServerSettingValue { return NewServerSettingValue("de_dust2") },
			wantRaw: "de_dust2",
			wantOK:  true,
		},
		{
			name:    "leading_zeros_survive_scan",
			setup:   func() ServerSettingValue { return scanValue([]byte("007")) },
			wantRaw: "007",
			wantOK:  true,
		},
		{
			name:    "hex_looking_text_survives_scan",
			setup:   func() ServerSettingValue { return scanValue([]byte("0x10")) },
			wantRaw: "0x10",
			wantOK:  true,
		},
		{
			name:    "float_keeps_decimal_notation",
			setup:   func() ServerSettingValue { return NewServerSettingValue(0.5) },
			wantRaw: "0.5",
			wantOK:  true,
		},
		{
			name:    "bool_true",
			setup:   func() ServerSettingValue { return NewServerSettingValue(true) },
			wantRaw: "true",
			wantOK:  true,
		},
		{
			name:    "empty_string_is_present",
			setup:   func() ServerSettingValue { return NewServerSettingValue("") },
			wantRaw: "",
			wantOK:  true,
		},
		{
			name:    "nil_is_absent",
			setup:   func() ServerSettingValue { return NewServerSettingValue(nil) },
			wantRaw: "",
			wantOK:  false,
		},
		{
			name:    "scanned_null_column_is_absent",
			setup:   func() ServerSettingValue { return scanValue(nil) },
			wantRaw: "",
			wantOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			value := test.setup()

			// ACT
			raw, ok := value.Raw()

			// ASSERT
			assert.Equal(t, test.wantOK, ok, "presence mismatch")
			assert.Equal(t, test.wantRaw, raw, "raw mismatch")
		})
	}
}

func TestServerSettingValue_ValuePreservesRawText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  driver.Value
	}{
		{
			name:  "leading_zeros",
			input: []byte("007"),
			want:  "007",
		},
		{
			name:  "hex_looking_text",
			input: []byte("0x10"),
			want:  "0x10",
		},
		{
			name:  "plain_int",
			input: []byte("32"),
			want:  "32",
		},
		{
			name:  "bool",
			input: []byte("true"),
			want:  "true",
		},
		{
			name:  "null",
			input: []byte("null"),
			want:  nil,
		},
		{
			name:  "null_column",
			input: nil,
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			value := scanValue(test.input)

			// ACT
			result, err := value.Value()

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, test.want, result)
		})
	}
}

func scanValue(raw any) ServerSettingValue {
	var value ServerSettingValue
	_ = value.Scan(raw)

	return value
}
