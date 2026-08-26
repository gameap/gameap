package plugin

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errAssetFetch = errors.New("asset fetch boom")

func TestManager_BuildPluginAssets_PopulatesFilesystems(t *testing.T) {
	t.Parallel()
	m := NewManager(ManagerConfig{})
	mock := &mockPluginService{
		getAssetsFunc: func(_ context.Context, _ *proto.GetAssetsRequest) (*proto.GetAssetsResponse, error) {
			return &proto.GetAssetsResponse{
				I18NFiles:     []*proto.AssetFile{{Path: "es.json", Content: []byte("es-translations")}},
				FrontendFiles: []*proto.AssetFile{{Path: "plugins/demo/meta.json", Content: []byte("meta")}},
			}, nil
		},
	}

	i18nFS, frontendFS := m.buildPluginAssets(context.Background(), mock, "demo")
	require.NotNil(t, i18nFS)
	require.NotNil(t, frontendFS)

	es, err := fs.ReadFile(i18nFS, "es.json")
	require.NoError(t, err)
	assert.Equal(t, "es-translations", string(es))

	meta, err := fs.ReadFile(frontendFS, "plugins/demo/meta.json")
	require.NoError(t, err)
	assert.Equal(t, "meta", string(meta))
}

func TestManager_BuildPluginAssets_NilWhenAbsent(t *testing.T) {
	t.Parallel()
	m := NewManager(ManagerConfig{})

	t.Run("empty_response", func(t *testing.T) {
		t.Parallel()

		i18nFS, frontendFS := m.buildPluginAssets(context.Background(), &mockPluginService{}, "demo")
		assert.Nil(t, i18nFS)
		assert.Nil(t, frontendFS)
	})

	t.Run("error_response", func(t *testing.T) {
		t.Parallel()
		mock := &mockPluginService{
			getAssetsFunc: func(_ context.Context, _ *proto.GetAssetsRequest) (*proto.GetAssetsResponse, error) {
				return nil, errAssetFetch
			},
		}

		i18nFS, frontendFS := m.buildPluginAssets(context.Background(), mock, "demo")
		assert.Nil(t, i18nFS)
		assert.Nil(t, frontendFS)
	})
}

func TestBuildAssetFS_ValidFiles(t *testing.T) {
	t.Parallel()
	fsys := buildAssetFS("p1", "frontend", []*proto.AssetFile{
		{Path: "es.json", Content: []byte("es-content")},
		{Path: "assets/app.js", Content: []byte("app")},
	})
	require.NotNil(t, fsys)

	es, err := fs.ReadFile(fsys, "es.json")
	require.NoError(t, err)
	assert.Equal(t, "es-content", string(es))

	app, err := fs.ReadFile(fsys, "assets/app.js")
	require.NoError(t, err)
	assert.Equal(t, "app", string(app))
}

func TestBuildAssetFS_ReturnsNilForEmptyInput(t *testing.T) {
	t.Parallel()
	assert.Nil(t, buildAssetFS("p1", "i18n", nil))
	assert.Nil(t, buildAssetFS("p1", "i18n", []*proto.AssetFile{}))
}

func TestBuildAssetFS_SkipsUnusableEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		asset *proto.AssetFile
	}{
		{name: "parent_traversal", asset: &proto.AssetFile{Path: "../escape.json", Content: []byte("x")}},
		{name: "absolute_path", asset: &proto.AssetFile{Path: "/abs.json", Content: []byte("x")}},
		{name: "dot_root", asset: &proto.AssetFile{Path: ".", Content: []byte("x")}},
		{name: "empty_content", asset: &proto.AssetFile{Path: "empty.json", Content: nil}},
		{name: "oversized", asset: &proto.AssetFile{Path: "big.json", Content: make([]byte, maxAssetFileSize+1)}},
		{name: "nil_entry", asset: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fsys := buildAssetFS("p1", "i18n", []*proto.AssetFile{
				tt.asset,
				{Path: "ok.json", Content: []byte("ok")},
			})
			require.NotNil(t, fsys)

			entries, err := fs.ReadDir(fsys, ".")
			require.NoError(t, err)
			require.Len(t, entries, 1, "only the valid file must remain")
			assert.Equal(t, "ok.json", entries[0].Name())
		})
	}
}

func TestBuildAssetFS_ReturnsNilWhenAllSkipped(t *testing.T) {
	t.Parallel()
	assert.Nil(t, buildAssetFS("p1", "i18n", []*proto.AssetFile{
		{Path: "../escape.json", Content: []byte("x")},
	}))
}

func TestBuildAssetFS_StopsAtTotalSizeLimit(t *testing.T) {
	t.Parallel()
	// One shared 8 MiB chunk referenced from many entries keeps the test cheap
	// while the accumulated size crosses the aggregate cap.
	chunk := make([]byte, maxAssetFileSize)
	fit := maxAssetTotalSize / maxAssetFileSize

	assets := make([]*proto.AssetFile, 0, fit+1)
	for i := 0; i <= fit; i++ {
		assets = append(assets, &proto.AssetFile{Path: fmt.Sprintf("f%d.bin", i), Content: chunk})
	}

	fsys := buildAssetFS("p1", "frontend", assets)
	require.NotNil(t, fsys)

	entries, err := fs.ReadDir(fsys, ".")
	require.NoError(t, err)
	require.Len(t, entries, fit, "files past the aggregate cap must be dropped")
}
