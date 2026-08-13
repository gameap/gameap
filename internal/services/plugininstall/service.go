package plugininstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin"
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

// Checksum returns the SHA-256 of a plugin file in the lowercase hex form the
// store publishes and the plugins.checksum column stores. It is computed from
// the bytes that were actually written rather than copied from store metadata,
// so the recorded value always describes the file on disk.
func Checksum(wasmBytes []byte) string {
	sum := sha256.Sum256(wasmBytes)

	return hex.EncodeToString(sum[:])
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

	return &domain.Plugin{
		ID:                  dbID,
		Name:                loaded.Info.Name,
		Version:             loaded.Info.Version,
		Description:         loaded.Info.Description,
		Author:              loaded.Info.Author,
		APIVersion:          loaded.Info.ApiVersion,
		Filename:            new(filename),
		Source:              new(source),
		Checksum:            new(Checksum(wasmBytes)),
		RequiredPermissions: permissions,
		AllowedPermissions:  permissions,
		Status:              domain.PluginStatusActive,
		InstalledAt:         new(time.Now()),
	}
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

// Change reasons carried on a sync hint. They only reach logs — the receiving
// instance re-reads the database and decides for itself.
const (
	ReasonInstall   = "install"
	ReasonUpdate    = "update"
	ReasonUninstall = "uninstall"
)

// AfterChange is the tail of every plugin lifecycle handler: it refreshes this
// instance's event subscriptions, drops what the reconciler recorded (so its
// next pass adopts the handler's work instead of redoing it) and nudges the
// other instances.
//
// Call it only once the database row and the plugin file are both final.
// Publishing earlier lets a fast peer act on a row that is about to change —
// on an uninstall it would re-download the file this handler is about to
// delete.
func AfterChange(
	ctx context.Context,
	refresher SubscriptionRefresher,
	notifier SyncNotifier,
	pluginID domain.Uint64ID,
	reason string,
) {
	RefreshSubscriptions(ctx, refresher)

	if notifier == nil {
		return
	}

	notifier.Forget(pluginID)
	notifier.Notify(context.WithoutCancel(ctx), pluginID, reason)
}

func TryLoadPlugin(
	ctx context.Context,
	loader *plugin.Loader,
	repo repositories.PluginRepository,
	pluginRecord *domain.Plugin,
	filename string,
) error {
	if loader == nil {
		return nil
	}

	// LoadWithID records the database ID to manager ID mapping itself, as one
	// critical section with the load.
	_, err := loader.LoadWithID(ctx, filename, uint64(pluginRecord.ID))
	if err != nil {
		slog.ErrorContext(ctx, "failed to load plugin",
			slog.String("filename", filename),
			slog.String("error", err.Error()))

		pluginRecord.Status = domain.PluginStatusError
		_ = repo.Save(ctx, pluginRecord)

		return errors.WithMessage(err, "failed to load plugin")
	}

	return nil
}
