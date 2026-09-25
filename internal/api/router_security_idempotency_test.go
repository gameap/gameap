// API Security Tests for OWASP API Security Top 10:2023 — Idempotency-Key.
// Categories:
//   - API1:2023 — Broken Object Level Authorization: a stored response belongs to
//     the user who made the request; another user sending the same key must not
//     receive it.
//   - API3:2023 — Broken Object Property Level Authorization and API8:2023 —
//     Security Misconfiguration: routes whose responses carry secrets must not
//     store them for replay.
//   - API5:2023 — Broken Function Level Authorization: a replay must not bypass
//     the PAT ability checks that run before the idempotency layer.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRouterSecurity_API1_IdempotencyKeyScopedToUser verifies API1:2023 — the
// same key from another user does not replay the first user's stored response.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_IdempotencyKeyScopedToUser(t *testing.T) {
	env := setupSecurityTest(t)
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)
	userToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	admin := postIdempotent(t, env, "/api/servers/1/start", adminToken, "shared-key", nil)
	require.Equalf(t, http.StatusOK, admin.Code, "body=%s", admin.Body.String())

	user := postIdempotent(t, env, "/api/servers/1/start", userToken, "shared-key", nil)

	assert.Empty(t, user.Header().Get("Idempotent-Replayed"),
		"another user's stored response must not be replayed")
	assert.NotEqual(t, admin.Body.String(), user.Body.String())
}

// TestRouterSecurity_API3_SecretRoutesDoNotStoreResponses verifies API3:2023 /
// API8:2023 — POST /api/tokens ignores the key, so the plaintext token is
// never stored for replay and every request mints a new one.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API3_SecretRoutesDoNotStoreResponses(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)
	body := []byte(`{"token_name":"billing","abilities":["server:list"]}`)

	first := postIdempotent(t, env, "/api/tokens", token, "token-key", body)
	second := postIdempotent(t, env, "/api/tokens", token, "token-key", body)

	require.Equalf(t, http.StatusOK, first.Code, "body=%s", first.Body.String())
	require.Equalf(t, http.StatusOK, second.Code, "body=%s", second.Body.String())
	assert.Empty(t, second.Header().Get("Idempotent-Replayed"))

	var firstToken, secondToken struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstToken))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondToken))
	assert.NotEmpty(t, firstToken.Token)
	assert.NotEqual(t, firstToken.Token, secondToken.Token)
}

// TestRouterSecurity_API3_RconOutputIsNotStored verifies API3:2023 / API8:2023 —
// POST /api/servers/{server}/rcon ignores the key: RCON output can carry cvar
// values and player IPs, so it is never stored for replay and every request
// runs.
//
//nolint:paralleltest // api.CreateRouter mutates the package-global ability-check audit sink.
func TestRouterSecurity_API3_RconOutputIsNotStored(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)
	body := []byte(`{"command":"status"}`)

	first := postIdempotent(t, env, "/api/servers/1/rcon", token, "rcon-key", body)
	second := postIdempotent(t, env, "/api/servers/1/rcon", token, "rcon-key", body)

	// The fixture server is offline, so the handler itself answers 503, which
	// the idempotency layer would have stored and replayed.
	require.Equalf(t, http.StatusServiceUnavailable, first.Code, "body=%s", first.Body.String())
	assert.Equal(t, http.StatusServiceUnavailable, second.Code, second.Body.String())
	assert.Empty(t, second.Header().Get("Idempotent-Replayed"))
}

// TestRouterSecurity_API5_ReplayDoesNotBypassPATAbilities verifies API5:2023 —
// a token lacking the route's ability is refused before the idempotency layer,
// even when the key already holds a stored response of the same user.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_ReplayDoesNotBypassPATAbilities(t *testing.T) {
	env := setupSecurityTest(t)
	startToken := issuePAT(t, env, env.fixtures.AdminUser, []domain.PATAbility{domain.PATAbilityServerStart})
	listToken := issuePAT(t, env, env.fixtures.AdminUser, []domain.PATAbility{domain.PATAbilityServerList})

	stored := postIdempotent(t, env, "/api/servers/1/start", startToken, "start-key", nil)
	require.Equalf(t, http.StatusOK, stored.Code, "body=%s", stored.Body.String())

	replayAttempt := postIdempotent(t, env, "/api/servers/1/start", listToken, "start-key", nil)

	assert.Equal(t, http.StatusForbidden, replayAttempt.Code)
	assert.Empty(t, replayAttempt.Header().Get("Idempotent-Replayed"))
	assert.Contains(t, replayAttempt.Body.String(), "missing required ability: server:start")
}

//nolint:paralleltest // api.CreateRouter mutates the package-global ability-check audit sink.
func TestRouterSecurity_IdempotencyRechecksServerAccess(t *testing.T) {
	endpoints := []struct {
		method    string
		path      string
		abilities []domain.AbilityName
	}{
		{http.MethodPost, "/api/servers/1/start", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerStart}},
		{http.MethodPost, "/api/servers/1/stop", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerStop}},
		{http.MethodPost, "/api/servers/1/restart", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerRestart}},
		{http.MethodPost, "/api/servers/1/update", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate}},
		{http.MethodPost, "/api/servers/1/install", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate}},
		{http.MethodPost, "/api/servers/1/reinstall", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate}},
		{http.MethodPost, "/api/servers/1/rcon/players/kick", []domain.AbilityName{domain.AbilityNameGameServerRconPlayers}},
		{http.MethodPost, "/api/servers/1/rcon/players/ban", []domain.AbilityName{domain.AbilityNameGameServerRconPlayers}},
		{http.MethodPost, "/api/servers/1/console", []domain.AbilityName{domain.AbilityNameGameServerConsoleSend}},
		{http.MethodPost, "/api/servers/1/tasks", []domain.AbilityName{domain.AbilityNameGameServerTasks}},
		{http.MethodPut, "/api/servers/1/tasks/999", []domain.AbilityName{domain.AbilityNameGameServerTasks}},
		{http.MethodDelete, "/api/servers/1/tasks/999", []domain.AbilityName{domain.AbilityNameGameServerTasks}},
		{http.MethodPut, "/api/servers/1/settings", []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerSettings}},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.method+endpoint.path, func(t *testing.T) {
			changes := []struct {
				name   string
				status int
				apply  func(*securityTestEnv)
			}{
				{"ownership_revoked", http.StatusNotFound, func(env *securityTestEnv) {
					require.NoError(t, env.container.ServerRepository().SetUserServers(env.ctx, env.fixtures.RegularUser.ID, nil))
				}},
				{"server_blocked", http.StatusForbidden, func(env *securityTestEnv) {
					server := *env.fixtures.Server1
					server.Blocked = true
					require.NoError(t, env.container.ServerRepository().Save(env.ctx, &server))
				}},
			}
			for _, ability := range endpoint.abilities {
				changes = append(changes, struct {
					name   string
					status int
					apply  func(*securityTestEnv)
				}{"revoke_" + string(ability), http.StatusForbidden, func(env *securityTestEnv) {
					require.NoError(t, env.container.RBAC().RevokeOrForbidUserAbilitiesForEntity(
						env.ctx, env.fixtures.RegularUser.ID, 1, domain.EntityTypeServer, []domain.AbilityName{ability},
					))
				}})
			}

			for _, change := range changes {
				t.Run(change.name, func(t *testing.T) {
					env := setupSecurityTest(t)
					token := issuePASETOToken(t, env, env.fixtures.RegularUser)
					send := func() *httptest.ResponseRecorder {
						req := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewBufferString(`{}`))
						req.Header.Set("Authorization", "Bearer "+token)
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Idempotency-Key", "server-request")

						return doRequestRaw(t, env, req)
					}

					first := send()
					retry := send()
					require.Equal(t, first.Code, retry.Code, retry.Body.String())
					require.Equal(t, "true", retry.Header().Get("Idempotent-Replayed"))

					change.apply(env)
					denied := send()
					assert.Equal(t, change.status, denied.Code, denied.Body.String())
					assert.Empty(t, denied.Header().Get("Idempotent-Replayed"))
				})
			}
		})
	}
}

//nolint:paralleltest // api.CreateRouter mutates the package-global ability-check audit sink.
func TestRouterSecurity_IdempotencyPreservesRoutesWithoutCommonAbility(t *testing.T) {
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPost, "/api/servers/1/rcon/players/kick"},
		{http.MethodPost, "/api/servers/1/console"},
		{http.MethodPost, "/api/servers/1/tasks"},
		{http.MethodPut, "/api/servers/1/tasks/999"},
		{http.MethodDelete, "/api/servers/1/tasks/999"},
	} {
		t.Run(endpoint.method+endpoint.path, func(t *testing.T) {
			env := setupSecurityTest(t)
			token := issuePASETOToken(t, env, env.fixtures.RegularUser)
			require.NoError(t, env.container.RBAC().RevokeOrForbidUserAbilitiesForEntity(
				env.ctx, env.fixtures.RegularUser.ID, 1, domain.EntityTypeServer,
				[]domain.AbilityName{domain.AbilityNameGameServerCommon},
			))
			send := func() *httptest.ResponseRecorder {
				req := httptest.NewRequest(endpoint.method, endpoint.path, bytes.NewBufferString(`{}`))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", "server-request")

				return doRequestRaw(t, env, req)
			}

			first := send()
			retry := send()
			assert.Equal(t, first.Code, retry.Code, retry.Body.String())
			assert.Equal(t, first.Body.String(), retry.Body.String())
			assert.Equal(t, "true", retry.Header().Get("Idempotent-Replayed"))
		})
	}
}
