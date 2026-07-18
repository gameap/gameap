package plugininstall

import (
	"context"
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

func BuildPluginRecord(
	dbID domain.Uint64ID,
	loaded *pkgplugin.LoadedPlugin,
	filename string,
	source string,
) *domain.Plugin {
	return &domain.Plugin{
		ID:          dbID,
		Name:        loaded.Info.Name,
		Version:     loaded.Info.Version,
		Description: loaded.Info.Description,
		Author:      loaded.Info.Author,
		APIVersion:  loaded.Info.ApiVersion,
		Filename:    new(filename),
		Source:      new(source),
		Status:      domain.PluginStatusActive,
		InstalledAt: new(time.Now()),
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

	loaded, err := loader.LoadWithID(ctx, filename, uint64(pluginRecord.ID))
	if err != nil {
		slog.ErrorContext(ctx, "failed to load plugin",
			slog.String("filename", filename),
			slog.String("error", err.Error()))

		pluginRecord.Status = domain.PluginStatusError
		_ = repo.Save(ctx, pluginRecord)

		return errors.WithMessage(err, "failed to load plugin")
	}

	loader.RegisterPluginID(pluginRecord.ID, loaded.Info.Id)

	return nil
}
