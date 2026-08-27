package login

import (
	"context"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
)

// captchaVerifier is the subset of *captcha.Service the login handler needs.
// A nil verifier (or one whose Enabled() is false) skips captcha entirely.
type captchaVerifier interface {
	Enabled() bool
	Verify(ctx context.Context, token, remoteIP string) error
}

// adminChecker is the subset of the RBAC service the login handler needs to
// decide whether a user holds the admin ability, which gates the MFA nudge.
type adminChecker interface {
	Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
}

// TwoFactorChallengeStore is the narrow cache surface IssueTwoFactorChallenge
// needs. Both the password login and the SSO ticket exchange satisfy it.
type TwoFactorChallengeStore interface {
	Set(ctx context.Context, key string, value any, options ...cache.Option) error
}

// MFANudgeSaver is the narrow repository surface EvaluateMFANudge needs: it
// only ever writes back the first-shown timestamp it just stamped.
type MFANudgeSaver interface {
	Save(ctx context.Context, user *domain.User) error
}
