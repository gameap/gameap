package twofactorverify

import (
	"context"

	"github.com/gameap/gameap/internal/cache"
)

// twoFactorValidator is the slice of the two-factor manager this handler
// needs: validate a TOTP code (with replay guard) or consume a recovery code.
type twoFactorValidator interface {
	ValidateTOTP(encryptedSecret, code string, lastStep *int64) (ok bool, usedStep int64, err error)
	ConsumeRecoveryCode(encoded, input string) (updated string, ok bool, err error)
}

// tokenCache is the narrow cache surface used to read, refresh and consume the
// single-use challenge entry.
type tokenCache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, options ...cache.Option) error
	Delete(ctx context.Context, key string) error
}
