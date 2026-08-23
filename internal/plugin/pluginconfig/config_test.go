package pluginconfig_test

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCipher(t *testing.T) *secret.Cipher {
	t.Helper()

	cipher, err := secret.NewCipher("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)

	return cipher
}

func TestSecretEnvelope_round_trip_bound_to_plugin_and_key(t *testing.T) {
	t.Parallel()

	cipher := newCipher(t)

	envelope, err := pluginconfig.EncryptSecret(cipher, 7, "api_key", "s3cret")
	require.NoError(t, err)

	ciphertext, ok := pluginconfig.IsSecretEnvelope(envelope)
	require.True(t, ok)
	assert.NotEqual(t, "s3cret", ciphertext)
	assert.NotContains(t, ciphertext, "s3cret")

	plain, err := pluginconfig.DecryptSecret(cipher, 7, "api_key", ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", plain)

	_, err = pluginconfig.DecryptSecret(cipher, 8, "api_key", ciphertext)
	require.Error(t, err)

	_, err = pluginconfig.DecryptSecret(cipher, 7, "other_key", ciphertext)
	require.Error(t, err)
}

func TestIsSecretEnvelope_rejects_other_shapes(t *testing.T) {
	t.Parallel()

	_, ok := pluginconfig.IsSecretEnvelope("enc:abc")
	assert.False(t, ok)

	_, ok = pluginconfig.IsSecretEnvelope(map[string]any{"$secret": "x", "extra": "y"})
	assert.False(t, ok)

	_, ok = pluginconfig.IsSecretEnvelope(map[string]any{"$secret": 1})
	assert.False(t, ok)

	_, ok = pluginconfig.IsSecretEnvelope(nil)
	assert.False(t, ok)
}

func TestEncryptSecret_with_disabled_cipher_keeps_plaintext_in_the_envelope(t *testing.T) {
	t.Parallel()

	envelope, err := pluginconfig.EncryptSecret(nil, 7, "api_key", "plain")
	require.NoError(t, err)

	ciphertext, ok := pluginconfig.IsSecretEnvelope(envelope)
	require.True(t, ok)
	assert.Equal(t, "plain", ciphertext)

	resolved, err := pluginconfig.Resolve(secret.Disabled(), 7, map[string]any{"api_key": envelope})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"api_key": "plain"}, resolved)
}

func TestResolve_converts_and_decrypts(t *testing.T) {
	t.Parallel()

	cipher := newCipher(t)

	envelope, err := pluginconfig.EncryptSecret(cipher, 7, "token", "t0ken")
	require.NoError(t, err)

	resolved, err := pluginconfig.Resolve(cipher, 7, map[string]any{
		"token":   envelope,
		"port":    float64(8080),
		"ratio":   0.5,
		"enabled": true,
		"name":    "srv",
		"absent":  nil,
		"nested":  map[string]any{"a": 1, "b": "x"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"token":   "t0ken",
		"port":    "8080",
		"ratio":   "0.5",
		"enabled": "true",
		"name":    "srv",
		"nested":  `{"a":1,"b":"x"}`,
	}, resolved)

	empty, err := pluginconfig.Resolve(cipher, 7, nil)
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestResolve_decrypt_failure_names_the_key_only(t *testing.T) {
	t.Parallel()

	cipher := newCipher(t)

	envelope, err := pluginconfig.EncryptSecret(cipher, 7, "token", "t0ken")
	require.NoError(t, err)

	other, err := secret.NewCipher("another-key-another-key-another!")
	require.NoError(t, err)

	_, err = pluginconfig.Resolve(other, 7, map[string]any{"token": envelope})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `configuration key "token"`)
	assert.NotContains(t, err.Error(), "t0ken")
}

func TestEffective_overlays_schema_defaults(t *testing.T) {
	t.Parallel()

	plugin := &domain.Plugin{
		ID:     7,
		Config: map[string]any{"port": float64(9000)},
		ConfigSchema: new(`{"properties": {
			"port": {"type": "integer", "default": 8080},
			"region": {"type": "string", "default": "eu"}
		}}`),
	}

	effective, err := pluginconfig.Effective(nil, plugin)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"port": "9000", "region": "eu"}, effective)

	assert.Nil(t, plugin.Config["region"], "defaults are never written to the row")
}

func TestSchemaFromManifest(t *testing.T) {
	t.Parallel()

	plugin := &domain.Plugin{}

	assert.False(t, pluginconfig.SchemaFromManifest(plugin, &proto.PluginInfo{}))
	assert.Nil(t, plugin.ConfigSchema)

	assert.True(t, pluginconfig.SchemaFromManifest(plugin, &proto.PluginInfo{ConfigSchema: `{"properties": {}}`}))
	require.NotNil(t, plugin.ConfigSchema)
	assert.Equal(t, `{"properties": {}}`, *plugin.ConfigSchema)

	assert.False(t, pluginconfig.SchemaFromManifest(plugin, &proto.PluginInfo{ConfigSchema: `{"properties": {}}`}))

	assert.True(t, pluginconfig.SchemaFromManifest(plugin, nil))
	assert.Nil(t, plugin.ConfigSchema)
}

func TestView_masks_secrets_and_lists_them(t *testing.T) {
	t.Parallel()

	envelope, err := pluginconfig.EncryptSecret(newCipher(t), 7, "api_key", "s3cret")
	require.NoError(t, err)

	plugin := &domain.Plugin{
		ID:           7,
		Config:       map[string]any{"api_key": envelope, "port": float64(8080)},
		ConfigSchema: new(`{"properties": {"api_key": {"type": "string", "format": "secret"}, "port": {"type": "integer"}}}`),
	}

	view := pluginconfig.NewView(plugin)
	assert.Equal(t, map[string]any{"port": float64(8080)}, view.Values)
	assert.Equal(t, []string{"api_key"}, view.SecretsSet)
	require.NotNil(t, view.Schema)
	assert.Len(t, view.Schema.Properties, 2)
	assert.Empty(t, view.SchemaError)

	assert.Equal(t, []string{"api_key", "port"}, pluginconfig.Keys(plugin.Config))

	plugin.ConfigSchema = new("{")
	broken := pluginconfig.NewView(plugin)
	assert.Nil(t, broken.Schema)
	assert.NotEmpty(t, broken.SchemaError)
}
