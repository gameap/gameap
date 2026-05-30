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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAdminChecker struct{ isAdmin bool }

func (s stubAdminChecker) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return s.isAdmin, nil
}

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
}
