// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — the ticket must be single-use, time
//     bounded, and must never be accepted in any other shape; single sign-on
//     must not bypass a second factor.
//   - API1:2023 Broken Object Level Authorization — a ticket must never yield
//     an administrative session, even if the account was promoted after the
//     ticket was minted.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package ssoexchange

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-jwt-secret-for-sso-exchange-handler"

type stubAdminChecker struct {
	admins map[uint]bool
}

func (s stubAdminChecker) Can(_ context.Context, userID uint, _ []domain.AbilityName) (bool, error) {
	return s.admins[userID], nil
}

type exchangeFixture struct {
	handler  *Handler
	cache    cache.Cache
	user     *domain.User
	userRepo *inmemory.UserRepository
	admins   map[uint]bool
}

func newExchangeFixture(t *testing.T, twoFactor bool) *exchangeFixture {
	t.Helper()

	userRepo := inmemory.NewUserRepository()

	user := &domain.User{
		Login:            "customer",
		Email:            "customer@example.com",
		TwoFactorEnabled: twoFactor,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	admins := map[uint]bool{}
	c := cache.NewInMemory()

	handler := NewHandler(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		stubAdminChecker{admins: admins},
		c,
		"",
		api.NewResponder(),
		nil,
	)

	return &exchangeFixture{handler: handler, cache: c, user: user, userRepo: userRepo, admins: admins}
}

// storeTicket puts a ticket into the cache the same way the minting handler
// would, so the tests exercise the real payload contract.
func (f *exchangeFixture) storeTicket(t *testing.T, mutate func(p *auth.SSOTicketPayload)) string {
	t.Helper()

	ticket := auth.SSOTicketPrefix + "fixture-sso-secret-abcdef0123456789abcdef"

	payload := auth.SSOTicketPayload{
		UserID:     f.user.ID,
		Login:      f.user.Login,
		IssuerID:   99,
		RedirectTo: "/servers/7",
		ExpiresAt:  time.Now().Add(time.Minute).Unix(),
	}

	if mutate != nil {
		mutate(&payload)
	}

	encoded, err := auth.MarshalSSOTicketPayload(payload)
	require.NoError(t, err)
	require.NoError(t, f.cache.Set(context.Background(), auth.SSOTicketCacheKey(ticket), encoded))

	return ticket
}

func (f *exchangeFixture) exchange(t *testing.T, ticket string) *httptest.ResponseRecorder {
	t.Helper()

	body := bytes.NewBufferString(`{"ticket":"` + ticket + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/exchange", body)
	req.RemoteAddr = "203.0.113.10:5555"

	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, req)

	return recorder
}

func TestExchange_IssuesSessionAndConsumesTicket(t *testing.T) {
	// ARRANGE
	f := newExchangeFixture(t, false)
	ticket := f.storeTicket(t, nil)

	// ACT
	recorder := f.exchange(t, ticket)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Token      string `json:"token"`
		ExpiresIn  int64  `json:"expires_in"`
		RedirectTo string `json:"redirect_to"`
		User       struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	assert.NotEmpty(t, response.Token)
	assert.Equal(t, "customer", response.User.Login)
	assert.Equal(t, "/servers/7", response.RedirectTo)
	assert.Positive(t, response.ExpiresIn)

	// A replay must find nothing.
	replay := f.exchange(t, ticket)
	assert.Equal(t, http.StatusUnauthorized, replay.Code)
	assert.NotContains(t, replay.Body.String(), "token")
}

func TestExchange_TwoFactorAccountGetsChallengeNotSession(t *testing.T) {
	// ARRANGE
	f := newExchangeFixture(t, true)
	ticket := f.storeTicket(t, nil)

	// ACT
	recorder := f.exchange(t, ticket)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		TwoFactorRequired bool   `json:"two_factor_required"`
		ChallengeToken    string `json:"challenge_token"`
		Token             string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	assert.True(t, response.TwoFactorRequired)
	assert.NotEmpty(t, response.ChallengeToken)
	assert.Empty(t, response.Token, "single sign-on must not hand out a session before the second factor")

	// The ticket is spent either way: one shot at the second factor per ticket.
	replay := f.exchange(t, ticket)
	assert.Equal(t, http.StatusUnauthorized, replay.Code)
}

func TestExchange_Rejections(t *testing.T) {
	tests := []struct {
		name       string
		ticket     func(t *testing.T, f *exchangeFixture) string
		setup      func(f *exchangeFixture)
		wantStatus int
	}{
		{
			name:       "unknown_ticket",
			ticket:     func(_ *testing.T, _ *exchangeFixture) string { return auth.SSOTicketPrefix + "nope" },
			wantStatus: http.StatusUnauthorized,
		},
		{
			// A session token or PAT must never be accepted here.
			name:       "wrong_prefix",
			ticket:     func(_ *testing.T, _ *exchangeFixture) string { return "glst_some-short-lived-token" },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "expired_ticket",
			ticket: func(t *testing.T, f *exchangeFixture) string {
				t.Helper()

				return f.storeTicket(t, func(p *auth.SSOTicketPayload) {
					p.ExpiresAt = time.Now().Add(-time.Second).Unix()
				})
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "ip_bound_to_someone_else",
			ticket: func(t *testing.T, f *exchangeFixture) string {
				t.Helper()

				return f.storeTicket(t, func(p *auth.SSOTicketPayload) {
					p.ClientIP = "198.51.100.4"
				})
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "target_promoted_to_admin_after_minting",
			ticket: func(t *testing.T, f *exchangeFixture) string {
				t.Helper()

				ticket := f.storeTicket(t, nil)
				f.admins[f.user.ID] = true

				return ticket
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "user_deleted_after_minting",
			ticket: func(t *testing.T, f *exchangeFixture) string {
				t.Helper()

				return f.storeTicket(t, func(p *auth.SSOTicketPayload) {
					p.UserID = 4242
				})
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// ARRANGE
			f := newExchangeFixture(t, false)
			if test.setup != nil {
				test.setup(f)
			}
			ticket := test.ticket(t, f)

			// ACT
			recorder := f.exchange(t, ticket)

			// ASSERT
			assert.Equal(t, test.wantStatus, recorder.Code, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), `"token"`)
		})
	}
}

func TestExchange_MissingTicketIsRejected(t *testing.T) {
	// ARRANGE
	f := newExchangeFixture(t, false)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/exchange", bytes.NewBufferString(`{}`))
	recorder := httptest.NewRecorder()

	// ACT
	f.handler.ServeHTTP(recorder, req)

	// ASSERT
	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
}
