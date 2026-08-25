package deletenode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginDispatcher struct {
	mu     sync.Mutex
	cancel bool
	sync   []pluginproto.EventType
	async  []pluginproto.EventType
}

func (f *fakePluginDispatcher) DispatchNodeEvent(
	_ context.Context, eventType pluginproto.EventType, _ *domain.Node, _ map[string]string,
) *pkgplugin.EventDispatchResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sync = append(f.sync, eventType)

	if f.cancel {
		return &pkgplugin.EventDispatchResult{Cancelled: true, CancelledBy: "inventory", CancelMessage: "node is leased"}
	}

	return &pkgplugin.EventDispatchResult{}
}

func (f *fakePluginDispatcher) DispatchNodeEventAsync(
	_ context.Context, eventType pluginproto.EventType, _ *domain.Node, _ map[string]string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.async = append(f.async, eventType)
}

func seedNode(t *testing.T) *inmemory.NodeRepository {
	t.Helper()

	now := time.Now()
	nodesRepo := inmemory.NewNodeRepository()
	require.NoError(t, nodesRepo.Save(context.Background(), &domain.Node{
		ID: 1, Name: "test-node", OS: "linux", Location: "dc-1", CreatedAt: &now, UpdatedAt: &now,
	}))

	return nodesRepo
}

func deleteNodeRequest() *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/api/dedicated_servers/1", nil)
	req = req.WithContext(deleteNodeAuthCtx())

	return mux.SetURLVars(req, map[string]string{"id": "1"})
}

func TestHandler_plugin_events(t *testing.T) {
	t.Parallel()

	t.Run("pre_delete_cancel_keeps_the_node", func(t *testing.T) {
		t.Parallel()

		nodesRepo := seedNode(t)
		dispatcher := &fakePluginDispatcher{cancel: true}
		handler := NewHandler(nodesRepo, inmemory.NewServerRepository(), api.NewResponder(), nil, dispatcher)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, deleteNodeRequest())

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "cancelled by inventory: node is leased")

		nodes, err := nodesRepo.Find(context.Background(), filters.FindNodeByIDs(1), nil, nil)
		require.NoError(t, err)
		require.Len(t, nodes, 1)
		assert.Nil(t, nodes[0].DeletedAt)

		assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE}, dispatcher.sync)
		assert.Empty(t, dispatcher.async)
	})

	t.Run("deletion_publishes_pre_delete_then_deleted", func(t *testing.T) {
		t.Parallel()

		dispatcher := &fakePluginDispatcher{}
		handler := NewHandler(seedNode(t), inmemory.NewServerRepository(), api.NewResponder(), nil, dispatcher)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, deleteNodeRequest())

		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
		assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE}, dispatcher.sync)
		assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_NODE_DELETED}, dispatcher.async)
	})

	t.Run("node_with_servers_publishes_nothing", func(t *testing.T) {
		t.Parallel()

		serversRepo := inmemory.NewServerRepository()
		require.NoError(t, serversRepo.Save(context.Background(), &domain.Server{Name: "cs", DSID: 1}))

		dispatcher := &fakePluginDispatcher{}
		handler := NewHandler(seedNode(t), serversRepo, api.NewResponder(), nil, dispatcher)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, deleteNodeRequest())

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Empty(t, dispatcher.sync)
		assert.Empty(t, dispatcher.async)
	})
}
