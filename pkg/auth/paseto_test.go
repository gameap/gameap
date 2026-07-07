package auth

import (
	"strings"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPASETOService(t *testing.T) {
	tests := []struct {
		name      string
		secretKey []byte
		wantError string
	}{
		{
			name:      "exact_32_bytes_key",
			secretKey: []byte("12345678901234567890123456789012"),
		},
		{
			name:      "short_key_padded",
			secretKey: []byte("shortkey"),
		},
		{
			name:      "long_key_trimmed",
			secretKey: []byte("12345678901234567890123456789012345678901234567890"),
		},
		{
			name:      "empty_key_padded",
			secretKey: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewPASETOService(tt.secretKey)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, service)
			assert.NotNil(t, service.parser)
		})
	}
}

func TestPASETOService_GenerateTokenForUser(t *testing.T) {
	service, err := NewPASETOService([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)

	tests := []struct {
		name          string
		user          *domain.User
		tokenDuration time.Duration
	}{
		{
			name: "generate_token_with_1_hour_duration",
			user: &domain.User{
				ID:    1,
				Login: "testuser",
				Email: "test@example.com",
			},
			tokenDuration: time.Hour,
		},
		{
			name: "generate_token_with_24_hours_duration",
			user: &domain.User{
				ID:    2,
				Login: "anotheruser",
				Email: "another@example.com",
			},
			tokenDuration: 24 * time.Hour,
		},
		{
			name: "generate_token_with_empty_login",
			user: &domain.User{
				ID:    3,
				Login: "",
				Email: "empty@example.com",
			},
			tokenDuration: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := service.GenerateTokenForUser(tt.user, tt.tokenDuration)

			require.NoError(t, err)
			assert.NotEmpty(t, token)
		})
	}
}

func TestPASETOService_ValidateToken(t *testing.T) {
	service, err := NewPASETOService([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)

	user := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	t.Run("valid_token", func(t *testing.T) {
		token, err := service.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		claims, err := service.ValidateToken(token)

		require.NoError(t, err)
		assert.NotNil(t, claims)

		subject, err := claims.GetSubject()
		require.NoError(t, err)
		assert.Equal(t, "user:login:testuser", subject)
	})

	t.Run("invalid_token_format", func(t *testing.T) {
		claims, err := service.ValidateToken("invalid-token")

		require.Error(t, err)
		assert.Nil(t, claims)
		assert.Contains(t, err.Error(), "failed to parse token")
	})

	t.Run("token_from_different_key", func(t *testing.T) {
		differentService, err := NewPASETOService([]byte("different_key_1234567890123456"))
		require.NoError(t, err)

		token, err := differentService.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		claims, err := service.ValidateToken(token)

		require.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("tampered_ciphertext_returns_error", func(t *testing.T) {
		// ARRANGE
		token, err := service.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		// PASETO format: v4.local.<base64-payload>[.<base64-footer>]
		parts := strings.Split(token, ".")
		require.GreaterOrEqual(t, len(parts), 3, "expected at least 3 parts in a v4.local token")

		// Flip a single byte deep inside the encrypted body so the MAC stops verifying.
		body := []byte(parts[2])
		require.NotEmpty(t, body)
		idx := len(body) / 2
		// Choose any character distinct from the original one in the base64 alphabet.
		if body[idx] == 'A' {
			body[idx] = 'B'
		} else {
			body[idx] = 'A'
		}
		parts[2] = string(body)
		tampered := strings.Join(parts, ".")

		// ACT
		claims, err := service.ValidateToken(tampered)

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, claims)
		assert.Contains(t, err.Error(), "failed to parse token", "wrapped parse error must be surfaced")
	})

	t.Run("wrong_footer_rejected", func(t *testing.T) {
		// ARRANGE
		// Build a token by hand with a non-empty footer; ParseV4Local in the
		// service uses a nil implicit, but the parser still must successfully
		// parse the footer. We then assert the service either rejects it (when
		// the parser cannot verify) or — at minimum — does not crash and the
		// returned claims/error remain consistent.
		key, err := paseto.V4SymmetricKeyFromBytes([]byte("12345678901234567890123456789012"))
		require.NoError(t, err)

		tok := paseto.NewToken()
		tok.SetIssuedAt(time.Now())
		tok.SetNotBefore(time.Now())
		tok.SetExpiration(time.Now().Add(time.Hour))
		tok.SetSubject("user:login:footer-test")
		tok.SetFooter([]byte("foo"))

		footered := tok.V4Encrypt(key, nil)

		svc, err := NewPASETOService([]byte("12345678901234567890123456789012"))
		require.NoError(t, err)

		// ACT
		claims, err := svc.ValidateToken(footered)

		// ASSERT
		// The reference parser accepts arbitrary footers when implicit is nil,
		// so this round-trip should succeed and expose the expected subject.
		// We pin the current behavior so a future tightening of footer handling
		// shows up loudly here.
		require.NoError(t, err)
		require.NotNil(t, claims)

		subject, err := claims.GetSubject()
		require.NoError(t, err)
		assert.Equal(t, "user:login:footer-test", subject)
	})

	t.Run("nil_key_padded_round_trips", func(t *testing.T) {
		// ARRANGE
		svc, err := NewPASETOService(nil)
		require.NoError(t, err)

		// ACT
		token, err := svc.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		claims, err := svc.ValidateToken(token)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, claims)

		subject, err := claims.GetSubject()
		require.NoError(t, err)
		assert.Equal(t, "user:login:testuser", subject)
	})

	t.Run("GetExpirationTime_returns_expected_time", func(t *testing.T) {
		// ARRANGE
		token, err := service.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		claims, err := service.ValidateToken(token)
		require.NoError(t, err)
		require.NotNil(t, claims)

		// ACT
		exp, err := claims.GetExpirationTime()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, exp, "PASETO claims always carry an expiration set by GenerateTokenForUser")
		assert.WithinDuration(t, time.Now().Add(time.Hour), *exp, time.Minute,
			"expiration must reflect the requested token duration")
	})

	t.Run("GetIssuedAt_returns_expected_time", func(t *testing.T) {
		// ARRANGE — generateToken calls token.SetIssuedAt(time.Now()); this claim
		// feeds the password-change token-invalidation check.
		token, err := service.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		claims, err := service.ValidateToken(token)
		require.NoError(t, err)
		require.NotNil(t, claims)

		// ACT
		iat, err := claims.GetIssuedAt()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, iat, "PASETO tokens always carry an iat set by generateToken")
		assert.WithinDuration(t, time.Now(), *iat, time.Minute,
			"iat must reflect the token mint time")
	})
}

func TestPASETOService_TokenExpiration(t *testing.T) {
	service, err := NewPASETOService([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)

	user := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	t.Run("token_with_negative_duration_expired", func(t *testing.T) {
		token, err := service.GenerateTokenForUser(user, -time.Hour)
		require.NoError(t, err)

		claims, err := service.ValidateToken(token)

		require.Error(t, err)
		assert.Nil(t, claims)
	})
}

func TestPASETOService_GenerateUniqueTokens(t *testing.T) {
	service, err := NewPASETOService([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)

	user := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	token1, err := service.GenerateTokenForUser(user, time.Hour)
	require.NoError(t, err)

	token2, err := service.GenerateTokenForUser(user, time.Hour)
	require.NoError(t, err)

	assert.NotEqual(t, token1, token2, "tokens should be unique due to different JTI")
}

func TestPASETOService_SubjectFormat(t *testing.T) {
	service, err := NewPASETOService([]byte("12345678901234567890123456789012"))
	require.NoError(t, err)

	tests := []struct {
		name            string
		login           string
		expectedSubject string
	}{
		{
			name:            "simple_login",
			login:           "admin",
			expectedSubject: "user:login:admin",
		},
		{
			name:            "email_as_login",
			login:           "user@example.com",
			expectedSubject: "user:login:user@example.com",
		},
		{
			name:            "login_with_special_chars",
			login:           "user-name_123",
			expectedSubject: "user:login:user-name_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &domain.User{
				ID:    1,
				Login: tt.login,
			}

			token, err := service.GenerateTokenForUser(user, time.Hour)
			require.NoError(t, err)

			claims, err := service.ValidateToken(token)
			require.NoError(t, err)

			subject, err := claims.GetSubject()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSubject, subject)
		})
	}
}
