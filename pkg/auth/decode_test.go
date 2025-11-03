package auth

import (
	"encoding/ascii85"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeWithPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "base64_prefix_valid_data",
			input:    []byte("base64:" + base64.StdEncoding.EncodeToString([]byte("hello world"))),
			expected: []byte("hello world"),
		},
		{
			name:     "base64_prefix_empty_data",
			input:    []byte("base64:"),
			expected: []byte{},
		},
		{
			name:     "base64_prefix_invalid_data",
			input:    []byte("base64:invalid-base64!!!"),
			expected: []byte("base64:invalid-base64!!!"),
		},
		{
			name:     "base64_prefix_special_characters",
			input:    []byte("base64:" + base64.StdEncoding.EncodeToString([]byte("test@#$%^&*()"))),
			expected: []byte("test@#$%^&*()"),
		},
		{
			name:     "base64_prefix_binary_data",
			input:    []byte("base64:" + base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02, 0xFF})),
			expected: []byte{0x00, 0x01, 0x02, 0xFF},
		},
		{
			name:     "no_prefix_plain_text",
			input:    []byte("plain text without prefix"),
			expected: []byte("plain text without prefix"),
		},
		{
			name:     "no_prefix_empty",
			input:    []byte(""),
			expected: []byte(""),
		},
		{
			name:     "partial_prefix_base64",
			input:    []byte("base6:test"),
			expected: []byte("base6:test"),
		},
		{
			name:     "case_sensitive_prefix",
			input:    []byte("BASE64:" + base64.StdEncoding.EncodeToString([]byte("test"))),
			expected: []byte("BASE64:" + base64.StdEncoding.EncodeToString([]byte("test"))),
		},
		{
			name:     "prefix_in_middle",
			input:    []byte("some base64: data"),
			expected: []byte("some base64: data"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeWithPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeWithPrefix_ASCII85(t *testing.T) {
	// Helper function to encode data with ascii85
	encodeASCII85 := func(data []byte) string {
		encoded := make([]byte, ascii85.MaxEncodedLen(len(data)))
		n := ascii85.Encode(encoded, data)

		return string(encoded[:n])
	}

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "ascii85_prefix_valid_data",
			input:    []byte("ascii85:" + encodeASCII85([]byte("hello world"))),
			expected: []byte("hello world"),
		},
		{
			name:     "ascii85_prefix_empty_data",
			input:    []byte("ascii85:"),
			expected: []byte{},
		},
		{
			name:     "ascii85_prefix_invalid_data",
			input:    []byte("ascii85:invalid!!!"),
			expected: []byte("ascii85:invalid!!!"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeWithPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDecodeWithPrefix_MultipleEncodings(t *testing.T) {
	testData := []byte("secret-key-123")

	t.Run("base64_encoding", func(t *testing.T) {
		encoded := "base64:" + base64.StdEncoding.EncodeToString(testData)
		result := DecodeWithPrefix([]byte(encoded))
		assert.Equal(t, testData, result)
	})

	t.Run("no_encoding", func(t *testing.T) {
		result := DecodeWithPrefix(testData)
		assert.Equal(t, testData, result)
	})
}

func TestDecodeWithPrefix_LongData(t *testing.T) {
	// Test with long data to ensure buffer handling is correct
	longData := make([]byte, 1024)
	for i := range longData {
		longData[i] = byte(i % 256)
	}

	encoded := "base64:" + base64.StdEncoding.EncodeToString(longData)
	result := DecodeWithPrefix([]byte(encoded))
	assert.Equal(t, longData, result)
}
