package getconfig_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/getconfig"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeResolver struct {
	byManagerID map[string]domain.Uint64ID
}

func (f fakeResolver) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	dbID, ok := f.byManagerID[managerID]

	return dbID, ok
}

func get(t *testing.T, h *getconfig.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/"+id+"/config", nil)
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestGetConfig_masks_secrets_and_renders_the_schema(t *testing.T) {
	t.Parallel()

	cipher, err := secret.NewCipher("getconfig-test-key-0123456789ab")
	require.NoError(t, err)

	dbID := pkgplugin.ParsePluginID("configured-plugin")
	envelope, err := pluginconfig.EncryptSecret(cipher, uint64(dbID), "api_key", "s3cret")
	require.NoError(t, err)

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.Plugin{
		ID: dbID, Name: "Configured", Version: "1.0.0", Status: domain.PluginStatusActive,
		Config: map[string]any{"api_key": envelope, "port": int64(9000), "note": "free"},
		ConfigSchema: new(`{"properties": {
			"api_key": {"type": "string", "format": "secret", "title": "API key"},
			"port": {"type": "integer", "default": 8080}
		}, "required": ["api_key"]}`),
	}))

	h := getconfig.NewHandler(repo, fakeResolver{byManagerID: map[string]domain.Uint64ID{"declared-id": dbID}}, api.NewResponder())

	rec := get(t, h, "declared-id")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "s3cret")
	assert.NotContains(t, rec.Body.String(), "enc:")

	var body struct {
		ID     string `json:"id"`
		Schema struct {
			Properties []struct {
				Name     string `json:"name"`
				Type     string `json:"type"`
				Secret   bool   `json:"secret"`
				Required bool   `json:"required"`
				Title    string `json:"title"`
				Default  any    `json:"default"`
			} `json:"properties"`
			AdditionalProperties bool `json:"additional_properties"`
		} `json:"schema"`
		Values     map[string]any `json:"values"`
		SecretsSet []string       `json:"secrets_set"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.Equal(t, pkgplugin.CompactPluginID(dbID), body.ID)
	assert.Equal(t, map[string]any{"port": float64(9000), "note": "free"}, body.Values)
	assert.Equal(t, []string{"api_key"}, body.SecretsSet)
	require.Len(t, body.Schema.Properties, 2)
	assert.Equal(t, "api_key", body.Schema.Properties[0].Name)
	assert.True(t, body.Schema.Properties[0].Secret)
	assert.True(t, body.Schema.Properties[0].Required)
	assert.Equal(t, "API key", body.Schema.Properties[0].Title)
	assert.Equal(t, float64(8080), body.Schema.Properties[1].Default)
	assert.True(t, body.Schema.AdditionalProperties)
}

func TestGetConfig_free_form_and_invalid_schema(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewPluginRepository()
	freeID := pkgplugin.ParsePluginID("free-form-plugin")
	require.NoError(t, repo.Save(context.Background(), &domain.Plugin{
		ID: freeID, Name: "Free", Version: "1.0.0", Status: domain.PluginStatusActive,
		Config: map[string]any{"webhook": "https://example.com"},
	}))
	brokenID := pkgplugin.ParsePluginID("broken-schema-plugin")
	require.NoError(t, repo.Save(context.Background(), &domain.Plugin{
		ID: brokenID, Name: "Broken", Version: "1.0.0", Status: domain.PluginStatusActive, ConfigSchema: new("{"),
	}))

	h := getconfig.NewHandler(repo, nil, api.NewResponder())

	rec := get(t, h, pkgplugin.CompactPluginID(freeID))
	require.Equal(t, http.StatusOK, rec.Code)

	var free map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &free))
	assert.Nil(t, free["schema"])
	assert.Equal(t, map[string]any{"webhook": "https://example.com"}, free["values"])
	assert.Equal(t, []any{}, free["secrets_set"])

	rec = get(t, h, pkgplugin.CompactPluginID(brokenID))
	require.Equal(t, http.StatusOK, rec.Code)

	var broken map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &broken))
	assert.Nil(t, broken["schema"])
	assert.NotEmpty(t, broken["schema_error"])
}

func TestGetConfig_unknown_plugin(t *testing.T) {
	t.Parallel()

	h := getconfig.NewHandler(inmemory.NewPluginRepository(), nil, api.NewResponder())

	rec := get(t, h, pkgplugin.CompactPluginID(pkgplugin.ParsePluginID("missing-plugin")))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
