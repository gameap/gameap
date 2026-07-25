package inmemory

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type pluginTaskKey struct {
	pluginID domain.Uint64ID
	name     string
}

type PluginScheduledTaskRepository struct {
	mu     sync.RWMutex
	tasks  map[pluginTaskKey]*domain.PluginScheduledTask
	nextID atomic.Uint64
}

func NewPluginScheduledTaskRepository() *PluginScheduledTaskRepository {
	return &PluginScheduledTaskRepository{
		tasks: make(map[pluginTaskKey]*domain.PluginScheduledTask),
	}
}

func (r *PluginScheduledTaskRepository) Upsert(_ context.Context, task *domain.PluginScheduledTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	task.UpdatedAt = &now

	key := pluginTaskKey{pluginID: task.PluginID, name: task.Name}

	if existing, ok := r.tasks[key]; ok {
		task.ID = existing.ID
		task.CreatedAt = existing.CreatedAt
	} else {
		task.ID = r.nextID.Add(1)
		if task.CreatedAt == nil || task.CreatedAt.IsZero() {
			task.CreatedAt = &now
		}
	}

	saved := *task
	r.tasks[key] = &saved

	return nil
}

func (r *PluginScheduledTaskRepository) Delete(
	_ context.Context,
	pluginID domain.Uint64ID,
	name string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tasks, pluginTaskKey{pluginID: pluginID, name: name})

	return nil
}

func (r *PluginScheduledTaskRepository) DeleteByPlugin(
	_ context.Context,
	pluginID domain.Uint64ID,
) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deleted := 0
	for key := range r.tasks {
		if key.pluginID == pluginID {
			delete(r.tasks, key)
			deleted++
		}
	}

	return deleted, nil
}

func (r *PluginScheduledTaskRepository) FindAll(_ context.Context) ([]domain.PluginScheduledTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tasks := make([]domain.PluginScheduledTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		tasks = append(tasks, *task)
	}

	sortPluginScheduledTasks(tasks)

	return tasks, nil
}

func (r *PluginScheduledTaskRepository) FindByPlugin(
	_ context.Context,
	pluginID domain.Uint64ID,
) ([]domain.PluginScheduledTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tasks []domain.PluginScheduledTask
	for key, task := range r.tasks {
		if key.pluginID == pluginID {
			tasks = append(tasks, *task)
		}
	}

	sortPluginScheduledTasks(tasks)

	return tasks, nil
}

func sortPluginScheduledTasks(tasks []domain.PluginScheduledTask) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].PluginID != tasks[j].PluginID {
			return tasks[i].PluginID < tasks[j].PluginID
		}

		return tasks[i].Name < tasks[j].Name
	})
}
