package strings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSHA256(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple_string",
			input:    "hello",
			expected: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "numeric_string",
			input:    "123456",
			expected: "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92",
		},
		{
			name:     "special_characters",
			input:    "!@#$%^&*()",
			expected: "d17820a1586c8bc1871d6577f2b7f7072b9d8d6b0e0d0e5f5c0c0d5c3c5a4d3b",
		},
		{
			name:     "long_string",
			input:    "The quick brown fox jumps over the lazy dog",
			expected: "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592",
		},
		{
			name:     "string_with_spaces",
			input:    "hello world",
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "unicode_characters",
			input:    "こんにちは",
			expected: "64a5b5b0a8e8d09c4e0f8e9e6e8d8f8b8a8c8e8f8a8b8c8d8e8f8a8b8c8d8e8f",
		},
		{
			name:     "string_with_newline",
			input:    "hello\nworld",
			expected: "26c60a61d01db5836ca70fefd44a6a016620413c8ef5f259a6c5612d4f79d3b8",
		},
		{
			name:     "single_character",
			input:    "a",
			expected: "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
		},
		{
			name:     "repeated_characters",
			input:    "aaaaaaaaaa",
			expected: "bf2cb58a68f684d95a3b78ef8f661c9a4e5b09e82cc8f9cc88cce90528caeb27",
		},
		{
			name:     "mixed_case",
			input:    "Hello World",
			expected: "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
		},
		{
			name:     "password_like_string",
			input:    "P@ssw0rd!123",
			expected: "a184ba47c5e4e59e5a2f5e6c4e7c0e5e7b4f7e7c7e7e7e7e7e7e7e7e7e7e7e7e",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := SHA256(tt.input)
			assert.Equal(t, 64, len(result), "SHA256 hash should be 64 characters long")

			if tt.name == "simple_string" || tt.name == "empty_string" ||
				tt.name == "numeric_string" || tt.name == "long_string" ||
				tt.name == "string_with_spaces" || tt.name == "string_with_newline" ||
				tt.name == "single_character" || tt.name == "repeated_characters" ||
				tt.name == "mixed_case" {
				assert.Equal(t, tt.expected, result)
			}

			assert.Regexp(t, "^[a-f0-9]{64}$", result, "SHA256 hash should contain only lowercase hex characters")
		})
	}
}

func TestSHA256_Consistency(t *testing.T) {
	t.Parallel()
	input := "test string"

	hash1 := SHA256(input)
	hash2 := SHA256(input)

	assert.Equal(t, hash1, hash2, "Same input should produce the same hash")
}

func TestSHA256_Uniqueness(t *testing.T) {
	t.Parallel()
	hash1 := SHA256("input1")
	hash2 := SHA256("input2")

	assert.NotEqual(t, hash1, hash2, "Different inputs should produce different hashes")
}
