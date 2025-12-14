package plugin

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// HostLibrary represents a host function library that can be instantiated.
type HostLibrary interface {
	// Instantiate registers the host functions into the given runtime.
	Instantiate(ctx context.Context, r wazero.Runtime) error
}

// LoadedPlugin represents a loaded plugin instance.
type LoadedPlugin struct {
	Info           *proto.PluginInfo
	Instance       proto.PluginService
	Config         map[string]string
	Enabled        bool
	HTTPRoutes     []*proto.HTTPRoute
	FrontendBundle []byte

	runtime wazero.Runtime
}

// Close releases the plugin resources.
func (p *LoadedPlugin) Close(ctx context.Context) error {
	if p.runtime != nil {
		return p.runtime.Close(ctx)
	}

	return nil
}

// ManagerConfig holds configuration for the plugin manager.
type ManagerConfig struct {
	Libraries []HostLibrary
}

// Manager handles plugin lifecycle.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]*LoadedPlugin
	config  ManagerConfig
	closed  bool
}

// NewManager creates a new plugin manager.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		plugins: make(map[string]*LoadedPlugin),
		config:  cfg,
	}
}

// Load loads a plugin from WASM bytes.
func (m *Manager) Load(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
) (*LoadedPlugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrManagerClosed
	}

	r, module, err := m.initializeRuntime(ctx, wasmBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to initialize runtime")
	}

	plugin, err := m.createPluginWrapper(module)
	if err != nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after plugin wrapper creation failure",
				slog.String("error", closeErr.Error()),
				slog.String("plugin_error", err.Error()),
			)
		}

		return nil, errors.WithMessage(err, "failed to create plugin wrapper")
	}

	loadedPlugin, err := m.initializePlugin(ctx, r, plugin, config)
	if err != nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after plugin initialization failure",
				slog.String("error", closeErr.Error()),
				slog.String("plugin_error", err.Error()),
			)
		}

		return nil, err
	}

	id := CompactPluginID(ParsePluginID(loadedPlugin.Info.Id))

	m.plugins[id] = loadedPlugin

	return loadedPlugin, nil
}

func (m *Manager) initializeRuntime(
	ctx context.Context,
	wasmBytes []byte,
) (wazero.Runtime, api.Module, error) {
	r := wazero.NewRuntime(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after WASI instantiation failure",
				slog.String("error", closeErr.Error()),
				slog.String("wasi_error", err.Error()),
			)
		}

		return nil, nil, errors.Wrap(err, "failed to instantiate WASI")
	}

	// Instantiate the env module for AssemblyScript support
	envLib := &EnvHostLibrary{}
	if err := envLib.Instantiate(ctx, r); err != nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after env module instantiation failure",
				slog.String("error", closeErr.Error()),
				slog.String("env_error", err.Error()),
			)
		}

		return nil, nil, errors.Wrap(err, "failed to instantiate env module")
	}

	for _, lib := range m.config.Libraries {
		if err := lib.Instantiate(ctx, r); err != nil {
			closeErr := r.Close(ctx)
			if closeErr != nil {
				slog.Warn("failed to close runtime after host library instantiation failure",
					slog.String("error", closeErr.Error()),
					slog.String("library_error", err.Error()),
				)
			}

			return nil, nil, errors.WithMessage(err, "failed to instantiate host library")
		}
	}

	code, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after WASM module compilation failure",
				slog.String("error", closeErr.Error()),
				slog.String("compilation_error", err.Error()),
			)
		}

		return nil, nil, errors.Wrap(err, "failed to compile WASM module")
	}

	// Try _initialize first (TinyGo), fall back to _start (standard Go)
	// Configure WASI with stdout/stderr for runtime error messages
	moduleConfig := wazero.NewModuleConfig().
		WithStartFunctions("_initialize", "_start").
		WithStdout(io.Discard).
		WithStderr(io.Discard).
		WithSysWalltime()

	module, err := r.InstantiateModule(ctx, code, moduleConfig)

	//nolint:nestif
	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() != 0 {
			closeErr := r.Close(ctx)
			if closeErr != nil {
				slog.Warn("failed to close runtime after module instantiation failure",
					slog.String("error", closeErr.Error()),
					slog.String("instantiation_error", err.Error()),
				)
			}

			return nil, nil, errors.Wrapf(ErrUnexpectedExitCode, "exit code: %d", exitErr.ExitCode())
		} else if !errors.As(err, &exitErr) {
			closeErr := r.Close(ctx)
			if closeErr != nil {
				slog.Warn("failed to close runtime after module instantiation failure",
					slog.String("error", closeErr.Error()),
					slog.String("instantiation_error", err.Error()),
				)
			}

			return nil, nil, errors.Wrap(err, "failed to instantiate module")
		}
	}

	if err = m.verifyAPIVersion(ctx, r, module); err != nil {
		return nil, nil, err
	}

	return r, module, nil
}

func (m *Manager) initializePlugin(
	ctx context.Context,
	r wazero.Runtime,
	plugin proto.PluginService,
	config map[string]string,
) (*LoadedPlugin, error) {
	info, err := plugin.GetInfo(ctx, &proto.GetInfoRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get plugin info")
	}

	if _, exists := m.plugins[info.Id]; exists {
		return nil, errors.Wrapf(ErrPluginAlreadyLoaded, "plugin: %s", info.Id)
	}

	initResp, err := plugin.Initialize(ctx, &proto.InitializeRequest{
		Context: &proto.PluginContext{PluginId: info.Id},
		Config:  config,
	})
	if err != nil {
		return nil, errors.Wrap(err, "plugin initialization failed")
	}

	if initResp.Result != nil && !initResp.Result.Success {
		errMsg := "unknown error"
		if initResp.Result.Error != nil {
			errMsg = *initResp.Result.Error
		}

		return nil, errors.Wrapf(ErrInitializationFailed, "%s", errMsg)
	}

	httpRoutes, err := m.fetchAndValidateHTTPRoutes(ctx, plugin, info.Id)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get HTTP routes")
	}

	var frontendBundle []byte
	bundleResp, err := plugin.GetFrontendBundle(ctx, &proto.GetFrontendBundleRequest{})
	if err != nil {
		slog.Debug("plugin has no frontend bundle",
			slog.String("plugin_id", info.Id),
			slog.String("error", err.Error()),
		)
	} else if bundleResp.HasBundle && len(bundleResp.Bundle) > 0 {
		frontendBundle = bundleResp.Bundle
	}

	return &LoadedPlugin{
		Info:           info,
		Instance:       plugin,
		Config:         config,
		Enabled:        true,
		HTTPRoutes:     httpRoutes,
		FrontendBundle: frontendBundle,
		runtime:        r,
	}, nil
}

func (m *Manager) verifyAPIVersion(ctx context.Context, r wazero.Runtime, module api.Module) error {
	apiVersion := module.ExportedFunction("plugin_service_api_version")
	if apiVersion == nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after missing api_version export",
				slog.String("error", closeErr.Error()),
			)
		}

		return errors.WithMessage(ErrExportNotFound, "plugin_service_api_version")
	}

	results, err := apiVersion.Call(ctx)
	if err != nil {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after api_version call failure",
				slog.String("error", closeErr.Error()),
				slog.String("api_version_error", err.Error()),
			)
		}

		return errors.Wrap(err, "failed to call api_version")
	}

	if len(results) != 1 || results[0] != proto.PluginServicePluginAPIVersion {
		closeErr := r.Close(ctx)
		if closeErr != nil {
			slog.Warn("failed to close runtime after api_version mismatch",
				slog.String("error", closeErr.Error()),
			)
		}

		return errors.Wrapf(ErrAPIVersionMismatch, "host=%d, plugin=%d",
			proto.PluginServicePluginAPIVersion, results[0])
	}

	return nil
}

// createPluginWrapper creates a plugin service wrapper from a module.
func (m *Manager) createPluginWrapper(module api.Module) (proto.PluginService, error) {
	exports := []string{
		"plugin_service_get_info",
		"plugin_service_initialize",
		"plugin_service_shutdown",
		"plugin_service_handle_event",
		"plugin_service_get_subscribed_events",
		"plugin_service_get_http_routes",
		"plugin_service_handle_http_request",
		"malloc",
		"free",
	}

	funcs := make(map[string]api.Function)
	for _, name := range exports {
		fn := module.ExportedFunction(name)
		if fn == nil {
			return nil, errors.WithMessagef(ErrExportNotFound, "failed to find export: %s", name)
		}

		funcs[name] = fn
	}

	// Optional exports (not all plugins implement these)
	getFrontendBundle := module.ExportedFunction("plugin_service_get_frontend_bundle")

	return &pluginServiceWrapper{
		module:              module,
		malloc:              funcs["malloc"],
		free:                funcs["free"],
		getinfo:             funcs["plugin_service_get_info"],
		initialize:          funcs["plugin_service_initialize"],
		shutdown:            funcs["plugin_service_shutdown"],
		handleevent:         funcs["plugin_service_handle_event"],
		getsubscribedevents: funcs["plugin_service_get_subscribed_events"],
		gethttproutes:       funcs["plugin_service_get_http_routes"],
		handlehttprequest:   funcs["plugin_service_handle_http_request"],
		getfrontendbundle:   getFrontendBundle,
	}, nil
}

// Unload unloads a plugin by ID.
func (m *Manager) Unload(ctx context.Context, pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginID]
	if !exists {
		return errors.Wrapf(ErrPluginNotFound, "plugin: %s", pluginID)
	}

	_, err := plugin.Instance.Shutdown(ctx, &proto.ShutdownRequest{
		Context: &proto.PluginContext{PluginId: pluginID},
	})
	if err != nil {
		slog.Warn("plugin shutdown failed",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)
	}

	if err := plugin.Close(ctx); err != nil {
		return errors.WithMessage(err, "failed to close plugin")
	}

	delete(m.plugins, pluginID)

	return nil
}

// GetPlugin returns a loaded plugin by ID.
func (m *Manager) GetPlugin(pluginID string) (*LoadedPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[pluginID]

	return plugin, exists
}

// GetPlugins returns all loaded plugins.
func (m *Manager) GetPlugins() []*LoadedPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]*LoadedPlugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}

	return plugins
}

// Shutdown gracefully shuts down the plugin manager.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true

	var errs []error
	for pluginID, plugin := range m.plugins {
		_, _ = plugin.Instance.Shutdown(ctx, &proto.ShutdownRequest{
			Context: &proto.PluginContext{PluginId: pluginID},
		})

		if err := plugin.Close(ctx); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to close plugin %s", pluginID))
		}
	}

	m.plugins = make(map[string]*LoadedPlugin)

	if len(errs) > 0 {
		return joinErrors(errs)
	}

	return nil
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	err := errs[0]
	for i := 1; i < len(errs); i++ {
		err = errors.Wrap(err, errs[i].Error())
	}

	return err
}

// GetHTTPRoutes returns all HTTP routes from all loaded plugins.
// Returns a map of plugin ID to their routes.
func (m *Manager) GetHTTPRoutes() map[string][]*proto.HTTPRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes := make(map[string][]*proto.HTTPRoute)
	for pluginID, p := range m.plugins {
		if p.Enabled && len(p.HTTPRoutes) > 0 {
			routes[pluginID] = p.HTTPRoutes
		}
	}

	return routes
}

// fetchAndValidateHTTPRoutes fetches HTTP routes from a plugin and validates them.
func (m *Manager) fetchAndValidateHTTPRoutes(
	ctx context.Context,
	plugin proto.PluginService,
	pluginID string,
) ([]*proto.HTTPRoute, error) {
	resp, err := plugin.GetHTTPRoutes(ctx, &proto.GetHTTPRoutesRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to call GetHTTPRoutes")
	}

	if resp == nil || len(resp.Routes) == 0 {
		return nil, nil
	}

	for _, route := range resp.Routes {
		if err := validateRoutePath(route.Path); err != nil {
			return nil, errors.Wrapf(err, "invalid route path %q for plugin %s", route.Path, pluginID)
		}

		if len(route.Methods) == 0 {
			return nil, errors.Errorf("route %q for plugin %s has no methods defined", route.Path, pluginID)
		}

		for _, method := range route.Methods {
			if !isValidHTTPMethod(method) {
				return nil, errors.Errorf("invalid HTTP method %q for route %q in plugin %s", method, route.Path, pluginID)
			}
		}
	}

	return resp.Routes, nil
}

// validPathRegex matches valid route path characters including path parameters.
var validPathRegex = regexp.MustCompile(`^(/[a-zA-Z0-9_\-{}]+)+$|^/$`)

// validateRoutePath validates a plugin route path.
func validateRoutePath(path string) error {
	if path == "" {
		return errors.New("path cannot be empty")
	}

	if !strings.HasPrefix(path, "/") {
		return errors.New("path must start with '/'")
	}

	if strings.Contains(path, "..") {
		return errors.New("path cannot contain '..'")
	}

	if strings.Contains(path, "//") {
		return errors.New("path cannot contain '//'")
	}

	if !validPathRegex.MatchString(path) {
		return errors.New("path contains invalid characters")
	}

	return nil
}

// isValidHTTPMethod checks if the given method is a valid HTTP method for plugin routes.
func isValidHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}
