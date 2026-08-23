package updateconfig_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/updateconfig"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSchema = `{
  "properties": {
    "api_key": {"type": "string", "format": "secret"},
    "port": {"type": "integer", "minimum": 1, "maximum": 65535, "default": 8080},
    "enabled": {"type": "boolean", "default": true},
    "region": {"type": "string", "enum": ["eu", "us"]}
  },
  "required": ["port"]
}`

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) ofType(eventType audit.EventType) []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	var events []audit.Event
	for _, event := range a.events {
		if event.Type == eventType {
			events = append(events, event)
		}
	}

	return events
}

// fakeLoader records reloads and holds; it re-reads the row like the real
// loader does and marks it active.
type fakeLoader struct {
	repo      *inmemory.PluginRepository
	reloadErr error
	reloads   []domain.Uint64ID
	holds     int
	releases  int
}

func (f *fakeLoader) Reload(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error) {
	f.reloads = append(f.reloads, dbID)

	plugins, err := f.repo.Find(ctx, filters.FindPluginByIDs(dbID), nil, nil)
	if err != nil || len(plugins) == 0 {
		return nil, nil, errors.New("not found")
	}

	row := plugins[0]

	if f.reloadErr != nil {
		return &row, nil, f.reloadErr
	}

	return &row, &pkgplugin.LoadedPlugin{Info: &proto.PluginInfo{Id: "cfg"}, Enabled: true}, nil
}

func (f *fakeLoader) Hold(domain.Uint64ID) func() {
	f.holds++

	return func() { f.releases++ }
}

type fakeNotifier struct {
	calls []string
}

func (f *fakeNotifier) Notify(_ context.Context, pluginID domain.Uint64ID, action string) {
	f.calls = append(f.calls, pkgplugin.CompactPluginID(pluginID)+":"+action)
}

func testCipher(t *testing.T) *secret.Cipher {
	t.Helper()

	cipher, err := secret.NewCipher("updateconfig-test-key-0123456789")
	require.NoError(t, err)

	return cipher
}

type env struct {
	repo     *inmemory.PluginRepository
	loader   *fakeLoader
	notifier *fakeNotifier
	audit    *auditCapture
	handler  *updateconfig.Handler
	id       string
	dbID     domain.Uint64ID
}

func newEnv(t *testing.T, cipher *secret.Cipher, requireEncryption bool, schema *string, config map[string]any) *env {
	t.Helper()

	repo := inmemory.NewPluginRepository()
	dbID := pkgplugin.ParsePluginID("configurable-plugin")
	require.NoError(t, repo.Save(context.Background(), &domain.Plugin{
		ID: dbID, Name: "Configurable", Version: "1.0.0", Status: domain.PluginStatusActive,
		ConfigSchema: schema, Config: config,
	}))

	e := &env{
		repo:     repo,
		loader:   &fakeLoader{repo: repo},
		notifier: &fakeNotifier{},
		audit:    &auditCapture{},
		id:       pkgplugin.CompactPluginID(dbID),
		dbID:     dbID,
	}
	e.handler = updateconfig.NewHandler(repo, e.loader, cipher, requireEncryption, e.notifier, api.NewResponder(), e.audit)

	return e
}

func (e *env) put(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/api/admin/plugins/"+e.id+"/config", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": e.id})
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)

	return rec
}

func (e *env) stored(t *testing.T) domain.Plugin {
	t.Helper()

	plugins, err := e.repo.Find(context.Background(), filters.FindPluginByIDs(e.dbID), nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1)

	return plugins[0]
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())

	return body
}

func TestUpdateConfig_stores_typed_values_and_encrypts_secrets(t *testing.T) {
	t.Parallel()

	cipher := testCipher(t)
	e := newEnv(t, cipher, true, new(testSchema), nil)

	rec := e.put(t, `{"values": {"api_key": "s3cret", "port": 9000, "enabled": false, "region": "us", "note": "free"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored := e.stored(t)
	assert.Equal(t, int64(9000), stored.Config["port"])
	assert.Equal(t, false, stored.Config["enabled"])
	assert.Equal(t, "us", stored.Config["region"])
	assert.Equal(t, "free", stored.Config["note"], "undeclared keys are kept as strings")

	ciphertext, isEnvelope := pluginconfig.IsSecretEnvelope(stored.Config["api_key"])
	require.True(t, isEnvelope)
	assert.NotContains(t, ciphertext, "s3cret")

	plain, err := pluginconfig.DecryptSecret(cipher, uint64(e.dbID), "api_key", ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", plain)

	body := decode(t, rec)
	assert.Equal(t, true, body["reloaded"])
	assert.Equal(t, "active", body["status"])
	assert.Equal(t, []any{"api_key"}, body["secrets_set"])
	assert.NotContains(t, rec.Body.String(), "s3cret")
	values, ok := body["values"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, values, "api_key")
	assert.Equal(t, float64(9000), values["port"])

	assert.Equal(t, []domain.Uint64ID{e.dbID}, e.loader.reloads)
	assert.Equal(t, 1, e.loader.holds)
	assert.Equal(t, 1, e.loader.releases)
	assert.Equal(t, []string{e.id + ":config"}, e.notifier.calls)

	events := e.audit.ofType(audit.EventPluginConfigUpdate)
	require.Len(t, events, 1)
	assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
	for _, attr := range events[0].Extra {
		assert.NotContains(t, attr.Value.String(), "s3cret")
		assert.NotContains(t, attr.Value.String(), "9000")
	}

	reloads := e.audit.ofType(audit.EventPluginReloaded)
	require.Len(t, reloads, 1)
	assert.Equal(t, audit.OutcomeSuccess, reloads[0].Outcome)
}

func TestUpdateConfig_keeps_omitted_secret_and_clears_empty_one(t *testing.T) {
	t.Parallel()

	cipher := testCipher(t)
	envelope, err := pluginconfig.EncryptSecret(cipher, uint64(pkgplugin.ParsePluginID("configurable-plugin")), "api_key", "old")
	require.NoError(t, err)

	e := newEnv(t, cipher, true, new(testSchema), map[string]any{"api_key": envelope, "port": int64(80)})

	rec := e.put(t, `{"values": {"port": 81}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored := e.stored(t)
	assert.Equal(t, envelope, stored.Config["api_key"], "an omitted secret keeps its stored value")
	assert.Equal(t, int64(81), stored.Config["port"])

	rec = e.put(t, `{"values": {"port": 81, "api_key": null}}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, envelope, e.stored(t).Config["api_key"], "null keeps it too")

	rec = e.put(t, `{"values": {"port": 81, "api_key": ""}}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, e.stored(t).Config, "api_key", "an empty string clears the secret")
	assert.Equal(t, []any{}, decode(t, rec)["secrets_set"])
}

func TestUpdateConfig_removes_keys_missing_from_the_request(t *testing.T) {
	t.Parallel()

	e := newEnv(t, nil, false, new(testSchema), map[string]any{"port": int64(80), "region": "eu", "legacy": "x"})

	rec := e.put(t, `{"values": {"port": 80}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored := e.stored(t)
	assert.Equal(t, map[string]any{"port": int64(80)}, stored.Config)

	events := e.audit.ofType(audit.EventPluginConfigUpdate)
	require.Len(t, events, 1)
	assert.Equal(t, "[legacy region]", attrValue(events[0], "removed_keys"))
	assert.Equal(t, "[]", attrValue(events[0], "changed_keys"))
}

func TestUpdateConfig_validation_failures_answer_422_with_field_errors(t *testing.T) {
	t.Parallel()

	e := newEnv(t, nil, false, new(testSchema), nil)

	rec := e.put(t, `{"values": {"port": 70000, "region": "asia", "enabled": "yes", "extra": 5}}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())

	body := decode(t, rec)
	assert.Equal(t, "plugins.config_invalid_title", body["title"])
	assert.Equal(t, map[string]any{
		"port":    "must be between 1 and 65535",
		"region":  "must be one of: eu, us",
		"enabled": "must be a boolean",
		"extra":   "must be a string",
	}, body["errors"])

	assert.Empty(t, e.loader.reloads)
	assert.Empty(t, e.audit.ofType(audit.EventPluginConfigUpdate))

	// port is required but defaulted, so an empty body is a valid save.
	rec = e.put(t, `{"values": {}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	strict := newEnv(t, nil, false, new(`{"properties": {"name": {"type": "string"}}, "required": ["name"]}`), nil)
	rec = strict.put(t, `{"values": {}}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, map[string]any{"name": "is required"}, decode(t, rec)["errors"])
}

func TestUpdateConfig_encryption_requirement(t *testing.T) {
	t.Parallel()

	t.Run("refused_without_a_key_when_required", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t, secret.Disabled(), true, new(testSchema), nil)

		rec := e.put(t, `{"values": {"port": 80, "api_key": "s3cret"}}`)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
		body := decode(t, rec)
		assert.Equal(t, "plugins.config_encryption_required", body["title"])
		assert.Equal(t, map[string]any{"api_key": "encryption is not configured"}, body["errors"])
		assert.Empty(t, e.stored(t).Config)
	})

	t.Run("stored_in_the_envelope_when_not_required", func(t *testing.T) {
		t.Parallel()

		e := newEnv(t, secret.Disabled(), false, new(testSchema), nil)

		rec := e.put(t, `{"values": {"port": 80, "api_key": "s3cret"}}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		ciphertext, ok := pluginconfig.IsSecretEnvelope(e.stored(t).Config["api_key"])
		require.True(t, ok)
		assert.Equal(t, "s3cret", ciphertext)
		assert.NotContains(t, rec.Body.String(), "s3cret", "still masked in the response")
	})
}

func TestUpdateConfig_free_form_without_schema(t *testing.T) {
	t.Parallel()

	e := newEnv(t, nil, false, nil, map[string]any{"old": "value"})

	rec := e.put(t, `{"values": {"webhook": "https://example.com", "bad key": "x", "count": 3}}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, map[string]any{
		"bad key": "must match ^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$",
		"count":   "must be a string",
	}, decode(t, rec)["errors"])

	rec = e.put(t, `{"values": {"webhook": "https://example.com"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, map[string]any{"webhook": "https://example.com"}, e.stored(t).Config)
	assert.Nil(t, decode(t, rec)["schema"])
}

func TestUpdateConfig_skips_reload_for_disabled_plugins(t *testing.T) {
	t.Parallel()

	e := newEnv(t, nil, false, nil, nil)
	row := e.stored(t)
	row.Status = domain.PluginStatusDisabled
	require.NoError(t, e.repo.Save(context.Background(), &row))

	rec := e.put(t, `{"values": {"a": "b"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, false, decode(t, rec)["reloaded"])
	assert.Empty(t, e.loader.reloads)
	assert.Equal(t, map[string]any{"a": "b"}, e.stored(t).Config, "the save stands")
	assert.Equal(t, []string{e.id + ":config"}, e.notifier.calls)
}

func TestUpdateConfig_reload_failure_still_answers_200(t *testing.T) {
	t.Parallel()

	e := newEnv(t, nil, false, nil, nil)
	e.loader.reloadErr = errors.New("initialize failed: api_key is required")

	rec := e.put(t, `{"values": {"a": "b"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := decode(t, rec)
	assert.Equal(t, false, body["reloaded"])
	assert.Contains(t, body["reload_error"], "api_key is required")

	failed := e.audit.ofType(audit.EventPluginReloaded)
	require.Len(t, failed, 1)
	assert.Equal(t, audit.OutcomeFailure, failed[0].Outcome)
}

func TestUpdateConfig_errors(t *testing.T) {
	t.Parallel()

	e := newEnv(t, nil, false, nil, nil)

	rec := e.put(t, `{"values": `)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/plugins/missing/config", strings.NewReader(`{"values": {}}`))
	req = mux.SetURLVars(req, map[string]string{"id": pkgplugin.CompactPluginID(pkgplugin.ParsePluginID("missing"))})
	rec = httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func attrValue(event audit.Event, key string) string {
	for _, attr := range event.Extra {
		if attr.Key == key {
			return attr.Value.String()
		}
	}

	return ""
}
