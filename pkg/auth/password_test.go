package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
	}{
		{
			name:     "simple_password",
			password: "password1234",
		},
		{
			name:     "complex_password",
			password: "P@ssw0rd!#$%^&*()",
		},
		{
			name:     "empty_password",
			password: "",
		},
		{
			name:     "long_password_72_bytes",
			password: strings.Repeat("a", 72),
		},
		{
			name:     "long_password_128_bytes",
			password: strings.Repeat("a", 128),
		},
		{
			name:     "unicode_password",
			password: "пароль密码🔐ABCDE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hash, err := HashPassword(tt.password)

			require.NoError(t, err)
			assert.NotEmpty(t, hash)
			assert.NotEqual(t, tt.password, hash)

			needsRehash, err := VerifyPassword(hash, tt.password)
			assert.NoError(t, err)
			assert.False(t, needsRehash, "freshly hashed password must not request rehash")
		})
	}
}

func TestHashPassword_UniqueHashes(t *testing.T) {
	t.Parallel()

	password := "samepassword12"

	hash1, err := HashPassword(password)
	require.NoError(t, err)

	hash2, err := HashPassword(password)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2, "hashes should be unique due to different salts")
}

// OWASP ASVS 4.0.3 §2.1.2 requires that passwords of up to 128 characters
// are accepted without truncation. The SHA-256 pre-hash in HashPassword
// keeps the bcrypt input at a fixed 44 ASCII chars regardless of length.
func TestHashPassword_RoundTrip_128Chars(t *testing.T) {
	t.Parallel()

	password := strings.Repeat("a", 128)

	hash, err := HashPassword(password)
	require.NoError(t, err)

	needsRehash, err := VerifyPassword(hash, password)
	assert.NoError(t, err)
	assert.False(t, needsRehash)
}

func TestVerifyPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		password      string
		wrongPassword string
	}{
		{
			name:          "correct_password",
			password:      "correctpassword",
			wrongPassword: "wrongpassword",
		},
		{
			name:          "empty_password",
			password:      "",
			wrongPassword: "notempty",
		},
		{
			name:          "complex_password",
			password:      "C0mpl3x!@#$%",
			wrongPassword: "simple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hash, err := HashPassword(tt.password)
			require.NoError(t, err)

			needsRehash, err := VerifyPassword(hash, tt.password)
			assert.NoError(t, err, "correct password should verify successfully")
			assert.False(t, needsRehash, "current-scheme hash should not request rehash")

			needsRehash, err = VerifyPassword(hash, tt.wrongPassword)
			assert.Error(t, err, "wrong password should not verify")
			assert.Contains(t, err.Error(), "password verification failed")
			assert.False(t, needsRehash)
		})
	}
}

// Pre-migration accounts hold bcrypt-of-raw-password hashes. Verify must keep
// authenticating them and report needsRehash=true so the caller can upgrade
// the stored hash to the SHA-256+bcrypt scheme on the next successful login.
func TestVerifyPassword_LegacyHashStillVerifiesAndSignalsRehash(t *testing.T) {
	t.Parallel()

	password := "legacy-pw-12"
	legacyHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	needsRehash, err := VerifyPassword(string(legacyHash), password)

	assert.NoError(t, err)
	assert.True(t, needsRehash, "legacy hash must request an upgrade")
}

func TestVerifyPassword_NewHashDoesNotNeedRehash(t *testing.T) {
	t.Parallel()

	password := "current-scheme-pw"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	needsRehash, err := VerifyPassword(hash, password)

	assert.NoError(t, err)
	assert.False(t, needsRehash)
}

func TestVerifyPassword_WrongPassword_BothSchemes(t *testing.T) {
	t.Parallel()

	password := "secret-password"
	wrongPassword := "wrong-password"

	newHash, err := HashPassword(password)
	require.NoError(t, err)

	legacyHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	needsRehash, err := VerifyPassword(newHash, wrongPassword)
	require.Error(t, err)
	assert.False(t, needsRehash)

	needsRehash, err = VerifyPassword(string(legacyHash), wrongPassword)
	require.Error(t, err)
	assert.False(t, needsRehash)
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hashValue  string
		password   string
		wantErrMsg string
	}{
		{
			name:       "invalid_hash_format",
			hashValue:  "notavalidhash",
			password:   "anypassword",
			wantErrMsg: "password verification failed",
		},
		{
			name:       "empty_hash",
			hashValue:  "",
			password:   "anypassword",
			wantErrMsg: "password verification failed",
		},
		{
			name:       "truncated_hash",
			hashValue:  "$2a$10$truncated",
			password:   "anypassword",
			wantErrMsg: "password verification failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			needsRehash, err := VerifyPassword(tt.hashValue, tt.password)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
			assert.False(t, needsRehash)
		})
	}
}

func TestVerifyPassword_CaseSensitive(t *testing.T) {
	t.Parallel()

	password := "CaseSensitive"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	_, err = VerifyPassword(hash, "casesensitive")
	assert.Error(t, err, "password verification should be case-sensitive")

	_, err = VerifyPassword(hash, "CASESENSITIVE")
	assert.Error(t, err, "password verification should be case-sensitive")

	needsRehash, err := VerifyPassword(hash, "CaseSensitive")
	assert.NoError(t, err, "exact password should verify")
	assert.False(t, needsRehash)
}

func TestDefaultBcryptCost(t *testing.T) {
	t.Parallel()

	assert.Equal(t, bcrypt.DefaultCost, DefaultBcryptCost)
}

// TestSetDefaultBcryptCost_AcceptsValidRange — OWASP ASVS §2.4.4 — the
// active cost must move when the operator-configured value is in the
// allowed range. The post-test restore keeps subsequent tests using the
// historical default.
//
//nolint:paralleltest // mutates the package-level active bcrypt cost shared with other tests
func TestSetDefaultBcryptCost_AcceptsValidRange(t *testing.T) {
	t.Cleanup(func() { _ = SetDefaultBcryptCost(DefaultBcryptCost) })

	for _, cost := range []int{MinBcryptCost, 12, MaxBcryptCost} {
		require.NoError(t, SetDefaultBcryptCost(cost))
		assert.Equal(t, cost, ActiveBcryptCost())
	}
}

// TestSetDefaultBcryptCost_RejectsOutOfRange — OWASP ASVS §2.4.4 — a
// misconfigured AUTH_BCRYPT_COST must be refused so a deployment cannot
// ship a weaker default than the project floor (or a cost so high it
// becomes a DoS).
//
//nolint:paralleltest // mutates the package-level active bcrypt cost shared with other tests
func TestSetDefaultBcryptCost_RejectsOutOfRange(t *testing.T) {
	t.Cleanup(func() { _ = SetDefaultBcryptCost(DefaultBcryptCost) })

	for _, cost := range []int{0, MinBcryptCost - 1, MaxBcryptCost + 1, 100} {
		err := SetDefaultBcryptCost(cost)
		assert.Errorf(t, err, "cost=%d must be rejected", cost)
	}

	// Active cost is unchanged after a failed Set.
	assert.Equal(t, DefaultBcryptCost, ActiveBcryptCost())
}

// TestHashPassword_RespectsActiveCost — OWASP ASVS §2.4.4 — a hash produced
// after SetDefaultBcryptCost(N) carries cost=N. This is the round-trip the
// login rehash flow depends on.
//
//nolint:paralleltest // mutates the package-level active bcrypt cost shared with other tests
func TestHashPassword_RespectsActiveCost(t *testing.T) {
	t.Cleanup(func() { _ = SetDefaultBcryptCost(DefaultBcryptCost) })

	require.NoError(t, SetDefaultBcryptCost(MinBcryptCost+1))

	hash, err := HashPassword("password-with-active-cost")
	require.NoError(t, err)

	got, err := HashCost(hash)
	require.NoError(t, err)
	assert.Equal(t, MinBcryptCost+1, got)
}

// TestHashCost_ReturnsErrorForGarbage — OWASP ASVS §2.4.4 — HashCost over a
// non-bcrypt string surfaces an error rather than silently returning zero,
// so the login handler can fall back to a safe "rehash anyway" decision
// for unexpected stored values.
func TestHashCost_ReturnsErrorForGarbage(t *testing.T) {
	t.Parallel()

	_, err := HashCost("not-a-bcrypt-hash")
	assert.Error(t, err)
}
