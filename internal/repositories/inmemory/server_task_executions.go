package inmemory

import (
	"context"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
)

type ServerTaskExecutionRepository struct {
	mu     sync.RWMutex
	execs  map[string]*domain.ServerTaskExecution
	nextID atomic.Uint32
}

func NewServerTaskExecutionRepository() *ServerTaskExecutionRepository {
	return &ServerTaskExecutionRepository{
		execs: make(map[string]*domain.ServerTaskExecution),
	}
}

func (r *ServerTaskExecutionRepository) Create(
	_ context.Context, exec *domain.ServerTaskExecution,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.execs[exec.ExecutionID]; exists {
		return nil
	}

	now := time.Now()
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = now
	}
	if exec.UpdatedAt.IsZero() {
		exec.UpdatedAt = now
	}
	if exec.Status == "" {
		exec.Status = domain.ServerTaskExecutionStatusRunning
	}

	exec.ID = uint(r.nextID.Add(1))

	stored := *exec
	r.execs[exec.ExecutionID] = &stored

	return nil
}

func (r *ServerTaskExecutionRepository) UpdateFinish(
	_ context.Context,
	executionID string,
	patch repositories.ServerTaskExecutionFinishPatch,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	exec, ok := r.execs[executionID]
	if !ok {
		return nil
	}

	exec.Status = patch.Status
	if patch.ExitCode != nil {
		exec.ExitCode = patch.ExitCode
	}
	if patch.ErrorMessage != nil {
		exec.ErrorMessage = patch.ErrorMessage
	}
	finishedAt := patch.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	exec.FinishedAt = &finishedAt
	if patch.DurationMS != nil {
		exec.DurationMS = patch.DurationMS
	}
	if patch.OutputInline != nil {
		exec.OutputInline = patch.OutputInline
	}
	if patch.OutputStoragePath != nil {
		exec.OutputStoragePath = patch.OutputStoragePath
	}
	exec.UpdatedAt = finishedAt

	return nil
}

func (r *ServerTaskExecutionRepository) AppendOutputInline(
	_ context.Context, executionID string, chunk []byte, maxBytes int,
) error {
	if len(chunk) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	exec, ok := r.execs[executionID]
	if !ok {
		return nil
	}

	combined := ""
	if exec.OutputInline != nil {
		combined = *exec.OutputInline
	}
	combined += string(chunk)
	if maxBytes > 0 && len(combined) > maxBytes {
		combined = combined[len(combined)-maxBytes:]
	}
	exec.OutputInline = &combined
	exec.UpdatedAt = time.Now()

	return nil
}

func (r *ServerTaskExecutionRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	execs := make([]domain.ServerTaskExecution, 0, len(r.execs))
	for _, exec := range r.execs {
		execs = append(execs, *exec)
	}

	r.sortExecs(execs, order)

	return r.applyPagination(execs, pagination), nil
}

func (r *ServerTaskExecutionRepository) Find(
	_ context.Context,
	filter *filters.FindServerTaskExecution,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	execs := make([]domain.ServerTaskExecution, 0, len(r.execs))
	for _, exec := range r.execs {
		if !r.matches(exec, filter) {
			continue
		}
		execs = append(execs, *exec)
	}

	r.sortExecs(execs, order)

	return r.applyPagination(execs, pagination), nil
}

func (r *ServerTaskExecutionRepository) MarkAbandoned(
	_ context.Context,
	nodeID uint,
	keepExecutionIDs []string,
	reason string,
) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	keep := make(map[string]struct{}, len(keepExecutionIDs))
	for _, id := range keepExecutionIDs {
		keep[id] = struct{}{}
	}

	now := time.Now()
	count := 0
	reasonCopy := reason

	for _, exec := range r.execs {
		if exec.NodeID != nodeID {
			continue
		}
		if exec.Status != domain.ServerTaskExecutionStatusRunning {
			continue
		}
		if _, found := keep[exec.ExecutionID]; found {
			continue
		}

		exec.Status = domain.ServerTaskExecutionStatusFailed
		exec.ErrorMessage = &reasonCopy
		finished := now
		exec.FinishedAt = &finished
		exec.UpdatedAt = now
		count++
	}

	return count, nil
}

func (r *ServerTaskExecutionRepository) DeleteOlderThan(
	_ context.Context,
	status domain.ServerTaskExecutionStatus,
	before time.Time,
) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for execID, exec := range r.execs {
		if exec.Status != status {
			continue
		}
		if exec.FinishedAt == nil || !exec.FinishedAt.Before(before) {
			continue
		}
		delete(r.execs, execID)
		count++
	}

	return count, nil
}

func (r *ServerTaskExecutionRepository) matches(
	exec *domain.ServerTaskExecution, filter *filters.FindServerTaskExecution,
) bool {
	if filter == nil {
		return true
	}

	if len(filter.IDs) > 0 && !containsUint(filter.IDs, exec.ID) {
		return false
	}
	if len(filter.ExecutionIDs) > 0 && !containsString(filter.ExecutionIDs, exec.ExecutionID) {
		return false
	}
	if len(filter.ServerTaskIDs) > 0 && !containsUint(filter.ServerTaskIDs, exec.ServerTaskID) {
		return false
	}
	if len(filter.ServerIDs) > 0 && !containsUint(filter.ServerIDs, exec.ServerID) {
		return false
	}
	if len(filter.NodeIDs) > 0 && !containsUint(filter.NodeIDs, exec.NodeID) {
		return false
	}
	if len(filter.Statuses) > 0 && !containsStatus(filter.Statuses, exec.Status) {
		return false
	}
	if len(filter.NotStatuses) > 0 && containsStatus(filter.NotStatuses, exec.Status) {
		return false
	}
	if filter.StartedAfter != nil && exec.StartedAt.Before(*filter.StartedAfter) {
		return false
	}
	if filter.StartedBefore != nil && !exec.StartedAt.Before(*filter.StartedBefore) {
		return false
	}

	return true
}

func containsUint(xs []uint, x uint) bool {
	return slices.Contains(xs, x)
}

func containsString(xs []string, x string) bool {
	return slices.Contains(xs, x)
}

func containsStatus(xs []domain.ServerTaskExecutionStatus, x domain.ServerTaskExecutionStatus) bool {
	return slices.Contains(xs, x)
}

func (r *ServerTaskExecutionRepository) sortExecs(
	execs []domain.ServerTaskExecution, order []filters.Sorting,
) {
	if len(order) == 0 {
		sort.Slice(execs, func(i, j int) bool {
			return execs[i].StartedAt.After(execs[j].StartedAt)
		})

		return
	}

	sort.Slice(execs, func(i, j int) bool {
		for _, o := range order {
			c := compareExecField(&execs[i], &execs[j], o.Field)
			if c != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return c > 0
				}

				return c < 0
			}
		}

		return false
	})
}

func compareExecField(a, b *domain.ServerTaskExecution, field string) int {
	switch field {
	case "id":
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}

		return 0
	case "started_at":
		if a.StartedAt.Before(b.StartedAt) {
			return -1
		}
		if a.StartedAt.After(b.StartedAt) {
			return 1
		}

		return 0
	case "status":
		if a.Status < b.Status {
			return -1
		}
		if a.Status > b.Status {
			return 1
		}

		return 0
	default:
		return 0
	}
}

func (r *ServerTaskExecutionRepository) applyPagination(
	execs []domain.ServerTaskExecution, pagination *filters.Pagination,
) []domain.ServerTaskExecution {
	if pagination == nil {
		return execs
	}

	limit := pagination.Limit
	if limit == 0 {
		limit = filters.DefaultLimit
	}
	offset := pagination.Offset

	length := uint64(len(execs))
	if offset >= length {
		return []domain.ServerTaskExecution{}
	}

	end := min(offset+limit, length)

	return execs[offset:end]
}
