package base_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/base/mocks"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var errRolesLookupFailed = errors.New("db is down")

func TestEnsureRolesAllowedForSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		session        *auth.Session
		roles          []string
		adminRoles     []string
		adminRolesErr  error
		expectLookup   bool
		wantError      string
		wantHTTPStatus int
	}{
		{
			name:         "token_session_assigning_regular_role",
			session:      tokenSession(),
			roles:        []string{"user"},
			adminRoles:   []string{"admin"},
			expectLookup: true,
		},
		{
			name:           "token_session_assigning_admin_role",
			session:        tokenSession(),
			roles:          []string{"user", "admin"},
			adminRoles:     []string{"admin"},
			expectLookup:   true,
			wantError:      "personal access tokens cannot assign administrative roles",
			wantHTTPStatus: http.StatusForbidden,
		},
		{
			name:    "token_session_assigning_renamed_admin_role",
			session: tokenSession(),
			roles:   []string{"superuser"},
			// The ability, not the name, makes a role administrative.
			adminRoles:     []string{"superuser"},
			expectLookup:   true,
			wantError:      "personal access tokens cannot assign administrative roles",
			wantHTTPStatus: http.StatusForbidden,
		},
		{
			name:         "interactive_session_may_assign_admin_role",
			session:      interactiveSession(),
			roles:        []string{"admin"},
			expectLookup: false,
		},
		{
			name:         "empty_roles_skip_the_lookup",
			session:      tokenSession(),
			roles:        nil,
			expectLookup: false,
		},
		{
			name:          "lookup_failure_denies",
			session:       tokenSession(),
			roles:         []string{"user"},
			adminRolesErr: errRolesLookupFailed,
			expectLookup:  true,
			wantError:     "failed to list administrative roles",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctrl := gomock.NewController(t)
			rbac := mocks.NewMockRBAC(ctrl)

			ctx := auth.ContextWithSession(context.Background(), test.session)

			if test.expectLookup {
				rbac.EXPECT().
					AdministrativeRoles(gomock.Any()).
					Return(test.adminRoles, test.adminRolesErr)
			}

			// ACT
			err := base.EnsureRolesAllowedForSession(ctx, rbac, test.roles)

			// ASSERT
			if test.wantError == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantError)

			if test.wantHTTPStatus != 0 {
				var httpErr *api.WrappedError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, test.wantHTTPStatus, httpErr.HTTPStatus())
			}
		})
	}
}

func tokenSession() *auth.Session {
	return &auth.Session{
		User:  &domain.User{ID: 7, Login: "billing"},
		Token: &domain.PersonalAccessToken{ID: 3, Name: "whmcs"},
	}
}

func interactiveSession() *auth.Session {
	return &auth.Session{User: &domain.User{ID: 1, Login: "admin"}}
}

func TestEnsureTargetNotAdminForToken(t *testing.T) {
	t.Parallel()

	const targetUserID = uint(42)

	tests := []struct {
		name           string
		session        *auth.Session
		targetIsAdmin  bool
		canErr         error
		expectLookup   bool
		wantError      string
		wantSentinel   error
		wantHTTPStatus int
	}{
		{
			name:         "token_session_editing_regular_user",
			session:      tokenSession(),
			expectLookup: true,
		},
		{
			name:           "token_session_editing_administrator",
			session:        tokenSession(),
			targetIsAdmin:  true,
			expectLookup:   true,
			wantError:      "cannot modify administrators",
			wantSentinel:   base.ErrTokenCannotModifyAdmin,
			wantHTTPStatus: http.StatusForbidden,
		},
		{
			name:         "interactive_session_editing_administrator",
			session:      interactiveSession(),
			expectLookup: false,
		},
		{
			name:         "lookup_failure_denies",
			session:      tokenSession(),
			canErr:       errRolesLookupFailed,
			expectLookup: true,
			wantError:    "failed to check target permissions",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			rbac := mocks.NewMockRBAC(ctrl)

			ctx := auth.ContextWithSession(context.Background(), test.session)

			if test.expectLookup {
				rbac.EXPECT().
					Can(gomock.Any(), targetUserID, gomock.Any()).
					Return(test.targetIsAdmin, test.canErr)
			}

			err := base.EnsureTargetNotAdminForToken(ctx, rbac, targetUserID)

			if test.wantError == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantError)

			if test.wantSentinel != nil {
				assert.ErrorIs(t, err, test.wantSentinel)
			}

			if test.wantHTTPStatus != 0 {
				var httpErr *api.WrappedError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, test.wantHTTPStatus, httpErr.HTTPStatus())
			}
		})
	}
}

func TestEnsurePasswordChangeAllowedForSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		session         *auth.Session
		changesPassword bool
		wantError       string
		wantHTTPStatus  int
	}{
		{
			name:            "token_session_changing_password",
			session:         tokenSession(),
			changesPassword: true,
			wantError:       "cannot change passwords",
			wantHTTPStatus:  http.StatusForbidden,
		},
		{
			name:            "token_session_without_password",
			session:         tokenSession(),
			changesPassword: false,
		},
		{
			name:            "interactive_session_changing_password",
			session:         interactiveSession(),
			changesPassword: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := auth.ContextWithSession(context.Background(), test.session)

			err := base.EnsurePasswordChangeAllowedForSession(ctx, test.changesPassword)

			if test.wantError == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantError)
			assert.ErrorIs(t, err, base.ErrTokenCannotChangePassword)

			if test.wantHTTPStatus != 0 {
				var httpErr *api.WrappedError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, test.wantHTTPStatus, httpErr.HTTPStatus())
			}
		})
	}
}
