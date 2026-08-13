package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

var errRuntimeClose = errors.New("runtime close refused")

// closingRuntime lets a test make LoadedPlugin.Close fail. Only Close is ever
// called on the runtime from that path, so the embedded nil interface is safe.
type closingRuntime struct {
	wazero.Runtime

	err error
}

func (r closingRuntime) Close(_ context.Context) error {
	return r.err
}

// mockPluginService implements proto.PluginService for testing.
type mockPluginService struct {
	infoFunc                func(ctx context.Context, req *proto.GetInfoRequest) (*proto.PluginInfo, error)
	shutdownFunc            func(ctx context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error)
	handleEventFunc         func(ctx context.Context, event *proto.Event) (*proto.EventResult, error)
	getSubscribedEventsFunc func(ctx context.Context, req *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error)
	getAssetsFunc           func(ctx context.Context, req *proto.GetAssetsRequest) (*proto.GetAssetsResponse, error)
}

func (m *mockPluginService) GetInfo(
	ctx context.Context,
	req *proto.GetInfoRequest,
) (*proto.PluginInfo, error) {
	if m.infoFunc != nil {
		return m.infoFunc(ctx, req)
	}

	return &proto.PluginInfo{Id: "test-plugin"}, nil
}

func (m *mockPluginService) Initialize(
	_ context.Context,
	_ *proto.InitializeRequest,
) (*proto.InitializeResponse, error) {
	return &proto.InitializeResponse{}, nil
}

func (m *mockPluginService) Shutdown(
	ctx context.Context,
	req *proto.ShutdownRequest,
) (*proto.ShutdownResponse, error) {
	if m.shutdownFunc != nil {
		return m.shutdownFunc(ctx, req)
	}

	return &proto.ShutdownResponse{}, nil
}

func (m *mockPluginService) HandleEvent(
	ctx context.Context,
	event *proto.Event,
) (*proto.EventResult, error) {
	if m.handleEventFunc != nil {
		return m.handleEventFunc(ctx, event)
	}

	return &proto.EventResult{}, nil
}

func (m *mockPluginService) GetSubscribedEvents(
	ctx context.Context,
	req *proto.GetSubscribedEventsRequest,
) (*proto.GetSubscribedEventsResponse, error) {
	if m.getSubscribedEventsFunc != nil {
		return m.getSubscribedEventsFunc(ctx, req)
	}

	return &proto.GetSubscribedEventsResponse{}, nil
}

func (m *mockPluginService) GetHTTPRoutes(
	_ context.Context,
	_ *proto.GetHTTPRoutesRequest,
) (*proto.GetHTTPRoutesResponse, error) {
	return &proto.GetHTTPRoutesResponse{}, nil
}

func (m *mockPluginService) HandleHTTPRequest(
	_ context.Context,
	_ *proto.HTTPRequest,
) (*proto.HTTPResponse, error) {
	return &proto.HTTPResponse{}, nil
}

func (m *mockPluginService) GetFrontendBundle(
	_ context.Context,
	_ *proto.GetFrontendBundleRequest,
) (*proto.GetFrontendBundleResponse, error) {
	return &proto.GetFrontendBundleResponse{}, nil
}

func (m *mockPluginService) GetServerAbilities(
	_ context.Context,
	_ *proto.GetServerAbilitiesRequest,
) (*proto.GetServerAbilitiesResponse, error) {
	return &proto.GetServerAbilitiesResponse{}, nil
}

func (m *mockPluginService) GetAssets(
	ctx context.Context,
	req *proto.GetAssetsRequest,
) (*proto.GetAssetsResponse, error) {
	if m.getAssetsFunc != nil {
		return m.getAssetsFunc(ctx, req)
	}

	return &proto.GetAssetsResponse{}, nil
}

func TestValidateRoutePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError string
	}{
		{
			name: "valid_root_path",
			path: "/",
		},
		{
			name: "valid_simple_path",
			path: "/users",
		},
		{
			name: "valid_nested_path",
			path: "/api/v1/users",
		},
		{
			name: "valid_path_with_param",
			path: "/users/{id}",
		},
		{
			name: "valid_path_with_underscore",
			path: "/user_info",
		},
		{
			name: "valid_path_with_hyphen",
			path: "/user-info",
		},
		{
			name:      "empty_path_returns_error",
			path:      "",
			wantError: "path cannot be empty",
		},
		{
			name:      "path_without_leading_slash_returns_error",
			path:      "users",
			wantError: "path must start with '/'",
		},
		{
			name:      "path_with_double_dots_returns_error",
			path:      "/api/../users",
			wantError: "path cannot contain '..'",
		},
		{
			name:      "path_with_double_slash_returns_error",
			path:      "/api//users",
			wantError: "path cannot contain '//'",
		},
		{
			name:      "path_with_invalid_chars_returns_error",
			path:      "/api/users?query=1",
			wantError: "path contains invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRoutePath(tt.path)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsValidHTTPMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   bool
	}{
		{
			name:   "valid_GET",
			method: "GET",
			want:   true,
		},
		{
			name:   "valid_POST",
			method: "POST",
			want:   true,
		},
		{
			name:   "valid_PUT",
			method: "PUT",
			want:   true,
		},
		{
			name:   "valid_PATCH",
			method: "PATCH",
			want:   true,
		},
		{
			name:   "valid_DELETE",
			method: "DELETE",
			want:   true,
		},
		{
			name:   "valid_HEAD",
			method: "HEAD",
			want:   true,
		},
		{
			name:   "valid_OPTIONS",
			method: "OPTIONS",
			want:   true,
		},
		{
			name:   "lowercase_get_is_valid",
			method: "get",
			want:   true,
		},
		{
			name:   "mixedcase_Get_is_valid",
			method: "Get",
			want:   true,
		},
		{
			name:   "mixedcase_gEt_is_valid",
			method: "gEt",
			want:   true,
		},
		{
			name:   "invalid_CONNECT",
			method: "CONNECT",
			want:   false,
		},
		{
			name:   "invalid_TRACE",
			method: "TRACE",
			want:   false,
		},
		{
			name:   "invalid_INVALID",
			method: "INVALID",
			want:   false,
		},
		{
			name:   "invalid_empty",
			method: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidHTTPMethod(tt.method)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJoinErrors(t *testing.T) {
	tests := []struct {
		name      string
		errs      []error
		wantNil   bool
		wantError string
	}{
		{
			name:    "nil_slice_returns_nil",
			errs:    nil,
			wantNil: true,
		},
		{
			name:    "empty_slice_returns_nil",
			errs:    []error{},
			wantNil: true,
		},
		{
			name:      "single_error_returns_same_error",
			errs:      []error{errors.New("error1")},
			wantError: "error1",
		},
		{
			name:      "multiple_errors_joined",
			errs:      []error{errors.New("error1"), errors.New("error2"), errors.New("error3")},
			wantError: "error3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := joinErrors(tt.errs)

			if tt.wantNil {
				require.Nil(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	t.Run("creates_manager_with_empty_plugins_map", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		require.NotNil(t, manager.plugins)
		assert.Empty(t, manager.plugins)
	})

	t.Run("stores_config_correctly", func(t *testing.T) {
		cfg := ManagerConfig{
			Libraries:        []HostLibrary{},
			LibraryFactories: []HostLibraryFactory{},
		}
		manager := NewManager(cfg)

		assert.NotNil(t, manager.config)
	})

	t.Run("manager_not_closed_initially", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		assert.False(t, manager.closed)
	})
}

func TestGetPlugin(t *testing.T) {
	t.Run("returns_plugin_when_exists", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		expectedPlugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "test-plugin"},
			Enabled: true,
		}
		manager.plugins[NormalizePluginID("test-plugin")] = expectedPlugin

		plugin, exists := manager.GetPlugin("test-plugin")

		assert.True(t, exists)
		assert.Equal(t, expectedPlugin, plugin)
	})

	t.Run("returns_false_when_not_exists", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		plugin, exists := manager.GetPlugin("nonexistent")

		assert.False(t, exists)
		assert.Nil(t, plugin)
	})
}

func TestGetPlugins(t *testing.T) {
	t.Run("returns_empty_slice_when_no_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		plugins := manager.GetPlugins()

		require.NotNil(t, plugins)
		assert.Empty(t, plugins)
	})

	t.Run("returns_all_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		plugin1 := &LoadedPlugin{Info: &proto.PluginInfo{Id: "plugin1"}}
		plugin2 := &LoadedPlugin{Info: &proto.PluginInfo{Id: "plugin2"}}
		manager.plugins["plugin1"] = plugin1
		manager.plugins["plugin2"] = plugin2

		plugins := manager.GetPlugins()

		require.Len(t, plugins, 2)
	})
}

func TestGetHTTPRoutes(t *testing.T) {
	t.Run("returns_empty_map_when_no_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		routes := manager.GetHTTPRoutes()

		require.NotNil(t, routes)
		assert.Empty(t, routes)
	})

	t.Run("returns_routes_from_enabled_plugins_only", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["enabled-plugin"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "enabled-plugin"},
			Enabled: true,
			HTTPRoutes: []*proto.HTTPRoute{
				{Path: "/api/test", Methods: []string{"GET"}},
			},
		}

		routes := manager.GetHTTPRoutes()

		require.Len(t, routes, 1)
		require.Len(t, routes["enabled-plugin"], 1)
		assert.Equal(t, "/api/test", routes["enabled-plugin"][0].Path)
	})

	t.Run("skips_disabled_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["disabled-plugin"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "disabled-plugin"},
			Enabled: false,
			HTTPRoutes: []*proto.HTTPRoute{
				{Path: "/api/test", Methods: []string{"GET"}},
			},
		}

		routes := manager.GetHTTPRoutes()

		assert.Empty(t, routes)
	})

	t.Run("skips_plugins_without_routes", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["no-routes-plugin"] = &LoadedPlugin{
			Info:       &proto.PluginInfo{Id: "no-routes-plugin"},
			Enabled:    true,
			HTTPRoutes: nil,
		}

		routes := manager.GetHTTPRoutes()

		assert.Empty(t, routes)
	})
}

func TestGetAllServerAbilities(t *testing.T) {
	t.Run("returns_empty_slice_when_no_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		abilities := manager.GetAllServerAbilities()

		assert.Empty(t, abilities)
	})

	t.Run("returns_abilities_from_enabled_plugins_only", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["test-plugin"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "test-plugin"},
			Enabled: true,
			ServerAbilities: []*proto.ServerAbility{
				{Name: "ability1", Title: "Ability 1"},
				{Name: "ability2", Title: "Ability 2"},
			},
		}

		abilities := manager.GetAllServerAbilities()

		require.Len(t, abilities, 2)
	})

	t.Run("formats_ability_name_correctly", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["my-plugin"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "my-plugin"},
			Enabled: true,
			ServerAbilities: []*proto.ServerAbility{
				{Name: "manage", Title: "Manage Server"},
			},
		}

		abilities := manager.GetAllServerAbilities()

		require.Len(t, abilities, 1)
		assert.Equal(t, "plugin:my-plugin:manage", abilities[0].Name)
		assert.Equal(t, "Manage Server", abilities[0].Title)
		assert.Equal(t, "my-plugin", abilities[0].PluginID)
	})

	t.Run("skips_disabled_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["disabled-plugin"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "disabled-plugin"},
			Enabled: false,
			ServerAbilities: []*proto.ServerAbility{
				{Name: "ability", Title: "Ability"},
			},
		}

		abilities := manager.GetAllServerAbilities()

		assert.Empty(t, abilities)
	})

	t.Run("skips_plugins_without_abilities", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["no-abilities-plugin"] = &LoadedPlugin{
			Info:            &proto.PluginInfo{Id: "no-abilities-plugin"},
			Enabled:         true,
			ServerAbilities: nil,
		}

		abilities := manager.GetAllServerAbilities()

		assert.Empty(t, abilities)
	})
}

func TestLoadedPlugin_Close(t *testing.T) {
	t.Run("returns_nil_when_runtime_is_nil", func(t *testing.T) {
		plugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "test-plugin"},
			runtime: nil,
		}

		err := plugin.Close(context.Background())

		require.NoError(t, err)
	})
}

func TestUnload(t *testing.T) {
	t.Run("returns_error_when_plugin_not_found", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		err := manager.Unload(context.Background(), "nonexistent")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin not found")
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("removes_plugin_from_map", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins[NormalizePluginID("test-plugin")] = &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{},
			runtime:  nil,
		}

		err := manager.Unload(context.Background(), "test-plugin")

		require.NoError(t, err)
		_, exists := manager.plugins[NormalizePluginID("test-plugin")]
		assert.False(t, exists)
	})

	t.Run("disables_plugin_on_unload", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		plugin := &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Enabled:  true,
			Instance: &mockPluginService{},
			runtime:  nil,
		}
		manager.plugins[NormalizePluginID("test-plugin")] = plugin

		err := manager.Unload(context.Background(), "test-plugin")

		require.NoError(t, err)
		assert.False(t, plugin.IsEnabled())
	})

	t.Run("unloads_by_decimal_id_when_registered_under_compact_id", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins[NormalizePluginID("123")] = &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "123"},
			Instance: &mockPluginService{},
			runtime:  nil,
		}

		err := manager.Unload(context.Background(), "123")

		require.NoError(t, err)
		assert.Empty(t, manager.plugins)
	})

	t.Run("calls_shutdown_on_plugin", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		shutdownCalled := false
		manager.plugins[NormalizePluginID("test-plugin")] = &LoadedPlugin{
			Info: &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{
				shutdownFunc: func(_ context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
					shutdownCalled = true
					assert.Equal(t, "test-plugin", req.Context.PluginId)

					return &proto.ShutdownResponse{}, nil
				},
			},
			runtime: nil,
		}

		err := manager.Unload(context.Background(), "test-plugin")

		require.NoError(t, err)
		assert.True(t, shutdownCalled)
	})

	t.Run("deletes_the_entry_even_when_close_errors", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins[NormalizePluginID("test-plugin")] = &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{},
			runtime:  closingRuntime{err: errRuntimeClose},
		}

		err := manager.Unload(context.Background(), "test-plugin")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "runtime close refused")
		assert.Empty(t, manager.plugins)
	})

	t.Run("does_not_hold_the_write_lock_during_guest_shutdown", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		inShutdown := make(chan struct{})
		releaseShutdown := make(chan struct{})

		manager.plugins[NormalizePluginID("test-plugin")] = &LoadedPlugin{
			Info: &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{
				shutdownFunc: func(_ context.Context, _ *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
					close(inShutdown)
					<-releaseShutdown

					return &proto.ShutdownResponse{}, nil
				},
			},
			runtime: nil,
		}

		unloaded := make(chan error, 1)
		go func() {
			unloaded <- manager.Unload(context.Background(), "test-plugin")
		}()

		<-inShutdown

		readerDone := make(chan int, 1)
		go func() {
			readerDone <- len(manager.GetPlugins())
		}()

		select {
		case count := <-readerDone:
			assert.Equal(t, 0, count, "the entry must already be gone from the registry")
		case <-time.After(2 * time.Second):
			t.Error("GetPlugins blocked while the guest shutdown was in flight")
		}

		close(releaseShutdown)
		require.NoError(t, <-unloaded)
	})
}

func TestRegister(t *testing.T) {
	t.Run("adopts_a_transient_plugin", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		plugin := &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{},
		}

		err := manager.Register(plugin)

		require.NoError(t, err)
		registered, exists := manager.GetPlugin("test-plugin")
		require.True(t, exists)
		assert.Same(t, plugin, registered)
	})

	t.Run("returns_already_loaded_for_a_duplicate_id", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		first := &LoadedPlugin{Info: &proto.PluginInfo{Id: "test-plugin"}, Instance: &mockPluginService{}}
		second := &LoadedPlugin{Info: &proto.PluginInfo{Id: "test-plugin"}, Instance: &mockPluginService{}}
		require.NoError(t, manager.Register(first))

		err := manager.Register(second)

		require.ErrorIs(t, err, ErrPluginAlreadyLoaded)
		registered, exists := manager.GetPlugin("test-plugin")
		require.True(t, exists)
		assert.Same(t, first, registered, "the incumbent must stay registered")
	})

	t.Run("returns_manager_closed_after_shutdown", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		require.NoError(t, manager.Shutdown(context.Background()))

		err := manager.Register(&LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{},
		})

		require.ErrorIs(t, err, ErrManagerClosed)
	})

	t.Run("rejects_a_plugin_without_info", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		err := manager.Register(&LoadedPlugin{Instance: &mockPluginService{}})

		require.ErrorIs(t, err, ErrPluginNotInitialized)
	})
}

func TestReplace(t *testing.T) {
	t.Run("swaps_and_returns_the_previous_plugin", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		old := &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin", Version: "1.0.0"},
			Enabled:  true,
			Instance: &mockPluginService{},
		}
		fresh := &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin", Version: "2.0.0"},
			Enabled:  true,
			Instance: &mockPluginService{},
		}
		require.NoError(t, manager.Register(old))

		previous, err := manager.Replace(fresh)

		require.NoError(t, err)
		assert.Same(t, old, previous)
		assert.False(t, previous.IsEnabled(), "the displaced plugin must stop receiving deliveries")

		registered, exists := manager.GetPlugin("test-plugin")
		require.True(t, exists)
		assert.Same(t, fresh, registered)
		assert.True(t, registered.IsEnabled())
	})

	t.Run("returns_nil_previous_when_the_id_was_not_registered", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		fresh := &LoadedPlugin{Info: &proto.PluginInfo{Id: "test-plugin"}, Instance: &mockPluginService{}}

		previous, err := manager.Replace(fresh)

		require.NoError(t, err)
		assert.Nil(t, previous)
		require.Len(t, manager.GetPlugins(), 1)
	})

	t.Run("matches_the_incumbent_registered_under_another_id_form", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		old := &LoadedPlugin{Info: &proto.PluginInfo{Id: "123"}, Instance: &mockPluginService{}}
		require.NoError(t, manager.Register(old))

		previous, err := manager.Replace(&LoadedPlugin{
			Info:     &proto.PluginInfo{Id: CompactPluginID(ParsePluginID("123"))},
			Instance: &mockPluginService{},
		})

		require.NoError(t, err)
		assert.Same(t, old, previous)
		require.Len(t, manager.GetPlugins(), 1)
	})

	t.Run("returns_manager_closed_after_shutdown", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		require.NoError(t, manager.Shutdown(context.Background()))

		previous, err := manager.Replace(&LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Instance: &mockPluginService{},
		})

		require.ErrorIs(t, err, ErrManagerClosed)
		assert.Nil(t, previous)
	})
}

func TestShutdownPlugin(t *testing.T) {
	t.Run("returns_nil_for_a_nil_plugin", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		require.NoError(t, manager.ShutdownPlugin(context.Background(), nil))
	})

	t.Run("disables_the_plugin_and_reports_close_errors", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		plugin := &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "test-plugin"},
			Enabled:  true,
			Instance: &mockPluginService{},
			runtime:  closingRuntime{err: errRuntimeClose},
		}

		err := manager.ShutdownPlugin(context.Background(), plugin)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "runtime close refused")
		assert.False(t, plugin.IsEnabled())
	})
}

func TestShutdown(t *testing.T) {
	t.Run("sets_closed_flag", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})

		err := manager.Shutdown(context.Background())

		require.NoError(t, err)
		assert.True(t, manager.closed)
	})

	t.Run("clears_plugins_map", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.plugins["plugin1"] = &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "plugin1"},
			Instance: &mockPluginService{},
			runtime:  nil,
		}
		manager.plugins["plugin2"] = &LoadedPlugin{
			Info:     &proto.PluginInfo{Id: "plugin2"},
			Instance: &mockPluginService{},
			runtime:  nil,
		}

		err := manager.Shutdown(context.Background())

		require.NoError(t, err)
		assert.Empty(t, manager.plugins)
	})

	t.Run("passes_declared_plugin_id_to_shutdown_context", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		gotPluginID := ""
		// "my-custom-plugin" is not a canonical compact ID, so the map key
		// (normalized) differs from the declared Info.Id.
		manager.plugins[NormalizePluginID("my-custom-plugin")] = &LoadedPlugin{
			Info: &proto.PluginInfo{Id: "my-custom-plugin"},
			Instance: &mockPluginService{
				shutdownFunc: func(_ context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
					gotPluginID = req.Context.PluginId

					return &proto.ShutdownResponse{}, nil
				},
			},
			runtime: nil,
		}

		err := manager.Shutdown(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "my-custom-plugin", gotPluginID,
			"guest must receive its declared ID, not the normalized map key")
	})

	t.Run("calls_shutdown_on_all_plugins", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		shutdownCalls := make(map[string]bool)

		for _, id := range []string{"plugin1", "plugin2"} {
			pluginID := id
			manager.plugins[pluginID] = &LoadedPlugin{
				Info: &proto.PluginInfo{Id: pluginID},
				Instance: &mockPluginService{
					shutdownFunc: func(_ context.Context, _ *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
						shutdownCalls[pluginID] = true

						return &proto.ShutdownResponse{}, nil
					},
				},
				runtime: nil,
			}
		}

		err := manager.Shutdown(context.Background())

		require.NoError(t, err)
		assert.True(t, shutdownCalls["plugin1"])
		assert.True(t, shutdownCalls["plugin2"])
	})
}

func TestLoad(t *testing.T) {
	t.Run("returns_error_when_manager_closed", func(t *testing.T) {
		manager := NewManager(ManagerConfig{})
		manager.closed = true

		_, err := manager.Load(context.Background(), []byte{}, nil, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrManagerClosed)
	})
}
