package pluginscheduler

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// Repository persists task registrations; the subset of
// repositories.PluginScheduledTaskRepository the scheduler consumes.
type Repository interface {
	Upsert(ctx context.Context, task *domain.PluginScheduledTask) error
	Delete(ctx context.Context, pluginID domain.Uint64ID, name string) error
	DeleteByPlugin(ctx context.Context, pluginID domain.Uint64ID) (int, error)
	FindAll(ctx context.Context) ([]domain.PluginScheduledTask, error)
	FindByPlugin(ctx context.Context, pluginID domain.Uint64ID) ([]domain.PluginScheduledTask, error)
}

// PluginProvider resolves a loaded plugin by its manager registry ID.
// Satisfied by *pkgplugin.Manager.
type PluginProvider interface {
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
}

// ManagerIDResolver maps a plugin DB ID to the ID it is registered under in
// the manager. Satisfied by *internalplugin.Loader.
type ManagerIDResolver interface {
	GetPluginManagerID(dbID domain.Uint64ID) (string, bool)
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
