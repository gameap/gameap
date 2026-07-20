package mergefs_test

import (
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/gameap/gameap/pkg/mergefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeFS_Open_LayeredPrecedence(t *testing.T) {
	t.Parallel()

	// The scenario from the feature request: two plugin layers stacked above
	// the GameAP base. Earlier (plugin) layers win.
	plugin1 := fstest.MapFS{"es.json": &fstest.MapFile{Data: []byte("plugin1-es")}}
	plugin2 := fstest.MapFS{
		"de.json": &fstest.MapFile{Data: []byte("plugin2-de")},
		"en.json": &fstest.MapFile{Data: []byte("plugin2-en")},
	}
	base := fstest.MapFS{
		"ru.json": &fstest.MapFile{Data: []byte("base-ru")},
		"en.json": &fstest.MapFile{Data: []byte("base-en")},
	}

	m := mergefs.New(plugin1, plugin2, base)

	tests := []struct {
		name        string
		file        string
		wantContent string
		wantError   string
	}{
		{name: "en.json_resolves_from_plugin2_over_base", file: "en.json", wantContent: "plugin2-en"},
		{name: "ru.json_resolves_from_base", file: "ru.json", wantContent: "base-ru"},
		{name: "es.json_resolves_from_plugin1", file: "es.json", wantContent: "plugin1-es"},
		{name: "de.json_resolves_from_plugin2", file: "de.json", wantContent: "plugin2-de"},
		{name: "missing_file_returns_not_exist", file: "fr.json", wantError: "file does not exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := fs.ReadFile(m, tt.file)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.True(t, errors.Is(err, fs.ErrNotExist))

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, string(data))
		})
	}
}

func TestMergeFS_Open_InvalidPath(t *testing.T) {
	t.Parallel()

	m := mergefs.New(fstest.MapFS{"en.json": &fstest.MapFile{Data: []byte("x")}})

	for _, name := range []string{"", "/abs.json", "../escape.json", "a/../b.json"} {
		_, err := m.Open(name)
		require.Error(t, err)
		assert.True(t, errors.Is(err, fs.ErrInvalid), "path %q must be rejected as invalid", name)
	}
}

func TestMergeFS_Open_PreservesSeek(t *testing.T) {
	t.Parallel()

	// Both an embed-like MapFS layer and the in-memory FromFiles layer must
	// return seekable files, because http.ServeContent seeks index.html.
	fromFiles, err := mergefs.FromFiles(map[string][]byte{"mem.txt": []byte("0123456789")})
	require.NoError(t, err)

	m := mergefs.New(
		fstest.MapFS{"map.txt": &fstest.MapFile{Data: []byte("abcdefghij")}},
		fromFiles,
	)

	for _, name := range []string{"map.txt", "mem.txt"} {
		f, err := m.Open(name)
		require.NoError(t, err)

		seeker, ok := f.(io.ReadSeeker)
		require.True(t, ok, "file %q must implement io.ReadSeeker", name)

		_, err = seeker.Seek(5, io.SeekStart)
		require.NoError(t, err)

		rest, err := io.ReadAll(seeker)
		require.NoError(t, err)
		assert.Len(t, rest, 5)

		require.NoError(t, f.Close())
	}
}

func TestMergeFS_ReadDir_UnionDedupSort(t *testing.T) {
	t.Parallel()

	plugin := fstest.MapFS{
		"es.json":  &fstest.MapFile{Data: []byte("plugin-es")},
		"en.json":  &fstest.MapFile{Data: []byte("plugin-en")},
		"note.txt": &fstest.MapFile{Data: []byte("plugin-note")},
	}
	base := fstest.MapFS{
		"ru.json": &fstest.MapFile{Data: []byte("base-ru")},
		"en.json": &fstest.MapFile{Data: []byte("base-en")},
	}

	m := mergefs.New(plugin, base)

	entries, err := m.ReadDir(".")
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	assert.Equal(t, []string{"en.json", "es.json", "note.txt", "ru.json"}, names)

	// The deduplicated en.json entry must come from the winning (plugin) layer.
	data, err := fs.ReadFile(m, "en.json")
	require.NoError(t, err)
	assert.Equal(t, "plugin-en", string(data))
}

func TestMergeFS_ReadDir_MissingDir(t *testing.T) {
	t.Parallel()

	m := mergefs.New(fstest.MapFS{"en.json": &fstest.MapFile{Data: []byte("x")}})

	_, err := m.ReadDir("nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestMergeFS_ReadFile_OnDirectoryErrors(t *testing.T) {
	t.Parallel()

	m := mergefs.New(fstest.MapFS{"assets/app.js": &fstest.MapFile{Data: []byte("x")}})

	_, err := m.ReadFile("assets")
	require.Error(t, err)
}

func TestMergeFS_Stat_FirstMatch(t *testing.T) {
	t.Parallel()

	plugin := fstest.MapFS{"en.json": &fstest.MapFile{Data: []byte("plugin-en-longer")}}
	base := fstest.MapFS{"en.json": &fstest.MapFile{Data: []byte("base-en")}}

	m := mergefs.New(plugin, base)

	info, err := m.Stat("en.json")
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.EqualValues(t, len("plugin-en-longer"), info.Size())
}

func TestMergeFS_Dynamic_ReevaluatesLayers(t *testing.T) {
	t.Parallel()

	var layers []fs.FS
	m := mergefs.NewDynamic(func() []fs.FS { return layers })

	_, err := m.Open("en.json")
	require.Error(t, err, "no layers yet")
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	layers = []fs.FS{fstest.MapFS{"en.json": &fstest.MapFile{Data: []byte("late")}}}

	data, err := fs.ReadFile(m, "en.json")
	require.NoError(t, err)
	assert.Equal(t, "late", string(data))
}

func TestMergeFS_Dynamic_SkipsNilLayers(t *testing.T) {
	t.Parallel()

	m := mergefs.NewDynamic(func() []fs.FS {
		return []fs.FS{nil, fstest.MapFS{"en.json": &fstest.MapFile{Data: []byte("ok")}}, nil}
	})

	data, err := fs.ReadFile(m, "en.json")
	require.NoError(t, err)
	assert.Equal(t, "ok", string(data))
}

func TestMergeFS_DirectoryShadowing(t *testing.T) {
	t.Parallel()

	t.Run("earlier_file_shadows_later_directory", func(t *testing.T) {
		t.Parallel()

		m := mergefs.New(
			fstest.MapFS{"x": &fstest.MapFile{Data: []byte("iam-a-file")}},
			fstest.MapFS{"x/child.txt": &fstest.MapFile{Data: []byte("child")}},
		)

		data, err := fs.ReadFile(m, "x")
		require.NoError(t, err)
		assert.Equal(t, "iam-a-file", string(data))
	})

	t.Run("earlier_directory_shadows_later_file", func(t *testing.T) {
		t.Parallel()

		m := mergefs.New(
			fstest.MapFS{"x/child.txt": &fstest.MapFile{Data: []byte("child")}},
			fstest.MapFS{"x": &fstest.MapFile{Data: []byte("iam-a-file")}},
		)

		info, err := m.Stat("x")
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		entries, err := m.ReadDir("x")
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "child.txt", entries[0].Name())
	})
}

func TestMergeFS_FSCompliance(t *testing.T) {
	t.Parallel()

	m := mergefs.New(
		fstest.MapFS{
			"assets/app.js": &fstest.MapFile{Data: []byte("app")},
			"en.json":       &fstest.MapFile{Data: []byte("plugin-en")},
		},
		fstest.MapFS{
			"en.json": &fstest.MapFile{Data: []byte("base-en")},
			"ru.json": &fstest.MapFile{Data: []byte("base-ru")},
		},
	)

	require.NoError(t, fstest.TestFS(m, "assets/app.js", "en.json", "ru.json"))
}
