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
	t.Parallel()
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
	t.Parallel()
	c, err := NewCipher([]byte(testAppKey))
	require.NoError(t, err)

	a, err := c.Encrypt("same-secret")
	require.NoError(t, err)
	b, err := c.Encrypt("same-secret")
	require.NoError(t, err)

	assert.NotEqual(t, a, b, "fresh nonce must make ciphertexts differ")
}

func TestCipher_decrypt_errors(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			_, err := c.Decrypt(tt.ciphertext)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestCipher_wrong_key_cannot_decrypt(t *testing.T) {
	t.Parallel()
	enc, err := mustCipher(t, "key-one").Encrypt("secret")
	require.NoError(t, err)

	_, err = mustCipher(t, "key-two").Decrypt(enc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt two-factor secret")
}

func TestNewCipher_empty_key(t *testing.T) {
	t.Parallel()
	_, err := NewCipher(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two-factor encryption key is empty")
}

func TestManager_GenerateSecret(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, time.Now())

	secret, uri, err := m.GenerateSecret("admin@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, secret)
	assert.Contains(t, uri, "otpauth://totp/")
	assert.Contains(t, uri, "issuer=GameAP")
}

func TestManager_ValidateTOTP(t *testing.T) {
	t.Parallel()
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
		t.Parallel()
		ok, step, err := m.ValidateTOTP(encSecret, validCode, nil)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, currentStep, step)
	})

	t.Run("replay_of_used_step_rejected", func(t *testing.T) {
		t.Parallel()
		used := currentStep
		ok, _, err := m.ValidateTOTP(encSecret, validCode, &used)
		require.NoError(t, err)
		assert.False(t, ok, "a code at or below lastStep must be rejected")
	})

	t.Run("wrong_code_rejected", func(t *testing.T) {
		t.Parallel()
		ok, _, err := m.ValidateTOTP(encSecret, "000000", nil)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("previous_step_within_skew_accepted", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()
	m := newTestManager(t, time.Now())

	plain, encoded, err := m.GenerateRecoveryCodes()
	require.NoError(t, err)
	require.Len(t, plain, RecoveryCodeCount)
	assert.NotEmpty(t, encoded)
	assert.NotContains(t, encoded, plain[0], "stored blob must not contain plaintext codes")

	t.Run("consume_then_single_use", func(t *testing.T) {
		t.Parallel()
		updated, ok, err := m.ConsumeRecoveryCode(encoded, plain[0])
		require.NoError(t, err)
		require.True(t, ok)

		_, okAgain, err := m.ConsumeRecoveryCode(updated, plain[0])
		require.NoError(t, err)
		assert.False(t, okAgain, "a recovery code must work only once")
	})

	t.Run("tolerates_formatting", func(t *testing.T) {
		t.Parallel()
		_, ok, err := m.ConsumeRecoveryCode(encoded, "  "+plain[1]+"  ")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("unknown_code_rejected", func(t *testing.T) {
		t.Parallel()
		_, ok, err := m.ConsumeRecoveryCode(encoded, "zzzzz-zzzzz")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestChallengeToken_helpers(t *testing.T) {
	t.Parallel()
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

func TestWithIssuer_OverridesOtpauthLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		issuer     string
		wantIssuer string
	}{
		{
			name:       "custom_issuer_is_used",
			issuer:     "My Panel",
			wantIssuer: "My%20Panel",
		},
		{
			name:       "empty_issuer_keeps_the_default",
			issuer:     "",
			wantIssuer: DefaultIssuer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			m, err := NewManager([]byte(testAppKey), WithIssuer(tt.issuer))
			require.NoError(t, err)

			// ACT
			_, uri, err := m.GenerateSecret("alice")

			// ASSERT
			require.NoError(t, err)
			assert.Contains(t, uri, "issuer="+tt.wantIssuer, "otpauth URI must carry the issuer label")
		})
	}
}

func TestNewManager_RejectsEmptyAppKey(t *testing.T) {
	t.Parallel()
	// ACT
	m, err := NewManager(nil)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialise two-factor cipher")
	assert.Nil(t, m)
}

// A challenge payload has to survive whichever shape the cache backend hands
// back — the in-memory cache returns a string, Redis returns bytes.
func TestChallengePayload_RoundTripAcrossCacheShapes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	want := ChallengePayload{
		UserID:    7,
		Login:     "alice",
		Email:     "alice@example.com",
		Remember:  true,
		Attempts:  2,
		ExpiresAt: 1_700_000_300,
	}
	encoded, err := MarshalChallengePayload(want)
	require.NoError(t, err)

	tests := []struct {
		name string
		raw  any
	}{
		{name: "string_from_in_memory_cache", raw: encoded},
		{name: "bytes_from_redis_cache", raw: []byte(encoded)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got, decodeErr := UnmarshalChallengePayload(tt.raw)

			// ASSERT
			require.NoError(t, decodeErr)
			assert.Equal(t, want.UserID, got.UserID)
			assert.Equal(t, want.Login, got.Login)
			assert.Equal(t, want.Email, got.Email)
			assert.Equal(t, want.Remember, got.Remember, "the remember-me choice must survive the round trip")
			assert.Equal(t, want.Attempts, got.Attempts, "the attempt counter must survive the round trip")
			assert.Equal(t, want.ExpiresAt, got.ExpiresAt,
				"the deadline must stay an exact integer, not a float64")
		})
	}
}

func TestUnmarshalChallengePayload_Rejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       any
		wantError string
	}{
		{
			name:      "unsupported_type_is_refused",
			raw:       42,
			wantError: "unexpected 2fa challenge payload type",
		},
		{
			name:      "corrupt_json_is_refused",
			raw:       "{not-json",
			wantError: "failed to unmarshal 2fa challenge payload",
		},
		{
			name:      "nil_is_refused",
			raw:       nil,
			wantError: "unexpected 2fa challenge payload type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got, err := UnmarshalChallengePayload(tt.raw)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			assert.Equal(t, ChallengePayload{}, got, "no payload may be returned on a decode error")
		})
	}
}

func TestConsumeRecoveryCode_Rejections(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, time.Unix(1_700_000_000, 0))
	_, encoded, err := m.GenerateRecoveryCodes()
	require.NoError(t, err)

	tests := []struct {
		name      string
		encoded   string
		input     string
		wantError string
		wantOK    bool
	}{
		{
			name:    "blank_input_consumes_nothing",
			encoded: encoded,
			input:   "   ---   ",
			wantOK:  false,
		},
		{
			name:    "unknown_code_consumes_nothing",
			encoded: encoded,
			input:   "zzzz-zzzz",
			wantOK:  false,
		},
		{
			name:    "empty_code_set_consumes_nothing",
			encoded: "",
			input:   "abcd-efgh",
			wantOK:  false,
		},
		{
			name:      "corrupt_code_set_is_reported",
			encoded:   "{not-json",
			input:     "abcd-efgh",
			wantError: "failed to unmarshal recovery codes",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			updated, ok, consumeErr := m.ConsumeRecoveryCode(tt.encoded, tt.input)

			// ASSERT
			assert.Equal(t, tt.wantOK, ok)

			if tt.wantError != "" {
				require.Error(t, consumeErr)
				assert.Contains(t, consumeErr.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, consumeErr)
			}

			assert.Equal(t, tt.encoded, updated,
				"a rejected attempt must return the stored set unchanged")
		})
	}
}

// A secret that is not valid ciphertext must be reported, never treated as a
// silently failing validation.
func TestValidateTOTP_RejectsUndecryptableSecret(t *testing.T) {
	t.Parallel()
	// ARRANGE
	m := newTestManager(t, time.Unix(1_700_000_000, 0))

	// ACT
	ok, step, err := m.ValidateTOTP("not-base64-ciphertext", "123456", nil)

	// ASSERT
	require.Error(t, err)
	assert.False(t, ok)
	assert.Zero(t, step)
}
