package install_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/upload/install"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRefresher struct {
	calls int
}

func (f *fakeRefresher) RefreshSubscriptions(_ context.Context) error {
	f.calls++

	return nil
}

func TestInstall_refreshes_subscriptions(t *testing.T) {
	t.Parallel()
	mockManager := &mockLoaderManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:         "testplugin",
					Name:       "Test Plugin",
					Version:    "1.0.0",
					ApiVersion: "v1",
				},
			}, nil
		},
	}
	refresher := &fakeRefresher{}
	h := install.NewHandler(
		mockManager,
		inmemory.NewPluginRepository(),
		files.NewInMemoryFileManager(),
		nil,
		refresher,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, createMultipartRequest(t, "plugin.wasm", validWASMBytes()))

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Equal(t, 1, refresher.calls, "subscriptions must be refreshed after a runtime install")
}
