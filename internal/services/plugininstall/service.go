package plugininstall

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"

	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

var ErrPluginAlreadyInstalled = errors.New("plugin already installed")

func CheckNotInstalled(ctx context.Context, repo repositories.PluginRepository, dbID domain.Uint64ID) error {
	exists, err := repo.Exists(ctx, filters.FindPluginByIDs(dbID))
	if err != nil {
		return errors.WithMessage(err, "failed to check if plugin exists")
	}
	if exists {
		return api.WrapHTTPError(ErrPluginAlreadyInstalled, http.StatusConflict)
	}

	return nil
}

func BuildPluginRecord(
	dbID domain.Uint64ID,
	loaded *pkgplugin.LoadedPlugin,
	filename string,
	source string,
	wasmBytes []byte,
) *domain.Plugin {
	// Installing is an admin-only action and the dry-run endpoint shows the
	// manifest's required_permissions beforehand, so confirming the install
	// is the grant. AllowedPermissions stays the runtime source of truth and
	// can be narrowed later without touching the manifest.
	permissions := domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)

	record := &domain.Plugin{
		ID:                  dbID,
		Name:                loaded.Info.Name,
		Version:             loaded.Info.Version,
		Description:         loaded.Info.Description,
		Author:              loaded.Info.Author,
		APIVersion:          loaded.Info.ApiVersion,
		Filename:            new(filename),
		Source:              new(source),
		Checksum:            new(plugin.FileChecksum(wasmBytes)),
		RequiredPermissions: permissions,
		AllowedPermissions:  permissions,
		Status:              domain.PluginStatusActive,
		InstalledAt:         new(time.Now()),
	}

	pluginconfig.SchemaFromManifest(record, loaded.Info)

	return record
}

// RefreshSubscriptions refreshes plugin event subscriptions after a runtime
// install/uninstall/update. Detached from the request context so it survives
// the response being written; failure only logs — the plugin operation itself
// has already succeeded, events just lag until the next refresh.
func RefreshSubscriptions(ctx context.Context, refresher SubscriptionRefresher) {
	if refresher == nil {
		return
	}

	if err := refresher.RefreshSubscriptions(context.WithoutCancel(ctx)); err != nil {
		slog.WarnContext(ctx, "failed to refresh plugin subscriptions",
			slog.String("error", err.Error()))
	}
}

// TryLoadPlugin loads the freshly installed module and records the outcome
// on its row (the loader marks it active or in error). The loaded plugin is
// returned so callers that built the database record without a manifest (the
// store install path) can read PluginInfo from it.
func TryLoadPlugin(
	ctx context.Context,
	loader *plugin.Loader,
	pluginRecord *domain.Plugin,
) (*pkgplugin.LoadedPlugin, error) {
	if loader == nil {
		return nil, nil
	}

	loaded, err := loader.LoadRecord(ctx, pluginRecord)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load plugin")
	}

	return loaded, nil
}

// Sync actions reported to the other panel instances after a plugin row
// changed (plugininstall.SyncNotifier).
const (
	ActionInstall     = "install"
	ActionUpdate      = "update"
	ActionUninstall   = "uninstall"
	ActionReload      = "reload"
	ActionPermissions = "permissions"
	ActionConfig      = "config"
)

// SyncNotifier wakes the other panel instances so they reconcile their
// plugin runtime against the database; satisfied by *pluginsync.Service. The
// hint carries no state, so a nil notifier (single instance, sync disabled)
// costs nothing.
type SyncNotifier interface {
	Notify(ctx context.Context, pluginID domain.Uint64ID, action string)
}

// AfterChange performs the runtime follow-up of a plugin row change:
// rebuilding the local event subscriptions and hinting the other instances.
// Detached from the request context so it survives the response being
// written.
func AfterChange(
	ctx context.Context,
	refresher SubscriptionRefresher,
	notifier SyncNotifier,
	pluginID domain.Uint64ID,
	action string,
) {
	RefreshSubscriptions(ctx, refresher)
	Notify(ctx, notifier, pluginID, action)
}

// Notify hints the other instances; nil-safe.
func Notify(ctx context.Context, notifier SyncNotifier, pluginID domain.Uint64ID, action string) {
	if notifier == nil {
		return
	}

	notifier.Notify(context.WithoutCancel(ctx), pluginID, action)
}
