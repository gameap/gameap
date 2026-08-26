// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API1:2023 Broken Object Level Authorization — a ticket must never be
//     mintable for an administrator, or a scoped integration token becomes a
//     path to panel takeover.
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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

type mintFixture struct {
	handler *Handler
	cache   cache.Cache
	client  *domain.User
	admin   *domain.User
}

func newMintFixture(t *testing.T, ttl time.Duration) *mintFixture {
	t.Helper()

	userRepo := inmemory.NewUserRepository()

	client := &domain.User{Login: "customer", Email: "customer@example.com"}
	require.NoError(t, userRepo.Save(context.Background(), client))

	admin := &domain.User{Login: "root", Email: "root@example.com"}
	require.NoError(t, userRepo.Save(context.Background(), admin))

	c := cache.NewInMemory()

	handler := NewHandler(
		userRepo,
		stubAdminChecker{admins: map[uint]bool{admin.ID: true}},
		c,
		ttl,
		api.NewResponder(),
		nil,
	)

	return &mintFixture{handler: handler, cache: c, client: client, admin: admin}
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

func TestMint_RefusesAdministratorTarget(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newMintFixture(t, 60*time.Second)

	// ACT
	recorder := f.request(t, `{"user_id":`+itoa(f.admin.ID)+`}`, adminSession())

	// ASSERT
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), auth.SSOTicketPrefix)
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

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
