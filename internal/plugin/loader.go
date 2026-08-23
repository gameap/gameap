package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"log/slog"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// reloadTimeout bounds one reload (unload, read the wasm, load, refresh
// subscriptions); module start and every guest call inside it have their own
// budgets, this is only the outer guard.
const reloadTimeout = 2 * time.Minute

// markErrorTimeout bounds recording a failed load once the reload's own
// context may already be gone.
const markErrorTimeout = 15 * time.Second

type LoaderOption func(*Loader)

// WithStrictLoad makes LoadAll fail when any plugin fails to load, which the
// application turns into a startup failure. Off by default: a broken plugin
// is marked with status "error" and the panel starts without it.
func WithStrictLoad(strict bool) LoaderOption {
	return func(l *Loader) {
		l.strict = strict
	}
}

// WithSubscriptionRefresher lets Reload rebuild event subscriptions so the
// new module instance receives events.
func WithSubscriptionRefresher(refresher SubscriptionRefresher) LoaderOption {
	return func(l *Loader) {
		l.refresher = refresher
	}
}

type Loader struct {
	manager       LoaderManager
	fileManager   files.FileManager
	pluginRepo    repositories.PluginRepository
	autoLoadNames []string
	pluginsDir    string
	strict        bool
	refresher     SubscriptionRefresher

	mu        sync.RWMutex
	pluginIDs map[domain.Uint64ID]string
	recovery  *Supervisor

	// lifecycle serializes unload/load pairs per plugin: an operator reload,
	// the recovery supervisor and an update must not interleave.
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

// loadRecord loads one installed plugin and persists the outcome on its row.
func (l *Loader) loadRecord(ctx context.Context, plugin *domain.Plugin) error {
	unlock := l.lockPlugin(plugin.ID)
	defer unlock()

	filename := l.resolvePluginFilename(plugin)

	loaded, err := l.loadWithID(ctx, filename, uint64(plugin.ID))
	if err != nil {
		slog.ErrorContext(ctx, "failed to load plugin",
			slog.Uint64("plugin_id", uint64(plugin.ID)),
			slog.String("name", plugin.Name),
			slog.String("filename", filename),
			slog.String("error", err.Error()))

		l.markError(ctx, plugin, err)

		return err
	}

	l.RegisterPluginID(plugin.ID, loaded.Info.Id)
	l.markActive(ctx, plugin)

	return nil
}

func (l *Loader) Load(ctx context.Context, filename string) (*pkgplugin.LoadedPlugin, error) {
	return l.loadWithID(ctx, filename, 0)
}

// LoadWithID loads the wasm file for the given database plugin ID. Callers
// that persist the outcome themselves (install flows) use it directly.
func (l *Loader) LoadWithID(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	if pluginID != 0 {
		unlock := l.lockPlugin(domain.Uint64ID(pluginID))
		defer unlock()
	}

	return l.loadWithID(ctx, filename, pluginID)
}

func (l *Loader) loadWithID(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	pluginPath := path.Join(l.pluginsDir, filename)

	if !l.fileManager.Exists(ctx, pluginPath) {
		return nil, errors.Errorf("plugin file not found: %s", pluginPath)
	}

	wasmBytes, err := l.fileManager.Read(ctx, pluginPath)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read plugin file")
	}

	loaded, err := l.manager.Load(ctx, wasmBytes, nil, pluginID)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load plugin")
	}

	wasmHash := sha256.Sum256(wasmBytes)

	attr := []slog.Attr{
		{Key: "id", Value: slog.StringValue(loaded.Info.Id)},
		{Key: "name", Value: slog.StringValue(loaded.Info.Name)},
		{Key: "version", Value: slog.StringValue(loaded.Info.Version)},
		{Key: "wasm_hash", Value: slog.StringValue(hex.EncodeToString(wasmHash[:]))},
		{Key: "description", Value: slog.StringValue(loaded.Info.Description)},
		{Key: "author", Value: slog.StringValue(loaded.Info.Author)},
		{Key: "api_version", Value: slog.StringValue(loaded.Info.ApiVersion)},
	}
	if len(loaded.FrontendBundle) > 0 {
		attr = append(attr, slog.Attr{Key: "frontend_bundle_size", Value: slog.IntValue(len(loaded.FrontendBundle))})
	}

	slog.LogAttrs(ctx, slog.LevelInfo, "plugin loaded", attr...)

	return loaded, nil
}

// Unload stops a plugin by its manager ID. Pending automatic recovery for
// the plugin is cancelled: the caller (uninstall, update) owns its lifecycle
// from here on.
func (l *Loader) Unload(ctx context.Context, pluginID string) error {
	if dbID, ok := l.GetDBPluginID(pluginID); ok {
		l.Forget(dbID)

		unlock := l.lockPlugin(dbID)
		defer unlock()
	}

	return l.manager.Unload(ctx, pluginID)
}

// Reload restarts an installed plugin on demand: the running instance is
// unloaded, the wasm file is loaded again and the database row records the
// outcome. Any pending automatic recovery is dropped first. Returns the
// updated row together with the loaded instance. The work is detached from
// the caller's cancellation: an operator closing the browser tab must not
// leave the plugin half reloaded.
func (l *Loader) Reload(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error) {
	l.Forget(dbID)

	return l.reload(context.WithoutCancel(ctx), dbID)
}

// reload honours the caller's context (the recovery supervisor cancels it on
// shutdown) and adds the outer deadline.
func (l *Loader) reload(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error) {
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

	err = l.manager.Unload(ctx, l.managerIDFor(dbID))
	if err != nil && !errors.Is(err, pkgplugin.ErrPluginNotFound) {
		return plugin, nil, errors.WithMessage(err, "failed to unload plugin")
	}

	loaded, err := l.loadWithID(ctx, l.resolvePluginFilename(plugin), uint64(dbID))
	if err != nil {
		// A cancelled reload (panel shutting down) is not the plugin's
		// failure; the row keeps its previous state. A reload that ran out
		// of time is one, and is recorded on a fresh context: the expired
		// one could not reach the database any more.
		if !errors.Is(ctx.Err(), context.Canceled) {
			markCtx, markCancel := context.WithTimeout(context.WithoutCancel(ctx), markErrorTimeout)
			l.markError(markCtx, plugin, err)
			markCancel()
		}

		return plugin, nil, err
	}

	l.RegisterPluginID(dbID, loaded.Info.Id)
	l.markActive(ctx, plugin)
	l.refreshSubscriptions(ctx)

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

	if err := l.pluginRepo.Save(ctx, plugin); err != nil {
		slog.WarnContext(ctx, "failed to record plugin load error",
			slog.Uint64("plugin_id", uint64(plugin.ID)),
			slog.String("plugin", plugin.Name),
			slog.String("error", err.Error()))
	}
}

func (l *Loader) markActive(ctx context.Context, plugin *domain.Plugin) {
	plugin.MarkActive(time.Now())

	if err := l.pluginRepo.Save(ctx, plugin); err != nil {
		slog.WarnContext(ctx, "failed to update plugin last_loaded_at",
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

func (l *Loader) resolvePluginFilename(plugin *domain.Plugin) string {
	if plugin.Filename != nil && *plugin.Filename != "" {
		return *plugin.Filename
	}

	return strconv.FormatUint(uint64(plugin.ID), 10) + ".wasm"
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

	plugin := &domain.Plugin{
		ID:          pluginID,
		Name:        loaded.Info.Name,
		Version:     loaded.Info.Version,
		Description: loaded.Info.Description,
		Author:      loaded.Info.Author,
		APIVersion:  loaded.Info.ApiVersion,
		Filename:    new(filename),
		Status:      domain.PluginStatusActive,
		InstalledAt: new(time.Now()),
	}

	if err := l.pluginRepo.Save(ctx, plugin); err != nil {
		return errors.WithMessage(err, "failed to save plugin to database")
	}

	return nil
}
