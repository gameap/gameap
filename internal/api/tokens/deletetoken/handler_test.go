package deletetoken_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/tokens/deletetoken"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
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

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
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

func TestHandler_ServeHTTP(t *testing.T) {
	now := time.Now()

	testUser := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	testToken := &domain.PersonalAccessToken{
		ID:            0, // Will be set by repository
		TokenableType: domain.EntityTypeUser,
		TokenableID:   1,
		Name:          "Test Token",
		Token:         "test-token",
		Abilities:     &[]domain.PATAbility{},
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	otherUserToken := &domain.PersonalAccessToken{
		ID:            0, // Will be set by repository
		TokenableType: domain.EntityTypeUser,
		TokenableID:   2,
		Name:          "Other User Token",
		Token:         "other-token",
		Abilities:     &[]domain.PATAbility{},
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	tests := []struct {
		name          string
		tokenID       string
		setupTokens   []*domain.PersonalAccessToken
		authenticated bool
		wantStatus    int
		wantDeleted   bool
	}{
		{
			name:          "successful deletion",
			tokenID:       "1",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusNoContent,
			wantDeleted:   true,
		},
		{
			name:          "unauthenticated user",
			tokenID:       "1",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: false,
			wantStatus:    http.StatusUnauthorized,
			wantDeleted:   false,
		},
		{
			name:          "token not found",
			tokenID:       "999",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusNotFound,
			wantDeleted:   false,
		},
		{
			name:          "access denied for other user's token",
			tokenID:       "2",
			setupTokens:   []*domain.PersonalAccessToken{testToken, otherUserToken},
			authenticated: true,
			wantStatus:    http.StatusForbidden,
			wantDeleted:   false,
		},
		{
			name:          "invalid token id",
			tokenID:       "invalid",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusUnprocessableEntity,
			wantDeleted:   false,
		},
		{
			name:          "empty token id",
			tokenID:       "",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusUnprocessableEntity,
			wantDeleted:   false,
		},
		{
			name:          "negative token id",
			tokenID:       "-1",
			setupTokens:   []*domain.PersonalAccessToken{testToken},
			authenticated: true,
			wantStatus:    http.StatusUnprocessableEntity,
			wantDeleted:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokensRepo := inmemory.NewPersonalAccessTokenRepository()
			responder := api.NewResponder()

			var savedTokens []*domain.PersonalAccessToken
			for _, token := range tt.setupTokens {
				tokenCopy := *token // Make a copy to preserve original state
				err := tokensRepo.Save(context.Background(), &tokenCopy)
				require.NoError(t, err)
				savedTokens = append(savedTokens, &tokenCopy)
			}

			handler := deletetoken.NewHandler(tokensRepo, auth.NoopRevocation{}, responder, nil)

			req := httptest.NewRequest(http.MethodDelete, "/api/tokens/"+tt.tokenID, nil)
			if tt.tokenID != "" {
				req = mux.SetURLVars(req, map[string]string{"id": tt.tokenID})
			}

			if tt.authenticated {
				ctx := auth.ContextWithSession(req.Context(), &auth.Session{
					User: testUser,
				})
				req = req.WithContext(ctx)
			}

			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			tokens, err := tokensRepo.Find(context.Background(), nil, nil, nil)
			require.NoError(t, err)

			//nolint:nestif
			if tt.wantDeleted {
				// For successful deletion, the first saved token should be gone
				if len(savedTokens) > 0 && savedTokens[0] != nil {
					firstTokenID := savedTokens[0].ID
					for _, token := range tokens {
						assert.NotEqual(t, firstTokenID, token.ID, "Token should be deleted")
					}
				}
			} else {
				// For cases where deletion shouldn't happen, check that the first saved token still exists
				if len(savedTokens) > 0 && savedTokens[0] != nil {
					firstTokenID := savedTokens[0].ID
					foundOriginal := false
					for _, token := range tokens {
						if token.ID == firstTokenID {
							foundOriginal = true

							break
						}
					}
					assert.True(t, foundOriginal, "Original token should still exist")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — revoking a personal access token
//     is a credential-lifecycle event that must be recorded (OWASP ASVS
//     §7.2.1) so a token's revocation is auditable.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestHandler_Audit_SuccessfulRevokeIsRecorded covers OWASP API8:2023. A
// successful PAT deletion must emit exactly one token.pat.revoke event with
// outcome success, category token_op, the revoked token id as the resource,
// and the acting user as the actor.
func TestHandler_Audit_SuccessfulRevokeIsRecorded(t *testing.T) {
	// ARRANGE
	now := time.Now()
	owner := &domain.User{ID: 1, Login: "testuser", Email: "test@example.com"}

	tokensRepo := inmemory.NewPersonalAccessTokenRepository()
	tok := &domain.PersonalAccessToken{
		TokenableType: domain.EntityTypeUser,
		TokenableID:   1,
		Name:          "Test Token",
		Token:         "test-token",
		Abilities:     &[]domain.PATAbility{},
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, tokensRepo.Save(context.Background(), tok))

	recorder := &auditCapture{}
	handler := deletetoken.NewHandler(tokensRepo, auth.NoopRevocation{}, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodDelete, "/api/tokens/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: owner}))
	rr := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rr, req)

	// ASSERT
	require.Equal(t, http.StatusNoContent, rr.Code,
		"the owner must be able to revoke their token; body=%s", rr.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventPATRevoke),
		"exactly one token.pat.revoke event must be emitted per successful revocation")

	ev, ok := findEvent(events, audit.EventPATRevoke)
	require.True(t, ok, "a successful revocation must leave a token.pat.revoke audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryTokenOp, ev.Category)
	assert.Equal(t, "token", ev.ResourceType)
	assert.Equal(t, "1", ev.ResourceID, "the revoked token id must be recorded")
	assert.Equal(t, "revoke", ev.Action)
	assert.Equal(t, owner.ID, ev.ActorID, "the revoking user must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)
}

// TestHandler_Audit_ForbiddenRevokeIsNotRecorded covers OWASP API8:2023.
// An attempt to revoke another user's token is refused before deletion and
// must NOT emit a token.pat.revoke event.
func TestHandler_Audit_ForbiddenRevokeIsNotRecorded(t *testing.T) {
	// ARRANGE
	now := time.Now()
	attacker := &domain.User{ID: 1, Login: "testuser"}

	tokensRepo := inmemory.NewPersonalAccessTokenRepository()
	othersToken := &domain.PersonalAccessToken{
		TokenableType: domain.EntityTypeUser,
		TokenableID:   2, // belongs to a different user
		Name:          "Other User Token",
		Token:         "other-token",
		Abilities:     &[]domain.PATAbility{},
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, tokensRepo.Save(context.Background(), othersToken))

	recorder := &auditCapture{}
	handler := deletetoken.NewHandler(tokensRepo, auth.NoopRevocation{}, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodDelete, "/api/tokens/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: attacker}))
	rr := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rr, req)

	// ASSERT
	require.Equal(t, http.StatusForbidden, rr.Code,
		"a user must not revoke another user's token; body=%s", rr.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventPATRevoke),
		"a refused revocation must not be recorded as a successful token.pat.revoke")
}
