package inmemory_test

import (
	"slices"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestDaemonTaskRepository(t *testing.T) {
	t.Parallel()

	suite.Run(t, repotesting.NewDaemonTaskRepositorySuite(
		func(_ *testing.T) repositories.DaemonTaskRepository {
			return inmemory.NewDaemonTaskRepository()
		},
	))
}

// setupDaemonTaskRepo stores the given tasks and hands back the repository
// through its public interface. The tasks carry explicit IDs so Save keeps the
// CreatedAt values as written instead of stamping them with time.Now().
func setupDaemonTaskRepo(t *testing.T, tasks []domain.DaemonTask) repositories.DaemonTaskRepository {
	t.Helper()

	repo := inmemory.NewDaemonTaskRepository()

	for i := range tasks {
		require.NoError(t, repo.Save(t.Context(), &tasks[i]))
	}

	return repo
}

func daemonTaskIDs(tasks []domain.DaemonTask) []uint {
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}

	return ids
}

func daemonTasksSortFixture() []domain.DaemonTask {
	return []domain.DaemonTask{
		{
			ID:                1,
			DedicatedServerID: 30,
			ServerID:          new(uint(300)),
			Task:              domain.DaemonTaskTypeServerStop,
			Status:            domain.DaemonTaskStatusWorking,
		},
		{
			ID:                2,
			DedicatedServerID: 10,
			ServerID:          new(uint(100)),
			Task:              domain.DaemonTaskTypeServerRestart,
			Status:            domain.DaemonTaskStatusError,
		},
		{
			ID:                3,
			DedicatedServerID: 40,
			ServerID:          new(uint(400)),
			Task:              domain.DaemonTaskTypeServerStart,
			Status:            domain.DaemonTaskStatusSuccess,
		},
		{
			ID:                4,
			DedicatedServerID: 20,
			ServerID:          new(uint(200)),
			Task:              domain.DaemonTaskTypeServerUpdate,
			Status:            domain.DaemonTaskStatusCanceled,
		},
	}
}

func TestDaemonTaskRepository_SortByScalarField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    string
		wantAsc  []uint
		wantDesc []uint
	}{
		{
			name:     "id",
			field:    "id",
			wantAsc:  []uint{1, 2, 3, 4},
			wantDesc: []uint{4, 3, 2, 1},
		},
		{
			name:     "dedicated_server_id",
			field:    "dedicated_server_id",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			// node_id is an alias of dedicated_server_id.
			name:     "node_id",
			field:    "node_id",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			name:     "server_id",
			field:    "server_id",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			// gsrest < gsstart < gsstop < gsupd lexicographically.
			name:     "task",
			field:    "task",
			wantAsc:  []uint{2, 3, 1, 4},
			wantDesc: []uint{4, 1, 3, 2},
		},
		{
			// canceled < error < success < working lexicographically.
			name:     "status",
			field:    "status",
			wantAsc:  []uint{4, 2, 3, 1},
			wantDesc: []uint{1, 3, 2, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDaemonTaskRepo(t, daemonTasksSortFixture())

			// ACT
			asc, ascErr := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: tt.field, Direction: filters.SortDirectionAsc}}, nil,
			)
			desc, descErr := repo.Find(
				t.Context(), nil, []filters.Sorting{{Field: tt.field, Direction: filters.SortDirectionDesc}}, nil,
			)

			// ASSERT
			require.NoError(t, ascErr)
			require.NoError(t, descErr)
			require.Len(t, asc, len(tt.wantAsc))
			require.Len(t, desc, len(tt.wantDesc))
			assert.Equal(t, tt.wantAsc, daemonTaskIDs(asc), "ascending order by %s", tt.field)
			assert.Equal(t, tt.wantDesc, daemonTaskIDs(desc), "descending order by %s", tt.field)
		})
	}
}

func TestDaemonTaskRepository_SortByUnknownFieldFallsThroughToNextTerm(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupDaemonTaskRepo(t, daemonTasksSortFixture())

	// ACT
	tasks, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "no_such_column", Direction: filters.SortDirectionAsc},
		{Field: "dedicated_server_id", Direction: filters.SortDirectionAsc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, tasks, 4)
	assert.Equal(t, []uint{2, 4, 1, 3}, daemonTaskIDs(tasks),
		"an unknown sort column must tie so the next term decides the order")
}

func TestDaemonTaskRepository_SortByNullableColumns(t *testing.T) {
	t.Parallel()

	early := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)

	serverIDFixture := func() []domain.DaemonTask {
		return []domain.DaemonTask{
			{ID: 1, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
			{ID: 2, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
			{ID: 3, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, ServerID: new(uint(200))},
			{ID: 4, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, ServerID: new(uint(100))},
		}
	}

	createdAtFixture := func() []domain.DaemonTask {
		return []domain.DaemonTask{
			{ID: 1, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
			{ID: 2, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
			{ID: 3, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, CreatedAt: &late},
			{ID: 4, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, CreatedAt: &early},
		}
	}

	tests := []struct {
		name      string
		field     string
		tasks     []domain.DaemonTask
		direction filters.SortDirection
		wantIDs   []uint
	}{
		{
			name:      "server_id_ascending_puts_missing_value_first",
			field:     "server_id",
			tasks:     serverIDFixture(),
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "server_id_descending_puts_missing_value_last",
			field:     "server_id",
			tasks:     serverIDFixture(),
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
		{
			name:      "created_at_ascending_puts_missing_value_first",
			field:     "created_at",
			tasks:     createdAtFixture(),
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "created_at_descending_puts_missing_value_last",
			field:     "created_at",
			tasks:     createdAtFixture(),
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDaemonTaskRepo(t, tt.tasks)

			// ACT
			for range orderQueryRepeats {
				tasks, err := repo.Find(t.Context(), nil, []filters.Sorting{
					{Field: tt.field, Direction: tt.direction},
					{Field: "id", Direction: filters.SortDirectionAsc},
				}, nil)

				// ASSERT
				require.NoError(t, err)
				require.Len(t, tasks, 4)
				assert.Equal(t, tt.wantIDs, daemonTaskIDs(tasks),
					"tasks without %s must group together and the id term must break their tie", tt.field)
			}
		})
	}
}

func TestDaemonTaskRepository_SortByEqualCreatedAtKeepsNextTermDeciding(t *testing.T) {
	t.Parallel()

	shared := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)

	// ARRANGE
	repo := setupDaemonTaskRepo(t, []domain.DaemonTask{
		{ID: 1, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, CreatedAt: &shared},
		{ID: 2, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, CreatedAt: &later},
		{ID: 3, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, CreatedAt: &shared},
	})

	// ACT
	for range orderQueryRepeats {
		tasks, err := repo.Find(t.Context(), nil, []filters.Sorting{
			{Field: "created_at", Direction: filters.SortDirectionAsc},
			{Field: "id", Direction: filters.SortDirectionDesc},
		}, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, tasks, 3)
		assert.Equal(t, []uint{3, 1, 2}, daemonTaskIDs(tasks),
			"tasks sharing created_at must be ordered by the next sort term")
	}
}

func TestDaemonTaskRepository_SortByUpdatedAt(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Save stamps updated_at itself, so saving in sequence is the only way to
	// obtain a known chronological order.
	repo := setupDaemonTaskRepo(t, []domain.DaemonTask{
		{ID: 1, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
		{ID: 2, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
		{ID: 3, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec},
	})

	// ACT
	for range orderQueryRepeats {
		asc, ascErr := repo.Find(
			t.Context(), nil, []filters.Sorting{{Field: "updated_at", Direction: filters.SortDirectionAsc}}, nil,
		)
		desc, descErr := repo.Find(
			t.Context(), nil, []filters.Sorting{{Field: "updated_at", Direction: filters.SortDirectionDesc}}, nil,
		)

		// ASSERT
		require.NoError(t, ascErr)
		require.NoError(t, descErr)
		require.Len(t, asc, 3)
		require.Len(t, desc, 3)
		assert.True(t, slices.IsSortedFunc(asc, func(a, b domain.DaemonTask) int {
			return a.UpdatedAt.Compare(*b.UpdatedAt)
		}), "ascending updated_at must return the oldest task first")
		assert.True(t, slices.IsSortedFunc(desc, func(a, b domain.DaemonTask) int {
			return b.UpdatedAt.Compare(*a.UpdatedAt)
		}), "descending updated_at must return the newest task first")
		assert.ElementsMatch(t, []uint{1, 2, 3}, daemonTaskIDs(asc), "no task may be dropped by sorting")
	}
}

func TestDaemonTaskRepository_FindIntersectsServerIDsWithIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ids       []uint
		serverIDs []*uint
		wantIDs   []uint
	}{
		{
			name:      "intersection_keeps_only_tasks_matching_both_filters",
			ids:       []uint{1, 2, 3},
			serverIDs: []*uint{new(uint(100)), new(uint(300))},
			wantIDs:   []uint{1, 2},
		},
		{
			name:      "nil_server_id_is_skipped_and_does_not_widen_the_result",
			ids:       []uint{1, 2, 3},
			serverIDs: []*uint{nil, new(uint(300))},
			wantIDs:   []uint{2},
		},
		{
			name:      "only_nil_server_ids_produce_no_result",
			ids:       []uint{1, 2, 3},
			serverIDs: []*uint{nil},
			wantIDs:   []uint{},
		},
		{
			name:      "disjoint_sets_produce_no_result",
			ids:       []uint{3},
			serverIDs: []*uint{new(uint(100))},
			wantIDs:   []uint{},
		},
		{
			name:      "unknown_server_id_produces_no_result",
			ids:       []uint{1, 2, 3},
			serverIDs: []*uint{new(uint(999))},
			wantIDs:   []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupDaemonTaskRepo(t, []domain.DaemonTask{
				{ID: 1, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, ServerID: new(uint(100))},
				{ID: 2, DedicatedServerID: 1, Task: domain.DaemonTaskTypeCmdExec, ServerID: new(uint(300))},
				{ID: 3, DedicatedServerID: 2, Task: domain.DaemonTaskTypeCmdExec, ServerID: new(uint(500))},
			})

			// ACT
			tasks, err := repo.Find(t.Context(), &filters.FindDaemonTask{
				IDs:       tt.ids,
				ServerIDs: tt.serverIDs,
			}, []filters.Sorting{{Field: "id", Direction: filters.SortDirectionAsc}}, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, tasks, len(tt.wantIDs))
			assert.Equal(t, tt.wantIDs, daemonTaskIDs(tasks), "only the intersection of both filters must remain")
		})
	}
}
