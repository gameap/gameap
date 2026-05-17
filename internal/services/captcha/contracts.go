package captcha

import "context"

// Verifier validates a client-supplied captcha solution. A disabled verifier
// (Enabled() == false) is a no-op and lets every request through.
type Verifier interface {
	// Enabled reports whether a provider is configured. Callers gate the
	// captcha requirement on this so an unset CAPTCHA_PROVIDER is a no-op.
	Enabled() bool

	// Verify returns nil when the token is valid. A missing or rejected
	// token is reported as a 422 validation error. An upstream failure
	// returns nil when FailOpen is set, otherwise a 503-mapped error.
	Verify(ctx context.Context, token, remoteIP string) error
}
