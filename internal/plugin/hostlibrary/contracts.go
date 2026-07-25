package hostlibrary

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

// TaskScheduler is the scheduler surface the gameap-scheduler host library
// delegates to. Implementations must tolerate being called while the plugin
// manager lock is held: host functions run during guest Initialize/Shutdown.
type TaskScheduler interface {
	// AddTask registers or replaces (by PluginID+Name) a task definition.
	AddTask(ctx context.Context, task domain.PluginScheduledTask) error
	RemoveTask(ctx context.Context, pluginID domain.Uint64ID, name string) error
	ListTasks(ctx context.Context, pluginID domain.Uint64ID) ([]domain.PluginScheduledTask, error)
}
