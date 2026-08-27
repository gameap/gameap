// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — the ticket must be single-use, time
//     bounded, and must never be accepted in any other shape; single sign-on
//     must not bypass an enrolled second factor.
//   - API1:2023 Broken Object Level Authorization — a ticket must never yield a
//     session for an administrator other than the one that minted it, even if
//     the account was promoted after the ticket was minted.
//   - API2:2023 (again) — single sign-on must answer to the same admin-MFA
//     policy a password login does, so it cannot become a way around it.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package ssoexchange

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/mfanudge"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-jwt-secret-for-sso-exchange-handler"

const (
	// mfaGraceDays and enrollmentTokenTTL mirror the shipped defaults
	// (AUTH_MFA_HARD_FAIL_DAYS, AUTH_MFA_ENROLLMENT_TOKEN_TTL).
	mfaGraceDays       = 30
	enrollmentTokenTTL = 15 * time.Minute
)

type stubAdminChecker struct {
	admins map[uint]bool
}

func (s stubAdminChecker) Can(_ context.Context, userID uint, _ []domain.AbilityName) (bool, error) {
	return s.admins[userID], nil
}

// auditCapture is a concurrency-safe audit.Logger that records every event the
// handler emits (mirrors internal/api/auth/login/handler_test.go). Every refusal
// here answers the caller with the same 401, so the recorded reason is the only
// way a test can tell one from another.
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

// reasons returns the refusal reasons recorded so far, in order.
func (a *auditCapture) reasons() []string {
	var reasons []string

	for _, event := range a.snapshot() {
		if event.Reason != "" {
			reasons = append(reasons, event.Reason)
		}
	}

	return reasons
}

// hasAction reports whether an event with the given action was recorded.
func (a *auditCapture) hasAction(action string) bool {
	for _, event := range a.snapshot() {
		if event.Action == action {
			return true
		}
	}

	return false
}

type exchangeFixture struct {
	handler  *Handler
	cache    cache.Cache
	audit    *auditCapture
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
	auditLog := &auditCapture{}

	cfg := config.Config{}
	cfg.Auth.RequireMFAForAdmins = true
	cfg.Auth.MFAHardFailDays = mfaGraceDays

	handler := NewHandler(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		stubAdminChecker{admins: admins},
		c,
		"",
		api.NewResponder(),
		auditLog,
		mfanudge.New(cfg, nil),
		enrollmentTokenTTL,
	)

	return &exchangeFixture{
		handler:  handler,
		cache:    c,
		audit:    auditLog,
		user:     user,
		userRepo: userRepo,
		admins:   admins,
	}
}

// selfIssued marks the ticket as minted by its own target, the only shape an
// administrator's ticket is allowed to have.
func (f *exchangeFixture) selfIssued(p *auth.SSOTicketPayload) {
	p.IssuerID = f.user.ID
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
	t.Parallel()

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
	t.Parallel()

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

// An administrator only ever reaches the challenge, never a session: the ticket
// was minted by that same account, so it buys exactly what a password would.
func TestExchange_SelfIssuedAdministratorTicketGetsChallengeNotSession(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newExchangeFixture(t, true)
	f.admins[f.user.ID] = true
	ticket := f.storeTicket(t, f.selfIssued)

	// ACT
	recorder := f.exchange(t, ticket)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		TwoFactorRequired bool   `json:"two_factor_required"`
		ChallengeToken    string `json:"challenge_token"`
		Token             string `json:"token"`
		RedirectTo        string `json:"redirect_to"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	assert.True(t, response.TwoFactorRequired)
	assert.NotEmpty(t, response.ChallengeToken)
	assert.Empty(t, response.Token, "an administrator must never get a session straight from a ticket")
	assert.Equal(t, "/servers/7", response.RedirectTo)

	assert.True(t, f.audit.hasAction("sso_ticket_redeem_2fa"))
	assert.False(t, f.audit.hasAction("sso_ticket_redeem"), "the direct-session path must not be taken")
}

func TestExchange_AdministratorRejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(f *exchangeFixture) func(*auth.SSOTicketPayload)
		wantReason string
	}{
		{
			// The fixture's default issuer is 99, which is nobody.
			name:       "ticket_minted_by_somebody_else",
			mutate:     func(_ *exchangeFixture) func(*auth.SSOTicketPayload) { return nil },
			wantReason: "sso_target_is_other_admin",
		},
		{
			// Fails closed: a payload carrying no issuer at all never matches a
			// loaded user, whose id is never zero.
			name: "ticket_without_an_issuer",
			mutate: func(_ *exchangeFixture) func(*auth.SSOTicketPayload) {
				return func(p *auth.SSOTicketPayload) { p.IssuerID = 0 }
			},
			wantReason: "sso_target_is_other_admin",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newExchangeFixture(t, true)
			f.admins[f.user.ID] = true
			ticket := f.storeTicket(t, test.mutate(f))

			// ACT
			recorder := f.exchange(t, ticket)

			// ASSERT
			assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), `"token"`)
			assert.NotContains(t, recorder.Body.String(), `"challenge_token"`)
			assert.Equal(t, []string{test.wantReason}, f.audit.reasons())
		})
	}
}

// An administrator who has not enrolled a second factor yet is let in, not turned
// away: on a panel deployed for one customer that account is the customer, and
// refusing would leave them no way in at all. The admin-MFA policy then asks them
// to enrol, exactly as it would after a password login.
func TestExchange_AdministratorWithoutTwoFactorGetsSessionAndNudge(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newExchangeFixture(t, false)
	f.admins[f.user.ID] = true
	ticket := f.storeTicket(t, f.selfIssued)

	// ACT
	recorder := f.exchange(t, ticket)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Token                 string `json:"token"`
		TwoFactorRequired     bool   `json:"two_factor_required"`
		MFAEnrollmentRequired bool   `json:"mfa_enrollment_required"`
		RedirectTo            string `json:"redirect_to"`
		MFANudge              *struct {
			Required      bool `json:"required"`
			ShowNow       bool `json:"show_now"`
			HardFail      bool `json:"hard_fail"`
			DaysRemaining int  `json:"days_remaining"`
		} `json:"mfa_nudge"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	assert.NotEmpty(t, response.Token, "the customer must reach the panel")
	assert.False(t, response.TwoFactorRequired)
	assert.False(t, response.MFAEnrollmentRequired, "the grace window has not closed")
	assert.Equal(t, "/servers/7", response.RedirectTo)

	require.NotNil(t, response.MFANudge, "the panel must be told to ask for enrolment")
	assert.True(t, response.MFANudge.Required)
	assert.True(t, response.MFANudge.ShowNow)
	assert.False(t, response.MFANudge.HardFail)
	assert.Equal(t, mfaGraceDays, response.MFANudge.DaysRemaining)

	assert.True(t, f.audit.hasAction("sso_ticket_redeem"))

	// The grace window is counted from here, so the moment has to survive.
	users, err := f.userRepo.Find(context.Background(), filters.FindUserByIDs(f.user.ID), nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.NotNil(t, users[0].MFAFirstShownAt(), "the first-shown timestamp must be persisted")
}

// Once the grace window closes the ticket buys only what the password login would
// at that point: a session that can enrol a second factor and nothing else.
func TestExchange_AdministratorPastGraceWindowGetsEnrollmentSession(t *testing.T) {
	t.Parallel()

	// ARRANGE: the nudge was first shown 31 days ago.
	firstShown := time.Now().Add(-time.Duration(mfaGraceDays+1) * 24 * time.Hour)

	f := newExchangeFixture(t, false)
	f.admins[f.user.ID] = true
	f.user.SetMFAFirstShownAt(&firstShown)
	require.NoError(t, f.userRepo.Save(context.Background(), f.user))

	ticket := f.storeTicket(t, f.selfIssued)

	// ACT
	recorder := f.exchange(t, ticket)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Token                 string `json:"token"`
		ExpiresIn             int64  `json:"expires_in"`
		MFAEnrollmentRequired bool   `json:"mfa_enrollment_required"`
		MFANudge              *struct {
			HardFail bool `json:"hard_fail"`
		} `json:"mfa_nudge"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	assert.NotEmpty(t, response.Token)
	assert.True(t, response.MFAEnrollmentRequired)
	assert.Equal(t, int64(enrollmentTokenTTL.Seconds()), response.ExpiresIn,
		"an enrollment session lives for the enrollment TTL, not a full day")

	require.NotNil(t, response.MFANudge)
	assert.True(t, response.MFANudge.HardFail)

	assert.True(t, f.audit.hasAction("sso_ticket_redeem_enrollment"))
	assert.False(t, f.audit.hasAction("sso_ticket_redeem"), "this is not a full session")
}

// The nudge is an admin-only affair: a customer must never be shown it.
func TestExchange_RegularUserCarriesNoNudge(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newExchangeFixture(t, false)
	ticket := f.storeTicket(t, nil)

	// ACT
	recorder := f.exchange(t, ticket)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "mfa_nudge")
	assert.NotContains(t, recorder.Body.String(), "mfa_enrollment_required")
}

func TestExchange_Rejections(t *testing.T) {
	t.Parallel()

	// Every case answers 401 with the same body, so the recorded reason is what
	// keeps the table honest: without it a case could pass for the wrong cause.
	tests := []struct {
		name       string
		ticket     func(t *testing.T, f *exchangeFixture) string
		wantStatus int
		wantReason string
	}{
		{
			name:       "unknown_ticket",
			ticket:     func(_ *testing.T, _ *exchangeFixture) string { return auth.SSOTicketPrefix + "nope" },
			wantStatus: http.StatusUnauthorized,
			wantReason: "sso_ticket_not_found",
		},
		{
			// A session token or PAT must never be accepted here.
			name:       "wrong_prefix",
			ticket:     func(_ *testing.T, _ *exchangeFixture) string { return "glst_some-short-lived-token" },
			wantStatus: http.StatusUnauthorized,
			wantReason: "sso_ticket_malformed",
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
			wantReason: "sso_ticket_expired",
		},
		{
			// A payload with no positive deadline must fail closed rather than
			// be treated as a ticket that never expires.
			name: "ticket_without_expiry_is_rejected",
			ticket: func(t *testing.T, f *exchangeFixture) string {
				t.Helper()

				return f.storeTicket(t, func(p *auth.SSOTicketPayload) {
					p.ExpiresAt = 0
				})
			},
			wantStatus: http.StatusUnauthorized,
			wantReason: "sso_ticket_expired",
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
			wantReason: "sso_ticket_ip_mismatch",
		},
		{
			// The promotion happens after minting, so the ticket names an
			// issuer who is not the target: the ownership rule catches it.
			name: "target_promoted_to_admin_after_minting",
			ticket: func(t *testing.T, f *exchangeFixture) string {
				t.Helper()

				ticket := f.storeTicket(t, nil)
				f.admins[f.user.ID] = true

				return ticket
			},
			wantStatus: http.StatusUnauthorized,
			wantReason: "sso_target_is_other_admin",
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
			wantReason: "sso_user_missing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newExchangeFixture(t, false)
			ticket := test.ticket(t, f)

			// ACT
			recorder := f.exchange(t, ticket)

			// ASSERT
			assert.Equal(t, test.wantStatus, recorder.Code, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), `"token"`)
			assert.Equal(t, []string{test.wantReason}, f.audit.reasons())
		})
	}
}

func TestExchange_MissingTicketIsRejected(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newExchangeFixture(t, false)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/exchange", bytes.NewBufferString(`{}`))
	recorder := httptest.NewRecorder()

	// ACT
	f.handler.ServeHTTP(recorder, req)

	// ASSERT
	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
}
