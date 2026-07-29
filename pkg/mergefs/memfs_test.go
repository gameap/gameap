package mergefs_test

import (
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/gameap/gameap/pkg/mergefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromFiles_NestedTree(t *testing.T) {
	t.Parallel()

	fsys, err := mergefs.FromFiles(map[string][]byte{
		"es.json":                 []byte("es"),
		"assets/plugin/app.js":    []byte("app"),
		"assets/plugin/style.css": []byte("css"),
	})
	require.NoError(t, err)

	t.Run("opens_top_level_file", func(t *testing.T) {
		t.Parallel()

		data, err := fs.ReadFile(fsys, "es.json")
		require.NoError(t, err)
		assert.Equal(t, "es", string(data))
	})

	t.Run("opens_nested_file", func(t *testing.T) {
		t.Parallel()

		data, err := fs.ReadFile(fsys, "assets/plugin/app.js")
		require.NoError(t, err)
		assert.Equal(t, "app", string(data))
	})

	t.Run("lists_synthesised_directory", func(t *testing.T) {
		t.Parallel()

		entries, err := fs.ReadDir(fsys, "assets/plugin")
		require.NoError(t, err)

		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}

		assert.Equal(t, []string{"app.js", "style.css"}, names)
	})

	t.Run("lists_root", func(t *testing.T) {
		t.Parallel()

		entries, err := fs.ReadDir(fsys, ".")
		require.NoError(t, err)
		require.Len(t, entries, 2)

		var dirs, files int
		for _, e := range entries {
			if e.IsDir() {
				dirs++
			} else {
				files++
			}
		}

		assert.Equal(t, 1, dirs, "assets directory")
		assert.Equal(t, 1, files, "es.json")
	})
}

func TestFromFiles_InvalidPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "absolute", path: "/etc/passwd"},
		{name: "dot_dot_traversal", path: "../escape.json"},
		{name: "embedded_traversal", path: "assets/../../escape.json"},
		{name: "dot_root", path: "."},
		{name: "empty", path: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := mergefs.FromFiles(map[string][]byte{tt.path: []byte("x")})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid file path")
		})
	}
}

func TestFromFiles_SeekableFiles(t *testing.T) {
	t.Parallel()

	fsys, err := mergefs.FromFiles(map[string][]byte{"data.bin": []byte("0123456789")})
	require.NoError(t, err)

	f, err := fsys.Open("data.bin")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	seeker, ok := f.(io.ReadSeeker)
	require.True(t, ok)

	_, err = seeker.Seek(-3, io.SeekEnd)
	require.NoError(t, err)

	tail, err := io.ReadAll(seeker)
	require.NoError(t, err)
	assert.Equal(t, "789", string(tail))
}

func TestFromFiles_Empty(t *testing.T) {
	t.Parallel()

	fsys, err := mergefs.FromFiles(nil)
	require.NoError(t, err)

	entries, err := fs.ReadDir(fsys, ".")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFromFiles_FSCompliance(t *testing.T) {
	t.Parallel()

	fsys, err := mergefs.FromFiles(map[string][]byte{
		"es.json":              []byte("es"),
		"assets/plugin/app.js": []byte("app"),
	})
	require.NoError(t, err)

	require.NoError(t, fstest.TestFS(fsys, "es.json", "assets/plugin/app.js"))
}
