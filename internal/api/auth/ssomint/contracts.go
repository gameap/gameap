package ssomint

import (
	"context"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
)

// tokenCache is the narrow cache surface used to store the single-use ticket.
type tokenCache interface {
	Set(ctx context.Context, key string, value any, options ...cache.Option) error
}

// adminChecker is the slice of the RBAC service this handler needs: deciding
// whether the ticket target holds panel-wide admin rights.
type adminChecker interface {
	Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
}
