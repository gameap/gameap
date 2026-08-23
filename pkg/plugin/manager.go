package plugin

import (
	"context"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/pkg/mergefs"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// initializeTimeout bounds guest code executed during module start
// (_initialize/_start) and API version verification. Host-side WASM
// compilation is deliberately outside this budget: compiling a large module
// on a slow machine can take longer than any reasonable guest budget, and it
// is not something a plugin controls. Initialization calls after start
// (GetInfo, Initialize, …) are each bounded by the wrapper's default call
// timeout.
const initializeTimeout = 60 * time.Second

// HostLibrary represents a host function library that can be instantiated.
type HostLibrary interface {
	// Instantiate registers the host functions into the given runtime.
	Instantiate(ctx context.Context, r wazero.Runtime) error
}

// HostLibraryFactory creates host libraries that need per-plugin configuration.
type HostLibraryFactory interface {
	// Create returns a new HostLibrary instance configured for the given plugin.
	Create(pluginID uint64) HostLibrary
}

// LoadedPlugin represents a loaded plugin instance.
type LoadedPlugin struct {
	Info            *proto.PluginInfo
	Instance        proto.PluginService
	Config          map[string]string
	Enabled         bool
	HTTPRoutes      []*proto.HTTPRoute
	FrontendBundle  []byte
	FrontendStyles  []byte
	ServerAbilities []*proto.ServerAbility

	// Protocol is the optional ProtocolService implementation (RCON/Query
	// protocol extension). It is non-nil for every loaded plugin (the wrapper
	// implements it); plugins that export no protocol functions simply return
	// empty registrations.
	Protocol       protocol.ProtocolService
	RconProtocols  []*protocol.RconProtocol
	QueryProtocols []*protocol.QueryProtocol

	// I18nFS and FrontendFS hold the plugin's contributed translation and
	// frontend static files (nil when the plugin ships none). They are layered
	// over the built-in filesystems by the application container.
	I18nFS     fs.FS
	FrontendFS fs.FS

	// DBID is the database record the plugin was loaded for (0 for transient
	// loads); per-plugin host libraries and grants are keyed on it.
	DBID uint64

	// HostImports lists the host functions the module imports (gameap-*
	// modules only), sorted. A guest can only call what it imports, so this
	// is a static description of which host libraries the plugin uses.
	HostImports []HostImport

	// SubscribedEvents is what the plugin answered at load time; the
	// dispatcher asks again on every refresh, this copy only describes the
	// plugin (dry-run, admin UI) without another guest call.
	SubscribedEvents []proto.EventType

	// disableReason is set by DisableWithReason; nil when the plugin was
	// disabled silently (unload, shutdown). onDisabled is the manager's
	// DisableHook, wired by Load for registered plugins only.
	disableReason atomic.Pointer[string]
	onDisabled    DisableHook
	guestLogs     *guestLogs

	// disabled is atomic because it is flipped on Unload and on guest call
	// timeouts while dispatchers concurrently read it without the manager lock.
	disabled atomic.Bool
	runtime  wazero.Runtime
}

// IsEnabled reports whether the plugin should receive events and HTTP requests.
func (p *LoadedPlugin) IsEnabled() bool {
	return p.Enabled && !p.disabled.Load()
}

// HasScheduledTaskHandler reports whether the module exports the scheduled
// task handler; plugins compiled without the sdk/scheduler module do not.
func (p *LoadedPlugin) HasScheduledTaskHandler() bool {
	w, ok := p.Instance.(interface{ HasScheduledTaskHandler() bool })

	return ok && w.HasScheduledTaskHandler()
}

// HasArchiveEventsHandler reports whether the module exports the archive
// event callbacks; plugins that never call nodefs.RegisterArchiveEventsHandler
// do not.
func (p *LoadedPlugin) HasArchiveEventsHandler() bool {
	w, ok := p.Instance.(interface{ HasArchiveEventsHandler() bool })

	return ok && w.HasArchiveEventsHandler()
}

// Disable permanently stops event and HTTP delivery to the plugin. Used when
// its runtime is closed (unload) or misbehaving (call timeout); dispatchers
// may still hold a pointer to it until their subscriptions are refreshed.
func (p *LoadedPlugin) Disable() {
	p.disabled.Store(true)
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
	Libraries        []HostLibrary
	LibraryFactories []HostLibraryFactory

	// MaxMemoryBytes caps the linear memory of every plugin module; 0 keeps
	// the wazero default (4 GiB). Modules declaring a larger maximum are
	// clamped, modules whose initial memory exceeds the cap fail to load.
	MaxMemoryBytes uint64
	// MaxModuleBytes rejects wasm files above this size before compilation;
	// 0 disables the check.
	MaxModuleBytes int

	// CompilationCacheDir persists compiled code across panel restarts;
	// empty keeps the in-memory cache. DisableCompilationCache turns
	// caching off entirely.
	CompilationCacheDir     string
	DisableCompilationCache bool

	// GuestLogger receives the guests' stdout (debug) and stderr (warn);
	// nil means slog.Default().
	GuestLogger *slog.Logger

	// OnPluginDisabled is notified when a registered plugin is disabled at
	// runtime (call timeout, guest exit). See DisableHook.
	OnPluginDisabled DisableHook

	// Observer receives guest-call and host-call signals for metrics; nil
	// means nothing is reported.
	Observer Observer
}

// Manager handles plugin lifecycle.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]*LoadedPlugin
	config  ManagerConfig
	// cache shares compiled code between runtimes for the manager's
	// lifetime, so validating and then installing the same wasm (or
	// reloading it) compiles it only once. In-memory or directory-backed
	// (see newCompilationCache; the directory is written at compile time).
	// Deliberately never closed: Close tears down the shared engines while
	// transient modules handed to callers may still be alive; nil when
	// caching is disabled.
	cache  wazero.CompilationCache
	closed bool
}

// NewManager creates a new plugin manager.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		plugins: make(map[string]*LoadedPlugin),
		config:  cfg,
		cache:   newCompilationCache(cfg),
	}
}

// normalizePluginID converts any accepted plugin ID form (compact, decimal,
// arbitrary string) to the canonical compact form used as the registry key.
func normalizePluginID(pluginID string) string {
	return CompactPluginID(ParsePluginID(pluginID))
}

// Load loads a plugin from WASM bytes and registers it in the manager.
// pluginID is used to configure per-plugin host libraries (like storage).
// Pass 0 if the plugin ID is not known (e.g., during initial info discovery).
func (m *Manager) Load(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*LoadedPlugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrManagerClosed
	}

	loadedPlugin, err := m.load(ctx, wasmBytes, config, pluginID)
	if err != nil {
		return nil, err
	}

	id := normalizePluginID(loadedPlugin.Info.Id)
	if _, exists := m.plugins[id]; exists {
		if closeErr := loadedPlugin.Close(ctx); closeErr != nil {
			slog.Warn("failed to close duplicate plugin runtime",
				slog.String("plugin_id", loadedPlugin.Info.Id),
				slog.String("error", closeErr.Error()),
			)
		}

		return nil, errors.Wrapf(ErrPluginAlreadyLoaded, "plugin: %s", loadedPlugin.Info.Id)
	}

	loadedPlugin.onDisabled = m.config.OnPluginDisabled
	m.plugins[id] = loadedPlugin

	return loadedPlugin, nil
}

// LoadTransient loads a plugin without registering it in the manager.
// It is meant for validation and inspection flows (dry-run, pre-install
// checks): the plugin never receives events or HTTP requests, and the caller
// owns the returned instance and must release it with Close.
func (m *Manager) LoadTransient(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*LoadedPlugin, error) {
	// The read lock is held through the whole load so Shutdown (write lock)
	// cannot complete while a transient runtime is still initializing.
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrManagerClosed
	}

	return m.load(ctx, wasmBytes, config, pluginID)
}

func (m *Manager) load(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*LoadedPlugin, error) {
	if m.config.MaxModuleBytes > 0 && len(wasmBytes) > m.config.MaxModuleBytes {
		return nil, errors.Wrapf(ErrModuleTooLarge, "%d bytes exceeds the limit of %d bytes",
			len(wasmBytes), m.config.MaxModuleBytes)
	}

	// Detached from the caller so a dropped HTTP request does not close the
	// module mid-initialization. Guest execution stays bounded: module start
	// inside initializeRuntime, and every initialization call below via the
	// wrapper's default call timeout.
	loadCtx := context.WithoutCancel(ctx)

	logs := newGuestLogs(m.config.GuestLogger)

	r, module, imports, err := m.initializeRuntime(loadCtx, wasmBytes, pluginID, logs)
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

	if wrapper, ok := plugin.(*pluginServiceWrapper); ok {
		wrapper.guestLogs = logs
		wrapper.observer = observerOrNop(m.config.Observer)
		wrapper.pluginID = pluginID
	}

	loadedPlugin, err := m.initializePlugin(loadCtx, r, plugin, config, pluginID, logs)
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

	loadedPlugin.HostImports = imports
	wireGuestHooks(loadedPlugin)

	return loadedPlugin, nil
}

// wireGuestHooks links the wrapper's runtime signals to the loaded plugin:
// a guest that terminates its module is disabled with a reason instead of
// failing every later call.
func wireGuestHooks(loadedPlugin *LoadedPlugin) {
	wrapper, ok := loadedPlugin.Instance.(*pluginServiceWrapper)
	if !ok {
		return
	}

	wrapper.onClosed = func(err error) {
		loadedPlugin.DisableWithReason(DisableReasonGuestExited + " (" + shortErrorText(err) + ")")
	}
}

// initializeRuntime builds the runtime, registers WASI and every host library,
// compiles and starts the module. Besides the module it returns the host
// functions the module imports (see LoadedPlugin.HostImports).
func (m *Manager) initializeRuntime(
	ctx context.Context,
	wasmBytes []byte,
	pluginID uint64,
	logs *guestLogs,
) (wazero.Runtime, api.Module, []HostImport, error) {
	r := wazero.NewRuntimeWithConfig(ctx, m.runtimeConfig())

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		closeRuntimeAfterFailure(ctx, r, "WASI instantiation", err)

		return nil, nil, nil, errors.Wrap(err, "failed to instantiate WASI")
	}

	// Instantiate the env module for AssemblyScript support
	envLib := &EnvHostLibrary{}
	if err := envLib.Instantiate(ctx, r); err != nil {
		closeRuntimeAfterFailure(ctx, r, "env module instantiation", err)

		return nil, nil, nil, errors.Wrap(err, "failed to instantiate env module")
	}

	// Host libraries register through the observed runtime so every host
	// function they export is counted; transient loads are not observed.
	libraryRuntime := r
	if pluginID != 0 {
		libraryRuntime = observeHostCalls(r, m.config.Observer, pluginID)
	}

	if err := m.instantiateLibraries(ctx, libraryRuntime, pluginID); err != nil {
		closeRuntimeAfterFailure(ctx, r, "host library instantiation", err)

		return nil, nil, nil, err
	}

	code, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		closeRuntimeAfterFailure(ctx, r, "WASM module compilation", err)

		return nil, nil, nil, errors.Wrap(err, "failed to compile WASM module")
	}

	imports := hostImports(code)

	// Try _initialize first (TinyGo), fall back to _start (standard Go).
	// Guest stdout/stderr go to the panel log (see guestlog.go) so panics
	// and runtime faults are visible.
	moduleConfig := wazero.NewModuleConfig().
		WithStartFunctions("_initialize", "_start").
		WithFSConfig(wazero.NewFSConfig()).
		WithStdout(logs.stdoutWriter()).
		WithStderr(logs.stderrWriter()).
		WithSysWalltime()

	// Module start runs guest code; only from this point does the guest
	// budget apply (compilation above is host work).
	startCtx, cancel := context.WithTimeout(ctx, initializeTimeout)
	defer cancel()

	module, err := r.InstantiateModule(startCtx, code, moduleConfig)

	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() != 0 {
			closeRuntimeAfterFailure(ctx, r, "module instantiation", err)

			return nil, nil, nil, errors.Wrapf(ErrUnexpectedExitCode, "exit code: %d", exitErr.ExitCode())
		} else if !errors.As(err, &exitErr) {
			closeRuntimeAfterFailure(ctx, r, "module instantiation", err)

			return nil, nil, nil, errors.Wrap(err, "failed to instantiate module")
		}
	}

	if err = m.verifyAPIVersion(startCtx, r, module); err != nil {
		return nil, nil, nil, err
	}

	return r, module, imports, nil
}

// HostImport is one host function a module imports.
type HostImport struct {
	// Module is the host module name, e.g. "gameap-nodecmd".
	Module string
	// Function is the exported host function name, e.g. "execute_command".
	Function string
}

// hostImportPrefix selects the panel's host libraries among a module's
// imports; WASI and the AssemblyScript env module are left out.
const hostImportPrefix = "gameap-"

// hostImports lists the panel host functions a compiled module imports,
// sorted by module and function, without duplicates.
func hostImports(code wazero.CompiledModule) []HostImport {
	seen := make(map[HostImport]struct{})
	imports := make([]HostImport, 0)

	for _, def := range code.ImportedFunctions() {
		module, function, ok := def.Import()
		if !ok || !strings.HasPrefix(module, hostImportPrefix) {
			continue
		}

		imp := HostImport{Module: module, Function: function}
		if _, dup := seen[imp]; dup {
			continue
		}

		seen[imp] = struct{}{}
		imports = append(imports, imp)
	}

	sort.Slice(imports, func(i, j int) bool {
		if imports[i].Module != imports[j].Module {
			return imports[i].Module < imports[j].Module
		}

		return imports[i].Function < imports[j].Function
	})

	return imports
}

// closeRuntimeAfterFailure releases a runtime whose setup failed at stage;
// a close failure is only logged, the setup error is what the caller reports.
func closeRuntimeAfterFailure(ctx context.Context, r wazero.Runtime, stage string, cause error) {
	if closeErr := r.Close(ctx); closeErr != nil {
		slog.Warn("failed to close runtime after "+stage+" failure",
			slog.String("error", closeErr.Error()),
			slog.String("cause", cause.Error()),
		)
	}
}

func (m *Manager) instantiateLibraries(ctx context.Context, r wazero.Runtime, pluginID uint64) error {
	for _, lib := range m.config.Libraries {
		if err := lib.Instantiate(ctx, r); err != nil {
			return errors.WithMessage(err, "failed to instantiate host library")
		}
	}

	for _, factory := range m.config.LibraryFactories {
		lib := factory.Create(pluginID)
		if err := lib.Instantiate(ctx, r); err != nil {
			return errors.WithMessage(err, "failed to instantiate factory host library")
		}
	}

	return nil
}

func (m *Manager) initializePlugin(
	ctx context.Context,
	r wazero.Runtime,
	plugin proto.PluginService,
	config map[string]string,
	pluginID uint64,
	logs *guestLogs,
) (*LoadedPlugin, error) {
	info, err := plugin.GetInfo(ctx, &proto.GetInfoRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get plugin info")
	}

	logs.SetPluginID(info.Id)

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
	var frontendStyles []byte
	bundleResp, err := plugin.GetFrontendBundle(ctx, &proto.GetFrontendBundleRequest{})
	if err != nil {
		slog.Debug("plugin has no frontend bundle",
			slog.String("plugin_id", info.Id),
			slog.String("error", err.Error()),
		)
	} else {
		if bundleResp.HasBundle && len(bundleResp.Bundle) > 0 {
			frontendBundle = bundleResp.Bundle
		}
		if bundleResp.HasStyles && len(bundleResp.Styles) > 0 {
			frontendStyles = bundleResp.Styles
		}
	}

	var serverAbilities []*proto.ServerAbility
	abilitiesResp, err := plugin.GetServerAbilities(ctx, &proto.GetServerAbilitiesRequest{})
	if err != nil {
		slog.Debug("plugin has no server abilities",
			slog.String("plugin_id", info.Id),
			slog.String("error", err.Error()),
		)
	} else if abilitiesResp != nil && len(abilitiesResp.Abilities) > 0 {
		serverAbilities = abilitiesResp.Abilities
	}

	protocolSvc, rconProtocols, queryProtocols := m.fetchProtocols(ctx, plugin, info.Id)

	i18nFS, frontendFS := m.buildPluginAssets(ctx, plugin, info.Id)

	var subscribedEvents []proto.EventType
	if resp, err := plugin.GetSubscribedEvents(ctx, &proto.GetSubscribedEventsRequest{}); err != nil {
		slog.Debug("failed to read plugin event subscriptions",
			slog.String("plugin_id", info.Id),
			slog.String("error", err.Error()),
		)
	} else if resp != nil {
		subscribedEvents = resp.Events
	}

	return &LoadedPlugin{
		Info:             info,
		Instance:         plugin,
		Config:           config,
		Enabled:          true,
		SubscribedEvents: subscribedEvents,
		HTTPRoutes:       httpRoutes,
		FrontendBundle:   frontendBundle,
		FrontendStyles:   frontendStyles,
		ServerAbilities:  serverAbilities,
		Protocol:         protocolSvc,
		RconProtocols:    rconProtocols,
		QueryProtocols:   queryProtocols,
		I18nFS:           i18nFS,
		FrontendFS:       frontendFS,
		DBID:             pluginID,
		guestLogs:        logs,
		runtime:          r,
	}, nil
}

// fetchProtocols reads the optional ProtocolService registrations. The module
// wrapper always implements ProtocolService; plugins that export no protocol
// functions simply return empty registrations.
func (m *Manager) fetchProtocols(
	ctx context.Context,
	plugin proto.PluginService,
	pluginID string,
) (protocol.ProtocolService, []*protocol.RconProtocol, []*protocol.QueryProtocol) {
	protocolSvc, ok := plugin.(protocol.ProtocolService)
	if !ok {
		return nil, nil, nil
	}

	var rconProtocols []*protocol.RconProtocol
	if resp, err := protocolSvc.GetRconProtocols(ctx, &protocol.GetRconProtocolsRequest{}); err != nil {
		slog.Debug("plugin has no rcon protocols",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)
	} else if resp != nil {
		rconProtocols = resp.Protocols
	}

	var queryProtocols []*protocol.QueryProtocol
	if resp, err := protocolSvc.GetQueryProtocols(ctx, &protocol.GetQueryProtocolsRequest{}); err != nil {
		slog.Debug("plugin has no query protocols",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)
	} else if resp != nil {
		queryProtocols = resp.Protocols
	}

	return protocolSvc, rconProtocols, queryProtocols
}

// Asset delivery limits bound how much a single plugin can push across the WASM
// boundary as translation/frontend files, so a misbehaving plugin cannot
// exhaust host memory.
const (
	maxAssetFileSize  = 8 << 20  // 8 MiB per file
	maxAssetTotalSize = 64 << 20 // 64 MiB per file group
)

// buildPluginAssets fetches the plugin's contributed translation and frontend
// files and turns each group into an in-memory fs.FS. A plugin without the
// GetAssets export, or with no valid files, contributes nil for that group.
func (m *Manager) buildPluginAssets(
	ctx context.Context,
	plugin proto.PluginService,
	pluginID string,
) (i18nFS, frontendFS fs.FS) {
	resp, err := plugin.GetAssets(ctx, &proto.GetAssetsRequest{})
	if err != nil {
		slog.Debug("plugin has no assets",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)

		return nil, nil
	}
	if resp == nil {
		return nil, nil
	}

	return buildAssetFS(pluginID, "i18n", resp.I18NFiles),
		buildAssetFS(pluginID, "frontend", resp.FrontendFiles)
}

// buildAssetFS validates and collects plugin-provided files into an fs.FS.
// Invalid paths, oversized files, and empty content are skipped and logged; the
// group is capped at maxAssetTotalSize. Returns nil when nothing usable remains.
func buildAssetFS(pluginID, group string, assets []*proto.AssetFile) fs.FS {
	if len(assets) == 0 {
		return nil
	}

	files := make(map[string][]byte, len(assets))
	var total int

	for _, asset := range assets {
		if asset == nil || len(asset.Content) == 0 {
			continue
		}

		if !fs.ValidPath(asset.Path) || asset.Path == "." {
			slog.Warn("skipping plugin asset with invalid path",
				slog.String("plugin_id", pluginID),
				slog.String("group", group),
				slog.String("path", asset.Path),
			)

			continue
		}

		if len(asset.Content) > maxAssetFileSize {
			slog.Warn("skipping oversized plugin asset",
				slog.String("plugin_id", pluginID),
				slog.String("group", group),
				slog.String("path", asset.Path),
				slog.Int("size", len(asset.Content)),
			)

			continue
		}

		if total+len(asset.Content) > maxAssetTotalSize {
			slog.Warn("plugin asset group exceeds size limit, ignoring remaining files",
				slog.String("plugin_id", pluginID),
				slog.String("group", group),
			)

			break
		}

		total += len(asset.Content)
		files[asset.Path] = asset.Content
	}

	if len(files) == 0 {
		return nil
	}

	assetFS, err := mergefs.FromFiles(files)
	if err != nil {
		slog.Warn("failed to build plugin asset filesystem",
			slog.String("plugin_id", pluginID),
			slog.String("group", group),
			slog.String("error", err.Error()),
		)

		return nil
	}

	return assetFS
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
	getServerAbilities := module.ExportedFunction("plugin_service_get_server_abilities")
	getAssets := module.ExportedFunction("plugin_service_get_assets")
	getRconProtocols := module.ExportedFunction("protocol_service_get_rcon_protocols")
	getQueryProtocols := module.ExportedFunction("protocol_service_get_query_protocols")
	rconOpen := module.ExportedFunction("protocol_service_rcon_open")
	rconExecute := module.ExportedFunction("protocol_service_rcon_execute")
	rconClose := module.ExportedFunction("protocol_service_rcon_close")
	queryServer := module.ExportedFunction("protocol_service_query_server")
	parsePlayers := module.ExportedFunction("protocol_service_parse_players")
	handleScheduledTask := module.ExportedFunction("scheduled_task_handler_handle_scheduled_task")
	handleArchiveProgress := module.ExportedFunction("archive_events_handler_handle_archive_progress")
	handleArchiveCompleted := module.ExportedFunction("archive_events_handler_handle_archive_completed")

	return &pluginServiceWrapper{
		gate:                make(chan struct{}, 1),
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
		getserverabilities:  getServerAbilities,
		getassets:           getAssets,
		getrconprotocols:    getRconProtocols,
		getqueryprotocols:   getQueryProtocols,
		rconopen:            rconOpen,
		rconexecute:         rconExecute,
		rconclose:           rconClose,
		queryserver:         queryServer,
		parseplayers:        parsePlayers,
		handlescheduledtask: handleScheduledTask,

		handlearchiveprogress:  handleArchiveProgress,
		handlearchivecompleted: handleArchiveCompleted,
	}, nil
}

// Unload unloads a plugin by ID. The ID may be given in any accepted form
// (compact, decimal, raw plugin info ID).
func (m *Manager) Unload(ctx context.Context, pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := normalizePluginID(pluginID)

	plugin, exists := m.plugins[id]
	if !exists {
		return errors.Wrapf(ErrPluginNotFound, "plugin: %s", pluginID)
	}

	plugin.Disable()

	_, err := plugin.Instance.Shutdown(ctx, &proto.ShutdownRequest{
		Context: &proto.PluginContext{PluginId: plugin.Info.Id},
	})
	if err != nil {
		slog.Warn("plugin shutdown failed",
			slog.String("plugin_id", plugin.Info.Id),
			slog.String("error", err.Error()),
		)
	}

	plugin.guestLogs.Flush()

	if err := plugin.Close(ctx); err != nil {
		return errors.WithMessage(err, "failed to close plugin")
	}

	delete(m.plugins, id)

	return nil
}

// GetPlugin returns a loaded plugin by ID in any accepted form. A nil
// manager (plugins disabled) has no plugins.
func (m *Manager) GetPlugin(pluginID string) (*LoadedPlugin, bool) {
	if m == nil {
		return nil, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[normalizePluginID(pluginID)]

	return plugin, exists
}

// GetPlugins returns all loaded plugins, ordered by plugin ID. The order is
// stable because callers derive order-sensitive output from it: the container
// layers plugin filesystems (so two plugins shipping the same path shadow each
// other deterministically) and the frontend handlers concatenate styles and
// bundles.
func (m *Manager) GetPlugins() []*LoadedPlugin {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.plugins))
	for id := range m.plugins {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	plugins := make([]*LoadedPlugin, 0, len(ids))
	for _, id := range ids {
		plugins = append(plugins, m.plugins[id])
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
		plugin.Disable()

		// The guest knows itself by its declared ID, not the normalized map key.
		_, _ = plugin.Instance.Shutdown(ctx, &proto.ShutdownRequest{
			Context: &proto.PluginContext{PluginId: plugin.Info.Id},
		})

		plugin.guestLogs.Flush()

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
		if p.IsEnabled() && len(p.HTTPRoutes) > 0 {
			routes[pluginID] = p.HTTPRoutes
		}
	}

	return routes
}

// ServerAbility represents a server ability with plugin context.
type ServerAbility struct {
	PluginID string
	Name     string
	Title    string
}

// GetAllServerAbilities returns all server abilities from all loaded plugins.
func (m *Manager) GetAllServerAbilities() []ServerAbility {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var abilities []ServerAbility
	for pluginID, p := range m.plugins {
		if !p.IsEnabled() || len(p.ServerAbilities) == 0 {
			continue
		}

		for _, ability := range p.ServerAbilities {
			abilities = append(abilities, ServerAbility{
				PluginID: pluginID,
				Name:     "plugin:" + pluginID + ":" + ability.Name,
				Title:    ability.Title,
			})
		}
	}

	return abilities
}

// RconProtocolRegistration is a plugin RCON protocol registration paired with
// the compact ID of the plugin that provides it.
type RconProtocolRegistration struct {
	PluginID string
	Protocol *protocol.RconProtocol
}

// QueryProtocolRegistration is a plugin Query protocol registration paired with
// the compact ID of the plugin that provides it.
type QueryProtocolRegistration struct {
	PluginID string
	Protocol *protocol.QueryProtocol
}

// GetAllRconProtocols returns every RCON protocol registration from enabled
// plugins. The registrations are ordered by plugin ID so resolution is stable.
func (m *Manager) GetAllRconProtocols() []RconProtocolRegistration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var regs []RconProtocolRegistration
	for pluginID, p := range m.plugins {
		if !p.IsEnabled() || len(p.RconProtocols) == 0 {
			continue
		}

		for _, pr := range p.RconProtocols {
			regs = append(regs, RconProtocolRegistration{PluginID: pluginID, Protocol: pr})
		}
	}

	slices.SortFunc(regs, func(a, b RconProtocolRegistration) int {
		return strings.Compare(a.PluginID, b.PluginID)
	})

	return regs
}

// GetAllQueryProtocols returns every Query protocol registration from enabled
// plugins, ordered by plugin ID.
func (m *Manager) GetAllQueryProtocols() []QueryProtocolRegistration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var regs []QueryProtocolRegistration
	for pluginID, p := range m.plugins {
		if !p.IsEnabled() || len(p.QueryProtocols) == 0 {
			continue
		}

		for _, pr := range p.QueryProtocols {
			regs = append(regs, QueryProtocolRegistration{PluginID: pluginID, Protocol: pr})
		}
	}

	slices.SortFunc(regs, func(a, b QueryProtocolRegistration) int {
		return strings.Compare(a.PluginID, b.PluginID)
	})

	return regs
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
