package plugin

import (
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_PluginByDBID(t *testing.T) {
	t.Parallel()

	registered := &LoadedPlugin{DBID: 7, Info: &proto.PluginInfo{Id: "seven"}}

	tests := []struct {
		name       string
		nilManager bool
		dbID       uint64
		wantPlugin *LoadedPlugin
	}{
		{
			name:       "nil_manager_has_no_plugins",
			nilManager: true,
			dbID:       7,
		},
		{
			name: "zero_db_id_is_never_registered",
			dbID: 0,
		},
		{
			name: "unknown_db_id_is_not_found",
			dbID: 8,
		},
		{
			name:       "registered_db_id_returns_the_plugin",
			dbID:       7,
			wantPlugin: registered,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var manager *Manager
			if !tt.nilManager {
				manager = NewManager(ManagerConfig{})
				manager.registerByDBID(registered)
			}

			// ACT
			plugin, ok := manager.PluginByDBID(tt.dbID)

			// ASSERT
			assert.Equal(t, tt.wantPlugin != nil, ok, "lookup result")

			if tt.wantPlugin == nil {
				assert.Nil(t, plugin)

				return
			}

			assert.Same(t, tt.wantPlugin, plugin, "the registered plugin itself is returned")
		})
	}
}

func TestManager_PluginByDBID_finds_a_plugin_whose_load_is_still_running(t *testing.T) {
	t.Parallel()

	// ARRANGE
	manager := NewManager(ManagerConfig{})
	loading := &LoadedPlugin{DBID: 12}

	// ACT
	manager.registerByDBID(loading)

	// ASSERT
	plugin, ok := manager.PluginByDBID(12)
	require.True(t, ok, "the id is claimed before GetInfo answered")
	assert.Same(t, loading, plugin)
	assert.Nil(t, plugin.Info, "Info is still nil at this point")
}

func TestManager_registerByDBID_returns_the_displaced_plugin(t *testing.T) {
	t.Parallel()

	// ARRANGE
	manager := NewManager(ManagerConfig{})
	first := &LoadedPlugin{DBID: 7, Info: &proto.PluginInfo{Id: "first"}}
	second := &LoadedPlugin{DBID: 7, Info: &proto.PluginInfo{Id: "second"}}

	// ACT
	firstPrevious := manager.registerByDBID(first)
	secondPrevious := manager.registerByDBID(second)

	// ASSERT
	assert.Nil(t, firstPrevious, "nothing was running for the id")
	assert.Same(t, first, secondPrevious, "the module already running for the id is handed back")

	current, ok := manager.PluginByDBID(7)
	require.True(t, ok)
	assert.Same(t, second, current, "the newest registration wins the index")
}

func TestManager_restoreByDBID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// registerOther replaces the failed entry before the rollback runs,
		// standing in for another load that claimed the id meanwhile.
		registerOther bool
		withPrevious  bool
		wantFound     bool
		wantRestored  string
	}{
		{
			name:      "drops_the_id_when_nothing_was_running",
			wantFound: false,
		},
		{
			name:         "brings_the_displaced_plugin_back",
			withPrevious: true,
			wantFound:    true,
			wantRestored: "previous",
		},
		{
			name:          "leaves_a_newer_registration_untouched",
			registerOther: true,
			wantFound:     true,
			wantRestored:  "other",
		},
		{
			name:          "never_overwrites_a_newer_registration_with_the_previous_one",
			registerOther: true,
			withPrevious:  true,
			wantFound:     true,
			wantRestored:  "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			const dbID uint64 = 7

			manager := NewManager(ManagerConfig{})

			var previous *LoadedPlugin
			if tt.withPrevious {
				previous = &LoadedPlugin{DBID: dbID, Info: &proto.PluginInfo{Id: "previous"}}
				manager.registerByDBID(previous)
			}

			failed := &LoadedPlugin{DBID: dbID, Info: &proto.PluginInfo{Id: "failed-load"}}
			manager.registerByDBID(failed)

			if tt.registerOther {
				manager.registerByDBID(&LoadedPlugin{DBID: dbID, Info: &proto.PluginInfo{Id: "other"}})
			}

			// ACT
			manager.restoreByDBID(dbID, failed, previous)

			// ASSERT
			plugin, ok := manager.PluginByDBID(dbID)
			require.Equal(t, tt.wantFound, ok, "index entry after rollback")

			if !tt.wantFound {
				assert.Nil(t, plugin)

				return
			}

			require.NotNil(t, plugin.Info)
			assert.Equal(t, tt.wantRestored, plugin.Info.Id, "plugin left in the index")
		})
	}
}

func TestManager_HostModules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		registered  *LoadedPlugin
		dbID        uint64
		wantModules []string
		wantFound   bool
	}{
		{
			name:       "unknown_db_id_is_not_found",
			registered: &LoadedPlugin{DBID: 7, HostModules: []string{"gameap-log"}},
			dbID:       8,
		},
		{
			name:        "known_db_id_lists_the_instantiated_modules",
			registered:  &LoadedPlugin{DBID: 7, HostModules: []string{"gameap-log", "gameap-servers"}},
			dbID:        7,
			wantModules: []string{"gameap-log", "gameap-servers"},
			wantFound:   true,
		},
		{
			name:        "plugin_without_host_modules_is_found_with_an_empty_list",
			registered:  &LoadedPlugin{DBID: 7},
			dbID:        7,
			wantModules: nil,
			wantFound:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			manager := NewManager(ManagerConfig{})
			manager.registerByDBID(tt.registered)

			// ACT
			modules, ok := manager.HostModules(tt.dbID)

			// ASSERT
			assert.Equal(t, tt.wantFound, ok, "lookup result")
			assert.Equal(t, tt.wantModules, modules, "reported host modules")
		})
	}
}

func TestManager_HostModules_returns_a_copy(t *testing.T) {
	t.Parallel()

	// ARRANGE
	manager := NewManager(ManagerConfig{})
	plugin := &LoadedPlugin{DBID: 7, HostModules: []string{"gameap-log", "gameap-servers"}}
	manager.registerByDBID(plugin)

	// ACT
	modules, ok := manager.HostModules(7)
	require.True(t, ok)
	require.Len(t, modules, 2)
	modules[0] = "tampered"

	// ASSERT
	assert.Equal(t, []string{"gameap-log", "gameap-servers"}, plugin.HostModules,
		"a caller cannot rewrite the plugin's module list through the returned slice")

	fresh, ok := manager.HostModules(7)
	require.True(t, ok)
	assert.Equal(t, []string{"gameap-log", "gameap-servers"}, fresh, "later readers still see the real list")
}
