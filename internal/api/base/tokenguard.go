package base

import (
	"context"
	"net/http"
	"slices"

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
