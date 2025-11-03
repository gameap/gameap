package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/samber/lo"
)

type DaemonTaskRepository struct {
	mu     sync.RWMutex
	tasks  map[uint]*domain.DaemonTask
	nextID uint32

	dedicatedServerIDIndex map[uint]map[uint]struct{}                    // dedicatedServerID -> taskIDs
	serverIDIndex          map[uint]map[uint]struct{}                    // serverID -> taskIDs
	taskTypeIndex          map[domain.DaemonTaskType]map[uint]struct{}   // taskType -> taskIDs
	statusIndex            map[domain.DaemonTaskStatus]map[uint]struct{} // status -> taskIDs
}

func NewDaemonTaskRepository() *DaemonTaskRepository {
	return &DaemonTaskRepository{
		tasks:                  make(map[uint]*domain.DaemonTask),
		dedicatedServerIDIndex: make(map[uint]map[uint]struct{}),
		serverIDIndex:          make(map[uint]map[uint]struct{}),
		taskTypeIndex:          make(map[domain.DaemonTaskType]map[uint]struct{}),
		statusIndex:            make(map[domain.DaemonTaskStatus]map[uint]struct{}),
	}
}

func (r *DaemonTaskRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tasks := make([]domain.DaemonTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		tasks = append(tasks, *task)
	}

	r.sortTasks(tasks, order)

	return r.applyPagination(tasks, pagination), nil
}

func (r *DaemonTaskRepository) Find(
	_ context.Context,
	filter *filters.FindDaemonTask,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	candidateIDs := r.getFilteredTaskIDs(filter)

	tasks := make([]domain.DaemonTask, 0, len(candidateIDs))
	for taskID := range candidateIDs {
		if task, exists := r.tasks[taskID]; exists {
			tasks = append(tasks, *task)
		}
	}

	r.sortTasks(tasks, order)

	return r.applyPagination(tasks, pagination), nil
}

func (r *DaemonTaskRepository) FindWithOutput(
	ctx context.Context,
	filter *filters.FindDaemonTask,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return r.Find(ctx, filter, order, pagination)
}

func (r *DaemonTaskRepository) Save(_ context.Context, task *domain.DaemonTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if task.UpdatedAt == nil || task.UpdatedAt.IsZero() {
		task.UpdatedAt = lo.ToPtr(time.Now())
	}

	if task.ID == 0 && (task.CreatedAt == nil || task.CreatedAt.IsZero()) {
		task.CreatedAt = lo.ToPtr(time.Now())
	}

	if task.ID != 0 {
		if oldTask, exists := r.tasks[task.ID]; exists {
			r.removeFromIndexes(oldTask)
		}
	} else {
		task.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	r.tasks[task.ID] = &domain.DaemonTask{
		ID:                task.ID,
		RunAftID:          task.RunAftID,
		CreatedAt:         task.CreatedAt,
		UpdatedAt:         task.UpdatedAt,
		DedicatedServerID: task.DedicatedServerID,
		ServerID:          task.ServerID,
		Task:              task.Task,
		Data:              task.Data,
		Cmd:               task.Cmd,
		Output:            task.Output,
		Status:            task.Status,
	}

	r.addToIndexes(r.tasks[task.ID])

	return nil
}

func (r *DaemonTaskRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if task, exists := r.tasks[id]; exists {
		r.removeFromIndexes(task)
	}

	delete(r.tasks, id)

	return nil
}

func (r *DaemonTaskRepository) AppendOutput(_ context.Context, id uint, output string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	task, exists := r.tasks[id]
	if !exists {
		return nil
	}

	if task.Output == nil {
		task.Output = &output
	} else {
		newOutput := *task.Output + output
		task.Output = &newOutput
	}

	return nil
}

func (r *DaemonTaskRepository) Count(_ context.Context, filter *filters.FindDaemonTask) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resultIDs := r.getFilteredTaskIDs(filter)

	return len(resultIDs), nil
}

func (r *DaemonTaskRepository) Exists(_ context.Context, filter *filters.FindDaemonTask) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		return false, nil
	}

	// Use the existing filtering logic to get candidate tasks
	candidateIDs := r.getFilteredTaskIDs(filter)

	// If filtering by IDs, verify all IDs exist
	if len(filter.IDs) > 0 {
		return len(candidateIDs) == len(filter.IDs), nil
	}

	// For other filters, return true if at least one task exists
	return len(candidateIDs) > 0, nil
}

func (r *DaemonTaskRepository) addToIndexes(task *domain.DaemonTask) {
	// DedicatedServerID index
	if r.dedicatedServerIDIndex[task.DedicatedServerID] == nil {
		r.dedicatedServerIDIndex[task.DedicatedServerID] = make(map[uint]struct{})
	}
	r.dedicatedServerIDIndex[task.DedicatedServerID][task.ID] = struct{}{}

	// ServerID index
	if task.ServerID != nil {
		if r.serverIDIndex[*task.ServerID] == nil {
			r.serverIDIndex[*task.ServerID] = make(map[uint]struct{})
		}
		r.serverIDIndex[*task.ServerID][task.ID] = struct{}{}
	}

	// Task type index
	if r.taskTypeIndex[task.Task] == nil {
		r.taskTypeIndex[task.Task] = make(map[uint]struct{})
	}
	r.taskTypeIndex[task.Task][task.ID] = struct{}{}

	// Status index
	if r.statusIndex[task.Status] == nil {
		r.statusIndex[task.Status] = make(map[uint]struct{})
	}
	r.statusIndex[task.Status][task.ID] = struct{}{}
}

func (r *DaemonTaskRepository) removeFromIndexes(task *domain.DaemonTask) {
	// DedicatedServerID index
	if taskSet, exists := r.dedicatedServerIDIndex[task.DedicatedServerID]; exists {
		delete(taskSet, task.ID)
		if len(taskSet) == 0 {
			delete(r.dedicatedServerIDIndex, task.DedicatedServerID)
		}
	}

	// ServerID index
	if task.ServerID != nil {
		if taskSet, exists := r.serverIDIndex[*task.ServerID]; exists {
			delete(taskSet, task.ID)
			if len(taskSet) == 0 {
				delete(r.serverIDIndex, *task.ServerID)
			}
		}
	}

	// Task type index
	if taskSet, exists := r.taskTypeIndex[task.Task]; exists {
		delete(taskSet, task.ID)
		if len(taskSet) == 0 {
			delete(r.taskTypeIndex, task.Task)
		}
	}

	// Status index
	if taskSet, exists := r.statusIndex[task.Status]; exists {
		delete(taskSet, task.ID)
		if len(taskSet) == 0 {
			delete(r.statusIndex, task.Status)
		}
	}
}

//nolint:gocyclo,gocognit
func (r *DaemonTaskRepository) getFilteredTaskIDs(filter *filters.FindDaemonTask) map[uint]struct{} {
	resultIDs := make(map[uint]struct{})

	if filter == nil {
		// No filter, return all setting IDs
		for taskID := range r.tasks {
			resultIDs[taskID] = struct{}{}
		}

		return resultIDs
	}

	// Start with the first available filter result
	switch {
	case len(filter.IDs) > 0:
		for _, id := range filter.IDs {
			if _, exists := r.tasks[id]; exists {
				resultIDs[id] = struct{}{}
			}
		}
	case len(filter.DedicatedServerIDs) > 0:
		for _, dsID := range filter.DedicatedServerIDs {
			if taskSet, exists := r.dedicatedServerIDIndex[dsID]; exists {
				for taskID := range taskSet {
					resultIDs[taskID] = struct{}{}
				}
			}
		}
	case len(filter.ServerIDs) > 0:
		for _, serverID := range filter.ServerIDs {
			if serverID == nil {
				continue
			}
			if taskSet, exists := r.serverIDIndex[*serverID]; exists {
				for taskID := range taskSet {
					resultIDs[taskID] = struct{}{}
				}
			}
		}
	case len(filter.Tasks) > 0:
		for _, taskType := range filter.Tasks {
			if taskSet, exists := r.taskTypeIndex[taskType]; exists {
				for taskID := range taskSet {
					resultIDs[taskID] = struct{}{}
				}
			}
		}
	case len(filter.Statuses) > 0:
		for _, status := range filter.Statuses {
			if taskSet, exists := r.statusIndex[status]; exists {
				for taskID := range taskSet {
					resultIDs[taskID] = struct{}{}
				}
			}
		}
	default:
		// No filters, return all tasks
		for taskID := range r.tasks {
			resultIDs[taskID] = struct{}{}
		}
	}

	// Apply intersections for multiple filters
	if len(filter.DedicatedServerIDs) > 0 && len(filter.IDs) > 0 {
		r.intersectWithDedicatedServerIDs(resultIDs, filter.DedicatedServerIDs)
	}
	if len(filter.ServerIDs) > 0 && (len(filter.IDs) > 0 || len(filter.DedicatedServerIDs) > 0) {
		r.intersectWithServerIDs(resultIDs, filter.ServerIDs)
	}
	if len(filter.Tasks) > 0 && (len(filter.IDs) > 0 || len(filter.DedicatedServerIDs) > 0 || len(filter.ServerIDs) > 0) {
		r.intersectWithTasks(resultIDs, filter.Tasks)
	}
	if len(filter.Statuses) > 0 &&
		(len(filter.IDs) > 0 || len(filter.DedicatedServerIDs) > 0 || len(filter.ServerIDs) > 0 || len(filter.Tasks) > 0) {
		r.intersectWithStatuses(resultIDs, filter.Statuses)
	}

	return resultIDs
}

func (r *DaemonTaskRepository) intersectWithDedicatedServerIDs(resultIDs map[uint]struct{}, dsIDs []uint) {
	validIDs := make(map[uint]struct{})
	for _, dsID := range dsIDs {
		if taskSet, exists := r.dedicatedServerIDIndex[dsID]; exists {
			for taskID := range taskSet {
				if _, exists := resultIDs[taskID]; exists {
					validIDs[taskID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *DaemonTaskRepository) intersectWithServerIDs(resultIDs map[uint]struct{}, serverIDs []*uint) {
	validIDs := make(map[uint]struct{})
	for _, serverID := range serverIDs {
		if serverID == nil {
			continue
		}

		if taskSet, exists := r.serverIDIndex[*serverID]; exists {
			for taskID := range taskSet {
				if _, exists := resultIDs[taskID]; exists {
					validIDs[taskID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *DaemonTaskRepository) intersectWithTasks(resultIDs map[uint]struct{}, tasks []domain.DaemonTaskType) {
	validIDs := make(map[uint]struct{})
	for _, taskType := range tasks {
		if taskSet, exists := r.taskTypeIndex[taskType]; exists {
			for taskID := range taskSet {
				if _, exists := resultIDs[taskID]; exists {
					validIDs[taskID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *DaemonTaskRepository) intersectWithStatuses(resultIDs map[uint]struct{}, statuses []domain.DaemonTaskStatus) {
	validIDs := make(map[uint]struct{})
	for _, status := range statuses {
		if taskSet, exists := r.statusIndex[status]; exists {
			for taskID := range taskSet {
				if _, exists := resultIDs[taskID]; exists {
					validIDs[taskID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *DaemonTaskRepository) sortTasks(tasks []domain.DaemonTask, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].ID < tasks[j].ID
		})

		return
	}

	sort.Slice(tasks, func(i, j int) bool {
		for _, o := range order {
			cmpRes := r.compareTasks(&tasks[i], &tasks[j], o.Field)
			if cmpRes != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cmpRes > 0
				}

				return cmpRes < 0
			}
		}

		return false
	})
}

func (r *DaemonTaskRepository) compareTasks(a, b *domain.DaemonTask, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "dedicated_server_id", "node_id":
		return cmp.Compare(a.DedicatedServerID, b.DedicatedServerID)
	case "server_id":
		// Handle nullable comparison
		if a.ServerID == nil && b.ServerID == nil {
			return 0
		}
		if a.ServerID == nil {
			return -1
		}
		if b.ServerID == nil {
			return 1
		}

		return cmp.Compare(*a.ServerID, *b.ServerID)
	case "task":
		return strings.Compare(string(a.Task), string(b.Task))
	case "status":
		return strings.Compare(string(a.Status), string(b.Status))
	case "created_at":
		if a.CreatedAt == nil && b.CreatedAt == nil {
			return 0
		}
		if a.CreatedAt == nil {
			return -1
		}
		if b.CreatedAt == nil {
			return 1
		}
		if a.CreatedAt.Before(*b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(*b.CreatedAt) {
			return 1
		}

		return 0
	case "updated_at":
		if a.UpdatedAt == nil && b.UpdatedAt == nil {
			return 0
		}
		if a.UpdatedAt == nil {
			return -1
		}
		if b.UpdatedAt == nil {
			return 1
		}
		if a.UpdatedAt.Before(*b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.After(*b.UpdatedAt) {
			return 1
		}

		return 0
	default:
		return 0
	}
}

func (r *DaemonTaskRepository) applyPagination(
	tasks []domain.DaemonTask,
	pagination *filters.Pagination,
) []domain.DaemonTask {
	if pagination == nil {
		return tasks
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(tasks) {
		return []domain.DaemonTask{}
	}

	end := min(offset+limit, len(tasks))

	return tasks[offset:end]
}
