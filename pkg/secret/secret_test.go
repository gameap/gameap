// Security tests for reversible at-rest credential encryption.
//
// OWASP API Security Top 10:2023 — API2:2023 Broken Authentication and
// API8:2023 Security Misconfiguration: the legacy gdaemon SSH password must
// be encrypted at rest (no plaintext credential in the database) yet still be
// recoverable to drive the daemon. Encryption must be authenticated
// (AES-256-GCM), tagged with an "enc:" prefix, and backward compatible with
// rows written before encryption was enabled. See security review finding #3b.
package secret_test

import (
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCipher_EncryptDecrypt_RoundTrip — OWASP API2:2023 / API8:2023:
// an enabled cipher emits an opaque "enc:"-prefixed token (never the
// plaintext) that decrypts back to the original value.
func TestCipher_EncryptDecrypt_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "typical_password", plaintext: "S3cr3t-P@ss!"},
		{name: "unicode_password", plaintext: "пароль-密码-🔑"},
		{name: "long_password", plaintext: strings.Repeat("x", 4096)},
		{name: "whitespace_password", plaintext: "  spaced  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			c, err := secret.NewCipher("encryption-key")
			require.NoError(t, err)

			// ACT
			enc, err := c.Encrypt(tt.plaintext)
			require.NoError(t, err)

			dec, err := c.Decrypt(enc)
			require.NoError(t, err)

			// ASSERT
			assert.True(t, strings.HasPrefix(enc, secret.EncPrefix),
				"ciphertext must carry the enc: prefix so it is distinguishable from legacy plaintext")
			assert.NotContains(t, enc, tt.plaintext,
				"the plaintext credential must never appear in the stored value")
			assert.Equal(t, tt.plaintext, dec, "decrypt must recover the exact plaintext")
		})
	}
}

// TestCipher_Encrypt_NonDeterministic — OWASP API2:2023: a fresh random
// nonce per encryption means encrypting the same secret twice yields
// different ciphertexts (no ECB-style equality leak), both decrypting back.
func TestCipher_Encrypt_NonDeterministic(t *testing.T) {
	// ARRANGE
	c, err := secret.NewCipher("k")
	require.NoError(t, err)

	// ACT
	a, err := c.Encrypt("same-secret")
	require.NoError(t, err)
	b, err := c.Encrypt("same-secret")
	require.NoError(t, err)

	// ASSERT
	assert.NotEqual(t, a, b, "GCM nonce reuse would be a critical weakness; ciphertexts must differ")

	da, err := c.Decrypt(a)
	require.NoError(t, err)
	db, err := c.Decrypt(b)
	require.NoError(t, err)
	assert.Equal(t, "same-secret", da)
	assert.Equal(t, "same-secret", db)
}

// TestCipher_Encrypt_NoOpCases — OWASP API8:2023: encryption is a no-op for
// empty input, an already-encrypted value (idempotent, never double-wrap)
// and a disabled cipher (backward compatible passthrough).
func TestCipher_Encrypt_NoOpCases(t *testing.T) {
	enabled, err := secret.NewCipher("k")
	require.NoError(t, err)

	preEncrypted, err := enabled.Encrypt("already")
	require.NoError(t, err)

	tests := []struct {
		name   string
		cipher *secret.Cipher
		input  string
		want   string
	}{
		{
			name:   "empty_input_is_returned_unchanged",
			cipher: enabled,
			input:  "",
			want:   "",
		},
		{
			name:   "already_encrypted_value_is_not_double_wrapped",
			cipher: enabled,
			input:  preEncrypted,
			want:   preEncrypted,
		},
		{
			name:   "disabled_cipher_passes_plaintext_through",
			cipher: secret.Disabled(),
			input:  "plain-pass",
			want:   "plain-pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got, err := tt.cipher.Encrypt(tt.input)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCipher_Decrypt_LegacyAndMisconfig — OWASP API2:2023 / API8:2023:
// legacy plaintext (no enc: prefix) passes through so old rows keep working;
// an enc: value with no key configured is a loud error rather than a silent
// empty credential; tampered ciphertext fails authentication.
func TestCipher_Decrypt_LegacyAndMisconfig(t *testing.T) {
	enabled, err := secret.NewCipher("k")
	require.NoError(t, err)

	validEnc, err := enabled.Encrypt("the-password")
	require.NoError(t, err)

	t.Run("legacy_plaintext_passes_through_with_enabled_cipher", func(t *testing.T) {
		// ARRANGE / ACT
		got, err := enabled.Decrypt("legacy-plaintext-password")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, "legacy-plaintext-password", got,
			"a value without the enc: prefix is legacy plaintext and must be returned as-is")
	})

	t.Run("legacy_plaintext_passes_through_with_disabled_cipher", func(t *testing.T) {
		// ARRANGE / ACT
		got, err := secret.Disabled().Decrypt("legacy-plaintext-password")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, "legacy-plaintext-password", got)
	})

	t.Run("encrypted_value_without_key_is_a_hard_error", func(t *testing.T) {
		// ARRANGE / ACT
		_, err := secret.Disabled().Decrypt(validEnc)

		// ASSERT — must be loud so a misconfiguration is not silently masked.
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ENCRYPTION_KEY is not configured")
	})

	t.Run("tampered_ciphertext_fails_authentication", func(t *testing.T) {
		// ARRANGE: flip the last base64 character of a valid token.
		tampered := validEnc[:len(validEnc)-1]
		if strings.HasSuffix(validEnc, "A") {
			tampered += "B"
		} else {
			tampered += "A"
		}

		// ACT
		_, err := enabled.Decrypt(tampered)

		// ASSERT — GCM must reject a modified ciphertext.
		require.Error(t, err)
	})

	t.Run("wrong_key_cannot_decrypt", func(t *testing.T) {
		// ARRANGE
		other, err := secret.NewCipher("a-different-key")
		require.NoError(t, err)

		// ACT
		_, err = other.Decrypt(validEnc)

		// ASSERT
		require.Error(t, err, "a value encrypted under one key must not decrypt under another")
	})

	t.Run("enc_prefixed_garbage_is_rejected_not_panicked", func(t *testing.T) {
		// ARRANGE / ACT
		_, err := enabled.Decrypt(secret.EncPrefix + "!!!not-base64!!!")

		// ASSERT
		require.Error(t, err)
	})
}

// TestCipher_Enabled — OWASP API8:2023: a cipher built from an empty key (the
// safe default when ENCRYPTION_KEY is unset) must report disabled.
func TestCipher_Enabled(t *testing.T) {
	withKey, err := secret.NewCipher("k")
	require.NoError(t, err)
	assert.True(t, withKey.Enabled(), "a configured key must enable encryption")

	emptyKey, err := secret.NewCipher("")
	require.NoError(t, err)
	assert.False(t, emptyKey.Enabled(), "an empty key must yield a disabled (passthrough) cipher")

	assert.False(t, secret.Disabled().Enabled(), "Disabled() must report not enabled")
}
