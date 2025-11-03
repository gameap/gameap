package auth

import (
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

var (
	signingMethod = jwt.SigningMethodHS384
)

type JWTClaims struct {
	jwt.RegisteredClaims
}

type JWTService struct {
	secretKey []byte
}

func NewJWTService(secretKey []byte) *JWTService {
	return &JWTService{
		secretKey: secretKey,
	}
}

func (j *JWTService) GenerateTokenForUser(user *domain.User, tokenDuration time.Duration) (string, error) {
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        xid.New().String(),
			Subject:   createSubjectFromLogin(user.Login),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "gameap-api",
		},
	}

	token := jwt.NewWithClaims(signingMethod, claims)

	result, err := token.SignedString(j.secretKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to sign token")
	}

	return result, nil
}

func (j *JWTService) ValidateToken(tokenString string) (Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return j.secretKey, nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to parse token")
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func createSubjectFromLogin(login string) string {
	b := strings.Builder{}
	b.Grow(len(login) + 11)
	b.WriteString("user:login:")
	b.WriteString(login)

	return b.String()
}
