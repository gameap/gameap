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

func TestIsASCIILetter(t *testing.T) {
	for _, c := range []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		assert.True(t, IsASCIILetter(c), "expected %q to be a letter", c)
	}
	for _, c := range []byte("0123456789-_./:\\") {
		assert.False(t, IsASCIILetter(c), "expected %q to not be a letter", c)
	}
}

func TestIsRelativeServerPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"plain_relative_path", "servers/x", true},
		{"single_segment", "servers", true},
		{"deep_relative_path", "servers/abc/def", true},
		{"backslash_relative_windows_style", `servers\x`, true},
		{"empty_is_invalid", "", false},
		{"leading_forward_slash", "/srv/gameap", false},
		{"leading_backslash", `\gameap`, false},
		{"windows_drive_letter_uppercase", `C:\gameap\servers\x`, false},
		{"windows_drive_letter_lowercase", `d:\data`, false},
		{"unc_share_path", `\\server\share`, false},
		{"contains_dot_dot_segment", "../etc/passwd", false},
		{"contains_dot_dot_segment_middle", "servers/../etc", false},
		{"contains_dot_dot_segment_backslash", `servers\..\etc`, false},
		{"single_dot_is_allowed_only_inside", "servers/./x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsRelativeServerPath(tt.input))
		})
	}
}
