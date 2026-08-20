package plugin

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestParsePluginID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected domain.Uint64ID
	}{
		{
			name:     "decimal_number",
			input:    "123",
			expected: 123,
		},
		{
			name:     "decimal_zero",
			input:    "0",
			expected: 0,
		},
		{
			name:     "large_decimal_number",
			input:    "18446744073709551615",
			expected: 18446744073709551615,
		},
		{
			name:     "base32_encoded_short_id",
			input:    "ae",
			expected: 1,
		},
		{
			name:     "base32_encoded_id",
			input:    "aaaaaaaaaaaac",
			expected: 1,
		},
		{
			name:     "base32_encoded_large_id",
			input:    "aaaaaaaatclia",
			expected: 10000000,
		},
		{
			name:     "string_id_hashed",
			input:    "my-plugin",
			expected: 0x8eb6a9b8ea53ef65,
		},
		{
			name:     "another_string_id_hashed",
			input:    "server-logger",
			expected: 0x519633e3bd3a577d,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ParsePluginID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompactPluginID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    domain.Uint64ID
		expected string
	}{
		{
			name:     "zero",
			input:    0,
			expected: "aa",
		},
		{
			name:     "one",
			input:    1,
			expected: "ae",
		},
		{
			name:     "large_number",
			input:    10000000,
			expected: "tclia",
		},
		{
			name:     "max_uint64",
			input:    18446744073709551615,
			expected: "7777777777776",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := CompactPluginID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompactPluginID_RoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   domain.Uint64ID
	}{
		{name: "zero", id: 0},
		{name: "one", id: 1},
		{name: "small", id: 42},
		{name: "medium", id: 10000000},
		{name: "large", id: 9223372036854775807},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			compact := CompactPluginID(tt.id)
			parsed := ParsePluginID(compact)
			assert.Equal(t, tt.id, parsed)
		})
	}
}
