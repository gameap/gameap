package strings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCryptoRandomString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "length_1",
			length: 1,
		},
		{
			name:   "length_8",
			length: 8,
		},
		{
			name:   "length_16",
			length: 16,
		},
		{
			name:   "length_32",
			length: 32,
		},
		{
			name:   "length_64",
			length: 64,
		},
		{
			name:   "length_128",
			length: 128,
		},
		{
			name:   "length_256",
			length: 256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := CryptoRandomString(tt.length)
			require.NoError(t, err)
			assert.Equal(t, tt.length, len(result), "Generated string should have the requested length")

			for _, c := range result {
				assert.Contains(t, characterSet, string(c), "All characters should be from the character set")
			}
		})
	}
}

func TestCryptoRandomString_ZeroLength(t *testing.T) {
	t.Parallel()
	result, err := CryptoRandomString(0)
	require.NoError(t, err)
	assert.Equal(t, "", result, "Zero length should return empty string")
}

func TestCryptoRandomString_Uniqueness(t *testing.T) {
	t.Parallel()
	length := 32
	iterations := 100
	generated := make(map[string]bool)

	for range iterations {
		result, err := CryptoRandomString(length)
		require.NoError(t, err)
		assert.NotContains(t, generated, result, "Generated strings should be unique")
		generated[result] = true
	}

	assert.Equal(t, iterations, len(generated), "All generated strings should be unique")
}

func TestCryptoRandomString_CharacterDistribution(t *testing.T) {
	t.Parallel()
	length := 1000
	result, err := CryptoRandomString(length)
	require.NoError(t, err)

	charCount := make(map[rune]int)
	for _, c := range result {
		charCount[c]++
	}

	assert.Greater(t, len(charCount), 1, "Should use multiple different characters")

	assert.Greater(t, len(charCount), len(characterSet)/4,
		"Should have reasonable character distribution (at least 25% of character set)")
}

func TestCryptoRandomString_OnlyValidCharacters(t *testing.T) {
	t.Parallel()
	length := 100
	result, err := CryptoRandomString(length)
	require.NoError(t, err)

	validChars := map[rune]bool{}
	for _, c := range characterSet {
		validChars[c] = true
	}

	for i, c := range result {
		assert.True(t, validChars[c], "Character at position %d ('%c') is not in the valid character set", i, c)
	}
}

func TestCryptoRandomString_NoWhitespace(t *testing.T) {
	t.Parallel()
	length := 100
	result, err := CryptoRandomString(length)
	require.NoError(t, err)

	for _, c := range result {
		assert.NotEqual(t, ' ', c, "Should not contain spaces")
		assert.NotEqual(t, '\t', c, "Should not contain tabs")
		assert.NotEqual(t, '\n', c, "Should not contain newlines")
		assert.NotEqual(t, '\r', c, "Should not contain carriage returns")
	}
}

func TestCryptoRandomString_NoSpecialCharacters(t *testing.T) {
	t.Parallel()
	length := 100
	result, err := CryptoRandomString(length)
	require.NoError(t, err)

	specialChars := "!@#$%^&*()_+-=[]{}|;:',.<>?/`~"
	for _, c := range result {
		assert.NotContains(t, specialChars, string(c), "Should not contain special characters")
	}
}

func TestCryptoRandomString_AlphanumericOnly(t *testing.T) {
	t.Parallel()
	length := 100
	result, err := CryptoRandomString(length)
	require.NoError(t, err)

	for _, c := range result {
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isNumeric := c >= '0' && c <= '9'
		assert.True(t, isAlpha || isNumeric, "Character '%c' should be alphanumeric", c)
	}
}

// TestCharacterSet_IsCompleteAlphanumericWithoutDuplicates pins the alphabet
// itself, independently of characterSet's own value. A past regression
// duplicated 'd' and dropped u-z; the other tests compare against characterSet
// and so cannot catch such a defect. Here the expectation is spelled out with
// explicit rune ranges instead.
func TestCharacterSet_IsCompleteAlphanumericWithoutDuplicates(t *testing.T) {
	t.Parallel()
	// ARRANGE / ACT — count every occurrence of each rune.
	seen := make(map[rune]int, len(characterSet))
	for _, c := range characterSet {
		seen[c]++
	}

	// ASSERT — 26 lowercase + 26 uppercase + 10 digits, each exactly once.
	assert.Len(t, characterSet, 62, "charset must be 26 + 26 + 10 = 62 characters")
	assert.Len(t, seen, 62, "charset must contain no duplicate characters")

	for c := 'a'; c <= 'z'; c++ {
		assert.Equalf(t, 1, seen[c], "lowercase %q must appear exactly once", c)
	}
	for c := 'A'; c <= 'Z'; c++ {
		assert.Equalf(t, 1, seen[c], "uppercase %q must appear exactly once", c)
	}
	for c := '0'; c <= '9'; c++ {
		assert.Equalf(t, 1, seen[c], "digit %q must appear exactly once", c)
	}
}
