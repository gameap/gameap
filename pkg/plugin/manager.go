package plugin

import (
	"cmp"
	"context"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
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

// HostLibraryCloser is implemented by factory-created host libraries that own
// per-plugin resources outliving a single host call — open connections,
// background operations. The manager closes them right after the plugin's
// runtime, so an unloaded plugin leaves nothing behind. Close runs while the
// manager lock is held, so it must not call back into the Manager.
type HostLibraryCloser interface {
	Close(ctx context.Context) error
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

	// HostModules names the host modules instantiated for this module
	// ("gameap-nodefs", ...), sorted; what gameap-host GetHostInfo reports.
	HostModules []string

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

	// libraries are the per-plugin host libraries holding resources of this
	// load (open SSH connections and the like); Close releases them.
	libraries []HostLibraryCloser
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

// HasSSHExecEventsHandler reports whether the module exports the ssh exec
// completion handler; plugins compiled without the sdk/ssh module do not.
func (p *LoadedPlugin) HasSSHExecEventsHandler() bool {
	w, ok := p.Instance.(interface{ HasSSHExecEventsHandler() bool })

	return ok && w.HasSSHExecEventsHandler()
}

// Disable permanently stops event and HTTP delivery to the plugin. Used when
// its runtime is closed (unload) or misbehaving (call timeout); dispatchers
// may still hold a pointer to it until their subscriptions are refreshed.
func (p *LoadedPlugin) Disable() {
	p.disabled.Store(true)
}

// Close releases the plugin: the runtime first, so the guest can no longer
// issue host calls, then the per-plugin host libraries holding its resources.
func (p *LoadedPlugin) Close(ctx context.Context) error {
	var err error
	if p.runtime != nil {
		err = p.runtime.Close(ctx)
	}

	closeHostLibraries(ctx, p.libraries)
	p.libraries = nil

	return err
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

	// byDBID indexes registered plugins by database id from the moment a
	// load starts, under its own lock: host libraries look their plugin up
	// from inside guest calls that run while mu is held (Initialize), so
	// this index must never take mu.
	stateMu sync.RWMutex
	byDBID  map[uint64]*LoadedPlugin
}

// NewManager creates a new plugin manager.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		plugins: make(map[string]*LoadedPlugin),
		config:  cfg,
		cache:   newCompilationCache(cfg),
		byDBID:  make(map[uint64]*LoadedPlugin),
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

	loadedPlugin, rollback, err := m.load(ctx, wasmBytes, config, pluginID, true)
	if err != nil {
		return nil, err
	}

	id := normalizePluginID(loadedPlugin.Info.Id)
	if _, exists := m.plugins[id]; exists {
		rollback()

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

	loadedPlugin, _, err := m.load(ctx, wasmBytes, config, pluginID, false)

	return loadedPlugin, err
}

// load builds a runtime for the module and initializes the plugin. With
// register set (Load, not LoadTransient) the plugin is reachable by database
// id from the moment its runtime starts, so host libraries called during
// Initialize (gameap-host) already find it; the index entry is rolled back
// when the load fails, and the returned rollback lets the caller undo it
// when it rejects the plugin afterwards (duplicate id).
func (m *Manager) load(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
	register bool,
) (*LoadedPlugin, func(), error) {
	rollback := func() {}

	if m.config.MaxModuleBytes > 0 && len(wasmBytes) > m.config.MaxModuleBytes {
		return nil, rollback, errors.Wrapf(ErrModuleTooLarge, "%d bytes exceeds the limit of %d bytes",
			len(wasmBytes), m.config.MaxModuleBytes)
	}

	// Detached from the caller so a dropped HTTP request does not close the
	// module mid-initialization. Guest execution stays bounded: module start
	// inside initializeRuntime, and every initialization call below via the
	// wrapper's default call timeout.
	loadCtx := context.WithoutCancel(ctx)

	logs := newGuestLogs(m.config.GuestLogger)

	loadedPlugin := &LoadedPlugin{
		Config:    config,
		Enabled:   true,
		DBID:      pluginID,
		guestLogs: logs,
	}

	if register && pluginID != 0 {
		previous := m.registerByDBID(loadedPlugin)
		rollback = func() { m.restoreByDBID(pluginID, loadedPlugin, previous) }

		defer func() {
			if loadedPlugin.runtime == nil {
				rollback()
			}
		}()
	}

	setup, err := m.initializeRuntime(loadCtx, wasmBytes, pluginID, logs)
	if err != nil {
		return nil, rollback, errors.WithMessage(err, "failed to initialize runtime")
	}

	loadedPlugin.HostModules = setup.hostModules

	plugin, err := m.createPluginWrapper(setup.module)
	if err != nil {
		closeRuntimeAfterFailure(ctx, setup.runtime, "plugin wrapper creation", err)
		closeHostLibraries(ctx, setup.closers)

		return nil, rollback, errors.WithMessage(err, "failed to create plugin wrapper")
	}

	if wrapper, ok := plugin.(*pluginServiceWrapper); ok {
		wrapper.guestLogs = logs
		wrapper.observer = observerOrNop(m.config.Observer)
		wrapper.pluginID = pluginID
	}

	// The runtime is closed first so the guest can no longer issue host calls,
	// then the libraries holding what it opened during Initialize.
	if err := m.initializePlugin(loadCtx, setup.runtime, plugin, loadedPlugin, logs); err != nil {
		closeRuntimeAfterFailure(ctx, setup.runtime, "plugin initialization", err)
		closeHostLibraries(ctx, setup.closers)

		return nil, rollback, err
	}

	// From here the plugin owns its per-plugin host libraries; Close releases
	// them together with the runtime.
	loadedPlugin.libraries = setup.closers
	loadedPlugin.HostImports = setup.imports
	wireGuestHooks(loadedPlugin)

	return loadedPlugin, rollback, nil
}

// registerByDBID indexes a plugin being loaded for its database id and
// returns the entry it displaces (a module already running for that id).
func (m *Manager) registerByDBID(plugin *LoadedPlugin) *LoadedPlugin {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	previous := m.byDBID[plugin.DBID]
	m.byDBID[plugin.DBID] = plugin

	return previous
}

// restoreByDBID undoes registerByDBID after a failed load: the displaced
// entry comes back, or the id is dropped when nothing was running.
func (m *Manager) restoreByDBID(dbID uint64, failed, previous *LoadedPlugin) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	if m.byDBID[dbID] != failed {
		return
	}

	if previous == nil {
		delete(m.byDBID, dbID)

		return
	}

	m.byDBID[dbID] = previous
}

// unregisterByDBID drops the index entry when it still points at plugin.
func (m *Manager) unregisterByDBID(plugin *LoadedPlugin) {
	if plugin.DBID == 0 {
		return
	}

	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	if m.byDBID[plugin.DBID] == plugin {
		delete(m.byDBID, plugin.DBID)
	}
}

// PluginByDBID reports the plugin registered for the database id, including
// one whose load is still in progress (Info is nil until GetInfo answered).
// A nil manager has no plugins.
func (m *Manager) PluginByDBID(dbID uint64) (*LoadedPlugin, bool) {
	if m == nil || dbID == 0 {
		return nil, false
	}

	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	plugin, ok := m.byDBID[dbID]

	return plugin, ok
}

// HostModules lists the host modules instantiated for the plugin.
func (m *Manager) HostModules(dbID uint64) ([]string, bool) {
	plugin, ok := m.PluginByDBID(dbID)
	if !ok {
		return nil, false
	}

	return slices.Clone(plugin.HostModules), true
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

// runtimeSetup is what initializeRuntime hands back: the started module, its
// runtime, the host functions it imports and the host modules instantiated
// for it.
type runtimeSetup struct {
	runtime     wazero.Runtime
	module      api.Module
	imports     []HostImport
	hostModules []string
	// closers are the per-plugin host libraries holding resources of this
	// load; the caller owns them once initializeRuntime returns.
	closers []HostLibraryCloser
}

// initializeRuntime builds the runtime, registers WASI and every host library,
// compiles and starts the module. Besides the module it returns the host
// functions the module imports (see LoadedPlugin.HostImports).
func (m *Manager) initializeRuntime(
	ctx context.Context,
	wasmBytes []byte,
	pluginID uint64,
	logs *guestLogs,
) (*runtimeSetup, error) {
	r := newWazeroRuntime(ctx, m.runtimeConfig())

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		closeRuntimeAfterFailure(ctx, r, "WASI instantiation", err)

		return nil, errors.Wrap(err, "failed to instantiate WASI")
	}

	// Instantiate the env module for AssemblyScript support
	envLib := &EnvHostLibrary{}
	if err := envLib.Instantiate(ctx, r); err != nil {
		closeRuntimeAfterFailure(ctx, r, "env module instantiation", err)

		return nil, errors.Wrap(err, "failed to instantiate env module")
	}

	// Host libraries register through the recording runtime (so the plugin
	// can be told which modules it has) and, for registered plugins, through
	// the observed one so every host function they export is counted.
	recorder := recordHostModules(r)

	var libraryRuntime wazero.Runtime = recorder
	if pluginID != 0 {
		libraryRuntime = observeHostCalls(recorder, m.config.Observer, pluginID)
	}

	// instantiateLibraries releases what it built itself when a later factory
	// fails; every failure below this point is ours to unwind.
	closers, err := m.instantiateLibraries(ctx, libraryRuntime, pluginID)
	if err != nil {
		closeRuntimeAfterFailure(ctx, r, "host library instantiation", err)

		return nil, err
	}

	code, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		closeRuntimeAfterFailure(ctx, r, "WASM module compilation", err)
		closeHostLibraries(ctx, closers)

		return nil, errors.Wrap(err, "failed to compile WASM module")
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
			closeHostLibraries(ctx, closers)

			return nil, errors.Wrapf(ErrUnexpectedExitCode, "exit code: %d", exitErr.ExitCode())
		} else if !errors.As(err, &exitErr) {
			closeRuntimeAfterFailure(ctx, r, "module instantiation", err)
			closeHostLibraries(ctx, closers)

			return nil, errors.Wrap(err, "failed to instantiate module")
		}
	}

	if err = m.verifyAPIVersion(startCtx, r, module); err != nil {
		closeRuntimeAfterFailure(ctx, r, "API version verification", err)
		closeHostLibraries(ctx, closers)

		return nil, err
	}

	return &runtimeSetup{
		runtime:     r,
		module:      module,
		imports:     imports,
		hostModules: recorder.Modules(),
		closers:     closers,
	}, nil
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

	slices.SortFunc(imports, func(a, b HostImport) int {
		return cmp.Or(
			cmp.Compare(a.Module, b.Module),
			cmp.Compare(a.Function, b.Function),
		)
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

func (m *Manager) instantiateLibraries(
	ctx context.Context,
	r wazero.Runtime,
	pluginID uint64,
) ([]HostLibraryCloser, error) {
	for _, lib := range m.config.Libraries {
		if err := lib.Instantiate(ctx, r); err != nil {
			return nil, errors.WithMessage(err, "failed to instantiate host library")
		}
	}

	var closers []HostLibraryCloser

	for _, factory := range m.config.LibraryFactories {
		lib := factory.Create(pluginID)
		closer, isCloser := lib.(HostLibraryCloser)

		if err := lib.Instantiate(ctx, r); err != nil {
			// Create and a partial Instantiate may already have allocated
			// plugin-scoped state, so this library is closed too, not only the
			// ones built before it. The libraries built so far already hold
			// resources for this plugin, and nothing else will ever close them.
			if isCloser {
				closeHostLibraries(ctx, []HostLibraryCloser{closer})
			}

			closeHostLibraries(ctx, closers)

			return nil, errors.WithMessage(err, "failed to instantiate factory host library")
		}

		if isCloser {
			closers = append(closers, closer)
		}
	}

	return closers, nil
}

// closeHostLibraries releases per-plugin host library resources. Failures are
// logged, never returned: they must not mask the reason the caller is
// unwinding.
func closeHostLibraries(ctx context.Context, closers []HostLibraryCloser) {
	for _, closer := range closers {
		if err := closer.Close(ctx); err != nil {
			slog.Warn("failed to close host library", slog.String("error", err.Error()))
		}
	}
}

// initializePlugin runs the guest's initialization sequence and fills the
// loaded plugin.
func (m *Manager) initializePlugin(
	ctx context.Context,
	r wazero.Runtime,
	plugin proto.PluginService,
	loaded *LoadedPlugin,
	logs *guestLogs,
) error {
	info, err := plugin.GetInfo(ctx, &proto.GetInfoRequest{})
	if err != nil {
		return errors.Wrap(err, "failed to get plugin info")
	}

	logs.SetPluginID(info.Id)

	initResp, err := plugin.Initialize(ctx, &proto.InitializeRequest{
		Context: &proto.PluginContext{PluginId: info.Id},
		Config:  loaded.Config,
	})
	if err != nil {
		return errors.Wrap(err, "plugin initialization failed")
	}

	if initResp.Result != nil && !initResp.Result.Success {
		errMsg := "unknown error"
		if initResp.Result.Error != nil {
			errMsg = *initResp.Result.Error
		}

		return errors.Wrapf(ErrInitializationFailed, "%s", errMsg)
	}

	httpRoutes, err := m.fetchAndValidateHTTPRoutes(ctx, plugin, info.Id)
	if err != nil {
		return errors.WithMessage(err, "failed to get HTTP routes")
	}

	loaded.Info = info
	loaded.Instance = plugin
	loaded.HTTPRoutes = httpRoutes
	loaded.runtime = r

	m.fetchOptionalDescriptors(ctx, plugin, loaded)

	return nil
}

// fetchOptionalDescriptors reads everything a plugin may but need not
// provide: frontend bundle, server abilities, protocols, assets and event
// subscriptions. A missing export is no failure.
func (m *Manager) fetchOptionalDescriptors(ctx context.Context, plugin proto.PluginService, loaded *LoadedPlugin) {
	pluginID := loaded.Info.Id

	bundleResp, err := plugin.GetFrontendBundle(ctx, &proto.GetFrontendBundleRequest{})
	if err != nil {
		slog.Debug("plugin has no frontend bundle",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)
	} else {
		if bundleResp.HasBundle && len(bundleResp.Bundle) > 0 {
			loaded.FrontendBundle = bundleResp.Bundle
		}
		if bundleResp.HasStyles && len(bundleResp.Styles) > 0 {
			loaded.FrontendStyles = bundleResp.Styles
		}
	}

	abilitiesResp, err := plugin.GetServerAbilities(ctx, &proto.GetServerAbilitiesRequest{})
	if err != nil {
		slog.Debug("plugin has no server abilities",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)
	} else if abilitiesResp != nil && len(abilitiesResp.Abilities) > 0 {
		loaded.ServerAbilities = abilitiesResp.Abilities
	}

	loaded.Protocol, loaded.RconProtocols, loaded.QueryProtocols = m.fetchProtocols(ctx, plugin, pluginID)

	loaded.I18nFS, loaded.FrontendFS = m.buildPluginAssets(ctx, plugin, pluginID)

	if resp, err := plugin.GetSubscribedEvents(ctx, &proto.GetSubscribedEventsRequest{}); err != nil {
		slog.Debug("failed to read plugin event subscriptions",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()),
		)
	} else if resp != nil {
		loaded.SubscribedEvents = resp.Events
	}
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
	handleExecCompleted := module.ExportedFunction("ssh_exec_events_handler_handle_exec_completed")

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
		handleexeccompleted:    handleExecCompleted,
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
	m.unregisterByDBID(plugin)

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

	m.stateMu.Lock()
	m.byDBID = make(map[uint64]*LoadedPlugin)
	m.stateMu.Unlock()

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
