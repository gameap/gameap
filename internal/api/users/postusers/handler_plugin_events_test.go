package postusers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/flexible"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginDispatcher struct {
	mu     sync.Mutex
	events []pluginproto.EventType
	logins []string
}

func (f *fakePluginDispatcher) DispatchUserEventAsync(
	_ context.Context, eventType pluginproto.EventType, user *domain.User, _ map[string]string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, eventType)
	f.logins = append(f.logins, user.Login)
}

func TestHandler_publishes_user_created(t *testing.T) {
	t.Parallel()

	usersRepo := inmemory.NewUserRepository()
	rbacRepo := inmemory.NewRBACRepository()
	require.NoError(t, rbacRepo.SaveRole(context.Background(), &domain.Role{Name: "user"}))

	dispatcher := &fakePluginDispatcher{}
	handler := NewHandler(
		services.NewUserService(usersRepo),
		inmemory.NewServerRepository(),
		rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
		services.NewNilTransactionManager(),
		dispatcher,
		api.NewResponder(),
	)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin", Email: "admin@example.com", User: &testUser1,
	})

	body, err := json.Marshal(createUserInput{
		Login: "newuser", Email: "newuser@example.com", Password: "password1234",
		Roles: []string{"user"}, Servers: []flexible.Uint{},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body)).WithContext(ctx))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_USER_CREATED}, dispatcher.events)
	assert.Equal(t, []string{"newuser"}, dispatcher.logins)

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body)).WithContext(ctx))
	assert.NotEqual(t, http.StatusCreated, w.Code, "duplicate login is refused")
	assert.Len(t, dispatcher.events, 1, "a refused creation publishes nothing")
}
