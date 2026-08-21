package plugininstall

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"

	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

var ErrPluginNameTaken = errors.New("another plugin is already installed under this name")

// FindInstalled returns the installed record for the plugin, or nil when the
// plugin is not installed. Callers distinguish install from update on the nil.
func FindInstalled(
	ctx context.Context,
	repo repositories.PluginRepository,
	dbID domain.Uint64ID,
) (*domain.Plugin, error) {
	plugins, err := repo.Find(ctx, filters.FindPluginByIDs(dbID), nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find installed plugin")
	}

	if len(plugins) == 0 {
		return nil, nil //nolint:nilnil // absence is not an error here
	}

	return &plugins[0], nil
}

// CheckNameAvailable guards the plugins.name unique index. Identity is the
// plugin ID, so two unrelated plugins sharing a manifest name pass the ID
// check and would only fail deep inside the repository with a driver error.
func CheckNameAvailable(
	ctx context.Context,
	repo repositories.PluginRepository,
	dbID domain.Uint64ID,
	name string,
) error {
	if name == "" {
		return nil
	}

	plugins, err := repo.Find(ctx, filters.FindPluginByNames(name), nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to check plugin name")
	}

	for i := range plugins {
		if plugins[i].ID != dbID {
			return api.WrapHTTPError(ErrPluginNameTaken, http.StatusConflict)
		}
	}

	return nil
}

// UnloadPlugin stops the running module so its file can be replaced: the
// manager refuses a second Load of the same ID with ErrPluginAlreadyLoaded.
// A plugin that is not loaded (errored at startup, or never active) is not an
// error — the update proceeds either way.
func UnloadPlugin(ctx context.Context, loader *plugin.Loader, dbID domain.Uint64ID) error {
	if loader == nil {
		return nil
	}

	managerID, ok := loader.GetPluginManagerID(dbID)
	if !ok {
		managerID = pkgplugin.CompactPluginID(dbID)
	}

	if err := loader.Unload(ctx, managerID); err != nil {
		if errors.Is(err, pkgplugin.ErrPluginNotFound) {
			return nil
		}

		return errors.WithMessage(err, "failed to unload plugin")
	}

	return nil
}

// ResolvePluginFilename keeps an updated plugin on the file it already uses.
// The two install paths disagree on the convention (uploads write
// "<dbID>.wasm", the store writes "<storeID>.wasm"), so writing the upload
// name over a store-installed plugin would leave the old file orphaned.
func ResolvePluginFilename(installed *domain.Plugin, dbID domain.Uint64ID) string {
	if installed != nil && installed.Filename != nil && *installed.Filename != "" {
		return *installed.Filename
	}

	return strconv.FormatUint(uint64(dbID), 10) + ".wasm"
}

// ApplyManifest copies what the uploaded build declares onto the record.
//
// Operator-managed fields are deliberately untouched: Source (a store plugin
// stays a store plugin, so it can still be updated from there), InstalledAt,
// Priority, Config, Category and Homepage.
//
// Grants only ever grow. RequiredPermissions mirrors the manifest, but
// AllowedPermissions is the union with what the plugin already holds: the
// files grant was handed to pre-existing installations by a migration
// (015_grant_files_permission_to_plugins) without appearing in any manifest,
// so replacing the list would silently revoke it. Widening is safe because it
// only happens on an explicit update, and the dry-run endpoint shows the new
// permissions before the operator confirms.
func ApplyManifest(record *domain.Plugin, loaded *pkgplugin.LoadedPlugin, filename string) {
	if record == nil || loaded == nil || loaded.Info == nil {
		return
	}

	required := domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)

	record.Name = loaded.Info.Name
	record.Version = loaded.Info.Version
	record.Description = loaded.Info.Description
	record.Author = loaded.Info.Author
	record.APIVersion = loaded.Info.ApiVersion
	record.RequiredPermissions = required
	record.AllowedPermissions = MergePermissions(record.AllowedPermissions, required)
	record.Filename = new(filename)
	record.Status = domain.PluginStatusActive
}

// MergePermissions returns the granted permissions plus everything the new
// build requires, preserving the order of the existing grants.
func MergePermissions(granted, required []domain.PluginPermission) []domain.PluginPermission {
	if len(granted) == 0 {
		return required
	}

	merged := slices.Clone(granted)
	for _, permission := range required {
		if !slices.Contains(merged, permission) {
			merged = append(merged, permission)
		}
	}

	return merged
}

// MissingPermissions lists what the uploaded build requires and the installed
// plugin has not been granted yet.
func MissingPermissions(installed *domain.Plugin, required []string) []string {
	if installed == nil || len(required) == 0 {
		return nil
	}

	var missing []string

	for _, value := range required {
		permission, ok := domain.ParsePluginPermission(value)
		if !ok {
			continue
		}

		if !slices.Contains(installed.AllowedPermissions, permission) {
			missing = append(missing, value)
		}
	}

	return missing
}
