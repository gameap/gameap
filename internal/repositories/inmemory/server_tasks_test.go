package inmemory_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskRepository(t *testing.T) {
	t.Parallel()

	serverRepo := inmemory.NewServerRepository()
	suite.Run(t, repotesting.NewServerTaskRepositorySuite(
		func(_ *testing.T) repositories.ServerTaskRepository {
			return inmemory.NewServerTaskRepository(serverRepo)
		},
		func(_ *testing.T) repositories.ServerRepository {
			return serverRepo
		},
	))
}

func TestServerTaskRepository_FilterByNodeIDs(t *testing.T) {
	t.Parallel()

	serverRepo := inmemory.NewServerRepository()
	taskRepo := inmemory.NewServerTaskRepository(serverRepo)

	ctx := context.Background()

	server1 := &domain.Server{
		UID:       uuid.New(),
		UUIDShort: "test1",
		Name:      "Server 1",
		DSID:      1,
		GameID:    "game1",
	}
	server2 := &domain.Server{
		UID:       uuid.New(),
		UUIDShort: "test2",
		Name:      "Server 2",
		DSID:      1,
		GameID:    "game1",
	}

	server3 := &domain.Server{
		UID:       uuid.New(),
		UUIDShort: "test3",
		Name:      "Server 3",
		DSID:      2,
		GameID:    "game1",
	}

	if err := serverRepo.Save(ctx, server1); err != nil {
		t.Fatalf("Failed to save server1: %v", err)
	}
	if err := serverRepo.Save(ctx, server2); err != nil {
		t.Fatalf("Failed to save server2: %v", err)
	}
	if err := serverRepo.Save(ctx, server3); err != nil {
		t.Fatalf("Failed to save server3: %v", err)
	}

	task1 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStart,
		ServerID:    server1.ID,
		ExecuteDate: time.Now(),
	}
	task2 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStop,
		ServerID:    server2.ID,
		ExecuteDate: time.Now(),
	}
	task3 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandRestart,
		ServerID:    server3.ID,
		ExecuteDate: time.Now(),
	}

	if err := taskRepo.Save(ctx, task1); err != nil {
		t.Fatalf("Failed to save task1: %v", err)
	}
	if err := taskRepo.Save(ctx, task2); err != nil {
		t.Fatalf("Failed to save task2: %v", err)
	}
	if err := taskRepo.Save(ctx, task3); err != nil {
		t.Fatalf("Failed to save task3: %v", err)
	}

	t.Run("Filter_by_Node_1", func(t *testing.T) {
		t.Parallel()

		filter := &filters.FindServerTask{
			NodeIDs: []uint{1},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 2 {
			t.Errorf("Expected 2 tasks for node 1, got %d", len(tasks))
		}

		for _, task := range tasks {
			if task.ServerID != server1.ID && task.ServerID != server2.ID {
				t.Errorf("Task %d belongs to unexpected server %d", task.ID, task.ServerID)
			}
		}
	})

	t.Run("Filter_by_Node_2", func(t *testing.T) {
		t.Parallel()

		filter := &filters.FindServerTask{
			NodeIDs: []uint{2},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 1 {
			t.Errorf("Expected 1 task for node 2, got %d", len(tasks))
		}

		if len(tasks) > 0 && tasks[0].ServerID != server3.ID {
			t.Errorf("Task belongs to unexpected server %d", tasks[0].ServerID)
		}
	})

	t.Run("Filter_by_Multiple_Nodes", func(t *testing.T) {
		t.Parallel()

		filter := &filters.FindServerTask{
			NodeIDs: []uint{1, 2},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 3 {
			t.Errorf("Expected 3 tasks for nodes 1 and 2, got %d", len(tasks))
		}
	})

	t.Run("Filter_by_Non_existent_Node", func(t *testing.T) {
		t.Parallel()

		filter := &filters.FindServerTask{
			NodeIDs: []uint{999},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 0 {
			t.Errorf("Expected 0 tasks for non-existent node, got %d", len(tasks))
		}
	})

	t.Run("Filter_by_Node_and_Command", func(t *testing.T) {
		t.Parallel()

		filter := &filters.FindServerTask{
			NodeIDs:  []uint{1},
			Commands: []domain.ServerTaskCommand{domain.ServerTaskCommandStart},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 1 {
			t.Errorf("Expected 1 task (start command on node 1), got %d", len(tasks))
		}

		if len(tasks) > 0 {
			if tasks[0].Command != domain.ServerTaskCommandStart {
				t.Errorf("Expected start command, got %s", tasks[0].Command)
			}
			if tasks[0].ServerID != server1.ID {
				t.Errorf("Expected server %d, got %d", server1.ID, tasks[0].ServerID)
			}
		}
	})
}

func TestServerTaskRepository_FilterByNodeIDs_WithoutServerRepo(t *testing.T) {
	t.Parallel()

	taskRepo := inmemory.NewServerTaskRepository(nil)

	ctx := context.Background()

	task := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStart,
		ServerID:    1,
		ExecuteDate: time.Now(),
	}

	if err := taskRepo.Save(ctx, task); err != nil {
		t.Fatalf("Failed to save task: %v", err)
	}

	filter := &filters.FindServerTask{
		NodeIDs: []uint{1},
	}
	tasks, err := taskRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		t.Fatalf("Failed to find tasks: %v", err)
	}

	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks when ServerRepository is not set, got %d", len(tasks))
	}
}

// orderQueryRepeats is how often an order-sensitive query is repeated. The
// repositories collect their rows from maps, whose iteration order Go
// randomises per range, so a single query only exercises one of the possible
// input orders of the comparator.
const orderQueryRepeats = 16

// setupServerTaskSortRepo stores the given tasks and hands back the repository
// through its public interface. The tasks carry explicit IDs so Save keeps the
// CreatedAt values as written instead of stamping them with time.Now().
func setupServerTaskSortRepo(t *testing.T, tasks []domain.ServerTask) repositories.ServerTaskRepository {
	t.Helper()

	repo := inmemory.NewServerTaskRepository(nil)

	for i := range tasks {
		require.NoError(t, repo.Save(t.Context(), &tasks[i]))
	}

	return repo
}

func serverTaskIDs(tasks []domain.ServerTask) []uint {
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}

	return ids
}

func serverTasksSortFixture() []domain.ServerTask {
	return []domain.ServerTask{
		{
			ID:           1,
			Command:      domain.ServerTaskCommandStop,
			ServerID:     30,
			Repeat:       3,
			RepeatPeriod: 30 * time.Second,
			Counter:      100,
			ExecuteDate:  time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:           2,
			Command:      domain.ServerTaskCommandRestart,
			ServerID:     10,
			Repeat:       1,
			RepeatPeriod: 10 * time.Second,
			Counter:      300,
			ExecuteDate:  time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:           3,
			Command:      domain.ServerTaskCommandStart,
			ServerID:     40,
			Repeat:       4,
			RepeatPeriod: 40 * time.Second,
			Counter:      200,
			ExecuteDate:  time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:           4,
			Command:      domain.ServerTaskCommandUpdate,
			ServerID:     20,
			Repeat:       2,
			RepeatPeriod: 20 * time.Second,
			Counter:      400,
			ExecuteDate:  time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func TestServerTaskRepository_SortByScalarField(t *testing.T) {
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
			// restart < start < stop < update lexicographically.
			name:     "command",
			field:    "command",
			wantAsc:  []uint{2, 3, 1, 4},
			wantDesc: []uint{4, 1, 3, 2},
		},
		{
			name:     "server_id",
			field:    "server_id",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			name:     "repeat",
			field:    "repeat",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			name:     "repeat_period",
			field:    "repeat_period",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
		{
			name:     "counter",
			field:    "counter",
			wantAsc:  []uint{1, 3, 2, 4},
			wantDesc: []uint{4, 2, 3, 1},
		},
		{
			name:     "execute_date",
			field:    "execute_date",
			wantAsc:  []uint{2, 4, 1, 3},
			wantDesc: []uint{3, 1, 4, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupServerTaskSortRepo(t, serverTasksSortFixture())

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
			assert.Equal(t, tt.wantAsc, serverTaskIDs(asc), "ascending order by %s", tt.field)
			assert.Equal(t, tt.wantDesc, serverTaskIDs(desc), "descending order by %s", tt.field)
		})
	}
}

func TestServerTaskRepository_SortByUnknownFieldFallsThroughToNextTerm(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := setupServerTaskSortRepo(t, serverTasksSortFixture())

	// ACT
	tasks, err := repo.Find(t.Context(), nil, []filters.Sorting{
		{Field: "no_such_column", Direction: filters.SortDirectionAsc},
		{Field: "server_id", Direction: filters.SortDirectionAsc},
	}, nil)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, tasks, 4)
	assert.Equal(t, []uint{2, 4, 1, 3}, serverTaskIDs(tasks),
		"an unknown sort column must tie so the next term decides the order")
}

func TestServerTaskRepository_SortByCreatedAtWithMissingValues(t *testing.T) {
	t.Parallel()

	early := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)

	fixture := func() []domain.ServerTask {
		return []domain.ServerTask{
			{ID: 1, Command: domain.ServerTaskCommandStart, ServerID: 1},
			{ID: 2, Command: domain.ServerTaskCommandStart, ServerID: 1},
			{ID: 3, Command: domain.ServerTaskCommandStart, ServerID: 1, CreatedAt: &late},
			{ID: 4, Command: domain.ServerTaskCommandStart, ServerID: 1, CreatedAt: &early},
		}
	}

	tests := []struct {
		name      string
		direction filters.SortDirection
		wantIDs   []uint
	}{
		{
			name:      "ascending_puts_missing_created_at_first",
			direction: filters.SortDirectionAsc,
			wantIDs:   []uint{1, 2, 4, 3},
		},
		{
			name:      "descending_puts_missing_created_at_last",
			direction: filters.SortDirectionDesc,
			wantIDs:   []uint{3, 4, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupServerTaskSortRepo(t, fixture())

			// ACT
			// Repeated because the repository collects the rows from a map, so
			// every query hands the comparator a different starting order.
			for range orderQueryRepeats {
				tasks, err := repo.Find(t.Context(), nil, []filters.Sorting{
					{Field: "created_at", Direction: tt.direction},
					{Field: "id", Direction: filters.SortDirectionAsc},
				}, nil)

				// ASSERT
				require.NoError(t, err)
				require.Len(t, tasks, 4)
				assert.Equal(t, tt.wantIDs, serverTaskIDs(tasks),
					"tasks without created_at must group together and the id term must break their tie")
			}
		})
	}
}

func TestServerTaskRepository_SortByEqualTimestampKeepsNextTermDeciding(t *testing.T) {
	t.Parallel()

	shared := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		field string
		tasks []domain.ServerTask
	}{
		{
			name:  "execute_date",
			field: "execute_date",
			tasks: []domain.ServerTask{
				{ID: 1, Command: domain.ServerTaskCommandStart, ServerID: 1, ExecuteDate: shared},
				{ID: 2, Command: domain.ServerTaskCommandStart, ServerID: 1, ExecuteDate: later},
				{ID: 3, Command: domain.ServerTaskCommandStart, ServerID: 1, ExecuteDate: shared},
			},
		},
		{
			name:  "created_at",
			field: "created_at",
			tasks: []domain.ServerTask{
				{ID: 1, Command: domain.ServerTaskCommandStart, ServerID: 1, CreatedAt: &shared},
				{ID: 2, Command: domain.ServerTaskCommandStart, ServerID: 1, CreatedAt: &later},
				{ID: 3, Command: domain.ServerTaskCommandStart, ServerID: 1, CreatedAt: &shared},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := setupServerTaskSortRepo(t, tt.tasks)

			// ACT
			for range orderQueryRepeats {
				tasks, err := repo.Find(t.Context(), nil, []filters.Sorting{
					{Field: tt.field, Direction: filters.SortDirectionAsc},
					{Field: "id", Direction: filters.SortDirectionDesc},
				}, nil)

				// ASSERT
				require.NoError(t, err)
				require.Len(t, tasks, 3)
				assert.Equal(t, []uint{3, 1, 2}, serverTaskIDs(tasks),
					"tasks sharing %s must be ordered by the next sort term", tt.field)
			}
		})
	}
}

func TestServerTaskRepository_SortByUpdatedAt(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Save stamps updated_at itself, so saving in sequence is the only way to
	// obtain a known chronological order.
	repo := setupServerTaskSortRepo(t, []domain.ServerTask{
		{ID: 1, Command: domain.ServerTaskCommandStart, ServerID: 1},
		{ID: 2, Command: domain.ServerTaskCommandStart, ServerID: 1},
		{ID: 3, Command: domain.ServerTaskCommandStart, ServerID: 1},
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
		assert.True(t, slices.IsSortedFunc(asc, func(a, b domain.ServerTask) int {
			return a.UpdatedAt.Compare(*b.UpdatedAt)
		}), "ascending updated_at must return the oldest task first")
		assert.True(t, slices.IsSortedFunc(desc, func(a, b domain.ServerTask) int {
			return b.UpdatedAt.Compare(*a.UpdatedAt)
		}), "descending updated_at must return the newest task first")
		assert.ElementsMatch(t, []uint{1, 2, 3}, serverTaskIDs(asc), "no task may be dropped by sorting")
	}
}
