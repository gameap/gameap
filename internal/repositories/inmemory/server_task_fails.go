package inmemory

import (
	"cmp"
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/samber/lo"
)

type ServerTaskFailRepository struct {
	mu     sync.RWMutex
	fails  map[uint]*domain.ServerTaskFail
	nextID uint32

	// Hash indexes for efficient filtering
	serverTaskIDIndex map[uint]map[uint]struct{} // serverTaskID -> failIDs
}

func NewServerTaskFailRepository() *ServerTaskFailRepository {
	return &ServerTaskFailRepository{
		fails:             make(map[uint]*domain.ServerTaskFail),
		serverTaskIDIndex: make(map[uint]map[uint]struct{}),
	}
}

func (r *ServerTaskFailRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskFail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fails := make([]domain.ServerTaskFail, 0, len(r.fails))
	for _, fail := range r.fails {
		fails = append(fails, *fail)
	}

	r.sortFails(fails, order)

	return r.applyPagination(fails, pagination), nil
}

func (r *ServerTaskFailRepository) Find(
	_ context.Context,
	filter *filters.FindServerTaskFail,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskFail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	candidateIDs := r.getFilteredFailIDs(filter)

	fails := make([]domain.ServerTaskFail, 0, len(candidateIDs))
	for failID := range candidateIDs {
		if fail, exists := r.fails[failID]; exists {
			fails = append(fails, *fail)
		}
	}

	r.sortFails(fails, order)

	return r.applyPagination(fails, pagination), nil
}

func (r *ServerTaskFailRepository) Save(_ context.Context, fail *domain.ServerTaskFail) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if fail.UpdatedAt == nil || fail.UpdatedAt.IsZero() {
		fail.UpdatedAt = lo.ToPtr(time.Now())
	}

	if fail.ID == 0 && (fail.CreatedAt == nil || fail.CreatedAt.IsZero()) {
		fail.CreatedAt = lo.ToPtr(time.Now())
	}

	if fail.ID != 0 {
		if oldFail, exists := r.fails[fail.ID]; exists {
			r.removeFromIndexes(oldFail)
		}
	} else {
		fail.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	r.fails[fail.ID] = &domain.ServerTaskFail{
		ID:           fail.ID,
		ServerTaskID: fail.ServerTaskID,
		Output:       fail.Output,
		CreatedAt:    fail.CreatedAt,
		UpdatedAt:    fail.UpdatedAt,
	}

	r.addToIndexes(r.fails[fail.ID])

	return nil
}

func (r *ServerTaskFailRepository) addToIndexes(fail *domain.ServerTaskFail) {
	// ServerTaskID index
	if r.serverTaskIDIndex[fail.ServerTaskID] == nil {
		r.serverTaskIDIndex[fail.ServerTaskID] = make(map[uint]struct{})
	}
	r.serverTaskIDIndex[fail.ServerTaskID][fail.ID] = struct{}{}
}

func (r *ServerTaskFailRepository) removeFromIndexes(fail *domain.ServerTaskFail) {
	// ServerTaskID index
	if failSet, exists := r.serverTaskIDIndex[fail.ServerTaskID]; exists {
		delete(failSet, fail.ID)
		if len(failSet) == 0 {
			delete(r.serverTaskIDIndex, fail.ServerTaskID)
		}
	}
}

func (r *ServerTaskFailRepository) getFilteredFailIDs(filter *filters.FindServerTaskFail) map[uint]struct{} {
	resultIDs := make(map[uint]struct{})

	if filter == nil {
		// No filter, return all fail IDs
		for failID := range r.fails {
			resultIDs[failID] = struct{}{}
		}

		return resultIDs
	}

	// Start with the first available filter result
	switch {
	case len(filter.IDs) > 0:
		for _, id := range filter.IDs {
			if _, exists := r.fails[id]; exists {
				resultIDs[id] = struct{}{}
			}
		}
	case len(filter.ServerTaskIDs) > 0:
		for _, serverTaskID := range filter.ServerTaskIDs {
			if failSet, exists := r.serverTaskIDIndex[serverTaskID]; exists {
				for failID := range failSet {
					resultIDs[failID] = struct{}{}
				}
			}
		}
	default:
		// No filters, return all fails
		for failID := range r.fails {
			resultIDs[failID] = struct{}{}
		}
	}

	// Apply intersections for multiple filters
	if len(filter.ServerTaskIDs) > 0 && len(filter.IDs) > 0 {
		r.intersectWithServerTaskIDs(resultIDs, filter.ServerTaskIDs)
	}

	// Apply time-based filters
	if filter.CreatedAfter != nil || filter.CreatedBefore != nil {
		r.filterByCreatedAt(resultIDs, filter)
	}

	return resultIDs
}

func (r *ServerTaskFailRepository) intersectWithServerTaskIDs(
	resultIDs map[uint]struct{},
	serverTaskIDs []uint,
) {
	validIDs := make(map[uint]struct{})
	for _, serverTaskID := range serverTaskIDs {
		if failSet, exists := r.serverTaskIDIndex[serverTaskID]; exists {
			for failID := range failSet {
				if _, exists := resultIDs[failID]; exists {
					validIDs[failID] = struct{}{}
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

func (r *ServerTaskFailRepository) filterByCreatedAt(
	resultIDs map[uint]struct{},
	filter *filters.FindServerTaskFail,
) {
	for failID := range resultIDs {
		fail, exists := r.fails[failID]
		if !exists {
			delete(resultIDs, failID)

			continue
		}

		if fail.CreatedAt == nil {
			delete(resultIDs, failID)

			continue
		}

		// CreatedAfter means >= (greater than or equal)
		if filter.CreatedAfter != nil && fail.CreatedAt.Before(*filter.CreatedAfter) {
			delete(resultIDs, failID)

			continue
		}

		// CreatedBefore means <= (less than or equal)
		if filter.CreatedBefore != nil && fail.CreatedAt.After(*filter.CreatedBefore) {
			delete(resultIDs, failID)
		}
	}
}

func (r *ServerTaskFailRepository) sortFails(fails []domain.ServerTaskFail, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(fails, func(i, j int) bool {
			return fails[i].ID < fails[j].ID
		})

		return
	}

	sort.Slice(fails, func(i, j int) bool {
		for _, o := range order {
			cm := r.compareFails(&fails[i], &fails[j], o.Field)
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

func (r *ServerTaskFailRepository) compareFails(a, b *domain.ServerTaskFail, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "server_task_id":
		return cmp.Compare(a.ServerTaskID, b.ServerTaskID)
	case "output":
		return cmp.Compare(a.Output, b.Output)
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

func (r *ServerTaskFailRepository) applyPagination(
	fails []domain.ServerTaskFail,
	pagination *filters.Pagination,
) []domain.ServerTaskFail {
	if pagination == nil {
		return fails
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(fails) {
		return []domain.ServerTaskFail{}
	}

	end := min(offset+limit, len(fails))

	return fails[offset:end]
}
