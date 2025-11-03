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
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
)

type ServerTaskRepository struct {
	mu     sync.RWMutex
	tasks  map[uint]*domain.ServerTask
	nextID uint32

	// Hash indexes for efficient filtering
	serverIDIndex map[uint]map[uint]struct{}                     // serverID -> taskIDs
	commandIndex  map[domain.ServerTaskCommand]map[uint]struct{} // command -> taskIDs

	// Repository dependencies for filtering
	serverRepo repositories.ServerRepository
}

func NewServerTaskRepository(
	serverRepo repositories.ServerRepository,
) *ServerTaskRepository {
	return &ServerTaskRepository{
		tasks:         make(map[uint]*domain.ServerTask),
		serverIDIndex: make(map[uint]map[uint]struct{}),
		commandIndex:  make(map[domain.ServerTaskCommand]map[uint]struct{}),

		serverRepo: serverRepo,
	}
}

func (r *ServerTaskRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tasks := make([]domain.ServerTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		tasks = append(tasks, *task)
	}

	r.sortTasks(tasks, order)

	return r.applyPagination(tasks, pagination), nil
}

func (r *ServerTaskRepository) Find(
	_ context.Context,
	filter *filters.FindServerTask,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	candidateIDs := r.getFilteredTaskIDs(filter)

	tasks := make([]domain.ServerTask, 0, len(candidateIDs))
	for taskID := range candidateIDs {
		if task, exists := r.tasks[taskID]; exists {
			tasks = append(tasks, *task)
		}
	}

	r.sortTasks(tasks, order)

	return r.applyPagination(tasks, pagination), nil
}

func (r *ServerTaskRepository) Save(_ context.Context, task *domain.ServerTask) error {
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

	r.tasks[task.ID] = &domain.ServerTask{
		ID:           task.ID,
		Command:      task.Command,
		ServerID:     task.ServerID,
		Repeat:       task.Repeat,
		RepeatPeriod: task.RepeatPeriod,
		Counter:      task.Counter,
		ExecuteDate:  task.ExecuteDate,
		Payload:      task.Payload,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}

	r.addToIndexes(r.tasks[task.ID])

	return nil
}

func (r *ServerTaskRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if task, exists := r.tasks[id]; exists {
		r.removeFromIndexes(task)
	}

	delete(r.tasks, id)

	return nil
}

func (r *ServerTaskRepository) addToIndexes(task *domain.ServerTask) {
	// ServerID index
	if r.serverIDIndex[task.ServerID] == nil {
		r.serverIDIndex[task.ServerID] = make(map[uint]struct{})
	}
	r.serverIDIndex[task.ServerID][task.ID] = struct{}{}

	// Command index
	if r.commandIndex[task.Command] == nil {
		r.commandIndex[task.Command] = make(map[uint]struct{})
	}
	r.commandIndex[task.Command][task.ID] = struct{}{}
}

func (r *ServerTaskRepository) removeFromIndexes(task *domain.ServerTask) {
	// ServerID index
	if taskSet, exists := r.serverIDIndex[task.ServerID]; exists {
		delete(taskSet, task.ID)
		if len(taskSet) == 0 {
			delete(r.serverIDIndex, task.ServerID)
		}
	}

	// Command index
	if taskSet, exists := r.commandIndex[task.Command]; exists {
		delete(taskSet, task.ID)
		if len(taskSet) == 0 {
			delete(r.commandIndex, task.Command)
		}
	}
}

//nolint:gocognit,gocyclo
func (r *ServerTaskRepository) getFilteredTaskIDs(filter *filters.FindServerTask) map[uint]struct{} {
	resultIDs := make(map[uint]struct{})

	if filter == nil {
		// No filter, return all task IDs
		for taskID := range r.tasks {
			resultIDs[taskID] = struct{}{}
		}

		return resultIDs
	}

	// Convert NodeIDs to ServerIDs if serverRepo is available
	var serverIDsFromNodes []uint
	nodeFilterActive := len(filter.NodeIDs) > 0
	if nodeFilterActive && r.serverRepo != nil {
		serverIDsFromNodes = r.getServerIDsForNodes(filter.NodeIDs)
	}

	// If NodeIDs filter is active but no servers found, return empty result
	if nodeFilterActive && len(serverIDsFromNodes) == 0 {
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
	case len(filter.ServersIDs) > 0:
		for _, serverID := range filter.ServersIDs {
			if taskSet, exists := r.serverIDIndex[serverID]; exists {
				for taskID := range taskSet {
					resultIDs[taskID] = struct{}{}
				}
			}
		}
	case len(serverIDsFromNodes) > 0:
		// Use server IDs from node filtering as primary filter
		for _, serverID := range serverIDsFromNodes {
			if taskSet, exists := r.serverIDIndex[serverID]; exists {
				for taskID := range taskSet {
					resultIDs[taskID] = struct{}{}
				}
			}
		}
	case len(filter.Commands) > 0:
		for _, command := range filter.Commands {
			if taskSet, exists := r.commandIndex[command]; exists {
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
	if len(filter.ServersIDs) > 0 && len(filter.IDs) > 0 {
		r.intersectWithServerIDs(resultIDs, filter.ServersIDs)
	}
	if len(serverIDsFromNodes) > 0 && (len(filter.IDs) > 0 || len(filter.ServersIDs) > 0) {
		r.intersectWithServerIDs(resultIDs, serverIDsFromNodes)
	}
	if len(filter.Commands) > 0 && (len(filter.IDs) > 0 || len(filter.ServersIDs) > 0 || len(serverIDsFromNodes) > 0) {
		r.intersectWithCommands(resultIDs, filter.Commands)
	}

	return resultIDs
}

// getServerIDsForNodes returns all server IDs that belong to the specified nodes.
func (r *ServerTaskRepository) getServerIDsForNodes(nodeIDs []uint) []uint {
	if r.serverRepo == nil {
		return nil
	}

	servers, err := r.serverRepo.Find(context.TODO(), filters.FindServerByNodeIDs(nodeIDs...), nil, nil)
	if err != nil {
		return nil
	}

	serverIDs := make([]uint, 0, len(servers))
	for _, server := range servers {
		serverIDs = append(serverIDs, server.ID)
	}

	return serverIDs
}

func (r *ServerTaskRepository) intersectWithServerIDs(resultIDs map[uint]struct{}, serverIDs []uint) {
	validIDs := make(map[uint]struct{})
	for _, serverID := range serverIDs {
		if taskSet, exists := r.serverIDIndex[serverID]; exists {
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

func (r *ServerTaskRepository) intersectWithCommands(
	resultIDs map[uint]struct{},
	commands []domain.ServerTaskCommand,
) {
	validIDs := make(map[uint]struct{})
	for _, command := range commands {
		if taskSet, exists := r.commandIndex[command]; exists {
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

func (r *ServerTaskRepository) sortTasks(tasks []domain.ServerTask, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].ID < tasks[j].ID
		})

		return
	}

	sort.Slice(tasks, func(i, j int) bool {
		for _, o := range order {
			cm := r.compareTasks(&tasks[i], &tasks[j], o.Field)
			if cm != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cm > 0
				}

				return cm < 0
			}
		}

		return false
	})
}

func (r *ServerTaskRepository) compareTasks(a, b *domain.ServerTask, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "command":
		return strings.Compare(string(a.Command), string(b.Command))
	case "server_id":
		return cmp.Compare(a.ServerID, b.ServerID)
	case "repeat":
		return cmp.Compare(a.Repeat, b.Repeat)
	case "repeat_period":
		return cmp.Compare(a.RepeatPeriod, b.RepeatPeriod)
	case "counter":
		return cmp.Compare(a.Counter, b.Counter)
	case "execute_date":
		if a.ExecuteDate.Before(b.ExecuteDate) {
			return -1
		}
		if a.ExecuteDate.After(b.ExecuteDate) {
			return 1
		}

		return 0
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

func (r *ServerTaskRepository) applyPagination(
	tasks []domain.ServerTask,
	pagination *filters.Pagination,
) []domain.ServerTask {
	if pagination == nil {
		return tasks
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(tasks) {
		return []domain.ServerTask{}
	}

	end := min(offset+limit, len(tasks))

	return tasks[offset:end]
}
