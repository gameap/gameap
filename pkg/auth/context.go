package auth

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type SessionKey struct{}

type Session struct {
	ID string // session ID

	Login string // User login
	Email string // User email

	User  *domain.User
	Token *domain.PersonalAccessToken

	// ShortLived is true when the session was established from a single-use
	// short-lived token. The scope guard uses it to reject such tokens on
	// endpoints that did not explicitly opt in (see ShortLivedScopeMiddleware).
	ShortLived bool
}

func (s *Session) IsAuthenticated() bool {
	return s != nil && s.User != nil && s.User.ID != 0
}

func (s *Session) IsTokenSession() bool {
	return s != nil && s.Token != nil && s.Token.ID != 0
}

func SessionFromContext(ctx context.Context) *Session {
	session, _ := ctx.Value(SessionKey{}).(*Session)

	return session
}

func ContextWithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, SessionKey{}, session)
}
