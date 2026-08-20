package postserver

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

func TestHandler_ServeHTTP_dispatches_server_created_event(t *testing.T) {
	t.Parallel()
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	require.NoError(t, nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"}))
	require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"}))
	require.NoError(t, gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"}))

	dispatcher := &fakePluginDispatcher{}
	handler := NewHandler(
		serverRepo, nodeRepo, gameRepo, gameModRepo,
		inmemory.NewDaemonTaskRepository(), inmemory.NewServerSettingRepository(),
		nil, dispatcher, api.NewResponder(),
	)

	body := []byte(`{
		"name": "My CS Server",
		"game_id": "cstrike",
		"ds_id": 1,
		"game_mod_id": 1,
		"server_ip": "192.168.1.100",
		"server_port": 27015
	}`)
	req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	require.Len(t, dispatcher.events, 1)
	assert.Equal(t, servercontrol.PluginEventServerCreated, dispatcher.events[0])
	require.Len(t, dispatcher.servers, 1)
	assert.NotZero(t, dispatcher.servers[0].ID, "event must carry the persisted server with its ID")
}
