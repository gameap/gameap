package strings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNumeric(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid_single_digit",
			input:    "5",
			expected: true,
		},
		{
			name:     "valid_multiple_digits",
			input:    "12345",
			expected: true,
		},
		{
			name:     "valid_zero",
			input:    "0",
			expected: true,
		},
		{
			name:     "valid_large_number",
			input:    "9876543210",
			expected: true,
		},
		{
			name:     "empty_string",
			input:    "",
			expected: false,
		},
		{
			name:     "alphabetic_characters",
			input:    "abc",
			expected: false,
		},
		{
			name:     "alphanumeric_mixed",
			input:    "123abc",
			expected: false,
		},
		{
			name:     "number_with_leading_zero",
			input:    "00123",
			expected: true,
		},
		{
			name:     "special_characters",
			input:    "!@#$",
			expected: false,
		},
		{
			name:     "number_with_space",
			input:    "123 456",
			expected: false,
		},
		{
			name:     "negative_number_with_sign",
			input:    "-123",
			expected: false,
		},
		{
			name:     "positive_number_with_sign",
			input:    "+123",
			expected: false,
		},
		{
			name:     "decimal_number",
			input:    "123.45",
			expected: false,
		},
		{
			name:     "number_with_comma",
			input:    "1,234",
			expected: false,
		},
		{
			name:     "whitespace_only",
			input:    "   ",
			expected: false,
		},
		{
			name:     "newline_character",
			input:    "123\n",
			expected: false,
		},
		{
			name:     "tab_character",
			input:    "123\t",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsNumeric(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty_string",
			input:    "",
			expected: true,
		},
		{
			name:     "lowercase_letters",
			input:    "abc",
			expected: true,
		},
		{
			name:     "digits_only",
			input:    "123",
			expected: true,
		},
		{
			name:     "with_underscore",
			input:    "hello_world",
			expected: true,
		},
		{
			name:     "with_hyphen",
			input:    "hello-world",
			expected: true,
		},
		{
			name:     "mixed_valid",
			input:    "hello_world-123",
			expected: true,
		},
		{
			name:     "uppercase_letters",
			input:    "Hello",
			expected: false,
		},
		{
			name:     "with_space",
			input:    "hello world",
			expected: false,
		},
		{
			name:     "with_special_chars",
			input:    "hello@world",
			expected: false,
		},
		{
			name:     "with_dot",
			input:    "hello.world",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsSlug(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
