// Security tests for the plugin catch-all chain (/api/plugins/{plugin_id}/**).
//
// The chain is the one place in the router where authentication is optional by
// design: a plugin declares per route, in its manifest, whether it needs a
// session or an admin. What a plugin may NOT decide is which kind of session
// counts — a short-lived token minted for a URL-borne download, or the stunted
// session of an admin still confined to 2FA enrollment, must be refused before
// the plugin is ever consulted. Both used to sail straight through.
//
// pluginRouteChain is exercised directly rather than through CreateRouter,
// because the in-memory container has no plugin manager and registerPluginRoutes
// therefore registers nothing.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginChainProbe is pluginRouteChain wrapped around a handler that records
// whether the request ever reached the plugin dispatcher.
type pluginChainProbe struct {
	handler   http.Handler
	container *testcontainer.InmemoryContainer
	reached   bool
}

// newPluginChainProbe pins AUTH_REQUIRE_MFA_FOR_ADMINS on: the enrollment
// guard captures the flag at wiring time, and with the feature off a leftover
// enrollment token is honoured as a full session by design (see
// MFAEnrollmentScopeMiddleware), which is not what these tests are about.
func newPluginChainProbe(t *testing.T) *pluginChainProbe {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)

	c.Config().Auth.RequireMFAForAdmins = true

	probe := &pluginChainProbe{container: c}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probe.reached = true
		w.WriteHeader(http.StatusOK)
	})

	probe.handler = pluginRouteChain(
		inner,
		middlewares.NewAuthMiddleware(
			c.AuthService(),
			c.UserService(),
			c.PersonalAccessTokenRepository(),
			auth.NewCacheRevocation(c.Cache()),
			c.Cache(),
			c.Responder(),
			c.AuditLogger(),
		),
		middlewares.NewCORSMiddleware(c.Config()),
		middlewares.NewRecoveryMiddleware(c.Responder()),
		middlewares.NewShortLivedScopeMiddleware(c.Responder(), c.AuditLogger()),
		middlewares.NewMFAEnrollmentScopeMiddleware(
			c.Responder(),
			c.AuditLogger(),
			c.Config().Auth.RequireMFAForAdmins,
		),
	)

	return probe
}

func (p *pluginChainProbe) do(bearerToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/testplugin/admin", nil)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	w := httptest.NewRecorder()
	p.handler.ServeHTTP(w, req)

	return w
}

// seedShortLivedTokenFor stores a short-lived token for user in the container's
// cache — exactly what POST /api/auth/short-lived-token persists — and returns
// the wire token.
func seedShortLivedTokenFor(t *testing.T, probe *pluginChainProbe, user *domain.User) string {
	t.Helper()

	token := auth.ShortLivedTokenPrefix + "pluginChainProbeSecret00112233445566778899xZ"

	encoded, err := auth.MarshalShortLivedPayload(auth.ShortLivedPayload{
		UserID: user.ID,
		Login:  user.Login,
		Email:  user.Email,
	})
	require.NoError(t, err)
	require.NoError(t, probe.container.Cache().Set(
		context.Background(), auth.ShortLivedCacheKey(token), encoded,
	))

	return token
}

//nolint:paralleltest // consistent with the rest of the router suite: CreateRouter-adjacent wiring touches package-global state.
func TestPluginRouteChain_RejectsShortLivedSession(t *testing.T) {
	// ARRANGE
	probe := newPluginChainProbe(t)

	fixtures, err := testcontainer.SetupFixtures(context.Background(), probe.container)
	require.NoError(t, err)

	token := seedShortLivedTokenFor(t, probe, fixtures.AdminUser)

	// ACT
	w := probe.do(token)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.False(t, probe.reached, "the plugin must not be reached by a short-lived token")
	assert.Contains(t, w.Body.String(), "short-lived token is not accepted on this endpoint")
}

//nolint:paralleltest // consistent with the rest of the router suite: CreateRouter-adjacent wiring touches package-global state.
func TestPluginRouteChain_RejectsMFAEnrollmentSession(t *testing.T) {
	// ARRANGE
	probe := newPluginChainProbe(t)

	fixtures, err := testcontainer.SetupFixtures(context.Background(), probe.container)
	require.NoError(t, err)

	token, err := probe.container.AuthService().GenerateMFAEnrollmentToken(fixtures.AdminUser, time.Hour)
	require.NoError(t, err)

	// ACT
	w := probe.do(token)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.False(t, probe.reached, "the plugin must not be reached by an enrollment-scoped session")
	assert.Contains(t, w.Body.String(), "session is restricted to two-factor enrollment")
}

//nolint:paralleltest // consistent with the rest of the router suite: CreateRouter-adjacent wiring touches package-global state.
func TestPluginRouteChain_PassesOrdinarySession(t *testing.T) {
	// ARRANGE
	probe := newPluginChainProbe(t)

	fixtures, err := testcontainer.SetupFixtures(context.Background(), probe.container)
	require.NoError(t, err)

	token, err := probe.container.AuthService().GenerateTokenForUser(fixtures.RegularUser, time.Hour)
	require.NoError(t, err)

	// ACT
	w := probe.do(token)

	// ASSERT
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.True(t, probe.reached, "an ordinary session must reach the plugin")
}

// TestPluginRouteChain_PassesAnonymousCaller pins the deliberate difference
// from the core route loop: the chain authenticates optionally, so a plugin
// that declares a public route keeps working. The plugin's own RequiresAuth /
// AdminOnly flags are what refuse an anonymous caller further in.
//
//nolint:paralleltest // consistent with the rest of the router suite: CreateRouter-adjacent wiring touches package-global state.
func TestPluginRouteChain_PassesAnonymousCaller(t *testing.T) {
	// ARRANGE
	probe := newPluginChainProbe(t)

	// ACT
	w := probe.do("")

	// ASSERT
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.True(t, probe.reached, "the plugin decides for itself whether a route is public")
}
