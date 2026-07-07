package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJWTService(t *testing.T) {
	secretKey := []byte("test-secret-key")
	service := NewJWTService(secretKey)

	require.NotNil(t, service)
	assert.Equal(t, secretKey, service.secretKey)
}

func TestJWTService_GenerateTokenForUser(t *testing.T) {
	tests := []struct {
		name          string
		user          *domain.User
		tokenDuration time.Duration
	}{
		{
			name: "valid_user",
			user: &domain.User{
				ID:    1,
				Login: "testuser",
				Email: "test@example.com",
			},
			tokenDuration: time.Hour,
		},
		{
			name: "user_with_special_characters_in_login",
			user: &domain.User{
				ID:    2,
				Login: "user@domain.com",
				Email: "user@domain.com",
			},
			tokenDuration: time.Minute * 30,
		},
		{
			name: "short_duration",
			user: &domain.User{
				ID:    3,
				Login: "shortuser",
				Email: "short@example.com",
			},
			tokenDuration: time.Second,
		},
		{
			name: "long_duration",
			user: &domain.User{
				ID:    4,
				Login: "longuser",
				Email: "long@example.com",
			},
			tokenDuration: time.Hour * 24 * 365,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewJWTService([]byte("test-secret-key"))

			token, err := service.GenerateTokenForUser(tt.user, tt.tokenDuration)

			require.NoError(t, err)
			assert.NotEmpty(t, token)

			claims, err := service.ValidateToken(token)
			require.NoError(t, err)

			subject, err := claims.GetSubject()
			require.NoError(t, err)
			assert.Equal(t, "user:login:"+tt.user.Login, subject)
		})
	}
}

func TestJWTService_ValidateToken(t *testing.T) {
	secretKey := []byte("test-secret-key")
	service := NewJWTService(secretKey)
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
		require.NotNil(t, claims)

		subject, err := claims.GetSubject()
		require.NoError(t, err)
		assert.Equal(t, "user:login:testuser", subject)
	})

	t.Run("expired_token", func(t *testing.T) {
		expiredClaims := JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-id",
				Subject:   "user:login:testuser",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				Issuer:    "gameap-api",
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS384, expiredClaims)
		tokenString, err := token.SignedString(secretKey)
		require.NoError(t, err)

		claims, err := service.ValidateToken(tokenString)
		assert.Error(t, err)
		assert.Nil(t, claims)
		assert.Contains(t, err.Error(), "token is expired")
	})

	t.Run("invalid_signature", func(t *testing.T) {
		otherService := NewJWTService([]byte("different-secret-key"))
		token, err := otherService.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		claims, err := service.ValidateToken(token)
		assert.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("malformed_token", func(t *testing.T) {
		claims, err := service.ValidateToken("not-a-valid-token")
		assert.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("empty_token", func(t *testing.T) {
		claims, err := service.ValidateToken("")
		assert.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("wrong_signing_method", func(t *testing.T) {
		wrongMethodClaims := JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "test-id",
				Subject:   "user:login:testuser",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "gameap-api",
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongMethodClaims)
		tokenString, err := token.SignedString(secretKey)
		require.NoError(t, err)

		// The service issues HS384 and pins that exact method, so an HS256
		// token — even signed with the right secret — must be rejected.
		claims, err := service.ValidateToken(tokenString)
		require.Error(t, err)
		require.Nil(t, claims)
	})

	t.Run("token_with_none_algorithm", func(t *testing.T) {
		noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1c2VyOmxvZ2luOnRlc3R1c2VyIn0."

		claims, err := service.ValidateToken(noneToken)
		assert.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("token_with_rsa_algorithm_rejected", func(t *testing.T) {
		rsaClaims := JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "rsa-id",
				Subject:   "user:login:testuser",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "gameap-api",
			},
		}

		privKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		token := jwt.NewWithClaims(jwt.SigningMethodRS256, rsaClaims)
		tokenString, err := token.SignedString(privKey)
		require.NoError(t, err)

		claims, err := service.ValidateToken(tokenString)
		require.Error(t, err)
		assert.Nil(t, claims)
		assert.Contains(t, err.Error(), "unexpected signing method")
	})

	t.Run("token_without_exp_field", func(t *testing.T) {
		// ARRANGE
		// jwt/v5 by default treats `exp` as optional, so a token without it
		// should still validate. This pins that behavior so that toggling
		// jwt.WithExpirationRequired() in production is detected here.
		noExpClaims := JWTClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:       "no-exp-id",
				Subject:  "user:login:no-exp",
				IssuedAt: jwt.NewNumericDate(time.Now()),
				Issuer:   "gameap-api",
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS384, noExpClaims)
		tokenString, err := token.SignedString(secretKey)
		require.NoError(t, err)

		// ACT
		claims, err := service.ValidateToken(tokenString)

		// ASSERT
		require.NoError(t, err, "tokens without exp must validate under jwt/v5 defaults")
		require.NotNil(t, claims)

		subject, err := claims.GetSubject()
		require.NoError(t, err)
		assert.Equal(t, "user:login:no-exp", subject)

		exp, err := claims.GetExpirationTime()
		require.NoError(t, err, "missing exp must not produce an error from the adapter")
		assert.Nil(t, exp, "missing exp must surface as a nil pointer through the adapter")
	})

	t.Run("GetExpirationTime_via_adapter_returns_time", func(t *testing.T) {
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
		require.NotNil(t, exp, "JWT generated with a 1h duration must expose a non-nil expiration")
		assert.WithinDuration(t, time.Now().Add(time.Hour), *exp, time.Minute,
			"adapter must surface the JWT exp claim as a *time.Time")
	})
}

func TestJWTService_TokenClaims(t *testing.T) {
	secretKey := []byte("test-secret-key")
	service := NewJWTService(secretKey)
	user := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	token, err := service.GenerateTokenForUser(user, time.Hour)
	require.NoError(t, err)

	parsedToken, err := jwt.ParseWithClaims(token, &JWTClaims{}, func(_ *jwt.Token) (any, error) {
		return secretKey, nil
	})
	require.NoError(t, err)

	claims, ok := parsedToken.Claims.(*JWTClaims)
	require.True(t, ok)

	t.Run("has_valid_subject", func(t *testing.T) {
		assert.Equal(t, "user:login:testuser", claims.Subject)
	})

	t.Run("has_valid_issuer", func(t *testing.T) {
		assert.Equal(t, "gameap-api", claims.Issuer)
	})

	t.Run("has_issued_at", func(t *testing.T) {
		require.NotNil(t, claims.IssuedAt)
		assert.WithinDuration(t, time.Now(), claims.IssuedAt.Time, time.Minute)
	})

	t.Run("has_expiration", func(t *testing.T) {
		require.NotNil(t, claims.ExpiresAt)
		assert.WithinDuration(t, time.Now().Add(time.Hour), claims.ExpiresAt.Time, time.Minute)
	})

	t.Run("has_unique_id", func(t *testing.T) {
		assert.NotEmpty(t, claims.ID)
	})
}

func TestJWTService_UniqueTokenIDs(t *testing.T) {
	service := NewJWTService([]byte("test-secret-key"))
	user := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	tokenIDs := make(map[string]bool)

	for range 100 {
		token, err := service.GenerateTokenForUser(user, time.Hour)
		require.NoError(t, err)

		parsedToken, err := jwt.ParseWithClaims(token, &JWTClaims{}, func(_ *jwt.Token) (any, error) {
			return []byte("test-secret-key"), nil
		})
		require.NoError(t, err)

		claims, ok := parsedToken.Claims.(*JWTClaims)
		require.True(t, ok)

		assert.False(t, tokenIDs[claims.ID], "duplicate token ID found")
		tokenIDs[claims.ID] = true
	}
}

func TestCreateSubjectFromLogin(t *testing.T) {
	tests := []struct {
		name     string
		login    string
		expected string
	}{
		{
			name:     "simple_login",
			login:    "testuser",
			expected: "user:login:testuser",
		},
		{
			name:     "email_as_login",
			login:    "user@example.com",
			expected: "user:login:user@example.com",
		},
		{
			name:     "login_with_special_chars",
			login:    "user-name_123",
			expected: "user:login:user-name_123",
		},
		{
			name:     "empty_login",
			login:    "",
			expected: "user:login:",
		},
		{
			name:     "login_with_unicode",
			login:    "пользователь",
			expected: "user:login:пользователь",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createSubjectFromLogin(tt.login)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJWTClaims_ImplementsClaims(t *testing.T) {
	claims := &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user:login:testuser",
		},
	}

	// JWTClaims is wrapped via jwtClaimsAdapter to satisfy the local
	// auth.Claims interface (which exposes expiration as *time.Time, not
	// *jwt.NumericDate). The adapter is exercised through ValidateToken.
	var _ Claims = jwtClaimsAdapter{inner: claims}

	subject, err := claims.GetSubject()
	require.NoError(t, err)
	assert.Equal(t, "user:login:testuser", subject)
}
