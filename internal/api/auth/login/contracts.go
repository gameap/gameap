package login

import "context"

// captchaVerifier is the subset of *captcha.Service the login handler needs.
// A nil verifier (or one whose Enabled() is false) skips captcha entirely.
type captchaVerifier interface {
	Enabled() bool
	Verify(ctx context.Context, token, remoteIP string) error
}
