// Route authorization matrix — the exhaustive, checked-in expectation of who
// may reach every endpoint of the /api tree.
//
// The other router_security_*_test.go files probe hand-picked endpoints. This
// one is the completeness net: it enumerates what apiRoutes actually registers
// and refuses to pass while a single route is missing from the matrix below.
// Adding an endpoint therefore forces an explicit decision about its audience,
// which is the one thing a spot-check table can never guarantee.
//
// It lives in package api (not api_test) because the route table is a local
// variable inside the unexported apiRoutes; walking the router it fills is the
// only way to enumerate the real thing rather than a copy that can drift.
//
// Every probe here is answered by middleware and short-circuits before the
// handler, so the suite never touches a daemon, a node or the plugin store.

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// routeClass is the audience a route is gated to by the router itself, before
// any object-level check the handler may add on top.
type routeClass int

const (
	// classGuest — reachable without a session at all.
	classGuest routeClass = iota
	// classAuthenticated — every authenticated user passes the router gates.
	// Which objects they may then touch is decided in the handler, by
	// serversbase.ServerFinder and serversbase.AbilityChecker.
	classAuthenticated
	// classAdmin — IsAdminMiddleware demands the global
	// "admin roles & permissions" ability, and TokenAdminGuardMiddleware keeps
	// personal access tokens out unless the route declares its own abilities.
	classAdmin
)

func (c routeClass) String() string {
	switch c {
	case classGuest:
		return "guest"
	case classAuthenticated:
		return "authenticated"
	case classAdmin:
		return "admin"
	default:
		return fmt.Sprintf("routeClass(%d)", int(c))
	}
}

type routeExpectation struct {
	class routeClass

	// patScoped records that the route declares CheckPATAbilities, so a
	// personal access token missing them is refused before the handler runs.
	// An authenticated route that leaves it false hands every token the full
	// authority of its owner, which is why note is then mandatory.
	patScoped bool

	// note justifies an authenticated route that deliberately accepts any
	// token. Required by TestRouteAuthzMatrix_UnscopedRoutesCarryAJustification.
	note string
}

const (
	noteSelfScoped = "self-scoped: the handler only ever reads or writes the calling user's own record"

	noteTaskStatus = "the completion signal of a control action; the action that created the task " +
		"(start/stop/restart/update/install) is already scoped, and the handler re-checks access to " +
		"the task's server. Scoping it to a single ability would break clients that legitimately " +
		"watch a task they were allowed to create"
)

// routeAuthzMatrix is the expectation, not a copy of the router: the class is
// asserted by driving real requests, and patScoped by driving a token that
// holds no abilities at all.
//
//nolint:gochecknoglobals
var routeAuthzMatrix = map[string]routeExpectation{
	// Health & public config.
	"GET /api/health":        {class: classGuest},
	"GET /api/config/public": {class: classGuest},

	// Auth.
	"POST /api/auth/login":             {class: classGuest},
	"POST /api/auth/2fa/verify":        {class: classGuest},
	"POST /api/auth/logout":            {class: classAuthenticated, note: noteSelfScoped},
	"POST /api/auth/short-lived-token": {class: classAuthenticated, note: noteSelfScoped},

	// Current user & profile.
	"GET /api/user":                        {class: classAuthenticated, note: noteSelfScoped},
	"GET /api/profile":                     {class: classAuthenticated, note: noteSelfScoped},
	"PUT /api/profile":                     {class: classAuthenticated, note: noteSelfScoped},
	"POST /api/profile/2fa/setup":          {class: classAuthenticated, note: noteSelfScoped},
	"POST /api/profile/2fa/confirm":        {class: classAuthenticated, note: noteSelfScoped},
	"DELETE /api/profile/2fa":              {class: classAuthenticated, note: noteSelfScoped},
	"POST /api/profile/2fa/recovery-codes": {class: classAuthenticated, note: noteSelfScoped},
	"POST /api/profile/2fa/snooze":         {class: classAuthenticated, note: noteSelfScoped},

	// Personal access tokens. posttoken additionally refuses token sessions
	// outright, so a PAT can never mint another one.
	"GET /api/tokens":           {class: classAuthenticated, note: noteSelfScoped},
	"POST /api/tokens":          {class: classAuthenticated, note: noteSelfScoped},
	"DELETE /api/tokens/{id}":   {class: classAuthenticated, note: noteSelfScoped},
	"GET /api/tokens/abilities": {class: classAuthenticated, note: noteSelfScoped},

	// Servers.
	"GET /api/servers":                    {class: classAuthenticated, patScoped: true},
	"POST /api/servers":                   {class: classAdmin, patScoped: true},
	"GET /api/servers/summary":            {class: classAuthenticated, patScoped: true},
	"GET /api/servers/search":             {class: classAdmin},
	"GET /api/servers/{id}":               {class: classAuthenticated, patScoped: true},
	"PUT /api/servers/{id}":               {class: classAdmin, patScoped: true},
	"DELETE /api/servers/{id}":            {class: classAdmin, patScoped: true},
	"GET /api/servers/{server}/abilities": {class: classAuthenticated, patScoped: true},
	"GET /api/servers/{server}/status":    {class: classAuthenticated, patScoped: true},
	"GET /api/servers/{server}/query":     {class: classAuthenticated, patScoped: true},
	"GET /api/user/servers_abilities":     {class: classAuthenticated, patScoped: true},

	// RCON.
	"GET /api/servers/{server}/rcon/features":           {class: classAuthenticated, patScoped: true},
	"GET /api/servers/{server}/rcon/fast_rcon":          {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/rcon":                   {class: classAuthenticated, patScoped: true},
	"GET /api/servers/{server}/rcon/players":            {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/rcon/players/message":   {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/rcon/players/{command}": {class: classAuthenticated, patScoped: true},

	// Console.
	"GET /api/servers/{server}/console":  {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/console": {class: classAuthenticated, patScoped: true},

	// Server control.
	"POST /api/servers/{server}/start":     {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/stop":      {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/restart":   {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/update":    {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/install":   {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/reinstall": {class: classAuthenticated, patScoped: true},

	// Scheduled server tasks & settings.
	"GET /api/servers/{server}/tasks":                 {class: classAuthenticated, patScoped: true},
	"POST /api/servers/{server}/tasks":                {class: classAuthenticated, patScoped: true},
	"PUT /api/servers/{server}/tasks/{id}":            {class: classAuthenticated, patScoped: true},
	"DELETE /api/servers/{server}/tasks/{id}":         {class: classAuthenticated, patScoped: true},
	"GET /api/servers/{server}/tasks/{id}/executions": {class: classAuthenticated, patScoped: true},
	"GET /api/servers/{server}/settings":              {class: classAuthenticated, patScoped: true},
	"PUT /api/servers/{server}/settings":              {class: classAuthenticated, patScoped: true},

	// File manager. Writing here means writing into the game server directory
	// on the node, so every route demands server:files.
	"GET /api/file-manager/{server}/initialize":                                {class: classAuthenticated, patScoped: true},
	"GET /api/file-manager/{server}/content":                                   {class: classAuthenticated, patScoped: true},
	"GET /api/file-manager/{server}/tree":                                      {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/delete":                                   {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/upload":                                   {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/upload/sessions":                          {class: classAuthenticated, patScoped: true},
	"PUT /api/file-manager/{server}/upload/sessions/{uploadID}/chunks/{index}": {class: classAuthenticated, patScoped: true},
	"GET /api/file-manager/{server}/upload/sessions/{uploadID}":                {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/upload/sessions/{uploadID}/complete":      {class: classAuthenticated, patScoped: true},
	"DELETE /api/file-manager/{server}/upload/sessions/{uploadID}":             {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/update-file":                              {class: classAuthenticated, patScoped: true},
	"GET /api/file-manager/{server}/download":                                  {class: classAuthenticated, patScoped: true},
	"GET /api/file-manager/{server}/download-archive":                          {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/rename":                                   {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/chmod":                                    {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/create-directory":                         {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/create-file":                              {class: classAuthenticated, patScoped: true},
	"GET /api/file-manager/{server}/stream-file":                               {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/paste":                                    {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/hash":                                     {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/archive":                                  {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/extract":                                  {class: classAuthenticated, patScoped: true},
	"POST /api/file-manager/{server}/archive-operations/{operationID}/cancel":  {class: classAuthenticated, patScoped: true},

	// Users.
	"GET /api/users":                                   {class: classAdmin},
	"POST /api/users":                                  {class: classAdmin},
	"GET /api/users/{id}":                              {class: classAdmin},
	"PUT /api/users/{id}":                              {class: classAdmin},
	"DELETE /api/users/{id}":                           {class: classAdmin},
	"GET /api/users/{id}/servers":                      {class: classAdmin},
	"GET /api/users/{id}/servers/{server}/permissions": {class: classAdmin},
	"PUT /api/users/{id}/servers/{server}/permissions": {class: classAdmin},

	// Nodes, and the /api/dedicated_servers aliases of the same handlers.
	"GET /api/nodes":                               {class: classAdmin},
	"GET /api/nodes/summary":                       {class: classAdmin},
	"GET /api/nodes/{id}":                          {class: classAdmin},
	"PUT /api/nodes/{id}":                          {class: classAdmin},
	"DELETE /api/nodes/{id}":                       {class: classAdmin},
	"GET /api/nodes/setup":                         {class: classAdmin},
	"GET /api/nodes/setup-key":                     {class: classAdmin},
	"POST /api/nodes/setup-key":                    {class: classAdmin},
	"DELETE /api/nodes/setup-key":                  {class: classAdmin},
	"GET /api/nodes/certificates.zip":              {class: classAdmin},
	"GET /api/nodes/{node}/busy_ports":             {class: classAdmin},
	"GET /api/nodes/{node}/ip_list":                {class: classAdmin},
	"GET /api/nodes/{id}/daemon":                   {class: classAdmin},
	"GET /api/nodes/{id}/logs.zip":                 {class: classAdmin},
	"GET /api/dedicated_servers":                   {class: classAdmin},
	"GET /api/dedicated_servers/summary":           {class: classAdmin},
	"GET /api/dedicated_servers/{id}":              {class: classAdmin},
	"PUT /api/dedicated_servers/{id}":              {class: classAdmin},
	"DELETE /api/dedicated_servers/{id}":           {class: classAdmin},
	"GET /api/dedicated_servers/setup":             {class: classAdmin},
	"GET /api/dedicated_servers/certificates.zip":  {class: classAdmin},
	"GET /api/dedicated_servers/{node}/busy_ports": {class: classAdmin},
	"GET /api/dedicated_servers/{node}/ip_list":    {class: classAdmin},
	"GET /api/dedicated_servers/{id}/daemon":       {class: classAdmin},
	"GET /api/dedicated_servers/{id}/logs.zip":     {class: classAdmin},

	// Games.
	"GET /api/games":                     {class: classAdmin},
	"POST /api/games":                    {class: classAdmin},
	"GET /api/games/{code}":              {class: classAdmin},
	"PUT /api/games/{code}":              {class: classAdmin},
	"DELETE /api/games/{code}":           {class: classAdmin},
	"GET /api/games/{code}/mods":         {class: classAdmin},
	"GET /api/games/{code}/export":       {class: classAdmin},
	"POST /api/games/upgrade":            {class: classAdmin},
	"POST /api/games/import/pelican-egg": {class: classAdmin},
	"POST /api/games/import/gameap":      {class: classAdmin},

	// Game mods.
	"GET /api/game_mods":                          {class: classAdmin},
	"POST /api/game_mods":                         {class: classAdmin},
	"GET /api/game_mods/{id}":                     {class: classAdmin},
	"PUT /api/game_mods/{id}":                     {class: classAdmin},
	"DELETE /api/game_mods/{id}":                  {class: classAdmin},
	"GET /api/game_mods/get_list_for_game/{game}": {class: classAdmin},

	// Daemon tasks. Reading one task is the sole non-admin route of the group:
	// the handler grants it only for a task bound to a server the caller may
	// act on, and only for a task type mapped in DaemonTaskTypeAbilities.
	"GET /api/gdaemon_tasks":              {class: classAdmin},
	"GET /api/gdaemon_tasks/{id}":         {class: classAuthenticated, patScoped: true},
	"GET /api/gdaemon_tasks/{id}/output":  {class: classAdmin},
	"POST /api/gdaemon_tasks/{id}/cancel": {class: classAdmin},

	// Client certificates.
	"GET /api/client_certificates":         {class: classAdmin},
	"POST /api/client_certificates":        {class: classAdmin},
	"DELETE /api/client_certificates/{id}": {class: classAdmin},

	// Plugin store and plugin administration.
	"GET /api/plugin-store/categories":            {class: classAdmin},
	"GET /api/plugin-store/labels":                {class: classAdmin},
	"GET /api/plugin-store/plugins":               {class: classAdmin},
	"GET /api/plugin-store/plugins/{id}":          {class: classAdmin},
	"GET /api/plugin-store/plugins/{id}/versions": {class: classAdmin},
	"GET /api/plugin-store/plugins/{id}/icon":     {class: classAdmin},
	"POST /api/plugin-store/plugins/{id}/install": {class: classAdmin},
	"POST /api/plugin-store/plugins/{id}/update":  {class: classAdmin},
	"GET /api/admin/plugins/loaded":               {class: classAdmin},
	"POST /api/admin/plugins/upload/dry-run":      {class: classAdmin},
	"POST /api/admin/plugins/upload/install":      {class: classAdmin},
	"POST /api/admin/plugins/{id}/upload":         {class: classAdmin},
	"POST /api/admin/plugins/{id}/reload":         {class: classAdmin},
	"PUT /api/admin/plugins/{id}/permissions":     {class: classAdmin},
	"DELETE /api/admin/plugins/{id}":              {class: classAdmin},
	"GET /api/admin/letsencrypt/status":           {class: classAdmin},

	// WebSocket upgrades.
	"GET /api/ws/tasks/{id}":                                       {class: classAuthenticated, note: noteTaskStatus},
	"GET /api/ws/servers/{server}/console":                         {class: classAuthenticated, patScoped: true},
	"GET /api/ws/servers/{server}/attach":                          {class: classAuthenticated, patScoped: true},
	"GET /api/ws/servers/{server}/metrics":                         {class: classAuthenticated, patScoped: true},
	"GET /api/ws/servers/{server}/file-manager/archive-operations": {class: classAuthenticated, patScoped: true},
	"GET /api/ws/nodes/metrics":                                    {class: classAdmin},
	"GET /api/ws/nodes/{id}/metrics":                               {class: classAdmin},
}

// healthProbeExempt is the one guest route the anonymous probe skips: the
// in-memory container has no *sql.DB, so the handler panics on Ping and the
// recovery middleware answers 500 — which would say nothing about auth.
const healthProbeExempt = "GET /api/health"

// TestRouteAuthzMatrix_CoversEveryRegisteredRoute is the completeness net.
// A new endpoint in apiRoutes fails here until someone states its audience.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouteAuthzMatrix_CoversEveryRegisteredRoute(t *testing.T) {
	registered, methodless := registeredAPIRoutes(t)

	require.NotEmpty(t, registered)

	for _, path := range methodless {
		assert.Truef(t, strings.HasPrefix(path, "/api/plugins/{plugin_id}"),
			"route %q is registered without an HTTP method and is not the plugin catch-all; "+
				"classify it in routeAuthzMatrix and teach this test how to probe it", path)
	}

	seen := make(map[string]bool, len(registered))

	var unclassified []string

	for _, key := range registered {
		seen[key] = true

		if _, ok := routeAuthzMatrix[key]; !ok {
			unclassified = append(unclassified, key)
		}
	}

	var stale []string

	for key := range routeAuthzMatrix {
		if !seen[key] {
			stale = append(stale, key)
		}
	}

	sort.Strings(unclassified)
	sort.Strings(stale)

	assert.Emptyf(t, unclassified,
		"these routes are registered but absent from routeAuthzMatrix: %s\n"+
			"add each one with its audience (classGuest / classAuthenticated / classAdmin) "+
			"and whether it scopes personal access tokens", strings.Join(unclassified, ", "))

	assert.Emptyf(t, stale,
		"these routes are in routeAuthzMatrix but no longer registered: %s\n"+
			"drop the entries or restore the routes", strings.Join(stale, ", "))
}

// TestRouteAuthzMatrix_UnscopedRoutesCarryAJustification keeps the exception
// list honest: an authenticated route that ignores a token's declared scope
// must say in writing why that is safe.
//
//nolint:paralleltest // consistent with the rest of the suite; the matrix is shared package state.
func TestRouteAuthzMatrix_UnscopedRoutesCarryAJustification(t *testing.T) {
	for _, key := range sortedMatrixKeys() {
		expectation := routeAuthzMatrix[key]

		if expectation.class != classAuthenticated || expectation.patScoped {
			continue
		}

		assert.NotEmptyf(t, expectation.note,
			"%s accepts any personal access token: declare CheckPATAbilities on it, "+
				"or record in the matrix note why every token may reach it", key)
	}
}

// TestRouteAuthzMatrix_AdminRoutesRejectRegularUser drives every admin route
// with a valid regular-user session. IsAdminMiddleware answers before the
// handler, so no request reaches a daemon or an external service.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouteAuthzMatrix_AdminRoutesRejectRegularUser(t *testing.T) {
	for _, key := range sortedMatrixKeys() {
		if routeAuthzMatrix[key].class != classAdmin {
			continue
		}

		t.Run(subtestName(key), func(t *testing.T) {
			env := setupMatrixEnv(t)
			token := env.tokenFor(t, env.fixtures.RegularUser)

			w := env.request(t, key, token)

			assert.Equalf(t, http.StatusForbidden, w.Code,
				"%s must reject a regular user with 403; got %d body=%s", key, w.Code, w.Body.String())
		})
	}
}

// TestRouteAuthzMatrix_NonGuestRoutesRejectAnonymous asserts the other half:
// no admin or authenticated route answers anything but 401 without a session.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouteAuthzMatrix_NonGuestRoutesRejectAnonymous(t *testing.T) {
	for _, key := range sortedMatrixKeys() {
		if routeAuthzMatrix[key].class == classGuest {
			continue
		}

		t.Run(subtestName(key), func(t *testing.T) {
			env := setupMatrixEnv(t)

			w := env.request(t, key, "")

			assert.Equalf(t, http.StatusUnauthorized, w.Code,
				"%s must reject an anonymous caller with 401; got %d body=%s", key, w.Code, w.Body.String())
		})
	}
}

// TestRouteAuthzMatrix_GuestRoutesAcceptAnonymous guards the inverse: a route
// declared public must not start demanding a session.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouteAuthzMatrix_GuestRoutesAcceptAnonymous(t *testing.T) {
	for _, key := range sortedMatrixKeys() {
		if routeAuthzMatrix[key].class != classGuest || key == healthProbeExempt {
			continue
		}

		t.Run(subtestName(key), func(t *testing.T) {
			env := setupMatrixEnv(t)

			w := env.request(t, key, "")

			assert.NotEqualf(t, http.StatusUnauthorized, w.Code,
				"%s is declared public and must not answer 401; body=%s", key, w.Body.String())
		})
	}
}

// TestRouteAuthzMatrix_ScopedRoutesRejectTokenWithoutAbilities drives a
// personal access token that holds no abilities at all. Every route the matrix
// marks patScoped must refuse it — for an admin route that verdict comes from
// TokenAdminGuardMiddleware, for the rest from PersonalAccessMiddleware.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouteAuthzMatrix_ScopedRoutesRejectTokenWithoutAbilities(t *testing.T) {
	for _, key := range sortedMatrixKeys() {
		if !routeAuthzMatrix[key].patScoped {
			continue
		}

		t.Run(subtestName(key), func(t *testing.T) {
			env := setupMatrixEnv(t)
			token := env.patFor(t, env.fixtures.AdminUser, []domain.PATAbility{})

			w := env.request(t, key, token)

			assert.Equalf(t, http.StatusForbidden, w.Code,
				"%s must reject a token that declares no abilities with 403; got %d body=%s",
				key, w.Code, w.Body.String())
		})
	}
}

// TestRouteAuthzMatrix_AdminRoutesRejectPersonalAccessToken covers the
// fail-closed rule of the route loop: an admin route that declares no
// abilities is unreachable by a token, however privileged its owner.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouteAuthzMatrix_AdminRoutesRejectPersonalAccessToken(t *testing.T) {
	for _, key := range sortedMatrixKeys() {
		expectation := routeAuthzMatrix[key]
		if expectation.class != classAdmin || expectation.patScoped {
			continue
		}

		t.Run(subtestName(key), func(t *testing.T) {
			env := setupMatrixEnv(t)
			token := env.patFor(t, env.fixtures.AdminUser, domain.GetAllAbilities())

			w := env.request(t, key, token)

			assert.Equalf(t, http.StatusForbidden, w.Code,
				"%s must reject a personal access token even for an admin owner; got %d body=%s",
				key, w.Code, w.Body.String())
		})
	}
}

// --- helpers ---------------------------------------------------------------

type matrixEnv struct {
	container *testcontainer.InmemoryContainer
	fixtures  *testcontainer.TestFixtures
	router    http.Handler
	ctx       context.Context
}

func setupMatrixEnv(t *testing.T) *matrixEnv {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)

	ctx := context.Background()

	fixtures, err := testcontainer.SetupFixtures(ctx, c)
	require.NoError(t, err)

	return &matrixEnv{
		container: c,
		fixtures:  fixtures,
		router:    CreateRouter(c),
		ctx:       ctx,
	}
}

func (e *matrixEnv) tokenFor(t *testing.T, user *domain.User) string {
	t.Helper()

	token, err := e.container.AuthService().GenerateTokenForUser(user, time.Hour)
	require.NoError(t, err)

	return token
}

func (e *matrixEnv) patFor(t *testing.T, user *domain.User, abilities []domain.PATAbility) string {
	t.Helper()

	secret, err := pkgstrings.CryptoRandomString(40)
	require.NoError(t, err)

	abilitiesCopy := abilities
	token := &domain.PersonalAccessToken{
		TokenableType: domain.EntityTypeUser,
		TokenableID:   user.ID,
		Name:          "route-matrix-token",
		Token:         pkgstrings.SHA256(secret),
		Abilities:     &abilitiesCopy,
	}

	require.NoError(t, e.container.PersonalAccessTokenRepository().Save(e.ctx, token))

	return fmt.Sprintf("%d|%s", token.ID, secret)
}

// request drives one matrix key. Path variables are filled with "1": the probe
// only needs mux to match the route, never the handler to find the object.
func (e *matrixEnv) request(t *testing.T, key, bearerToken string) *httptest.ResponseRecorder {
	t.Helper()

	method, path, ok := strings.Cut(key, " ")
	require.Truef(t, ok, "malformed matrix key %q, want \"METHOD /path\"", key)

	req := httptest.NewRequest(method, pathVarPattern.ReplaceAllString(path, "1"), nil)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	return w
}

//nolint:gochecknoglobals
var pathVarPattern = regexp.MustCompile(`\{[^{}]+\}`)

// registeredAPIRoutes enumerates what apiRoutes actually registered, as
// "METHOD /path" keys, plus the templates of any route registered without a
// method (today only the plugin catch-all, and only when a plugin manager
// exists — the in-memory container has none).
func registeredAPIRoutes(t *testing.T) (keys []string, methodless []string) {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)

	router := mux.NewRouter().StrictSlash(true)
	apiRoutes(c, router)

	walkErr := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		template, err := route.GetPathTemplate()
		if err != nil {
			return err
		}

		methods, err := route.GetMethods()
		if err != nil {
			methodless = append(methodless, template)

			return nil //nolint:nilerr // a method-less route is reported, not fatal
		}

		for _, method := range methods {
			keys = append(keys, method+" "+template)
		}

		return nil
	})
	require.NoError(t, walkErr)

	return keys, methodless
}

func sortedMatrixKeys() []string {
	keys := make([]string, 0, len(routeAuthzMatrix))
	for key := range routeAuthzMatrix {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// subtestName turns a matrix key into a name that survives -run filters.
func subtestName(key string) string {
	return strings.NewReplacer(" ", "_", "/", "_", "{", "", "}", "", ".", "_", "-", "_").Replace(key)
}
