package ssoexchange

import (
	"context"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
)

// tokenCache is the narrow cache surface used to consume the single-use ticket
// and, when the account has a second factor, to store the challenge that
// replaces the session. Pull consumes the ticket atomically, so a replay cannot
// read it between a separate Get and Delete.
type tokenCache interface {
	Pull(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, options ...cache.Option) error
}

// adminChecker is the slice of the RBAC service this handler needs: refusing
// to hand out a session for an administrator.
type adminChecker interface {
	Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
}
