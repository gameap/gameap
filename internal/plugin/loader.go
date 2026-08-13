package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type LoaderManager interface {
	Load(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	LoadTransient(
		ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64,
	) (*pkgplugin.LoadedPlugin, error)
	Register(loadedPlugin *pkgplugin.LoadedPlugin) error
	Replace(loadedPlugin *pkgplugin.LoadedPlugin) (*pkgplugin.LoadedPlugin, error)
	ShutdownPlugin(ctx context.Context, loadedPlugin *pkgplugin.LoadedPlugin) error
	Unload(ctx context.Context, pluginID string) error
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
	GetPlugins() []*pkgplugin.LoadedPlugin
	Shutdown(ctx context.Context) error
}

type Loader struct {
	manager       LoaderManager
	fileManager   files.FileManager
	pluginRepo    repositories.PluginRepository
	autoLoadNames []string
	pluginsDir    string

	// applyMu serialises everything that changes manager membership. Both the
	// admin HTTP handlers and the background reconciler mutate the same
	// registry, and a load is only complete once its ID mapping is recorded —
	// without this a reconcile pass could observe a plugin that is loaded but
	// not yet mapped and mistake it for a foreign module.
	//
	// Lock order is always applyMu then the manager's own lock. Nothing under
	// the manager lock may call back into the Loader, which is why no host
	// library is allowed to reach the plugin services.
	applyMu sync.Mutex

	mu        sync.RWMutex
	pluginIDs map[domain.Uint64ID]string
}

func NewLoader(
	manager LoaderManager,
	fileManager files.FileManager,
	pluginRepo repositories.PluginRepository,
	autoLoadNames []string,
	pluginsDir string,
) *Loader {
	return &Loader{
		manager:       manager,
		fileManager:   fileManager,
		pluginRepo:    pluginRepo,
		autoLoadNames: autoLoadNames,
		pluginsDir:    pluginsDir,
		pluginIDs:     make(map[domain.Uint64ID]string),
	}
}

// LoadAll loads every plugin the database marks active. A plugin that cannot be
// loaded is logged and skipped rather than aborting the run: one broken module
// must neither hide the plugins listed after it nor keep the panel from
// starting. Whatever fails here is retried by the reconciler.
func (l *Loader) LoadAll(ctx context.Context) error {
	l.processAutoLoad(ctx)

	plugins, err := l.pluginRepo.Find(ctx,
		filters.FindPluginByStatuses(domain.PluginStatusActive),
		nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to get enabled plugins")
	}

	for _, plugin := range plugins {
		filename := ResolveFilename(&plugin)

		if _, err := l.LoadWithID(ctx, filename, uint64(plugin.ID)); err != nil {
			slog.ErrorContext(ctx, "failed to load plugin, it will be retried in the background",
				slog.String("plugin", plugin.Name),
				slog.String("filename", filename),
				slog.String("error", err.Error()))

			continue
		}

		l.touchLastLoaded(ctx, plugin.ID, plugin.Name)
	}

	return nil
}

func (l *Loader) Load(ctx context.Context, filename string) (*pkgplugin.LoadedPlugin, error) {
	return l.LoadWithID(ctx, filename, 0)
}

// LoadWithID loads a plugin file and registers it, recording the mapping from
// the database ID to the ID the manager registered it under. The two steps are
// one critical section so no observer can catch the plugin in between.
func (l *Loader) LoadWithID(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	l.applyMu.Lock()
	defer l.applyMu.Unlock()

	wasmBytes, err := l.readPluginFile(ctx, filename)
	if err != nil {
		return nil, err
	}

	loaded, err := l.manager.Load(ctx, wasmBytes, nil, pluginID)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load plugin")
	}

	if pluginID != 0 {
		l.RegisterPluginID(domain.Uint64ID(pluginID), loaded.Info.Id)
	}

	logLoaded(ctx, loaded, wasmBytes, "plugin loaded")

	return loaded, nil
}

// Reload swaps a running plugin for a freshly built one. The new module is
// compiled and initialised before the old one is displaced, so the plugin is
// missing from the registry for a single map assignment instead of for the
// whole build. The displaced module is shut down afterwards; a failure there is
// logged, because the replacement is already serving.
func (l *Loader) Reload(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	l.applyMu.Lock()
	defer l.applyMu.Unlock()

	wasmBytes, err := l.readPluginFile(ctx, filename)
	if err != nil {
		return nil, err
	}

	loaded, err := l.manager.LoadTransient(ctx, wasmBytes, nil, pluginID)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load plugin")
	}

	previous, err := l.manager.Replace(loaded)
	if err != nil {
		if closeErr := loaded.Close(ctx); closeErr != nil {
			slog.WarnContext(ctx, "failed to close plugin runtime after failed replace",
				slog.String("plugin_id", loaded.Info.Id),
				slog.String("error", closeErr.Error()))
		}

		return nil, errors.WithMessage(err, "failed to replace plugin")
	}

	if pluginID != 0 {
		l.RegisterPluginID(domain.Uint64ID(pluginID), loaded.Info.Id)
	}

	if previous != nil {
		if err := l.manager.ShutdownPlugin(ctx, previous); err != nil {
			slog.WarnContext(ctx, "failed to shut down replaced plugin",
				slog.String("plugin_id", previous.Info.Id),
				slog.String("error", err.Error()))
		}
	}

	logLoaded(ctx, loaded, wasmBytes, "plugin reloaded")

	return loaded, nil
}

// Unload removes a plugin from the manager and drops its ID mapping. The
// mapping is dropped even when the manager reports a failure: it describes a
// registration that is gone either way, and a stale entry silently misroutes
// scheduled tasks and archive callbacks.
func (l *Loader) Unload(ctx context.Context, pluginID string) error {
	l.applyMu.Lock()
	defer l.applyMu.Unlock()

	err := l.manager.Unload(ctx, pluginID)

	l.unregisterManagerID(pkgplugin.NormalizePluginID(pluginID))

	return err
}

func (l *Loader) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	id, ok := l.pluginIDs[dbID]

	return id, ok
}

func (l *Loader) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	normalized := pkgplugin.NormalizePluginID(managerID)

	l.mu.RLock()
	defer l.mu.RUnlock()
	for dbID, mgrID := range l.pluginIDs {
		if mgrID == normalized {
			return dbID, true
		}
	}

	return 0, false
}

// RegisterPluginID records the manager ID a database ID is loaded under. The ID
// is normalized so lookups keyed on what GetPlugins reports match entries
// written from a plugin's self-declared manifest ID.
func (l *Loader) RegisterPluginID(dbID domain.Uint64ID, managerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pluginIDs[dbID] = pkgplugin.NormalizePluginID(managerID)
}

func (l *Loader) UnregisterPluginID(dbID domain.Uint64ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.pluginIDs, dbID)
}

func (l *Loader) unregisterManagerID(normalizedID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for dbID, mgrID := range l.pluginIDs {
		if mgrID == normalizedID {
			delete(l.pluginIDs, dbID)
		}
	}
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

func (l *Loader) touchLastLoaded(ctx context.Context, dbID domain.Uint64ID, name string) {
	if err := l.pluginRepo.TouchLastLoaded(ctx, dbID, time.Now()); err != nil {
		slog.WarnContext(ctx, "failed to update plugin last_loaded_at",
			slog.String("plugin", name),
			slog.String("error", err.Error()))
	}
}

func logLoaded(ctx context.Context, loaded *pkgplugin.LoadedPlugin, wasmBytes []byte, message string) {
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

	slog.LogAttrs(ctx, slog.LevelInfo, message, attr...)
}

// ResolveFilename returns the plugin file name recorded on the row, falling
// back to the decimal ID for rows installed before the column existed.
func ResolveFilename(plugin *domain.Plugin) string {
	if plugin.Filename != nil && *plugin.Filename != "" {
		return *plugin.Filename
	}

	return strconv.FormatUint(uint64(plugin.ID), 10) + ".wasm"
}

// processAutoLoad registers the plugins named by PLUGINS_AUTOLOAD in the
// database so the regular active-plugin path picks them up. A file that cannot
// be read or inspected is logged and skipped: an operator's stale autoload
// entry must not keep the rest of the plugins from starting.
func (l *Loader) processAutoLoad(ctx context.Context) {
	for _, filename := range l.autoLoadNames {
		if err := l.processAutoLoadFile(ctx, filename); err != nil {
			slog.ErrorContext(ctx, "failed to register autoload plugin",
				slog.String("filename", filename),
				slog.String("error", err.Error()))
		}
	}
}

func (l *Loader) processAutoLoadFile(ctx context.Context, filename string) error {
	wasmBytes, err := l.readPluginFile(ctx, filename)
	if err != nil {
		return err
	}

	loaded, err := l.manager.LoadTransient(ctx, wasmBytes, nil, 0)
	if err != nil {
		return errors.WithMessage(err, "failed to load plugin for info")
	}

	pluginID := pkgplugin.ParsePluginID(loaded.Info.Id)

	if err := loaded.Close(ctx); err != nil {
		slog.WarnContext(ctx, "failed to close temporary plugin",
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
