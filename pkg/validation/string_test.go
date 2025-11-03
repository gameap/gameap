package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid lowercase and digits",
			input:    "abc123",
			expected: true,
		},
		{
			name:     "valid only lowercase",
			input:    "abcdef",
			expected: true,
		},
		{
			name:     "valid only digits",
			input:    "123456",
			expected: true,
		},
		{
			name:     "invalid uppercase",
			input:    "Abc123",
			expected: false,
		},
		{
			name:     "invalid special characters",
			input:    "abc_123",
			expected: false,
		},
		{
			name:     "invalid hyphen",
			input:    "abc-123",
			expected: false,
		},
		{
			name:     "invalid space",
			input:    "abc 123",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAlphanumeric(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsAlphanumericMixed(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid lowercase and digits",
			input:    "abc123",
			expected: true,
		},
		{
			name:     "valid uppercase and digits",
			input:    "ABC123",
			expected: true,
		},
		{
			name:     "valid mixed case and digits",
			input:    "aBc123",
			expected: true,
		},
		{
			name:     "valid only lowercase",
			input:    "abcdef",
			expected: true,
		},
		{
			name:     "valid only uppercase",
			input:    "ABCDEF",
			expected: true,
		},
		{
			name:     "valid only digits",
			input:    "123456",
			expected: true,
		},
		{
			name:     "valid mixed case",
			input:    "AbCdEf",
			expected: true,
		},
		{
			name:     "invalid underscore",
			input:    "abc_123",
			expected: false,
		},
		{
			name:     "invalid hyphen",
			input:    "abc-123",
			expected: false,
		},
		{
			name:     "invalid space",
			input:    "abc 123",
			expected: false,
		},
		{
			name:     "invalid special characters",
			input:    "abc@123",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAlphanumericMixed(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid lowercase and digits",
			input:    "abc123",
			expected: true,
		},
		{
			name:     "valid with underscore",
			input:    "abc_123",
			expected: true,
		},
		{
			name:     "valid with hyphen",
			input:    "abc-123",
			expected: true,
		},
		{
			name:     "valid with underscore and hyphen",
			input:    "abc_123-def",
			expected: true,
		},
		{
			name:     "valid only lowercase",
			input:    "abcdef",
			expected: true,
		},
		{
			name:     "valid only digits",
			input:    "123456",
			expected: true,
		},
		{
			name:     "valid only underscores",
			input:    "___",
			expected: true,
		},
		{
			name:     "valid only hyphens",
			input:    "---",
			expected: true,
		},
		{
			name:     "valid slug format",
			input:    "my-slug_123",
			expected: true,
		},
		{
			name:     "invalid uppercase",
			input:    "Abc123",
			expected: false,
		},
		{
			name:     "invalid space",
			input:    "abc 123",
			expected: false,
		},
		{
			name:     "invalid special characters",
			input:    "abc@123",
			expected: false,
		},
		{
			name:     "invalid dot",
			input:    "abc.123",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSlug(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
