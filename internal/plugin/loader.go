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

func (l *Loader) LoadAll(ctx context.Context) error {
	if err := l.processAutoLoad(ctx); err != nil {
		return errors.WithMessage(err, "failed to process autoload plugins")
	}

	plugins, err := l.pluginRepo.Find(ctx,
		filters.FindPluginByStatuses(domain.PluginStatusActive),
		nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to get enabled plugins")
	}

	for _, plugin := range plugins {
		filename := l.resolvePluginFilename(&plugin)
		loaded, err := l.LoadWithID(ctx, filename, uint64(plugin.ID))
		if err != nil {
			return errors.WithMessagef(err, "failed to load plugin %s", plugin.Name)
		}

		l.mu.Lock()
		l.pluginIDs[plugin.ID] = loaded.Info.Id
		l.mu.Unlock()

		plugin.LastLoadedAt = new(time.Now())
		if err := l.pluginRepo.Save(ctx, &plugin); err != nil {
			slog.Warn("failed to update plugin last_loaded_at",
				slog.String("plugin", plugin.Name),
				slog.String("error", err.Error()))
		}
	}

	return nil
}

func (l *Loader) Load(ctx context.Context, filename string) (*pkgplugin.LoadedPlugin, error) {
	return l.LoadWithID(ctx, filename, 0)
}

func (l *Loader) LoadWithID(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
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

func (l *Loader) Unload(ctx context.Context, pluginID string) error {
	return l.manager.Unload(ctx, pluginID)
}

func (l *Loader) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	id, ok := l.pluginIDs[dbID]

	return id, ok
}

func (l *Loader) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for dbID, mgrID := range l.pluginIDs {
		if mgrID == managerID {
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

func (l *Loader) processAutoLoad(ctx context.Context) error {
	for _, filename := range l.autoLoadNames {
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

			continue
		}

		plugin := &domain.Plugin{
			ID:          pkgplugin.ParsePluginID(loaded.Info.Id),
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
	}

	return nil
}
