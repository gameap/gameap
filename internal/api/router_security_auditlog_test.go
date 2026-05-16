// API Security Tests for OWASP API Security Top 10:2023.
// Categories:
//   - API2:2023 — Broken Authentication (rejected-token audit trail)
//   - API5:2023 — Broken Function Level Authorization (admin-gate denial trail)
//   - API8:2023 — Security Misconfiguration (a security-relevant operation
//     that is not recorded is a detective-control gap; OWASP ASVS §7.1–7.2)
//
// These tests assert that the production router, wired end-to-end via the
// in-memory container, emits the expected audit events with the correct
// outcome and actor attribution for: an unauthenticated request to a
// protected route, an authenticated-but-unauthorized request to an admin
// route, and a successful sensitive operation (PAT creation).
//
// NOTE: api.CreateRouter builds the mux only — the process-level
// RequestContextMiddleware (request_id / client IP capture) is wired in
// container.createHTTPServer, not here. So this harness legitimately does not
// populate RequestInfo; request_id/ip enrichment is covered by the
// internal/audit middleware tests. These tests therefore assert
// event_type / outcome / actor only.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the router emits during a request.
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

// findEvent returns the first recorded event of the given type.
func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

// setupAuditRouterEnv builds an in-memory container with seeded fixtures and a
// capturing audit logger injected BEFORE the router is created (CreateRouter
// snapshots the logger into handlers and the process-wide ability-check sink).
func setupAuditRouterEnv(t *testing.T) (*securityTestEnv, *auditCapture) {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)

	recorder := &auditCapture{}
	c.SetAuditLogger(recorder)

	ctx := context.Background()
	fixtures, err := testcontainer.SetupFixtures(ctx, c)
	require.NoError(t, err)

	env := &securityTestEnv{
		container: c,
		fixtures:  fixtures,
		router:    api.CreateRouter(c),
		ctx:       ctx,
	}

	return env, recorder
}

// TestRouterSecurity_AuditLog_UnauthenticatedRequestIsRecorded covers OWASP
// API2:2023 / API8:2023. An unauthenticated request to a protected route must
// be rejected (401) AND leave an auth.token.rejected audit event with outcome
// failure and a stable, non-empty reason.
func TestRouterSecurity_AuditLog_UnauthenticatedRequestIsRecorded(t *testing.T) {
	// ARRANGE
	env, recorder := setupAuditRouterEnv(t)

	// ACT
	w := doRequest(t, env, http.MethodGet, "/api/user", "")

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "protected route must reject anon; body=%s", w.Body.String())

	ev, ok := findEvent(recorder.snapshot(), audit.EventAuthTokenRejected)
	require.True(t, ok, "a rejected-token audit event must be emitted for an unauthenticated request")
	assert.Equal(t, audit.OutcomeFailure, ev.Outcome, "a rejected auth attempt is a failure")
	assert.Equal(t, audit.CategoryAuthentication, ev.Category)
	assert.Equal(t, "missing_token", ev.Reason, "the failure reason must be the stable missing_token token")
	assert.NotEmpty(t, ev.Reason, "a rejection must always carry a reason for forensics")
}

// TestRouterSecurity_AuditLog_AdminGateDenialIsRecorded covers OWASP
// API5:2023 / API8:2023. An authenticated non-admin hitting an admin-only
// route must be denied (403) AND leave an access.denied audit event with
// outcome denied attributed to the authenticated principal.
func TestRouterSecurity_AuditLog_AdminGateDenialIsRecorded(t *testing.T) {
	// ARRANGE
	env, recorder := setupAuditRouterEnv(t)
	regularToken, err := env.container.AuthService().GenerateTokenForUser(env.fixtures.RegularUser, time.Hour)
	require.NoError(t, err)

	// ACT
	w := doRequest(t, env, http.MethodGet, "/api/users", regularToken)

	// ASSERT
	require.Equal(t, http.StatusForbidden, w.Code,
		"non-admin must be denied the admin route; body=%s", w.Body.String())

	ev, ok := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	require.True(t, ok, "an access-denied audit event must be emitted for an unauthorized admin-route access")
	assert.Equal(t, audit.OutcomeDenied, ev.Outcome, "an authorization refusal is a denial")
	assert.Equal(t, audit.CategoryAuthorization, ev.Category)
	assert.Equal(t, "admin", ev.ResourceType, "the admin gate guards the \"admin\" resource")
	assert.Equal(t, "admin_required", ev.Reason, "the stable denial reason must be recorded")
	assert.Equal(t, env.fixtures.RegularUser.ID, ev.ActorID,
		"the denied principal must be attributed to the authenticated non-admin user")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)
}

// TestRouterSecurity_AuditLog_SensitiveOpSuccessIsRecorded covers OWASP
// API8:2023. A successful sensitive operation (PAT creation) must be recorded
// with outcome success and the acting principal so token issuance is
// auditable (OWASP ASVS §7.2.1).
func TestRouterSecurity_AuditLog_SensitiveOpSuccessIsRecorded(t *testing.T) {
	// ARRANGE
	env, recorder := setupAuditRouterEnv(t)
	regularToken, err := env.container.AuthService().GenerateTokenForUser(env.fixtures.RegularUser, time.Hour)
	require.NoError(t, err)

	body := []byte(`{"token_name":"ci-token","abilities":["` + string(domain.PATAbilityServerList) + `"]}`)

	// ACT
	w := doRequestWithBody(t, env, http.MethodPost, "/api/tokens", regularToken, body)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "PAT creation must succeed; body=%s", w.Body.String())

	ev, ok := findEvent(recorder.snapshot(), audit.EventPATCreate)
	require.True(t, ok, "a PAT-create audit event must be emitted on success")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryTokenOp, ev.Category)
	assert.Equal(t, "token", ev.ResourceType)
	assert.Equal(t, "create", ev.Action)
	assert.NotEmpty(t, ev.ResourceID, "the created token id must be recorded as resource_id")
	assert.Equal(t, env.fixtures.RegularUser.ID, ev.ActorID,
		"the creating user must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)
}
