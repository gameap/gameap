package snooze

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

// adminChecker is the subset of the RBAC service used to decide whether the
// current user holds the admin ability, which gates the MFA nudge.
type adminChecker interface {
	Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
}
