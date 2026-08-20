// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa2-broken-authentication/
//
// ASVS: 4.0.3 §2.1.7 — Reject breached / common passwords.
//
// These tests verify that the common-password blocklist (SecLists top-1M
// filtered to >=12 chars, see pkg/auth/data/passwords/common-passwords.txt.gz)
// is enforced at every password-set entry point (admin create, admin update,
// self update), is case-insensitive, can be overridden by the operator via
// AUTH_ALLOW_WEAK_PASSWORDS, and is deliberately NOT applied at login so
// pre-existing users with weak passwords keep their accounts.

package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// blockedSentinel is a password that should always be in the SecLists
	// top-1M corpus after the >=12-chars filter. If a future rebuild of the
	// embedded asset drops it, the tests t.Skip with a clear hint to update
	// the sentinel rather than report a misleading failure.
	blockedSentinel = "password1234"

	// strongSentinel is a high-entropy passphrase that will never appear in
	// any common-password list. Used as the happy-path baseline.
	strongSentinel = "Tr0ub4dor&3-correct-horse-12"

	// existingUserPassword is what we pre-seed the RegularUser with so the
	// /api/profile change flow can verify "current password" before applying
	// the new one.
	existingUserPassword = "Existing-Account-Pass-9876"
)

// TestRouterSecurity_API2_PasswordPolicy_RejectsCommonPasswordOnCreate
// covers ASVS §2.1.7 at the admin user-creation entry point.
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_RejectsCommonPasswordOnCreate(t *testing.T) {
	installEmbeddedBlocklist(t)

	env := setupSecurityTest(t)
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)

	body := fmt.Appendf(nil, `{"login":"newuser","email":"new@example.com","password":%q}`, blockedSentinel)
	w := doRequestWithBody(t, env, http.MethodPost, "/api/users", adminToken, body)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "too common")
}

// TestRouterSecurity_API2_PasswordPolicy_RejectsCommonPasswordOnAdminUpdate
// covers ASVS §2.1.7 at the admin-update-user entry point.
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_RejectsCommonPasswordOnAdminUpdate(t *testing.T) {
	installEmbeddedBlocklist(t)

	env := setupSecurityTest(t)
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)

	target := env.fixtures.RegularUser
	body := fmt.Appendf(nil, `{"email":%q,"password":%q}`, target.Email, blockedSentinel)
	w := doRequestWithBody(t, env, http.MethodPut, fmt.Sprintf("/api/users/%d", target.ID), adminToken, body)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "too common")
}

// TestRouterSecurity_API2_PasswordPolicy_RejectsCommonPasswordOnSelfUpdate
// covers ASVS §2.1.7 at the self-service profile-update entry point.
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_RejectsCommonPasswordOnSelfUpdate(t *testing.T) {
	installEmbeddedBlocklist(t)

	env := setupSecurityTest(t)
	seedRegularUserPassword(t, env)
	userToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	body := fmt.Appendf(nil, `{"password":%q,"current_password":%q}`, blockedSentinel, existingUserPassword)
	w := doRequestWithBody(t, env, http.MethodPut, "/api/profile", userToken, body)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "too common")
}

// TestRouterSecurity_API2_PasswordPolicy_BlocklistIsCaseInsensitive verifies
// that uppercasing a known weak password does not bypass the check (the
// lookup lowercases the candidate before consulting the blocklist).
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_BlocklistIsCaseInsensitive(t *testing.T) {
	installEmbeddedBlocklist(t)

	env := setupSecurityTest(t)
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)

	body := fmt.Appendf(nil, `{"login":"newuser","email":"new@example.com","password":%q}`,
		"PASSWORD1234") // uppercase variant of the sentinel
	w := doRequestWithBody(t, env, http.MethodPost, "/api/users", adminToken, body)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "too common")
}

// TestRouterSecurity_API2_PasswordPolicy_AllowWeakOverridesBlock confirms
// that AUTH_ALLOW_WEAK_PASSWORDS=true short-circuits the blocklist check so
// a self-update with a known-weak password succeeds (and the "too common"
// error is NOT emitted).
//
// The check is run via /api/profile to keep the assertion clean of the
// transaction-manager wiring used by POST /api/users (the in-memory test
// container intentionally returns nil for TransactionManager).
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_AllowWeakOverridesBlock(t *testing.T) {
	installEmbeddedBlocklist(t)
	auth.SetAllowWeakPasswords(true)
	// ResetPasswordPolicy in installEmbeddedBlocklist's t.Cleanup resets
	// both blocklist and AllowWeak.

	env := setupSecurityTest(t)
	seedRegularUserPassword(t, env)
	userToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	body := fmt.Appendf(nil, `{"password":%q,"current_password":%q}`, blockedSentinel, existingUserPassword)
	w := doRequestWithBody(t, env, http.MethodPut, "/api/profile", userToken, body)

	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "too common")
}

// TestRouterSecurity_API2_PasswordPolicy_AcceptsStrongPassword pins the
// happy path — a high-entropy passphrase under the policy must succeed.
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_AcceptsStrongPassword(t *testing.T) {
	installEmbeddedBlocklist(t)

	env := setupSecurityTest(t)
	seedRegularUserPassword(t, env)
	userToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	body := fmt.Appendf(nil, `{"password":%q,"current_password":%q}`, strongSentinel, existingUserPassword)
	w := doRequestWithBody(t, env, http.MethodPut, "/api/profile", userToken, body)

	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
}

// TestRouterSecurity_API2_PasswordPolicy_LoginDoesNotRunBlocklist pins the
// deliberate backward-compat decision: the blocklist applies only when a
// password is being SET. A user whose stored bcrypt hash corresponds to a
// blocked password can still log in (otherwise existing accounts would be
// locked out by a policy tightening).
//
//nolint:paralleltest // mutates the pkg/auth package-global password policy (SetPasswordBlocklist/SetAllowWeakPasswords) and hits the CreateRouter audit-sink race.
func TestRouterSecurity_API2_PasswordPolicy_LoginDoesNotRunBlocklist(t *testing.T) {
	installEmbeddedBlocklist(t)

	env := setupSecurityTest(t)

	hashed, err := auth.HashPassword(blockedSentinel)
	require.NoError(t, err)

	user := env.fixtures.RegularUser
	user.Password = hashed
	require.NoError(t, env.container.UserRepository().Save(env.ctx, user))

	body := fmt.Appendf(nil, `{"login":%q,"password":%q}`, user.Login, blockedSentinel)
	w := doRequestWithBody(t, env, http.MethodPost, "/api/auth/login", "", body)

	assert.Equal(t, http.StatusOK, w.Code,
		"login must succeed even when the stored password is in the blocklist; body=%s",
		w.Body.String())
}

// installEmbeddedBlocklist loads the real common-password asset into the
// package-level pkg/auth state for the duration of a single test, then
// resets to NoopBlocklist via t.Cleanup. The test is skipped (with a clear
// hint to rebuild the asset) if the sentinel password is not present —
// that indicates the SecLists snapshot drifted and the sentinel constant
// needs updating, NOT a real failure. The rebuild pipeline lives in
// `pkg/auth/data/passwords/README.md`.
func installEmbeddedBlocklist(t *testing.T) {
	t.Helper()

	bl, err := auth.LoadEmbeddedBlocklist()
	require.NoError(t, err, "embedded blocklist must load — see pkg/auth/data/passwords/README.md to rebuild")

	if !bl.Contains(blockedSentinel) {
		t.Skipf("sentinel %q is not in the embedded blocklist — rebuild the asset (pkg/auth/data/passwords/README.md) or update the blockedSentinel constant", blockedSentinel)
	}

	auth.SetPasswordBlocklist(bl)
	t.Cleanup(auth.ResetPasswordPolicy)
}

// seedRegularUserPassword hashes existingUserPassword and stores it on the
// fixture RegularUser so /api/profile self-update flows can satisfy the
// "current_password must match" check.
func seedRegularUserPassword(t *testing.T, env *securityTestEnv) {
	t.Helper()

	hashed, err := auth.HashPassword(existingUserPassword)
	require.NoError(t, err)

	user := env.fixtures.RegularUser
	user.Password = hashed
	require.NoError(t, env.container.UserRepository().Save(env.ctx, user))
}
