package compatrust

import (
	"context"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// CI keeps this file on the HEAD leg only: the introspection fixture imports
// gameap-host and declares a config_schema, both of which exist only on the
// development panel (post-4.4.2). The tagged legs drop the file together with
// stubs_head_test.go.

const introspectionPluginID = "Hs7kQm2Xp4Lw1"

func TestRustPluginCompatHead_Introspection(t *testing.T) {
	t.Parallel()
	// ARRANGE: a plugin row with a granted permission and a stored secret, so
	// every gameap-host RPC answers real panel state rather than defaults.
	dbID := plugin.ParsePluginID(introspectionPluginID)
	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.Plugin{
		ID:                 dbID,
		Name:               "introspection",
		Version:            "0.1.0",
		Status:             domain.PluginStatusActive,
		AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionListenEvents},
		Config:             map[string]any{"retries": int64(7), "extra": "kept"},
	}))

	logs := &stubLogService{}
	var manager *plugin.Manager
	sink := &lazySink{get: func() *plugin.Manager { return manager }}

	manager = plugin.NewManager(plugin.ManagerConfig{
		Libraries: []plugin.HostLibrary{
			hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
				return log.Instantiate(ctx, r, logs)
			}),
		},
		LibraryFactories: []plugin.HostLibraryFactory{
			hostlibrary.NewHostHostLibraryFactory(repo, secret.Disabled(), sink, hostlibrary.HostInfo{
				PanelVersion:     "head",
				PluginAPIVersion: 1,
				InstanceID:       "compat",
			}),
		},
	})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	// ACT: load like the panel loader does — the stored configuration is the
	// caller's map, the schema defaults are overlaid by the manager.
	loadedPlugin, err := manager.Load(context.Background(), readFixtureWASM(t, "introspection"),
		map[string]string{"retries": "7", "extra": "kept"}, uint64(dbID))

	// ASSERT
	require.NoError(t, err, "fixture must load against the real gameap-host implementation")
	assert.Equal(t, introspectionPluginID, loadedPlugin.Info.Id)
	assert.Empty(t, loadedPlugin.ConfigSchemaError, "the fixture's config_schema must parse")
	assert.Equal(t, map[string]string{
		"greeting": "hello",
		"retries":  "7",
		"verbose":  "false",
		"extra":    "kept",
	}, loadedPlugin.Config, "schema defaults fill the keys the operator did not set")
	assert.Contains(t, loadedPlugin.HostModules, "gameap-host")
	assert.Contains(t, loadedPlugin.HostModules, "gameap-log")

	// The fixture read its grants, configuration and host info during
	// Initialize and reported its health; with no token it is degraded.
	messages := logMessages(logs)
	assert.Contains(t, messages, "[introspection] grants listen_events")
	assert.Contains(t, messages,
		"[introspection] get_config found=true has_schema=true error= values extra=kept,greeting=hello,retries=7,verbose=false")
	assert.Contains(t, messages, "[introspection] host panel=head api=1 instance=compat modules gameap-host,gameap-log")
	assert.Contains(t, messages, "[introspection] report accepted=true")

	health, ok := loadedPlugin.Health()
	require.True(t, ok, "ReportStatus must reach the loaded plugin")
	assert.Equal(t, plugin.HealthDegraded, health.Status)
	assert.Equal(t, "greeting=hello retries=7", health.Message)
	assert.Equal(t, map[string]string{"verbose": "false"}, health.Details)

	// ACT: the new event catalog — a user event is handled and a blocked
	// deletion is cancelled.
	subscribed, err := loadedPlugin.Instance.GetSubscribedEvents(context.Background(), &proto.GetSubscribedEventsRequest{})
	require.NoError(t, err)
	assert.Contains(t, subscribed.Events, proto.EventType_EVENT_TYPE_USER_PRE_DELETE)
	assert.Contains(t, subscribed.Events, proto.EventType_EVENT_TYPE_PLUGIN_LOADED)

	created, err := loadedPlugin.Instance.HandleEvent(context.Background(), userEvent(proto.EventType_EVENT_TYPE_USER_CREATED, "alice"))
	require.NoError(t, err)
	assert.True(t, created.Handled)
	assert.False(t, created.ShouldCancel)

	blocked, err := loadedPlugin.Instance.HandleEvent(context.Background(), userEvent(proto.EventType_EVENT_TYPE_USER_PRE_DELETE, "blocked"))
	require.NoError(t, err)
	assert.True(t, blocked.Handled)
	assert.True(t, blocked.ShouldCancel, "the fixture cancels the deletion of the login 'blocked'")
	assert.Contains(t, logMessages(logs), "[introspection] event EVENT_TYPE_USER_PRE_DELETE receiver="+introspectionPluginID+" user:blocked")

	// A server payload under the old catalog is not the fixture's business.
	other, err := loadedPlugin.Instance.HandleEvent(context.Background(), &proto.Event{
		Type:    proto.EventType_EVENT_TYPE_SERVER_POST_START,
		Context: &proto.PluginContext{PluginId: introspectionPluginID},
		Payload: &proto.Event_ServerEvent{ServerEvent: &proto.ServerEventPayload{Server: &domainproto.Server{Id: 1}}},
	})
	require.NoError(t, err)
	assert.False(t, other.Handled)
}

func userEvent(eventType proto.EventType, login string) *proto.Event {
	return &proto.Event{
		Type:    eventType,
		Context: &proto.PluginContext{PluginId: introspectionPluginID},
		Payload: &proto.Event_UserEvent{UserEvent: &proto.UserEventPayload{
			User: &domainproto.User{Id: 7, Login: login},
		}},
	}
}

func logMessages(logs *stubLogService) []string {
	entries := logs.Entries()
	messages := make([]string, 0, len(entries))

	for _, entry := range entries {
		messages = append(messages, strings.TrimSpace(entry.Message))
	}

	return messages
}

// lazySink forwards to the manager once it exists; the factory is built
// before the manager that owns it.
type lazySink struct {
	get func() *plugin.Manager
}

func (s *lazySink) SetHealth(dbID uint64, report plugin.HealthReport) bool {
	return s.get().SetHealth(dbID, report)
}

func (s *lazySink) HostModules(dbID uint64) ([]string, bool) {
	return s.get().HostModules(dbID)
}

func (s *lazySink) ManifestConfigSchema(dbID uint64) (string, bool) {
	return s.get().ManifestConfigSchema(dbID)
}
