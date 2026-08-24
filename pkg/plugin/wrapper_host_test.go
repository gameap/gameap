package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginServiceWrapper_HostModules(t *testing.T) {
	t.Parallel()
	plugin := loadSharedServerLoggerWASM(t)

	assert.Contains(t, plugin.HostModules, "gameap-host",
		"the example plugin imports gameap-host, so the module is instantiated for it")
	assert.Contains(t, plugin.HostModules, "gameap-log")
}

func TestPluginServiceWrapper_GetSubscribedEvents_includesNewCatalog(t *testing.T) {
	t.Parallel()
	plugin := loadSharedServerLoggerWASM(t)

	resp, err := plugin.Instance.GetSubscribedEvents(context.Background(), &proto.GetSubscribedEventsRequest{})
	require.NoError(t, err)

	for _, eventType := range []proto.EventType{
		proto.EventType_EVENT_TYPE_SERVER_SETTINGS_CHANGED,
		proto.EventType_EVENT_TYPE_USER_CREATED,
		proto.EventType_EVENT_TYPE_NODE_ONLINE,
		proto.EventType_EVENT_TYPE_PLUGIN_LOADED,
	} {
		assert.Contains(t, resp.Events, eventType)
	}
}

func TestPluginServiceWrapper_HandleEvent_userPayload(t *testing.T) {
	t.Parallel()
	plugin := loadSharedServerLoggerWASM(t)

	result, err := plugin.Instance.HandleEvent(context.Background(), &proto.Event{
		Type:    proto.EventType_EVENT_TYPE_USER_CREATED,
		Context: &proto.PluginContext{PluginId: plugin.Info.Id},
		Payload: &proto.Event_UserEvent{UserEvent: &proto.UserEventPayload{
			User: &domainproto.User{Id: 7, Login: "alice"},
		}},
	})
	require.NoError(t, err)
	assert.True(t, result.Handled)
	assert.False(t, result.ShouldCancel)
}
