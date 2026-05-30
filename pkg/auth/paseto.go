package auth

import (
	"log/slog"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

type PASETOService struct {
	key    paseto.V4SymmetricKey
	parser *paseto.Parser
}

func NewPASETOService(secretKey []byte) (*PASETOService, error) {
	// Append to 32 bytes if the key is shorter
	if len(secretKey) < 32 {
		slog.Warn(
			"Auth secret key is shorter than 32 bytes, appending '0' to the key",
			slog.Int("length", len(secretKey)),
		)

		for len(secretKey) < 32 {
			secretKey = append(secretKey, '0')
		}
	}

	// Trim to 32 bytes if the key is longer
	if len(secretKey) > 32 {
		slog.Warn(
			"Auth secret key is longer than 32 bytes, trimming the key to 32 bytes",
			slog.Int("length", len(secretKey)),
		)

		secretKey = secretKey[:32]
	}

	key, err := paseto.V4SymmetricKeyFromBytes(secretKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate symmetric key")
	}

	parser := paseto.NewParser()

	return &PASETOService{
		key:    key,
		parser: &parser,
	}, nil
}

func (p *PASETOService) GenerateTokenForUser(user *domain.User, tokenDuration time.Duration) (string, error) {
	return p.generateToken(user, tokenDuration, ""), nil
}

func (p *PASETOService) GenerateMFAEnrollmentToken(user *domain.User, tokenDuration time.Duration) (string, error) {
	return p.generateToken(user, tokenDuration, ScopeMFAEnrollment), nil
}

func (p *PASETOService) generateToken(user *domain.User, tokenDuration time.Duration, scope string) string {
	token := paseto.NewToken()

	token.SetJti(xid.New().String())
	token.SetIssuedAt(time.Now())
	token.SetNotBefore(time.Now())
	token.SetExpiration(time.Now().Add(tokenDuration))
	token.SetIssuer("gameap-api")
	token.SetSubject(createSubjectFromLogin(user.Login))

	if scope != "" {
		token.SetString("scope", scope)
	}

	return token.V4Encrypt(p.key, nil)
}

func (p *PASETOService) ValidateToken(tokenString string) (Claims, error) {
	token, err := p.parser.ParseV4Local(p.key, tokenString, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse token")
	}

	return pasetoClaims{token: token}, nil
}

// pasetoClaims adapts a parsed paseto.Token to the local auth.Claims interface
// (which exposes both subject and expiration in a transport-agnostic way).
type pasetoClaims struct {
	token *paseto.Token
}

func (c pasetoClaims) GetSubject() (string, error) {
	return c.token.GetSubject()
}

func (c pasetoClaims) GetExpirationTime() (*time.Time, error) {
	exp, err := c.token.GetExpiration()
	if err != nil {
		return nil, err //nolint:wrapcheck // surfacing library error verbatim
	}

	return &exp, nil
}

func (c pasetoClaims) GetScope() (string, error) {
	// GetString errors when the "scope" claim is absent (or not a string);
	// for us that simply means an ordinary, unscoped session.
	scope, err := c.token.GetString("scope")
	if err != nil {
		return "", nil //nolint:nilerr // an absent/!string "scope" claim is an unscoped session, not a failure
	}

	return scope, nil
}
