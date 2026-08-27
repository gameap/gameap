// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API1:2023 Broken Object Level Authorization — a ticket must never be
//     mintable for an administrator other than the account the request is
//     already authenticated as, or a scoped integration token becomes a path to
//     panel takeover.
//   - API2:2023 Broken Authentication — the ticket must be high-entropy,
//     short-lived and stored only by hash.
//   - API3:2023 Broken Object Property Level Authorization — the redirect
//     target must stay inside the panel origin.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package ssomint

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAdminChecker struct {
	admins map[uint]bool
	err    error
}

func (s stubAdminChecker) Can(_ context.Context, userID uint, _ []domain.AbilityName) (bool, error) {
	if s.err != nil {
		return false, s.err
	}

	return s.admins[userID], nil
}

// auditCapture is a concurrency-safe audit.Logger that records every event the
// handler emits (mirrors internal/api/auth/login/handler_test.go). The refusal
// reason and the admin_self attribute are only visible in the journal, so the
// tests have to read them from here.
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

// findEvent returns the first recorded event with the given action.
func (a *auditCapture) findEvent(action string) (audit.Event, bool) {
	for _, event := range a.snapshot() {
		if event.Action == action {
			return event, true
		}
	}

	return audit.Event{}, false
}

type mintFixture struct {
	handler *Handler
	cache   cache.Cache
	audit   *auditCapture
	client  *domain.User
	admin   *domain.User

	// adminMFA is an administrator who has already enrolled a second factor, so
	// the ticket minted for it lands on a challenge at the exchange.
	adminMFA *domain.User
}

func newMintFixture(t *testing.T, ttl time.Duration) *mintFixture {
	t.Helper()

	userRepo := inmemory.NewUserRepository()

	client := &domain.User{Login: "customer", Email: "customer@example.com"}
	require.NoError(t, userRepo.Save(context.Background(), client))

	admin := &domain.User{Login: "root", Email: "root@example.com"}
	require.NoError(t, userRepo.Save(context.Background(), admin))

	adminMFA := &domain.User{Login: "owner", Email: "owner@example.com", TwoFactorEnabled: true}
	require.NoError(t, userRepo.Save(context.Background(), adminMFA))

	c := cache.NewInMemory()
	auditLog := &auditCapture{}

	handler := NewHandler(
		userRepo,
		stubAdminChecker{admins: map[uint]bool{admin.ID: true, adminMFA.ID: true}},
		c,
		ttl,
		api.NewResponder(),
		auditLog,
	)

	return &mintFixture{
		handler:  handler,
		cache:    c,
		audit:    auditLog,
		client:   client,
		admin:    admin,
		adminMFA: adminMFA,
	}
}

func (f *mintFixture) request(t *testing.T, body string, session *auth.Session) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso/tickets", bytes.NewBufferString(body))
	if session != nil {
		req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	}

	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, req)

	return recorder
}

func adminSession() *auth.Session {
	return &auth.Session{User: &domain.User{ID: 999, Login: "root"}}
}

// selfSession authenticates as the fixture's own two-factor administrator, the
// way a billing integration's token does on a panel deployed for one customer.
func (f *mintFixture) selfSession() *auth.Session {
	return &auth.Session{User: f.adminMFA}
}

func TestMint_IssuesUsableTicketForRegularUser(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newMintFixture(t, 60*time.Second)

	// ACT
	recorder := f.request(t,
		`{"user_id":`+itoa(f.client.ID)+`,"redirect_to":"/servers/42"}`,
		adminSession(),
	)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Ticket     string `json:"ticket"`
		ExpiresIn  int64  `json:"expires_in"`
		RedirectTo string `json:"redirect_to"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	assert.True(t, strings.HasPrefix(response.Ticket, auth.SSOTicketPrefix))
	assert.Equal(t, int64(60), response.ExpiresIn)
	assert.Equal(t, "/servers/42", response.RedirectTo)

	// The secret itself is never stored — only a key derived from its hash.
	raw, err := f.cache.Get(context.Background(), auth.SSOTicketCacheKey(response.Ticket))
	require.NoError(t, err)

	payload, err := auth.UnmarshalSSOTicketPayload(raw)
	require.NoError(t, err)
	assert.Equal(t, f.client.ID, payload.UserID)
	assert.Equal(t, uint(999), payload.IssuerID)
	assert.Equal(t, "/servers/42", payload.RedirectTo)
	assert.NotContains(t, raw, strings.TrimPrefix(response.Ticket, auth.SSOTicketPrefix))
}

func TestMint_RefusesAnotherAdministratorAsTarget(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newMintFixture(t, 60*time.Second)

	// ACT: the session is user 999, the target is a different administrator.
	recorder := f.request(t, `{"user_id":`+itoa(f.admin.ID)+`}`, adminSession())

	// ASSERT
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "another administrator")
	assert.NotContains(t, recorder.Body.String(), auth.SSOTicketPrefix)
	assert.Equal(t, []string{"sso_target_is_other_admin"}, f.audit.reasons())
}

// An administrator logging themselves in is the case this endpoint exists for on
// a panel deployed for a single customer: the customer administers their own
// panel, so the billing button has to reach an administrative account. The ticket
// stays worth no more than the token already is — it reaches only the identity
// the token already carries.
func TestMint_IssuesTicketForSelfWhenAdministratorHasTwoFactor(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newMintFixture(t, 60*time.Second)

	// ACT
	recorder := f.request(t, `{"user_id":`+itoa(f.adminMFA.ID)+`}`, f.selfSession())

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Ticket string `json:"ticket"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, strings.HasPrefix(response.Ticket, auth.SSOTicketPrefix))

	raw, err := f.cache.Get(context.Background(), auth.SSOTicketCacheKey(response.Ticket))
	require.NoError(t, err)

	payload, err := auth.UnmarshalSSOTicketPayload(raw)
	require.NoError(t, err)
	assert.Equal(t, f.adminMFA.ID, payload.UserID)
	assert.Equal(t, payload.UserID, payload.IssuerID, "the issuer must be the target itself")

	// Without this attribute "the billing token logged in as the panel
	// administrator" is indistinguishable in the journal from "as a customer".
	event, ok := f.audit.findEvent("sso_ticket_issue")
	require.True(t, ok, "the issue must be recorded")
	assert.Contains(t, event.Extra, slog.Bool("admin_self", true))
}

// Whether the target has a second factor is the admin-MFA policy's business, not
// this endpoint's: a customer who administers their own panel has none on day one
// and must still be able to get in. The exchange applies the policy.
func TestMint_IssuesTicketForSelfWithoutTwoFactor(t *testing.T) {
	t.Parallel()

	// ARRANGE: an administrator without a second factor, minting for itself.
	f := newMintFixture(t, 60*time.Second)

	// ACT
	recorder := f.request(t, `{"user_id":`+itoa(f.admin.ID)+`}`, &auth.Session{User: f.admin})

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Empty(t, f.audit.reasons())

	event, ok := f.audit.findEvent("sso_ticket_issue")
	require.True(t, ok)
	assert.Contains(t, event.Extra, slog.Bool("admin_self", true))
}

func TestMint_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		session    *auth.Session
		wantStatus int
	}{
		{
			name:       "unauthenticated",
			body:       `{"user_id":1}`,
			session:    &auth.Session{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing_user_id",
			body:       `{}`,
			session:    adminSession(),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "unknown_user",
			body:       `{"user_id":4242}`,
			session:    adminSession(),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "absolute_redirect",
			body:       `{"user_id":1,"redirect_to":"https://evil.example.com/"}`,
			session:    adminSession(),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "protocol_relative_redirect",
			body:       `{"user_id":1,"redirect_to":"//evil.example.com/"}`,
			session:    adminSession(),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			// Browsers normalise /\host the same way as //host.
			name:       "backslash_redirect",
			body:       `{"user_id":1,"redirect_to":"/\\evil.example.com/"}`,
			session:    adminSession(),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "malformed_json",
			body:       `{`,
			session:    adminSession(),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newMintFixture(t, 60*time.Second)

			// ACT
			recorder := f.request(t, test.body, test.session)

			// ASSERT
			assert.Equal(t, test.wantStatus, recorder.Code, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), auth.SSOTicketPrefix)
		})
	}
}

func TestMint_TTLIsCapped(t *testing.T) {
	t.Parallel()

	// ARRANGE: an operator configures an hour; the handler must not honour it.
	f := newMintFixture(t, time.Hour)

	// ACT
	recorder := f.request(t, `{"user_id":`+itoa(f.client.ID)+`}`, adminSession())

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		ExpiresIn int64 `json:"expires_in"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, int64(maxTicketTTL.Seconds()), response.ExpiresIn)
}

func TestMint_TTLDefaultsAndHonoursConfiguredValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ttl           time.Duration
		wantExpiresIn int64
	}{
		{"zero_falls_back_to_default", 0, int64(defaultTicketTTL.Seconds())},
		{"negative_falls_back_to_default", -5 * time.Second, int64(defaultTicketTTL.Seconds())},
		{"valid_value_is_honoured", 90 * time.Second, 90},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			f := newMintFixture(t, test.ttl)

			recorder := f.request(t, `{"user_id":`+itoa(f.client.ID)+`}`, adminSession())

			require.Equal(t, http.StatusOK, recorder.Code)

			var response struct {
				ExpiresIn int64 `json:"expires_in"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.wantExpiresIn, response.ExpiresIn)
		})
	}
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
