package putserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginDispatcher struct {
	events  []servercontrol.PluginEventType
	servers []*domain.Server
}

func (f *fakePluginDispatcher) DispatchServerEventAsync(
	_ context.Context,
	eventType servercontrol.PluginEventType,
	server *domain.Server,
	_ map[string]string,
) {
	f.events = append(f.events, eventType)
	f.servers = append(f.servers, server)
}

func TestHandler_ServeHTTP_dispatches_server_updated_event(t *testing.T) {
	t.Parallel()
	serverRepo := inmemory.NewServerRepository()
	require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
		ID:         1,
		UID:        uuid.New(),
		UUIDShort:  "12345678",
		Name:       "Test Server",
		GameID:     "cstrike",
		DSID:       1,
		GameModID:  1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
	}))

	dispatcher := &fakePluginDispatcher{}
	handler := NewHandler(
		serverRepo,
		inmemory.NewNodeRepository(),
		inmemory.NewGameRepository(),
		inmemory.NewGameModRepository(),
		nil,
		dispatcher,
		nil,
		api.NewResponder(),
	)

	body := []byte(`{
		"enabled": 1,
		"installed": 1,
		"blocked": 0,
		"name": "Updated Server",
		"game_id": "cstrike",
		"ds_id": 1,
		"game_mod_id": 1,
		"server_ip": "192.168.1.100",
		"server_port": 27015
	}`)
	req := httptest.NewRequest(http.MethodPut, "/servers/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Len(t, dispatcher.events, 1)
	assert.Equal(t, servercontrol.PluginEventServerUpdated, dispatcher.events[0])
	require.Len(t, dispatcher.servers, 1)
	assert.Equal(t, "Updated Server", dispatcher.servers[0].Name)
}
