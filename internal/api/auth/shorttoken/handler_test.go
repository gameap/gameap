// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — minting a short-lived credential is an
//     authentication-sensitive operation. These tests pin that: an
//     unauthenticated caller is refused; an authenticated caller gets a
//     single-use token that is stored only as a hash with a bounded (≤10s)
//     lifetime; a PAT-derived token never carries more authority than the PAT
//     that minted it; and every issuance leaves a token_op audit record while
//     a refused attempt leaves none.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package shorttoken

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

func sessionUser() *domain.User {
	return &domain.User{ID: 42, Login: "alice", Email: "alice@example.com"}
}

// TestHandler_ServeHTTP covers OWASP API2:2023: only an authenticated caller
// may mint a token; the token is opaque, prefixed, single-use-shaped (stored
// only as its hash) and bounded to ≤10s; a PAT session propagates its
// abilities so the short-lived token is never broader than its origin.
func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		ttl            time.Duration
		session        *auth.Session
		expectedStatus int
		wantError      string
		wantExpiresIn  int64
		wantPATID      uint
		wantAbilities  []domain.PATAbility
	}{
		{
			name:           "unauthenticated_request_is_refused",
			ttl:            10 * time.Second,
			session:        nil,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:           "empty_session_is_refused",
			ttl:            10 * time.Second,
			session:        &auth.Session{},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "session_user_gets_a_short_lived_token",
			ttl:  10 * time.Second,
			session: &auth.Session{
				ID:    "sess-1",
				Login: "alice",
				Email: "alice@example.com",
				User:  sessionUser(),
			},
			expectedStatus: http.StatusOK,
			wantExpiresIn:  10,
		},
		{
			name: "ttl_above_cap_is_clamped_to_10s",
			ttl:  1 * time.Hour,
			session: &auth.Session{
				Login: "alice", Email: "alice@example.com", User: sessionUser(),
			},
			expectedStatus: http.StatusOK,
			wantExpiresIn:  10,
		},
		{
			name: "ttl_below_cap_is_preserved",
			ttl:  5 * time.Second,
			session: &auth.Session{
				Login: "alice", Email: "alice@example.com", User: sessionUser(),
			},
			expectedStatus: http.StatusOK,
			wantExpiresIn:  5,
		},
		{
			name: "non_positive_ttl_falls_back_to_cap",
			ttl:  0,
			session: &auth.Session{
				Login: "alice", Email: "alice@example.com", User: sessionUser(),
			},
			expectedStatus: http.StatusOK,
			wantExpiresIn:  10,
		},
		{
			name: "pat_session_propagates_abilities",
			ttl:  10 * time.Second,
			session: &auth.Session{
				Login: "alice",
				Email: "alice@example.com",
				User:  sessionUser(),
				Token: &domain.PersonalAccessToken{
					ID: 7,
					Abilities: &[]domain.PATAbility{
						domain.PATAbilityServerList,
						domain.PATAbilityServerConsole,
					},
				},
			},
			expectedStatus: http.StatusOK,
			wantExpiresIn:  10,
			wantPATID:      7,
			wantAbilities: []domain.PATAbility{
				domain.PATAbilityServerList,
				domain.PATAbilityServerConsole,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			c := cache.NewInMemory()
			recorder := &auditCapture{}
			handler := NewHandler(c, tt.ttl, api.NewResponder(), recorder)

			req := httptest.NewRequest(http.MethodPost, "/api/auth/short-lived-token", http.NoBody)
			if tt.session != nil {
				req = req.WithContext(auth.ContextWithSession(req.Context(), tt.session))
			}
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				assert.Contains(t, response["error"], tt.wantError)
				assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventShortLivedTokenIssue),
					"a refused mint must not leave an issuance audit record")

				return
			}

			token, ok := response["token"].(string)
			require.True(t, ok)
			assert.True(t, auth.IsShortLivedToken(token),
				"token must carry the short-lived prefix, got %q", token)
			assert.Greater(t, len(token), len(auth.ShortLivedTokenPrefix)+32,
				"token must contain a non-trivial secret")
			assert.Equal(t, float64(tt.wantExpiresIn), response["expires_in"])

			// The secret itself is never stored — only its hash, under the
			// derived cache key.
			raw, err := c.Get(context.Background(), auth.ShortLivedCacheKey(token))
			require.NoError(t, err, "the minted token must be persisted in the cache")

			payload, err := auth.UnmarshalShortLivedPayload(raw)
			require.NoError(t, err)
			assert.Equal(t, tt.session.User.ID, payload.UserID)
			assert.Equal(t, tt.session.User.Login, payload.Login)
			assert.Equal(t, tt.session.User.Email, payload.Email)
			assert.Equal(t, tt.wantPATID, payload.PATID)
			assert.Equal(t, tt.wantAbilities, payload.Abilities)

			require.Equal(t, 1, countEvents(recorder.snapshot(), audit.EventShortLivedTokenIssue),
				"a successful mint must leave exactly one issuance audit record")
		})
	}
}

// TestHandler_TokensAreUnique covers OWASP API2:2023: two mints must produce
// distinct secrets so one captured token cannot be predicted from another.
func TestHandler_TokensAreUnique(t *testing.T) {
	c := cache.NewInMemory()
	handler := NewHandler(c, 10*time.Second, api.NewResponder(), nil)
	session := &auth.Session{Login: "alice", Email: "alice@example.com", User: sessionUser()}

	mint := func() string {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/short-lived-token", http.NoBody)
		req = req.WithContext(auth.ContextWithSession(req.Context(), session))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

		return response["token"].(string)
	}

	first := mint()
	second := mint()

	assert.NotEqual(t, first, second, "each mint must yield a fresh secret")
}
