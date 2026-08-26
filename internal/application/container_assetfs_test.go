package application

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With no plugins loaded, the merged filesystems must resolve to the built-in
// base layers, proving the container wires them through correctly.

func TestContainer_I18nFS_ServesBaseTranslations(t *testing.T) {
	t.Parallel()

	c := newWiredContainer(t)

	data, err := fs.ReadFile(c.I18nFS(), "en.json")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestContainer_FrontendFS_ServesSPA(t *testing.T) {
	t.Parallel()

	c := newWiredContainer(t)

	f, err := c.FrontendFS().Open("index.html")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestContainer_PluginAssetLayers_BaseIsLastWithoutPlugins(t *testing.T) {
	t.Parallel()

	c := newWiredContainer(t)

	base := fstest.MapFS{"marker.json": &fstest.MapFile{Data: []byte("base")}}

	// No plugin manager has been built in this container, so the base layer is
	// the only layer — and it is always appended last.
	layers := c.pluginAssetLayers(pluginI18nFS, base)
	require.Len(t, layers, 1)

	got, err := fs.ReadFile(layers[0], "marker.json")
	require.NoError(t, err)
	assert.Equal(t, "base", string(got))
}
