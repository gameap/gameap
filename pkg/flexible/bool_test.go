package flexible

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBool_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantErr  bool
		expected bool
	}{
		// Boolean values
		{
			name:     "true boolean",
			jsonStr:  `true`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "false boolean",
			jsonStr:  `false`,
			wantErr:  false,
			expected: false,
		},
		// String values
		{
			name:     "string 'true'",
			jsonStr:  `"true"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string 'false'",
			jsonStr:  `"false"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string '1'",
			jsonStr:  `"1"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string '0'",
			jsonStr:  `"0"`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "string 'on'",
			jsonStr:  `"on"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string 'On'",
			jsonStr:  `"On"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string 'ON'",
			jsonStr:  `"ON"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string 'yes'",
			jsonStr:  `"yes"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string 'Yes'",
			jsonStr:  `"Yes"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "string 'YES'",
			jsonStr:  `"YES"`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "empty string",
			jsonStr:  `""`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "random string",
			jsonStr:  `"random"`,
			wantErr:  false,
			expected: false,
		},
		// Integer values
		{
			name:     "integer 1",
			jsonStr:  `1`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "integer 0",
			jsonStr:  `0`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "positive integer",
			jsonStr:  `42`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "negative integer",
			jsonStr:  `-1`,
			wantErr:  false,
			expected: true,
		},
		// Float values
		{
			name:     "float 1.0",
			jsonStr:  `1.0`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "float 0.0",
			jsonStr:  `0.0`,
			wantErr:  false,
			expected: false,
		},
		{
			name:     "positive float",
			jsonStr:  `3.14`,
			wantErr:  false,
			expected: true,
		},
		{
			name:     "negative float",
			jsonStr:  `-2.5`,
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
		// Invalid JSON
		{
			name:    "invalid JSON",
			jsonStr: `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fb Bool
			err := json.Unmarshal([]byte(tt.jsonStr), &fb)

			if (err != nil) != tt.wantErr {
				t.Errorf("Bool.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !tt.wantErr && bool(fb) != tt.expected {
				t.Errorf("Bool.UnmarshalJSON() = %v, want %v", bool(fb), tt.expected)
			}
		})
	}
}

func TestBool_MarshalJSON(t *testing.T) {
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
			result := tt.value.Bool()
			if result != tt.expected {
				t.Errorf("Bool.Bool() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBool_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "round trip true",
			input: `true`,
			want:  true,
		},
		{
			name:  "round trip false",
			input: `false`,
			want:  false,
		},
		{
			name:  "round trip string '1'",
			input: `"1"`,
			want:  true,
		},
		{
			name:  "round trip integer 0",
			input: `0`,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fb Bool
			if err := json.Unmarshal([]byte(tt.input), &fb); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			data, err := json.Marshal(fb)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var fb2 Bool
			if err := json.Unmarshal(data, &fb2); err != nil {
				t.Fatalf("Second unmarshal failed: %v", err)
			}

			if bool(fb2) != tt.want {
				t.Errorf("Round trip failed: got %v, want %v", bool(fb2), tt.want)
			}
		})
	}
}

func Test_anyToBool(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		// Boolean
		{name: "bool true", input: true, expected: true},
		{name: "bool false", input: false, expected: false},
		// String
		{name: "string 'true'", input: "true", expected: true},
		{name: "string 'false'", input: "false", expected: false},
		{name: "string '1'", input: "1", expected: true},
		{name: "string '0'", input: "0", expected: false},
		{name: "string 'on'", input: "on", expected: true},
		{name: "string 'On'", input: "On", expected: true},
		{name: "string 'ON'", input: "ON", expected: true},
		{name: "string 'yes'", input: "yes", expected: true},
		{name: "string 'Yes'", input: "Yes", expected: true},
		{name: "string 'YES'", input: "YES", expected: true},
		{name: "empty string", input: "", expected: false},
		{name: "random string", input: "random", expected: false},
		// Integer
		{name: "int 0", input: 0, expected: false},
		{name: "int 1", input: 1, expected: true},
		{name: "int positive", input: 42, expected: true},
		{name: "int negative", input: -1, expected: true},
		// Float64
		{name: "float64 0.0", input: 0.0, expected: false},
		{name: "float64 1.0", input: 1.0, expected: true},
		{name: "float64 positive", input: 3.14, expected: true},
		{name: "float64 negative", input: -2.5, expected: true},
		// Other types
		{name: "nil", input: nil, expected: false},
		{name: "slice", input: []int{1, 2, 3}, expected: false},
		{name: "map", input: map[string]int{"key": 1}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anyToBool(tt.input)
			assert.Equal(t, result, tt.expected)
		})
	}
}
