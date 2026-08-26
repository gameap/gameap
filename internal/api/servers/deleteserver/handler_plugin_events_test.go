package deleteserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginDispatcher struct {
	syncEvents   []servercontrol.PluginEventType
	asyncEvents  []servercontrol.PluginEventType
	asyncBatches int
	preResult    *servercontrol.PluginDispatchResult
}

func (f *fakePluginDispatcher) DispatchServerEvent(
	_ context.Context,
	eventType servercontrol.PluginEventType,
	_ *domain.Server,
	_ map[string]string,
) *servercontrol.PluginDispatchResult {
	f.syncEvents = append(f.syncEvents, eventType)
	if f.preResult != nil {
		return f.preResult
	}

	return &servercontrol.PluginDispatchResult{}
}

func (f *fakePluginDispatcher) DispatchServerEventsAsync(
	_ context.Context,
	eventTypes []servercontrol.PluginEventType,
	_ *domain.Server,
	_ map[string]string,
) {
	f.asyncEvents = append(f.asyncEvents, eventTypes...)
	f.asyncBatches++
}

func savePluginEventsTestServer(t *testing.T, serverRepo *inmemory.ServerRepository) {
	t.Helper()

	now := time.Now()
	u := uuid.New()
	server := &domain.Server{
		ID:         1,
		UID:        u,
		UUIDShort:  u.String()[0:8],
		Enabled:    true,
		Installed:  1,
		Name:       "Test Server",
		GameID:     "cstrike",
		DSID:       1,
		GameModID:  1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))
}

func TestHandler_ServeHTTP_plugin_cancels_deletion(t *testing.T) {
	t.Parallel()
	serverRepo := inmemory.NewServerRepository()
	savePluginEventsTestServer(t, serverRepo)
	dispatcher := &fakePluginDispatcher{
		preResult: &servercontrol.PluginDispatchResult{
			Cancelled:     true,
			CancelledBy:   "guard-plugin",
			CancelMessage: "backup in progress",
		},
	}
	handler := NewHandler(serverRepo, inmemory.NewDaemonTaskRepository(), nil, dispatcher, nil, api.NewResponder())

	req := httptest.NewRequest(http.MethodDelete, "/servers/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "cancelled by guard-plugin")
	assert.Equal(t, []servercontrol.PluginEventType{servercontrol.PluginEventServerPreDelete}, dispatcher.syncEvents)
	assert.Empty(t, dispatcher.asyncEvents, "post events must not fire for a cancelled deletion")

	servers, err := serverRepo.Find(context.Background(), filters.FindServerByIDs(1), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1, "server must not be deleted when a plugin cancels")
}

func TestHandler_ServeHTTP_dispatches_delete_events(t *testing.T) {
	t.Parallel()
	serverRepo := inmemory.NewServerRepository()
	savePluginEventsTestServer(t, serverRepo)
	dispatcher := &fakePluginDispatcher{}
	handler := NewHandler(serverRepo, inmemory.NewDaemonTaskRepository(), nil, dispatcher, nil, api.NewResponder())

	req := httptest.NewRequest(http.MethodDelete, "/servers/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, []servercontrol.PluginEventType{servercontrol.PluginEventServerPreDelete}, dispatcher.syncEvents)
	assert.Equal(t,
		[]servercontrol.PluginEventType{
			servercontrol.PluginEventServerPostDelete,
			servercontrol.PluginEventServerDeleted,
		},
		dispatcher.asyncEvents)
	assert.Equal(t, 1, dispatcher.asyncBatches, "both post events must go out as one ordered batch")

	servers, err := serverRepo.Find(context.Background(), filters.FindServerByIDs(1), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, servers)
}
