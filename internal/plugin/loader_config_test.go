package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCipher(t *testing.T) *secret.Cipher {
	t.Helper()

	cipher, err := secret.NewCipher("loader-config-test-key-0123456789")
	require.NoError(t, err)

	return cipher
}

// configCapturingManager records the configuration handed to Load and answers
// a plugin whose manifest declares a schema.
func configCapturingManager(schema string) (*mockPluginManager, *map[string]string) {
	var captured map[string]string

	manager := &mockPluginManager{
		loadFunc: func(_ context.Context, _ []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
			captured = config

			return &pkgplugin.LoadedPlugin{
				Info:    &proto.PluginInfo{Id: pluginIDString(pluginID), Name: "cfg", Version: "1.0.0", ConfigSchema: schema},
				Enabled: true,
				DBID:    pluginID,
			}, nil
		},
	}

	return manager, &captured
}

func TestLoader_LoadAll_passes_decrypted_configuration_with_schema_defaults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cipher := testCipher(t)
	repo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	manager, captured := configCapturingManager("")
	loader := NewLoader(manager, fileManager, repo, nil, "plugins", WithSecretCipher(cipher))

	plugin := seedPlugin(ctx, t, repo, 801, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

	envelope, err := pluginconfig.EncryptSecret(cipher, uint64(plugin.ID), "api_key", "s3cret")
	require.NoError(t, err)

	plugin.Config = map[string]any{"api_key": envelope, "port": float64(9000), "enabled": true}
	plugin.ConfigSchema = new(`{"properties": {"region": {"type": "string", "default": "eu"}, "port": {"type": "integer", "default": 80}}}`)
	require.NoError(t, repo.Save(ctx, plugin))

	require.NoError(t, loader.LoadAll(ctx))

	assert.Equal(t, map[string]string{
		"api_key": "s3cret",
		"port":    "9000",
		"enabled": "true",
		"region":  "eu",
	}, *captured)

	stored := findPlugin(ctx, t, repo, plugin.ID)
	assert.Nil(t, stored.Config["region"], "defaults are overlaid at load, never written to the row")
}

func TestLoader_load_persists_manifest_schema_on_the_row(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	const schema = `{"properties": {"token": {"type": "string", "format": "secret"}}}`
	manager, _ := configCapturingManager(schema)
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	plugin := seedPlugin(ctx, t, repo, 802, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

	_, err := loader.LoadRecord(ctx, plugin)
	require.NoError(t, err)

	stored := findPlugin(ctx, t, repo, plugin.ID)
	require.NotNil(t, stored.ConfigSchema)
	assert.Equal(t, schema, *stored.ConfigSchema)
	assert.True(t, stored.HasConfigSchema())
}

func TestLoader_decrypt_failure_marks_row_error_naming_the_key_only(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	manager, captured := configCapturingManager("")
	loader := NewLoader(manager, fileManager, repo, nil, "plugins", WithSecretCipher(testCipher(t)))

	plugin := seedPlugin(ctx, t, repo, 803, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

	other, err := secret.NewCipher("another-key-another-key-another-key")
	require.NoError(t, err)

	envelope, err := pluginconfig.EncryptSecret(other, uint64(plugin.ID), "api_key", "s3cret")
	require.NoError(t, err)

	plugin.Config = map[string]any{"api_key": envelope}
	require.NoError(t, repo.Save(ctx, plugin))

	_, err = loader.LoadRecord(ctx, plugin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `configuration key "api_key"`)
	assert.NotContains(t, err.Error(), "s3cret")
	assert.Nil(t, *captured, "the module is never built without its configuration")

	stored := findPlugin(ctx, t, repo, plugin.ID)
	assert.Equal(t, domain.PluginStatusError, stored.Status)
	require.NotNil(t, stored.LastError)
	assert.Contains(t, *stored.LastError, `configuration key "api_key"`)
	assert.NotContains(t, *stored.LastError, "s3cret")
}

func TestLoader_Reload_passes_the_rows_configuration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	manager, captured := configCapturingManager("")
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	plugin := seedPlugin(ctx, t, repo, 804, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))
	plugin.Config = map[string]any{"mode": "fast"}
	require.NoError(t, repo.Save(ctx, plugin))

	_, _, err := loader.Reload(ctx, plugin.ID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"mode": "fast"}, *captured)
}
