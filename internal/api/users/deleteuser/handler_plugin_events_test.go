package deleteuser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
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
	users  []uint64
}

func (f *fakePluginDispatcher) DispatchUserEvent(
	_ context.Context, eventType pluginproto.EventType, user *domain.User, _ map[string]string,
) *pkgplugin.EventDispatchResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sync = append(f.sync, eventType)
	f.users = append(f.users, uint64(user.ID))

	if f.cancel {
		return &pkgplugin.EventDispatchResult{Cancelled: true, CancelledBy: "billing", CancelMessage: "user has an open invoice"}
	}

	return &pkgplugin.EventDispatchResult{}
}

func (f *fakePluginDispatcher) DispatchUserEventAsync(
	_ context.Context, eventType pluginproto.EventType, user *domain.User, _ map[string]string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.async = append(f.async, eventType)
	f.users = append(f.users, uint64(user.ID))
}

func deleteRequest(ctx context.Context, id string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+id, nil)
	req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{
		Login: "admin", Email: "admin@example.com", User: &testUser1,
	}))

	return mux.SetURLVars(req, map[string]string{"id": id})
}

func TestHandler_plugin_events(t *testing.T) {
	t.Parallel()

	t.Run("pre_delete_cancel_keeps_the_user", func(t *testing.T) {
		t.Parallel()

		usersRepo := inmemory.NewUserRepository()
		require.NoError(t, usersRepo.Save(context.Background(), &domain.User{ID: 2, Login: "victim", Email: "v@example.com"}))

		dispatcher := &fakePluginDispatcher{cancel: true}
		handler := NewHandler(services.NewUserService(usersRepo), dispatcher, api.NewResponder())

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, deleteRequest(context.Background(), "2"))

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "cancelled by billing: user has an open invoice")

		users, err := usersRepo.Find(context.Background(), nil, nil, nil)
		require.NoError(t, err)
		assert.Len(t, users, 1)

		assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_USER_PRE_DELETE}, dispatcher.sync)
		assert.Empty(t, dispatcher.async, "a cancelled deletion publishes no USER_DELETED")
	})

	t.Run("deletion_publishes_pre_delete_then_deleted", func(t *testing.T) {
		t.Parallel()

		usersRepo := inmemory.NewUserRepository()
		require.NoError(t, usersRepo.Save(context.Background(), &domain.User{ID: 2, Login: "victim", Email: "v@example.com"}))

		dispatcher := &fakePluginDispatcher{}
		handler := NewHandler(services.NewUserService(usersRepo), dispatcher, api.NewResponder())

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, deleteRequest(context.Background(), "2"))

		require.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_USER_PRE_DELETE}, dispatcher.sync)
		assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_USER_DELETED}, dispatcher.async)
		assert.Equal(t, []uint64{2, 2}, dispatcher.users)
	})

	t.Run("unknown_user_publishes_nothing", func(t *testing.T) {
		t.Parallel()

		dispatcher := &fakePluginDispatcher{}
		handler := NewHandler(services.NewUserService(inmemory.NewUserRepository()), dispatcher, api.NewResponder())

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, deleteRequest(context.Background(), "2"))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, dispatcher.sync)
		assert.Empty(t, dispatcher.async)
	})
}
