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

	Scope string `json:"scope,omitempty"`
}

// jwtClaimsAdapter wraps *JWTClaims to satisfy the local auth.Claims interface,
// which exposes expiration as *time.Time (transport-agnostic) rather than the
// JWT-specific *jwt.NumericDate.
type jwtClaimsAdapter struct {
	inner *JWTClaims
}

func (a jwtClaimsAdapter) GetSubject() (string, error) {
	return a.inner.GetSubject()
}

func (a jwtClaimsAdapter) GetExpirationTime() (*time.Time, error) {
	exp, err := a.inner.GetExpirationTime()
	if err != nil {
		return nil, err //nolint:wrapcheck // surfacing library error verbatim
	}
	if exp == nil {
		return nil, nil
	}
	t := exp.Time

	return &t, nil
}

func (a jwtClaimsAdapter) GetScope() (string, error) {
	return a.inner.Scope, nil
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
	return j.generateToken(user, tokenDuration, "")
}

func (j *JWTService) GenerateMFAEnrollmentToken(user *domain.User, tokenDuration time.Duration) (string, error) {
	return j.generateToken(user, tokenDuration, ScopeMFAEnrollment)
}

func (j *JWTService) generateToken(user *domain.User, tokenDuration time.Duration, scope string) (string, error) {
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        xid.New().String(),
			Subject:   createSubjectFromLogin(user.Login),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "gameap-api",
		},
		Scope: scope,
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
		return jwtClaimsAdapter{inner: claims}, nil
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
