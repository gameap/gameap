// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins POST /api/profile/2fa/snooze: a snooze persists the 24h deadline in the
// user's metadata bag and the recomputed recommendation suppresses the modal
// for the snooze window.
package snooze

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAdminChecker struct {
	isAdmin bool
	err     error
}

func (s stubAdminChecker) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return s.isAdmin, s.err
}

var errRBACUnavailable = errors.New("rbac backend unavailable")

//nolint:unparam // require is kept for symmetry with the login package helper and to document intent
func nudgeConfig(require bool, hardFailDays int) config.Config {
	var cfg config.Config
	cfg.Auth.RequireMFAForAdmins = require
	cfg.Auth.MFAHardFailDays = hardFailDays

	return cfg
}

func TestHandler_Snooze(t *testing.T) {
	ctx := context.Background()
	clock := func() time.Time { return time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC) }

	t.Run("persists_snooze_and_suppresses_modal", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		shown := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		admin := &domain.User{Login: "admin", Email: "a@example.com", Password: "x"}
		admin.SetMFAFirstShownAt(&shown)
		require.NoError(t, repo.Save(ctx, admin))

		handler := NewHandler(
			repo, mfanudge.New(nudgeConfig(true, 30), clock), stubAdminChecker{isAdmin: true}, api.NewResponder(),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/snooze", nil)
		req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{User: admin, Login: admin.Login}))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		users, err := repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 1)
		require.NotNil(t, users[0].MFASnoozedUntil(), "the snooze deadline must be persisted")
		assert.True(t, users[0].MFASnoozedUntil().After(clock()))

		var resp struct {
			MFANudge *mfanudge.View `json:"mfa_nudge"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.MFANudge)
		assert.True(t, resp.MFANudge.Required)
		assert.False(t, resp.MFANudge.ShowNow, "the snooze must suppress the modal for its window")
	})

	t.Run("unauthenticated_is_rejected", func(t *testing.T) {
		handler := NewHandler(
			inmemory.NewUserRepository(), mfanudge.New(nudgeConfig(true, 30), clock),
			stubAdminChecker{isAdmin: true}, api.NewResponder(),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/snooze", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("non_admin_gets_null_nudge", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		user := &domain.User{Login: "regular", Email: "r@example.com", Password: "x"}
		require.NoError(t, repo.Save(ctx, user))

		handler := NewHandler(
			repo, mfanudge.New(nudgeConfig(true, 30), clock), stubAdminChecker{isAdmin: false}, api.NewResponder(),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/snooze", nil)
		req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{User: user, Login: user.Login}))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp struct {
			MFANudge *mfanudge.View `json:"mfa_nudge"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Nil(t, resp.MFANudge, "a non-admin has no nudge, so the recomputed block is null")
	})

	t.Run("two_factor_enabled_gets_null_nudge", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		secret := "encrypted-secret"
		user := &domain.User{
			Login: "admin", Email: "a@example.com", Password: "x",
			TwoFactorEnabled: true, TwoFactorSecret: &secret,
		}
		require.NoError(t, repo.Save(ctx, user))

		handler := NewHandler(
			repo, mfanudge.New(nudgeConfig(true, 30), clock), stubAdminChecker{isAdmin: true}, api.NewResponder(),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/snooze", nil)
		req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{User: user, Login: user.Login}))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var resp struct {
			MFANudge *mfanudge.View `json:"mfa_nudge"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Nil(t, resp.MFANudge, "a user who already has 2FA gets a null nudge after snoozing")
	})

	t.Run("rbac_error_returns_500", func(t *testing.T) {
		repo := inmemory.NewUserRepository()
		user := &domain.User{Login: "admin", Email: "a@example.com", Password: "x"}
		require.NoError(t, repo.Save(ctx, user))

		handler := NewHandler(
			repo, mfanudge.New(nudgeConfig(true, 30), clock),
			stubAdminChecker{err: errRBACUnavailable}, api.NewResponder(),
		)

		req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/snooze", nil)
		req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{User: user, Login: user.Login}))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code,
			"unlike login, the snooze endpoint is not fail-open: an RBAC error must surface as 500")
	})
}
