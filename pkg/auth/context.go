package auth

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type (
	SessionKey       struct{}
	DaemonSessionKey struct{}
)

type Session struct {
	ID string // session ID

	Login string // User login
	Email string // User email

	User  *domain.User
	Token *domain.PersonalAccessToken
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

type DaemonSession struct {
	Node *domain.Node
}

func DaemonSessionFromContext(ctx context.Context) *DaemonSession {
	session, _ := ctx.Value(DaemonSessionKey{}).(*DaemonSession)

	return session
}

func ContextWithDaemonSession(ctx context.Context, session *DaemonSession) context.Context {
	return context.WithValue(ctx, DaemonSessionKey{}, session)
}
