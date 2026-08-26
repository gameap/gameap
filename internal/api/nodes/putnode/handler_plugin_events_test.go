package putnode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginDispatcher struct {
	mu     sync.Mutex
	events []pluginproto.EventType
	names  []string
	extra  map[string]string
}

func (f *fakePluginDispatcher) DispatchNodeEventAsync(
	_ context.Context, eventType pluginproto.EventType, node *domain.Node, extraData map[string]string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, eventType)
	f.names = append(f.names, node.Name)
	f.extra = extraData
}

func TestHandler_publishes_node_updated_with_changed_field_names(t *testing.T) {
	t.Parallel()

	now := time.Now()
	repo := inmemory.NewNodeRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.Node{
		ID: 2, Name: "Test Node", OS: "linux", Location: "US", IPs: []string{"10.0.0.2"}, WorkPath: "/srv/gameap",
		GdaemonHost: "10.0.0.2", GdaemonPort: 12345, GdaemonAPIKey: "test-key", GdaemonServerCert: "certs/test.crt",
		ClientCertificateID: 1, CreatedAt: &now, UpdatedAt: &now,
	}))

	dispatcher := &fakePluginDispatcher{}
	handler := NewHandler(repo, &files.MockFileManager{}, secret.Disabled(), api.NewResponder(), nil, dispatcher)

	body, err := json.Marshal(updateNodeInput{
		Name:          new("Renamed Node"),
		Location:      new("EU"),
		GdaemonAPIKey: new("rotated-key"),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/nodes/2", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "2"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_NODE_UPDATED}, dispatcher.events)
	assert.Equal(t, []string{"Renamed Node"}, dispatcher.names)
	assert.Equal(t, map[string]string{"changed_fields": "name,location,gdaemon_api_key"}, dispatcher.extra,
		"secret fields appear by name only")
}
