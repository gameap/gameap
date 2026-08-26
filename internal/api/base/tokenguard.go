package base

import (
	"context"
	"net/http"
	"slices"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// EnsureRolesAllowedForSession refuses administrative role assignments when
// the caller authenticated with a personal access token.
//
// Admin routes are reachable by a scoped token (see CheckPATAbilities in the
// router) and "manage users" is one of those scopes. Without this check that
// scope would silently include "become an administrator": one request assigns
// the admin role to a fresh account. Such tokens live in third-party systems —
// a billing panel keeps one in its own database — so the blast radius of a
// leak has to stop at the scope it was granted. Interactive admin sessions are
// unaffected.
func EnsureRolesAllowedForSession(ctx context.Context, rbac RBAC, roles []string) error {
	if len(roles) == 0 {
		return nil
	}

	session := auth.SessionFromContext(ctx)
	if !session.IsTokenSession() {
		return nil
	}

	adminRoles, err := rbac.AdministrativeRoles(ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to list administrative roles")
	}

	for _, role := range roles {
		if slices.Contains(adminRoles, role) {
			return api.WrapHTTPError(
				errors.New("personal access tokens cannot assign administrative roles"),
				http.StatusForbidden,
			)
		}
	}

	return nil
}

// Sentinel errors for the target guards below, exported so a handler can tell a
// deliberate 403 denial (worth an audit entry) apart from an internal lookup
// failure returned by the same call.
var (
	ErrTokenCannotModifyAdmin    = errors.New("personal access tokens cannot modify administrators")
	ErrTokenCannotChangePassword = errors.New("personal access tokens cannot change passwords")
)

// EnsureTargetNotAdminForToken refuses any change a personal access token tries
// to make to an administrator.
//
// EnsureRolesAllowedForSession stops a token from granting the admin role, but
// says nothing about who is being edited. Without this a "manage users" token
// could still reset an administrator's password or strip their roles — the
// latter slips past the roles check entirely, since an empty list clears the
// assignments and never looks administrative. The same admin-target refusal
// already guards ssomint and ssoexchange; this applies it to user updates.
func EnsureTargetNotAdminForToken(ctx context.Context, rbac RBAC, targetUserID uint) error {
	session := auth.SessionFromContext(ctx)
	if !session.IsTokenSession() {
		return nil
	}

	isAdmin, err := rbac.Can(ctx, targetUserID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return errors.WithMessage(err, "failed to check target permissions")
	}

	if !isAdmin {
		return nil
	}

	return api.WrapHTTPError(ErrTokenCannotModifyAdmin, http.StatusForbidden)
}

// EnsurePasswordChangeAllowedForSession refuses a password change made through a
// personal access token. Provisioning a new customer is a token's job; resetting
// the password of an account that already exists is not, and allowing it would
// hand whoever steals the token a way to log in as any existing user.
func EnsurePasswordChangeAllowedForSession(ctx context.Context, changesPassword bool) error {
	if !changesPassword {
		return nil
	}

	session := auth.SessionFromContext(ctx)
	if !session.IsTokenSession() {
		return nil
	}

	return api.WrapHTTPError(ErrTokenCannotChangePassword, http.StatusForbidden)
}
