package postusers

import (
	"os"
	"testing"

	"github.com/gameap/gameap/pkg/auth"
)

// TestMain installs a noop password blocklist so the handler tests can keep
// using simple literals like "password1234" without tripping the common-
// password check (auth.ValidatePassword consults the package-level
// blocklist). The policy itself is exercised in
// `internal/api/router_security_password_policy_test.go` and
// `pkg/auth/policy_test.go`.
func TestMain(m *testing.M) {
	auth.SetPasswordBlocklist(auth.NoopBlocklist{})
	auth.SetAllowWeakPasswords(false)

	os.Exit(m.Run())
}
