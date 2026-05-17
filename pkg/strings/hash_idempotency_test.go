// Security tests for idempotent SHA-256 hashing of stored secrets.
//
// OWASP API Security Top 10:2023 — API2:2023 Broken Authentication: the
// gdaemon API key / token are stored hashed so a database read yields no
// usable credential. SHA256IfNeeded must be idempotent so a value that is
// already a digest (re-saved on an unrelated node update or replayed by a
// migration) is not double-hashed into an unusable value. See security
// review findings #3a/#4/#6.
package strings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsSHA256Hex — OWASP API2:2023 Broken Authentication.
func TestIsSHA256Hex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "canonical_lowercase_digest_is_recognized",
			input: SHA256("anything"),
			want:  true,
		},
		{
			name:  "all_zeroes_64_hex_is_recognized",
			input: "0000000000000000000000000000000000000000000000000000000000000000",
			want:  true,
		},
		{
			name:  "all_f_64_hex_is_recognized",
			input: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			want:  true,
		},
		{
			name:  "too_short_is_rejected",
			input: "abcdef",
			want:  false,
		},
		{
			name:  "63_chars_is_rejected",
			input: "000000000000000000000000000000000000000000000000000000000000000",
			want:  false,
		},
		{
			name:  "65_chars_is_rejected",
			input: "00000000000000000000000000000000000000000000000000000000000000000",
			want:  false,
		},
		{
			name:  "uppercase_hex_is_rejected_not_canonical",
			input: "ABCDEF0000000000000000000000000000000000000000000000000000000000",
			want:  false,
		},
		{
			name:  "non_hex_letter_is_rejected",
			input: "g000000000000000000000000000000000000000000000000000000000000000",
			want:  false,
		},
		{
			name:  "empty_string_is_rejected",
			input: "",
			want:  false,
		},
		{
			name:  "plaintext_secret_is_rejected",
			input: "my-super-secret-api-key",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got := IsSHA256Hex(tt.input)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSHA256IfNeeded — OWASP API2:2023 Broken Authentication: a plaintext
// secret is hashed exactly once; an already-hashed value passes through
// unchanged so re-saving a hashed column never corrupts the credential.
func TestSHA256IfNeeded(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plaintext_is_hashed",
			input: "plain-api-key",
			want:  SHA256("plain-api-key"),
		},
		{
			name:  "empty_string_is_hashed",
			input: "",
			want:  SHA256(""),
		},
		{
			name:  "already_hashed_value_passes_through_unchanged",
			input: SHA256("plain-api-key"),
			want:  SHA256("plain-api-key"),
		},
		{
			name:  "uppercase_hex_lookalike_is_treated_as_plaintext_and_hashed",
			input: "ABCDEF0000000000000000000000000000000000000000000000000000000000",
			want:  SHA256("ABCDEF0000000000000000000000000000000000000000000000000000000000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got := SHA256IfNeeded(tt.input)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSHA256IfNeeded_Idempotent — OWASP API2:2023 Broken Authentication:
// applying the function repeatedly (e.g. node saved many times, migration
// re-run) must converge after the first hash and never change again.
func TestSHA256IfNeeded_Idempotent(t *testing.T) {
	// ARRANGE
	const plaintext = "rotate-me"

	// ACT
	once := SHA256IfNeeded(plaintext)
	twice := SHA256IfNeeded(once)
	thrice := SHA256IfNeeded(twice)

	// ASSERT
	require.Equal(t, SHA256(plaintext), once, "first application hashes the plaintext")
	assert.Equal(t, once, twice, "second application must not re-hash an existing digest")
	assert.Equal(t, once, thrice, "the function must be a fixed point after the first hash")
	assert.NotEqual(t, plaintext, once, "the plaintext must never survive")
}
