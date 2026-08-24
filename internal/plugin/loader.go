package plugin

import (
	"context"
	stderrors "errors"
	"log/slog"
	"path"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

// reloadTimeout bounds one reload (unload, read the wasm, load, refresh
// subscriptions); module start and every guest call inside it have their own
// budgets, this is only the outer guard.
const reloadTimeout = 2 * time.Minute

// markErrorTimeout bounds recording a failed load once the reload's own
// context may already be gone.
const markErrorTimeout = 15 * time.Second

// Triggers recorded on plugin lifecycle events (extra_data "trigger").
const (
	TriggerInstall   = "install"
	TriggerReload    = "reload"
	TriggerRecovery  = "recovery"
	TriggerSync      = "sync"
	TriggerUnload    = "unload"
	TriggerUninstall = "uninstall"
	TriggerStartup   = "startup"
)

type LoaderOption func(*Loader)

// WithStrictLoad makes LoadAll fail when any plugin fails to load, which the
// application turns into a startup failure. Off by default: a broken plugin
// is marked with status "error" and the panel starts without it.
func WithStrictLoad(strict bool) LoaderOption {
	return func(l *Loader) {
		l.strict = strict
	}
}

// WithPermissionEnforcement tells the loader whether the panel applies the
// recorded grants (PLUGIN_PERMISSIONS_ENFORCE), which only changes what the
// missing-permissions warning promises. Off by default, like the variable.
func WithPermissionEnforcement(enforced bool) LoaderOption {
	return func(l *Loader) {
		l.enforcePermissions = enforced
	}
}

// WithSubscriptionRefresher lets Reload rebuild event subscriptions so the
// new module instance receives events.
func WithSubscriptionRefresher(refresher SubscriptionRefresher) LoaderOption {
	return func(l *Loader) {
		l.refresher = refresher
	}
}

// WithLifecycleEvents publishes PLUGIN_LOADED / PLUGIN_UNLOADED /
// PLUGIN_ERROR events to the other plugins.
func WithLifecycleEvents(events LifecycleEvents) LoaderOption {
	return func(l *Loader) {
		l.events = events
	}
}

// RuntimeState is what this instance knows about one installed plugin: whether
// a module runs for it, the fingerprint of the row that module was built
// from, and the fingerprint of the last load attempt (success or failure).
type RuntimeState struct {
	Present     bool
	Enabled     bool
	Fingerprint string
	Attempted   string
}

// applyOptions tunes one pass of apply.
type applyOptions struct {
	// force replaces a running module even when its fingerprint matches.
	force bool
	// persist writes the outcome (status, last error, generation, schema)
	// to the row; the reconciler leaves the shared row alone.
	persist bool
	// startup suppresses the per-plugin subscription refresh and the loaded
	// event: LoadAll refreshes once afterwards and no subscriber exists yet.
	startup bool
	trigger string
}

type Loader struct {
	manager       LoaderManager
	fileManager   files.FileManager
	pluginRepo    repositories.PluginRepository
	autoLoadNames []string
	pluginsDir    string
	strict        bool
	refresher     SubscriptionRefresher
	events        LifecycleEvents

	enforcePermissions bool

	mu           sync.RWMutex
	pluginIDs    map[domain.Uint64ID]string
	fingerprints map[domain.Uint64ID]string
	attempts     map[domain.Uint64ID]string
	holds        map[domain.Uint64ID]int
	recovery     *Supervisor

	// lifecycle serializes unload/load pairs per plugin: an operator reload,
	// the recovery supervisor, the reconciler and an update must not
	// interleave.
	lifecycleMu sync.Mutex
	lifecycle   map[domain.Uint64ID]*sync.Mutex
}

func NewLoader(
	manager LoaderManager,
	fileManager files.FileManager,
	pluginRepo repositories.PluginRepository,
	autoLoadNames []string,
	pluginsDir string,
	opts ...LoaderOption,
) *Loader {
	loader := &Loader{
		manager:       manager,
		fileManager:   fileManager,
		pluginRepo:    pluginRepo,
		autoLoadNames: autoLoadNames,
		pluginsDir:    pluginsDir,
		pluginIDs:     make(map[domain.Uint64ID]string),
		fingerprints:  make(map[domain.Uint64ID]string),
		attempts:      make(map[domain.Uint64ID]string),
		holds:         make(map[domain.Uint64ID]int),
		lifecycle:     make(map[domain.Uint64ID]*sync.Mutex),
	}

	for _, opt := range opts {
		opt(loader)
	}

	return loader
}

// LoadAll loads every installed plugin that should run: status "active" and
// status "error" (the last attempt failed, the cause may be gone). Plugins
// the operator disabled stay off. A plugin that fails is recorded as such
// and skipped; only in strict mode does LoadAll report the failures, which
// makes the panel refuse to start. A database failure is always reported.
func (l *Loader) LoadAll(ctx context.Context) error {
	var failures []error

	if err := l.processAutoLoad(ctx); err != nil {
		failures = append(failures, errors.WithMessage(err, "failed to process autoload plugins"))
	}

	plugins, err := l.pluginRepo.Find(ctx,
		filters.FindPluginByStatuses(domain.PluginStatusActive, domain.PluginStatusError),
		nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to get enabled plugins")
	}

	for i := range plugins {
		if err := l.loadRecord(ctx, &plugins[i]); err != nil {
			failures = append(failures, errors.WithMessagef(err, "failed to load plugin %s", plugins[i].Name))
		}
	}

	if len(failures) == 0 {
		return nil
	}

	slog.WarnContext(ctx, "some plugins failed to load",
		slog.Int("failed", len(failures)),
		slog.Bool("strict", l.strict))

	if l.strict {
		return stderrors.Join(failures...)
	}

	return nil
}

// loadRecord loads one installed plugin at startup and persists the outcome.
func (l *Loader) loadRecord(ctx context.Context, plugin *domain.Plugin) error {
	unlock := l.lockPlugin(plugin.ID)
	defer unlock()

	_, _, err := l.apply(ctx, plugin, applyOptions{persist: true, startup: true, trigger: TriggerStartup})

	return err
}

// LoadRecord loads a freshly installed or updated plugin and persists the
// outcome on its row. A module already running for the row (the reconciler
// may have loaded it first) is adopted when it was built from the same row.
func (l *Loader) LoadRecord(ctx context.Context, plugin *domain.Plugin) (*pkgplugin.LoadedPlugin, error) {
	unlock := l.lockPlugin(plugin.ID)
	defer unlock()

	loaded, _, err := l.apply(ctx, plugin, applyOptions{persist: true, trigger: TriggerInstall})

	return loaded, err
}

// ApplyRecord makes the runtime match the row without writing the row back:
// loads an absent module, replaces one built from a different row or
// disabled at runtime, leaves an up-to-date one alone. It is what the
// multi-instance reconciler calls; a plugin held by a handler (update,
// uninstall, configuration) answers ErrPluginHeld.
func (l *Loader) ApplyRecord(ctx context.Context, plugin *domain.Plugin) (bool, error) {
	unlock := l.lockPlugin(plugin.ID)
	defer unlock()

	if l.isHeld(plugin.ID) {
		return false, ErrPluginHeld
	}

	ctx, cancel := context.WithTimeout(ctx, reloadTimeout)
	defer cancel()

	_, changed, err := l.apply(ctx, plugin, applyOptions{trigger: TriggerSync})

	return changed, err
}

// UnloadRecord stops the module running for the row, if any, and drops what
// this instance remembers about it. The row is left alone; trigger tells the
// other plugins why (TriggerSync, TriggerUninstall).
func (l *Loader) UnloadRecord(ctx context.Context, dbID domain.Uint64ID, trigger string) (bool, error) {
	l.Forget(dbID)

	unlock := l.lockPlugin(dbID)
	defer unlock()

	managerID := l.managerIDFor(dbID)

	running, present := l.manager.GetPlugin(managerID)
	if !present {
		l.forgetRuntime(dbID)

		return false, nil
	}

	err := l.manager.Unload(ctx, managerID)
	if err != nil && !errors.Is(err, pkgplugin.ErrPluginNotFound) {
		return false, errors.WithMessage(err, "failed to unload plugin")
	}

	l.forgetRuntime(dbID)

	l.refreshSubscriptions(ctx)
	l.emitUnloaded(ctx, dbID, running.Info, trigger)

	return true, nil
}

// Hold keeps the reconciler from loading the plugin while a handler runs a
// multi-step operation on it (unload, replace the file, save the row, load).
// Nested holds stack; the returned func releases one.
func (l *Loader) Hold(dbID domain.Uint64ID) func() {
	l.mu.Lock()
	l.holds[dbID]++
	l.mu.Unlock()

	var once sync.Once

	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()

			if l.holds[dbID] <= 1 {
				delete(l.holds, dbID)

				return
			}

			l.holds[dbID]--
		})
	}
}

func (l *Loader) isHeld(dbID domain.Uint64ID) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.holds[dbID] > 0
}

// RuntimeState reports what this instance runs for the plugin.
func (l *Loader) RuntimeState(dbID domain.Uint64ID) RuntimeState {
	if l == nil {
		return RuntimeState{}
	}

	running, present := l.manager.GetPlugin(l.managerIDFor(dbID))

	l.mu.RLock()
	defer l.mu.RUnlock()

	return RuntimeState{
		Present:     present,
		Enabled:     present && running.IsEnabled(),
		Fingerprint: l.fingerprints[dbID],
		Attempted:   l.attempts[dbID],
	}
}

// apply is the one place a module is built for a row. The caller holds the
// plugin's lifecycle lock.
func (l *Loader) apply(
	ctx context.Context,
	plugin *domain.Plugin,
	opts applyOptions,
) (*pkgplugin.LoadedPlugin, bool, error) {
	fingerprint := Fingerprint(plugin)
	managerID := l.managerIDFor(plugin.ID)

	running, present := l.manager.GetPlugin(managerID)
	if present && !opts.force && running.IsEnabled() && l.loadedFingerprint(plugin.ID) == fingerprint {
		if opts.persist {
			l.markActive(ctx, plugin)
		}

		return running, false, nil
	}

	// A forced reload always asks the manager to unload: it is the source of
	// truth about what runs, and a module it does not know is no failure.
	// Replacing the module cancels any pending automatic recovery, except
	// when the recovery itself is what reloads (its attempt series goes on).
	if present || opts.force {
		if opts.trigger != TriggerRecovery {
			l.Forget(plugin.ID)
		}

		err := l.manager.Unload(ctx, managerID)
		if err != nil && !errors.Is(err, pkgplugin.ErrPluginNotFound) {
			return nil, false, errors.WithMessage(err, "failed to unload plugin")
		}

		l.forgetRuntime(plugin.ID)
	}

	loaded, err := l.loadModule(ctx, plugin)

	l.recordAttempt(plugin.ID, fingerprint)

	if err != nil {
		l.logLoadFailure(ctx, plugin, err)

		// A cancelled load (panel shutting down) is not the plugin's
		// failure; the row keeps its previous state. A load that ran out
		// of time is one, and is recorded on a fresh context: the expired
		// one could not reach the database any more.
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, false, err
		}

		if opts.persist {
			markCtx, markCancel := context.WithTimeout(context.WithoutCancel(ctx), markErrorTimeout)
			l.markError(markCtx, plugin, err)
			markCancel()
		}

		l.emitPluginEvent(ctx, proto.EventType_EVENT_TYPE_PLUGIN_ERROR, plugin, nil, opts.trigger, err)

		return nil, false, err
	}

	l.RegisterPluginID(plugin.ID, loaded.Info.Id)
	l.recordFingerprint(plugin.ID, fingerprint)

	if opts.persist {
		l.markActive(ctx, plugin)
	}

	l.warnMissingPermissions(ctx, plugin, loaded)

	if opts.startup {
		return loaded, true, nil
	}

	l.refreshSubscriptions(ctx)
	l.emitPluginEvent(ctx, proto.EventType_EVENT_TYPE_PLUGIN_LOADED, plugin, loaded.Info, opts.trigger, nil)

	return loaded, true, nil
}

// loadModule reads the wasm file and builds the module for the row.
func (l *Loader) loadModule(ctx context.Context, plugin *domain.Plugin) (*pkgplugin.LoadedPlugin, error) {
	wasmBytes, err := l.readPluginFile(ctx, ResolveFilename(plugin))
	if err != nil {
		return nil, err
	}

	loaded, err := l.manager.Load(ctx, wasmBytes, nil, uint64(plugin.ID))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load plugin")
	}

	logLoaded(ctx, loaded, wasmBytes)

	return loaded, nil
}

func (l *Loader) logLoadFailure(ctx context.Context, plugin *domain.Plugin, err error) {
	slog.ErrorContext(ctx, "failed to load plugin",
		slog.Uint64("plugin_id", uint64(plugin.ID)),
		slog.String("name", plugin.Name),
		slog.String("filename", ResolveFilename(plugin)),
		slog.String("error", err.Error()))
}

// warnMissingPermissions points out, once per load, the host functions the
// module imports without holding the grant that gates them. While the panel
// enforces permissions every such call is refused at runtime; without
// enforcement the calls pass, and the warning tells the operator what to
// grant before that changes. Event subscriptions are checked by the
// dispatcher when it refreshes them.
func (l *Loader) warnMissingPermissions(ctx context.Context, plugin *domain.Plugin, loaded *pkgplugin.LoadedPlugin) {
	missing := MissingPermissions(UsedPermissions(loaded.HostImports, nil), plugin.AllowedPermissions)
	if len(missing) == 0 {
		return
	}

	message := "plugin imports host functions it is not granted; those calls will be refused"
	if !l.enforcePermissions {
		message = "plugin imports host functions it is not granted; the calls are allowed for now, " +
			"but will be refused once PLUGIN_PERMISSIONS_ENFORCE is enabled"
	}

	slog.WarnContext(ctx, message,
		slog.Uint64("plugin_id", uint64(plugin.ID)),
		slog.String("name", plugin.Name),
		slog.Any("missing_permissions", PermissionNames(missing)))
}

// Load loads a wasm file that has no database record (no grants); kept for
// callers that inspect a module by file name.
func (l *Loader) Load(ctx context.Context, filename string) (*pkgplugin.LoadedPlugin, error) {
	wasmBytes, err := l.readPluginFile(ctx, filename)
	if err != nil {
		return nil, err
	}

	loaded, err := l.manager.Load(ctx, wasmBytes, nil, 0)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load plugin")
	}

	logLoaded(ctx, loaded, wasmBytes)

	return loaded, nil
}

func (l *Loader) readPluginFile(ctx context.Context, filename string) ([]byte, error) {
	pluginPath := path.Join(l.pluginsDir, filename)

	if !l.fileManager.Exists(ctx, pluginPath) {
		return nil, errors.Errorf("plugin file not found: %s", pluginPath)
	}

	wasmBytes, err := l.fileManager.Read(ctx, pluginPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read plugin file")
	}

	return wasmBytes, nil
}

func logLoaded(ctx context.Context, loaded *pkgplugin.LoadedPlugin, wasmBytes []byte) {
	attr := []slog.Attr{
		{Key: "id", Value: slog.StringValue(loaded.Info.Id)},
		{Key: "name", Value: slog.StringValue(loaded.Info.Name)},
		{Key: "version", Value: slog.StringValue(loaded.Info.Version)},
		{Key: "wasm_hash", Value: slog.StringValue(FileChecksum(wasmBytes))},
		{Key: "description", Value: slog.StringValue(loaded.Info.Description)},
		{Key: "author", Value: slog.StringValue(loaded.Info.Author)},
		{Key: "api_version", Value: slog.StringValue(loaded.Info.ApiVersion)},
	}
	if len(loaded.FrontendBundle) > 0 {
		attr = append(attr, slog.Attr{Key: "frontend_bundle_size", Value: slog.IntValue(len(loaded.FrontendBundle))})
	}

	slog.LogAttrs(ctx, slog.LevelInfo, "plugin loaded", attr...)
}

// Unload stops a plugin by its manager ID. Pending automatic recovery for
// the plugin is cancelled: the caller (uninstall, update) owns its lifecycle
// from here on.
func (l *Loader) Unload(ctx context.Context, pluginID string) error {
	dbID, known := l.GetDBPluginID(pluginID)
	if known {
		l.Forget(dbID)

		unlock := l.lockPlugin(dbID)
		defer unlock()
	}

	running, present := l.manager.GetPlugin(pluginID)

	err := l.manager.Unload(ctx, pluginID)
	// A module the manager does not know is gone either way. Any other
	// failure leaves it registered there, so this instance keeps its mapping
	// and the next pass retries the unload instead of losing track of a
	// module that is still loaded.
	if err != nil && !errors.Is(err, pkgplugin.ErrPluginNotFound) {
		return err
	}

	if known {
		l.forgetRuntime(dbID)
	}

	if err != nil {
		return err
	}

	if known && present {
		l.emitUnloaded(ctx, dbID, running.Info, TriggerUnload)
	}

	return nil
}

// Reload restarts an installed plugin on demand: the running instance is
// unloaded, the wasm file is loaded again and the database row records the
// outcome. Any pending automatic recovery is dropped first, and the row's
// generation is bumped so every other panel instance restarts the module
// too. Returns the updated row together with the loaded instance. The work
// is detached from the caller's cancellation: an operator closing the
// browser tab must not leave the plugin half reloaded.
func (l *Loader) Reload(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error) {
	l.Forget(dbID)

	return l.reload(context.WithoutCancel(ctx), dbID, TriggerReload, true)
}

// reload honours the caller's context (the recovery supervisor cancels it on
// shutdown) and adds the outer deadline.
func (l *Loader) reload(
	ctx context.Context,
	dbID domain.Uint64ID,
	trigger string,
	bumpGeneration bool,
) (*domain.Plugin, *pkgplugin.LoadedPlugin, error) {
	unlock := l.lockPlugin(dbID)
	defer unlock()

	ctx, cancel := context.WithTimeout(ctx, reloadTimeout)
	defer cancel()

	plugin, err := l.findPlugin(ctx, dbID)
	if err != nil {
		return nil, nil, err
	}

	switch plugin.Status {
	case domain.PluginStatusDisabled:
		return plugin, nil, ErrPluginDisabled
	case domain.PluginStatusUpdating:
		return plugin, nil, ErrPluginUpdating
	case domain.PluginStatusActive, domain.PluginStatusError:
	}

	if bumpGeneration {
		plugin.Generation++
	}

	loaded, _, err := l.apply(ctx, plugin, applyOptions{force: true, persist: true, trigger: trigger})
	if err != nil {
		return plugin, nil, err
	}

	return plugin, loaded, nil
}

// Forget drops any pending automatic recovery of the plugin.
func (l *Loader) Forget(dbID domain.Uint64ID) {
	if l == nil {
		return
	}

	l.mu.RLock()
	recovery := l.recovery
	l.mu.RUnlock()

	if recovery != nil {
		recovery.Forget(dbID)
	}
}

func (l *Loader) setRecovery(recovery *Supervisor) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.recovery = recovery
}

func (l *Loader) findPlugin(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, error) {
	plugins, err := l.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return nil, ErrPluginNotInstalled
	}

	return &plugins[0], nil
}

// managerIDFor resolves the manager key of a plugin loaded on this instance,
// falling back to the compact form of its database ID.
func (l *Loader) managerIDFor(dbID domain.Uint64ID) string {
	if managerID, ok := l.GetPluginManagerID(dbID); ok {
		return managerID
	}

	return pkgplugin.CompactPluginID(dbID)
}

func (l *Loader) markError(ctx context.Context, plugin *domain.Plugin, loadErr error) {
	plugin.MarkError(LoadErrorText(loadErr), time.Now())
	l.saveLoadState(ctx, plugin, "failed to record plugin load error")
}

func (l *Loader) markActive(ctx context.Context, plugin *domain.Plugin) {
	plugin.MarkActive(time.Now())
	l.saveLoadState(ctx, plugin, "failed to update plugin load state")
}

// saveLoadState persists only the load outcome columns, so a load finishing
// seconds after the row was read never overwrites a concurrent edit of the
// configuration or the grants.
func (l *Loader) saveLoadState(ctx context.Context, plugin *domain.Plugin, message string) {
	if err := l.pluginRepo.UpdateLoadState(ctx, plugin.ID, plugin.LoadState()); err != nil {
		slog.WarnContext(ctx, message,
			slog.Uint64("plugin_id", uint64(plugin.ID)),
			slog.String("plugin", plugin.Name),
			slog.String("error", err.Error()))
	}
}

func (l *Loader) refreshSubscriptions(ctx context.Context) {
	if l.refresher == nil {
		return
	}

	if err := l.refresher.RefreshSubscriptions(ctx); err != nil {
		slog.WarnContext(ctx, "failed to refresh plugin subscriptions after reload",
			slog.String("error", err.Error()))
	}
}

func (l *Loader) lockPlugin(dbID domain.Uint64ID) func() {
	l.lifecycleMu.Lock()
	mu, ok := l.lifecycle[dbID]
	if !ok {
		mu = &sync.Mutex{}
		l.lifecycle[dbID] = mu
	}
	l.lifecycleMu.Unlock()

	mu.Lock()

	return mu.Unlock
}

func (l *Loader) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	if l == nil {
		return "", false
	}

	l.mu.RLock()
	defer l.mu.RUnlock()
	id, ok := l.pluginIDs[dbID]

	return id, ok
}

// GetDBPluginID resolves the database ID of a loaded plugin from its manager
// ID in any accepted form (raw info ID, compact or decimal).
func (l *Loader) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	if l == nil {
		return 0, false
	}

	wanted := pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(managerID))

	l.mu.RLock()
	defer l.mu.RUnlock()
	for dbID, mgrID := range l.pluginIDs {
		if mgrID == managerID || pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(mgrID)) == wanted {
			return dbID, true
		}
	}

	return 0, false
}

func (l *Loader) RegisterPluginID(dbID domain.Uint64ID, managerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pluginIDs[dbID] = managerID
}

func (l *Loader) recordFingerprint(dbID domain.Uint64ID, fingerprint string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fingerprints[dbID] = fingerprint
}

func (l *Loader) recordAttempt(dbID domain.Uint64ID, fingerprint string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[dbID] = fingerprint
}

func (l *Loader) loadedFingerprint(dbID domain.Uint64ID) string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.fingerprints[dbID]
}

// forgetRuntime drops the ID mapping and the fingerprint of a module that is
// gone; a stale entry would misroute scheduled tasks and archive callbacks.
func (l *Loader) forgetRuntime(dbID domain.Uint64ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.pluginIDs, dbID)
	delete(l.fingerprints, dbID)
}

// processAutoLoad registers the plugins named in PLUGINS_AUTOLOAD in the
// database so the regular load pass picks them up. Every entry is attempted;
// the failures are joined into one error.
func (l *Loader) processAutoLoad(ctx context.Context) error {
	var failures []error

	for _, filename := range l.autoLoadNames {
		if err := l.registerAutoLoad(ctx, filename); err != nil {
			slog.ErrorContext(ctx, "autoload plugin failed",
				slog.String("file", filename),
				slog.String("error", err.Error()))

			failures = append(failures, errors.WithMessagef(err, "autoload %s", filename))
		}
	}

	return stderrors.Join(failures...)
}

func (l *Loader) registerAutoLoad(ctx context.Context, filename string) error {
	pluginPath := path.Join(l.pluginsDir, filename)
	if !l.fileManager.Exists(ctx, pluginPath) {
		return errors.Errorf("autoload plugin file not found: %s", pluginPath)
	}

	wasmBytes, err := l.fileManager.Read(ctx, pluginPath)
	if err != nil {
		return errors.WithMessage(err, "failed to read plugin file")
	}

	loaded, err := l.manager.LoadTransient(ctx, wasmBytes, nil, 0)
	if err != nil {
		return errors.WithMessagef(err, "failed to load plugin for info %s", filename)
	}

	pluginID := pkgplugin.ParsePluginID(loaded.Info.Id)

	if err := loaded.Close(ctx); err != nil {
		slog.Warn("failed to close temporary plugin",
			slog.String("plugin", loaded.Info.Id),
			slog.String("error", err.Error()))
	}

	existing, err := l.pluginRepo.Find(ctx,
		filters.FindPluginByIDs(pluginID), nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to check existing plugin")
	}

	if len(existing) > 0 {
		if existing[0].Status != domain.PluginStatusActive {
			existing[0].Status = domain.PluginStatusActive
			if err := l.pluginRepo.Save(ctx, &existing[0]); err != nil {
				return errors.WithMessage(err, "failed to activate plugin")
			}
		}

		return nil
	}

	// Autoload is the operator's own decision (a file they placed and named
	// in PLUGINS_AUTOLOAD), so the declared permissions are granted exactly
	// as an upload install would. The checksum is recorded once, at row
	// creation: autoload files are per instance, and rewriting it on every
	// restart would make the reload fingerprint flap across instances.
	permissions := domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)

	plugin := &domain.Plugin{
		ID:                  pluginID,
		Name:                loaded.Info.Name,
		Version:             loaded.Info.Version,
		Description:         loaded.Info.Description,
		Author:              loaded.Info.Author,
		APIVersion:          loaded.Info.ApiVersion,
		Filename:            new(filename),
		Source:              new("file://" + filename),
		Checksum:            new(FileChecksum(wasmBytes)),
		RequiredPermissions: permissions,
		AllowedPermissions:  permissions,
		Status:              domain.PluginStatusActive,
		InstalledAt:         new(time.Now()),
	}

	if err := l.pluginRepo.Save(ctx, plugin); err != nil {
		return errors.WithMessage(err, "failed to save plugin to database")
	}

	return nil
}
