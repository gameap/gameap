package twofactor

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAppKey = "test-application-secret-key-for-2fa"

func newTestManager(t *testing.T, now time.Time) *Manager {
	t.Helper()

	m, err := NewManager([]byte(testAppKey), WithClock(func() time.Time { return now }))
	require.NoError(t, err)

	return m
}

func TestCipher_round_trip(t *testing.T) {
	c, err := NewCipher([]byte(testAppKey))
	require.NoError(t, err)

	enc, err := c.Encrypt("JBSWY3DPEHPK3PXP")
	require.NoError(t, err)
	assert.NotEqual(t, "JBSWY3DPEHPK3PXP", enc)

	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	assert.Equal(t, "JBSWY3DPEHPK3PXP", dec)
}

func TestCipher_encrypt_is_non_deterministic(t *testing.T) {
	c, err := NewCipher([]byte(testAppKey))
	require.NoError(t, err)

	a, err := c.Encrypt("same-secret")
	require.NoError(t, err)
	b, err := c.Encrypt("same-secret")
	require.NoError(t, err)

	assert.NotEqual(t, a, b, "fresh nonce must make ciphertexts differ")
}

func TestCipher_decrypt_errors(t *testing.T) {
	tests := []struct {
		name       string
		ciphertext string
		wantError  string
	}{
		{name: "not_base64", ciphertext: "!!!not-base64!!!", wantError: "failed to decode ciphertext"},
		{name: "too_short", ciphertext: "YWJj", wantError: "ciphertext too short"},
	}

	c, err := NewCipher([]byte(testAppKey))
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Decrypt(tt.ciphertext)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestCipher_wrong_key_cannot_decrypt(t *testing.T) {
	enc, err := mustCipher(t, "key-one").Encrypt("secret")
	require.NoError(t, err)

	_, err = mustCipher(t, "key-two").Decrypt(enc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt two-factor secret")
}

func TestNewCipher_empty_key(t *testing.T) {
	_, err := NewCipher(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-factor encryption key is empty")
}

func TestManager_GenerateSecret(t *testing.T) {
	m := newTestManager(t, time.Now())

	secret, uri, err := m.GenerateSecret("admin@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, secret)
	assert.Contains(t, uri, "otpauth://totp/")
	assert.Contains(t, uri, "issuer=GameAP")
}

func TestManager_ValidateTOTP(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	m := newTestManager(t, now)

	secret, _, err := m.GenerateSecret("admin")
	require.NoError(t, err)
	encSecret, err := m.EncryptSecret(secret)
	require.NoError(t, err)

	currentStep := now.Unix() / int64(totpPeriod)
	validCode, err := totp.GenerateCodeCustom(secret, now, validateOpts())
	require.NoError(t, err)

	t.Run("valid_code_accepted_and_step_returned", func(t *testing.T) {
		ok, step, err := m.ValidateTOTP(encSecret, validCode, nil)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, currentStep, step)
	})

	t.Run("replay_of_used_step_rejected", func(t *testing.T) {
		used := currentStep
		ok, _, err := m.ValidateTOTP(encSecret, validCode, &used)
		require.NoError(t, err)
		assert.False(t, ok, "a code at or below lastStep must be rejected")
	})

	t.Run("wrong_code_rejected", func(t *testing.T) {
		ok, _, err := m.ValidateTOTP(encSecret, "000000", nil)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("previous_step_within_skew_accepted", func(t *testing.T) {
		prev := now.Add(-time.Duration(totpPeriod) * time.Second)
		prevCode, err := totp.GenerateCodeCustom(secret, prev, validateOpts())
		require.NoError(t, err)

		ok, step, err := m.ValidateTOTP(encSecret, prevCode, nil)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, currentStep-1, step)
	})
}

func TestManager_RecoveryCodes(t *testing.T) {
	m := newTestManager(t, time.Now())

	plain, encoded, err := m.GenerateRecoveryCodes()
	require.NoError(t, err)
	require.Len(t, plain, RecoveryCodeCount)
	assert.NotEmpty(t, encoded)
	assert.NotContains(t, encoded, plain[0], "stored blob must not contain plaintext codes")

	t.Run("consume_then_single_use", func(t *testing.T) {
		updated, ok, err := m.ConsumeRecoveryCode(encoded, plain[0])
		require.NoError(t, err)
		require.True(t, ok)

		_, okAgain, err := m.ConsumeRecoveryCode(updated, plain[0])
		require.NoError(t, err)
		assert.False(t, okAgain, "a recovery code must work only once")
	})

	t.Run("tolerates_formatting", func(t *testing.T) {
		_, ok, err := m.ConsumeRecoveryCode(encoded, "  "+plain[1]+"  ")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("unknown_code_rejected", func(t *testing.T) {
		_, ok, err := m.ConsumeRecoveryCode(encoded, "zzzzz-zzzzz")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestChallengeToken_helpers(t *testing.T) {
	token := ChallengeTokenPrefix + "abc123"
	assert.True(t, IsChallengeToken(token))
	assert.False(t, IsChallengeToken("glst_abc123"))
	assert.False(t, IsChallengeToken("eyJhbGci"))

	keyA := ChallengeCacheKey(token)
	keyB := ChallengeCacheKey(ChallengeTokenPrefix + "different")
	assert.NotEqual(t, keyA, keyB)
	assert.NotContains(t, keyA, "abc123", "raw secret must never appear in the cache key")

	encoded, err := MarshalChallengePayload(ChallengePayload{UserID: 7, Login: "admin", Remember: true})
	require.NoError(t, err)

	got, err := UnmarshalChallengePayload(encoded)
	require.NoError(t, err)
	assert.Equal(t, uint(7), got.UserID)
	assert.True(t, got.Remember)
}

func mustCipher(t *testing.T, key string) *Cipher {
	t.Helper()

	c, err := NewCipher([]byte(key))
	require.NoError(t, err)

	return c
}
