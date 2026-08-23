package putuser

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginDispatcher struct {
	mu     sync.Mutex
	events []pluginproto.EventType
	extra  map[string]string
}

func (f *fakePluginDispatcher) DispatchUserEventAsync(
	_ context.Context, eventType pluginproto.EventType, _ *domain.User, extraData map[string]string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, eventType)
	f.extra = extraData
}

func TestHandler_publishes_user_updated_with_changed_field_names(t *testing.T) {
	t.Parallel()

	usersRepo := inmemory.NewUserRepository()
	rbacRepo := inmemory.NewRBACRepository()
	dispatcher := &fakePluginDispatcher{}
	handler := NewHandler(
		services.NewUserService(usersRepo),
		inmemory.NewServerRepository(),
		rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
		services.NewNilTransactionManager(),
		api.NewResponder(),
		nil,
		dispatcher,
	)

	now := time.Now()
	name := "Original"
	require.NoError(t, usersRepo.Save(context.Background(), &domain.User{
		ID: 1, Login: "user", Email: "original@example.com", Password: "$2a$10$test", Name: &name,
		CreatedAt: &now, UpdatedAt: &now,
	}))

	w := doPutUser(t, handler, updateUserInput{
		Email:    "updated@example.com",
		Name:     new("Original"),
		Password: new("new-password-1234"),
		Roles:    []string{},
		Servers:  []flexible.Uint{},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_USER_UPDATED}, dispatcher.events)
	assert.Equal(t, map[string]string{"changed_fields": "email,password,roles,servers"}, dispatcher.extra,
		"names only: the unchanged name is not listed, the password value never is")
}
