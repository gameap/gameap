package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/configschema"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginServiceWrapper_ConfigSchemaAndHealthReport(t *testing.T) {
	t.Parallel()
	plugin := loadSharedServerLoggerWASM(t)

	info, err := plugin.Instance.GetInfo(context.Background(), &proto.GetInfoRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, info.ConfigSchema, "the example plugin declares a config schema")

	schema, err := configschema.Parse(info.ConfigSchema)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"log_level": "info"}, schema.Defaults())
	assert.Equal(t, map[string]string{"log_level": "info"}, plugin.Config,
		"the manager overlays the schema defaults on the configuration handed to Initialize")
	assert.Empty(t, plugin.ConfigSchemaError)
	assert.Contains(t, plugin.HostModules, "gameap-host")

	reports := sharedHostStub.snapshot()
	require.NotEmpty(t, reports, "Initialize reports the plugin health through gameap-host")
	assert.Equal(t, host.HealthStatus_HEALTH_STATUS_DEGRADED, reports[0].Status,
		"without a webhook the example reports itself degraded")
	assert.Equal(t, "info", reports[0].Details["log_level"])
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
