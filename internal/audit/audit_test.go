// API Security Tests for OWASP API Security Top 10:2023.
// Category: API8:2023 — Security Misconfiguration.
//
// These tests pin actor-identity derivation and the redaction contract of
// the audit helpers (OWASP ASVS 4.0.3 §7.1.3, §7.1.4 — every security event
// must record *who* acted, and submitted-but-unauthenticated identifiers must
// never be recorded as the actor). A regression here would make the audit
// trail attribute actions to the wrong principal or leak/blur identity.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package audit_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogger records the last Event it received. Safe for concurrent use
// so it can also back the router integration test.
type captureLogger struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *captureLogger) Record(_ context.Context, e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureLogger) last() (audit.Event, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return audit.Event{}, false
	}

	return c.events[len(c.events)-1], true
}

// extraString returns the string value of the first Extra attr with key.
func extraString(e audit.Event, key string) (string, bool) {
	for _, a := range e.Extra {
		if a.Key == key {
			return a.Value.String(), true
		}
	}

	return "", false
}

func userSessionCtx(id uint, login string) context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		User: &domain.User{ID: id, Login: login},
	})
}

func patSessionCtx(id uint, login string, tokenID uint) context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		User:  &domain.User{ID: id, Login: login},
		Token: &domain.PersonalAccessToken{ID: tokenID},
	})
}

func shortLivedSessionCtx(id uint, login string) context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		User:       &domain.User{ID: id, Login: login},
		ShortLived: true,
	})
}

func patDerivedShortLivedSessionCtx(id uint, login string, tokenID uint) context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		User:       &domain.User{ID: id, Login: login},
		Token:      &domain.PersonalAccessToken{ID: tokenID},
		ShortLived: true,
	})
}

// TestActorDerivation covers OWASP API8:2023.
// The actor on an event with no preset AuthMethod must be derived from the
// request context: user session → session, PAT session → pat, daemon
// session → daemon, nothing → anonymous.
func TestActorDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ctx            context.Context
		wantAuthMethod audit.AuthMethod
		wantActorID    uint
		wantActorLogin string
	}{
		{
			name:           "user_session_yields_session_actor",
			ctx:            userSessionCtx(7, "alice"),
			wantAuthMethod: audit.AuthMethodSession,
			wantActorID:    7,
			wantActorLogin: "alice",
		},
		{
			name:           "pat_session_yields_pat_actor",
			ctx:            patSessionCtx(7, "alice", 99),
			wantAuthMethod: audit.AuthMethodPAT,
			wantActorID:    7,
			wantActorLogin: "alice",
		},
		{
			name:           "no_session_yields_anonymous",
			ctx:            context.Background(),
			wantAuthMethod: audit.AuthMethodAnonymous,
			wantActorID:    0,
			wantActorLogin: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			recorder := &captureLogger{}

			// ACT
			// SensitiveOp leaves AuthMethod empty, so actor derivation runs.
			audit.SensitiveOp(tt.ctx, recorder, audit.EventNodeUpdate, audit.CategoryNodeOp,
				"node", "1", "update")

			// ASSERT
			got, ok := recorder.last()
			require.True(t, ok, "an event must have been recorded")
			assert.Equal(t, tt.wantAuthMethod, got.AuthMethod, "auth_method must reflect the session kind")
			assert.Equal(t, tt.wantActorID, got.ActorID, "actor_id must be taken from the session principal")
			assert.Equal(t, tt.wantActorLogin, got.ActorLogin, "actor_login must be taken from the session principal")
		})
	}
}

// TestActorDerivation_ShortLivedSession covers OWASP API8:2023.
// An action performed with a single-use short-lived token must be attributed
// to the short-lived auth method (so the audit trail distinguishes it from a
// full session) while still recording the acting principal. The ShortLived
// classification must take precedence over the PAT classification: a
// PAT-derived short-lived session is short-lived first, so a leaked URL token
// is never logged as if it were the long-lived PAT itself.
func TestActorDerivation_ShortLivedSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ctx            context.Context
		wantAuthMethod audit.AuthMethod
		wantActorID    uint
		wantActorLogin string
	}{
		{
			name:           "session_derived_short_lived_yields_shortlived_actor",
			ctx:            shortLivedSessionCtx(7, "alice"),
			wantAuthMethod: audit.AuthMethodShortLived,
			wantActorID:    7,
			wantActorLogin: "alice",
		},
		{
			name:           "pat_derived_short_lived_still_yields_shortlived_actor",
			ctx:            patDerivedShortLivedSessionCtx(7, "alice", 99),
			wantAuthMethod: audit.AuthMethodShortLived,
			wantActorID:    7,
			wantActorLogin: "alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			recorder := &captureLogger{}

			// ACT
			// SensitiveOp leaves AuthMethod empty, so actor derivation runs.
			audit.SensitiveOp(tt.ctx, recorder, audit.EventNodeUpdate, audit.CategoryNodeOp,
				"node", "1", "update")

			// ASSERT
			got, ok := recorder.last()
			require.True(t, ok, "an event must have been recorded")
			assert.Equal(t, tt.wantAuthMethod, got.AuthMethod,
				"a short-lived session must be attributed shortlived, never session/pat")
			assert.Equal(t, tt.wantActorID, got.ActorID,
				"actor_id must still be taken from the session principal")
			assert.Equal(t, tt.wantActorLogin, got.ActorLogin,
				"actor_login must still be taken from the session principal")
		})
	}
}

// TestActorDerivation_PresetAuthMethodNotOverwritten covers OWASP API8:2023.
// When a helper sets AuthMethod explicitly the context-derived identity must
// not clobber it (e.g. LoginSuccess carries the freshly-authenticated user
// before the session exists in context).
func TestActorDerivation_PresetAuthMethodNotOverwritten(t *testing.T) {
	t.Parallel()

	t.Run("login_success_keeps_explicit_actor_even_with_no_session", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		recorder := &captureLogger{}

		// ACT
		audit.LoginSuccess(context.Background(), recorder, 42, "carol")

		// ASSERT
		got, ok := recorder.last()
		require.True(t, ok)
		assert.Equal(t, audit.AuthMethodSession, got.AuthMethod, "LoginSuccess must keep its explicit session method")
		assert.Equal(t, uint(42), got.ActorID, "explicit actor id must not be reset to anonymous")
		assert.Equal(t, "carol", got.ActorLogin, "explicit actor login must not be reset to anonymous")
		assert.Equal(t, audit.OutcomeSuccess, got.Outcome)
		assert.Equal(t, audit.EventLoginSuccess, got.Type)
	})

	t.Run("preset_auth_method_survives_a_conflicting_context_session", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		recorder := &captureLogger{}
		// A user session is in context, but LoginSuccess presets the actor.
		ctx := userSessionCtx(1, "from-context")

		// ACT
		audit.LoginSuccess(ctx, recorder, 42, "explicit")

		// ASSERT
		got, ok := recorder.last()
		require.True(t, ok)
		assert.Equal(t, uint(42), got.ActorID, "preset actor must win over the context session")
		assert.Equal(t, "explicit", got.ActorLogin, "preset actor must win over the context session")
	})
}

// TestLoginFailureAndBlocked_NeverRecordActorLogin covers OWASP API8:2023.
// The submitted identifier of a failed/blocked login is attacker-controlled
// and unauthenticated. It must be recorded as the attempted_login Extra attr,
// the actor must stay anonymous, and ActorLogin must remain empty.
func TestLoginFailureAndBlocked_NeverRecordActorLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		invoke    func(ctx context.Context, l audit.Logger)
		wantType  audit.EventType
		wantOut   audit.Outcome
		wantLogin string
	}{
		{
			name: "login_failure_puts_submitted_login_in_attempted_login",
			invoke: func(ctx context.Context, l audit.Logger) {
				audit.LoginFailure(ctx, l, "victim-account", "invalid_credentials")
			},
			wantType:  audit.EventLoginFailure,
			wantOut:   audit.OutcomeFailure,
			wantLogin: "victim-account",
		},
		{
			name: "login_blocked_puts_submitted_login_in_attempted_login",
			invoke: func(ctx context.Context, l audit.Logger) {
				audit.LoginBlocked(ctx, l, "victim-account", "ip")
			},
			wantType:  audit.EventLoginBlocked,
			wantOut:   audit.OutcomeBlocked,
			wantLogin: "victim-account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			recorder := &captureLogger{}

			// ACT
			tt.invoke(context.Background(), recorder)

			// ASSERT
			got, ok := recorder.last()
			require.True(t, ok)
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.wantOut, got.Outcome)
			assert.Equal(t, audit.AuthMethodAnonymous, got.AuthMethod, "a refused login must stay anonymous")
			assert.Zero(t, got.ActorID, "no actor id for an unauthenticated attempt")
			assert.Empty(t, got.ActorLogin, "submitted identifier must never be recorded as actor_login")

			attempted, ok := extraString(got, "attempted_login")
			require.True(t, ok, "attempted_login Extra attr must be present")
			assert.Equal(t, tt.wantLogin, attempted, "submitted identifier must be recorded as attempted_login")
		})
	}
}

// TestLoginFailure_DoesNotDeriveActorFromContextSession covers OWASP API8:2023.
// Even if a (stale) session lingers in context, a failed login must not be
// attributed to that principal — it presets AuthMethod=anonymous.
func TestLoginFailure_DoesNotDeriveActorFromContextSession(t *testing.T) {
	t.Parallel()

	// ARRANGE
	recorder := &captureLogger{}
	ctx := userSessionCtx(7, "alice")

	// ACT
	audit.LoginFailure(ctx, recorder, "alice", "invalid_credentials")

	// ASSERT
	got, ok := recorder.last()
	require.True(t, ok)
	assert.Equal(t, audit.AuthMethodAnonymous, got.AuthMethod)
	assert.Zero(t, got.ActorID, "a failed login must not borrow the context session's actor id")
	assert.Empty(t, got.ActorLogin)
}

// TestHelpers_EventShape covers OWASP API8:2023.
// Locks the (type, category, outcome, reason) tuple each thin helper builds
// so the stable audit contract cannot drift.
func TestHelpers_EventShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		invoke       func(ctx context.Context, l audit.Logger)
		wantType     audit.EventType
		wantCategory audit.Category
		wantOutcome  audit.Outcome
		wantReason   string
	}{
		{
			name: "token_rejected",
			invoke: func(ctx context.Context, l audit.Logger) {
				audit.TokenRejected(ctx, l, "missing_token")
			},
			wantType:     audit.EventAuthTokenRejected,
			wantCategory: audit.CategoryAuthentication,
			wantOutcome:  audit.OutcomeFailure,
			wantReason:   "missing_token",
		},
		{
			name: "daemon_rejected",
			invoke: func(ctx context.Context, l audit.Logger) {
				audit.DaemonRejected(ctx, l, "bad_token")
			},
			wantType:     audit.EventAuthDaemonRejected,
			wantCategory: audit.CategoryAuthentication,
			wantOutcome:  audit.OutcomeFailure,
			wantReason:   "bad_token",
		},
		{
			name: "access_denied",
			invoke: func(ctx context.Context, l audit.Logger) {
				audit.AccessDenied(ctx, l, "server", "5", "missing_ability")
			},
			wantType:     audit.EventAccessDenied,
			wantCategory: audit.CategoryAuthorization,
			wantOutcome:  audit.OutcomeDenied,
			wantReason:   "missing_ability",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			recorder := &captureLogger{}

			// ACT
			tt.invoke(context.Background(), recorder)

			// ASSERT
			got, ok := recorder.last()
			require.True(t, ok)
			assert.Equal(t, tt.wantType, got.Type, "event_type is part of the stable contract")
			assert.Equal(t, tt.wantCategory, got.Category)
			assert.Equal(t, tt.wantOutcome, got.Outcome)
			assert.Equal(t, tt.wantReason, got.Reason)
		})
	}
}

// TestAccessDenied_DerivesActorAndCarriesResource covers OWASP API8:2023.
// AccessDenied runs after authentication, so the denied principal must be
// derived from context and the targeted resource recorded for forensics.
func TestAccessDenied_DerivesActorAndCarriesResource(t *testing.T) {
	t.Parallel()

	// ARRANGE
	recorder := &captureLogger{}
	ctx := userSessionCtx(2, "bob")

	// ACT
	audit.AccessDenied(ctx, recorder, "server", "9", "missing_ability",
		slog.String("required_abilities", "server:start"))

	// ASSERT
	got, ok := recorder.last()
	require.True(t, ok)
	assert.Equal(t, audit.AuthMethodSession, got.AuthMethod, "the denied principal must be attributed")
	assert.Equal(t, uint(2), got.ActorID)
	assert.Equal(t, "bob", got.ActorLogin)
	assert.Equal(t, "server", got.ResourceType)
	assert.Equal(t, "9", got.ResourceID)

	abilities, ok := extraString(got, "required_abilities")
	require.True(t, ok, "extra context must be forwarded")
	assert.Equal(t, "server:start", abilities)
}

// TestSensitiveOp_RecordsSuccessWithResource covers OWASP API8:2023.
// SensitiveOp records the successful execution of a sensitive operation with
// the acting principal and the targeted resource/action.
func TestSensitiveOp_RecordsSuccessWithResource(t *testing.T) {
	t.Parallel()

	// ARRANGE
	recorder := &captureLogger{}
	ctx := patSessionCtx(7, "alice", 99)

	// ACT
	audit.SensitiveOp(ctx, recorder, audit.EventPATCreate, audit.CategoryTokenOp,
		"token", "42", "create", slog.Int("abilities", 2))

	// ASSERT
	got, ok := recorder.last()
	require.True(t, ok)
	assert.Equal(t, audit.EventPATCreate, got.Type)
	assert.Equal(t, audit.CategoryTokenOp, got.Category)
	assert.Equal(t, audit.OutcomeSuccess, got.Outcome, "a sensitive op records success")
	assert.Equal(t, audit.AuthMethodPAT, got.AuthMethod, "actor must be derived from the PAT session")
	assert.Equal(t, uint(7), got.ActorID)
	assert.Equal(t, "token", got.ResourceType)
	assert.Equal(t, "42", got.ResourceID)
	assert.Equal(t, "create", got.Action)
}
