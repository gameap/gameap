package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DaemonTaskRepositorySuite struct {
	suite.Suite

	repo repositories.DaemonTaskRepository

	fn func(t *testing.T) repositories.DaemonTaskRepository
}

func NewDaemonTaskRepositorySuite(fn func(t *testing.T) repositories.DaemonTaskRepository) *DaemonTaskRepositorySuite {
	return &DaemonTaskRepositorySuite{
		fn: fn,
	}
}

func (s *DaemonTaskRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_task", func(t *testing.T) {
		// ARRANGE
		task := &domain.DaemonTask{
			DedicatedServerID: 1,
			ServerID:          new(uint(10)),
			Task:              domain.DaemonTaskTypeServerStart,
			Data:              new("test data"),
			Cmd:               new("start command"),
			Output:            new(""),
			Status:            domain.DaemonTaskStatusWaiting,
		}

		// ACT
		err := s.repo.Save(ctx, task)

		// ASSERT
		require.NoError(t, err)
		assert.NotZero(t, task.ID)
		assert.NotNil(t, task.CreatedAt)
		assert.NotNil(t, task.UpdatedAt)

		find, err := s.repo.FindWithOutput(ctx, filters.FindDaemonTaskByIDs(task.ID), nil, nil)
		require.NoError(t, err)

		require.Len(t, find, 1)
		savedTask := find[0]
		assert.Equal(t, task.ID, savedTask.ID)
		assert.Equal(t, task.RunAftID, savedTask.RunAftID)
		assert.InDelta(t, task.CreatedAt.Unix(), savedTask.CreatedAt.Unix(), 1.0)
		assert.InDelta(t, task.UpdatedAt.Unix(), savedTask.UpdatedAt.Unix(), 1.0)
		assert.Equal(t, task.DedicatedServerID, savedTask.DedicatedServerID)
		assert.Equal(t, task.ServerID, savedTask.ServerID)
		assert.Equal(t, task.Task, savedTask.Task)
		assert.Equal(t, task.Data, savedTask.Data)
		assert.Equal(t, task.Cmd, savedTask.Cmd)
		assert.Equal(t, task.Output, savedTask.Output)
		assert.Equal(t, task.Status, savedTask.Status)
	})

	s.T().Run("update_existing_task", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 2,
			ServerID:          new(uint(20)),
			Task:              domain.DaemonTaskTypeServerStop,
			Data:              new("original data"),
			Cmd:               new("stop command"),
			Output:            new("original output"),
			Status:            domain.DaemonTaskStatusWaiting,
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		originalID := task.ID
		originalCreatedAt := task.CreatedAt
		originalUpdatedAt := task.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		task.Status = domain.DaemonTaskStatusSuccess
		task.Output = new("updated output")
		task.Data = new("updated data")

		err = s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.Equal(t, originalID, task.ID)
		assert.Equal(t, originalCreatedAt, task.CreatedAt)
		assert.True(t, task.UpdatedAt.After(*originalUpdatedAt))
		assert.Equal(t, domain.DaemonTaskStatusSuccess, task.Status)
		assert.Equal(t, "updated output", *task.Output)
		assert.Equal(t, "updated data", *task.Data)
	})

	s.T().Run("update_existing_task_without_output", func(t *testing.T) {
		// ARRANGE
		task := &domain.DaemonTask{
			DedicatedServerID: 2,
			ServerID:          new(uint(20)),
			Task:              domain.DaemonTaskTypeServerStop,
			Data:              new("original data"),
			Cmd:               new("stop command"),
			Output:            new("original output"),
			Status:            domain.DaemonTaskStatusWaiting,
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		originalID := task.ID
		originalCreatedAt := task.CreatedAt
		originalUpdatedAt := task.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		taskToUpdate := &domain.DaemonTask{
			ID:                originalID,
			RunAftID:          task.RunAftID,
			CreatedAt:         originalCreatedAt,
			UpdatedAt:         originalUpdatedAt,
			DedicatedServerID: task.DedicatedServerID,
			ServerID:          task.ServerID,
			Task:              task.Task,
			Data:              new("updated data"),
			Cmd:               task.Cmd,
			Output:            nil, // Simulate not updating the output\
			Status:            domain.DaemonTaskStatusSuccess,
		}

		// ACT
		err = s.repo.Save(ctx, taskToUpdate)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, originalID, taskToUpdate.ID)
		assert.InDelta(t, originalCreatedAt.Unix(), taskToUpdate.CreatedAt.Unix(), 1.0)
		assert.True(t, taskToUpdate.UpdatedAt.After(*originalUpdatedAt))
		assert.Equal(t, domain.DaemonTaskStatusSuccess, taskToUpdate.Status)

		find, err := s.repo.FindWithOutput(ctx, filters.FindDaemonTaskByIDs(task.ID), nil, nil)
		require.NoError(t, err)

		require.Len(t, find, 1)
		updatedTask := find[0]
		assert.Equal(t, taskToUpdate.ID, updatedTask.ID)
		assert.Equal(t, taskToUpdate.RunAftID, updatedTask.RunAftID)
		assert.InDelta(t, taskToUpdate.CreatedAt.Unix(), updatedTask.CreatedAt.Unix(), 1.0)
		assert.InDelta(t, taskToUpdate.UpdatedAt.Unix(), updatedTask.UpdatedAt.Unix(), 1.0)
		assert.Equal(t, taskToUpdate.DedicatedServerID, updatedTask.DedicatedServerID)
		assert.Equal(t, taskToUpdate.ServerID, updatedTask.ServerID)
		assert.Equal(t, taskToUpdate.Task, updatedTask.Task)
		assert.Equal(t, taskToUpdate.Data, updatedTask.Data)
		assert.Equal(t, taskToUpdate.Cmd, updatedTask.Cmd)
		assert.Equal(t, task.Output, updatedTask.Output)
		assert.Equal(t, taskToUpdate.Status, updatedTask.Status)
	})

	s.T().Run("insert_task_with_nil_server_id", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 3,
			ServerID:          nil,
			Task:              domain.DaemonTaskTypeCmdExec,
			Cmd:               new("some command"),
			Status:            domain.DaemonTaskStatusWaiting,
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.NotZero(t, task.ID)
		assert.Nil(t, task.ServerID)
	})

	s.T().Run("auto_set_timestamps_on_insert", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 4,
			Task:              domain.DaemonTaskTypeServerInstall,
			Status:            domain.DaemonTaskStatusWaiting,
		}

		beforeSave := time.Now()
		err := s.repo.Save(ctx, task)
		afterSave := time.Now()

		require.NoError(t, err)
		require.NotNil(t, task.CreatedAt)
		require.NotNil(t, task.UpdatedAt)
		assert.True(t, task.CreatedAt.After(beforeSave) || task.CreatedAt.Equal(beforeSave))
		assert.True(t, task.CreatedAt.Before(afterSave) || task.CreatedAt.Equal(afterSave))
		assert.True(t, task.UpdatedAt.After(beforeSave) || task.UpdatedAt.Equal(beforeSave))
		assert.True(t, task.UpdatedAt.Before(afterSave) || task.UpdatedAt.Equal(afterSave))
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryFindAll() {
	ctx := context.Background()

	tasks := []*domain.DaemonTask{
		{
			DedicatedServerID: 1,
			ServerID:          new(uint(100)),
			Task:              domain.DaemonTaskTypeServerStart,
			Status:            domain.DaemonTaskStatusWaiting,
			Output:            new("output1"),
		},
		{
			DedicatedServerID: 2,
			ServerID:          new(uint(200)),
			Task:              domain.DaemonTaskTypeServerStop,
			Status:            domain.DaemonTaskStatusWorking,
			Output:            new("output2"),
		},
		{
			DedicatedServerID: 3,
			ServerID:          new(uint(300)),
			Task:              domain.DaemonTaskTypeServerRestart,
			Status:            domain.DaemonTaskStatusSuccess,
			Output:            new("output3"),
		},
	}

	for _, task := range tasks {
		require.NoError(s.T(), s.repo.Save(ctx, task))
	}

	s.T().Run("find_all_tasks", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		results, err := s.repo.FindAll(ctx, nil, pagination)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})

	s.T().Run("find_all_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryFind() {
	ctx := context.Background()

	task1 := &domain.DaemonTask{
		DedicatedServerID: 10,
		ServerID:          new(uint(101)),
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
		Output:            new("find_output1"),
	}
	task2 := &domain.DaemonTask{
		DedicatedServerID: 20,
		ServerID:          new(uint(102)),
		Task:              domain.DaemonTaskTypeServerStop,
		Status:            domain.DaemonTaskStatusWorking,
		Output:            new("find_output2"),
	}
	task3 := &domain.DaemonTask{
		DedicatedServerID: 10,
		ServerID:          new(uint(103)),
		Task:              domain.DaemonTaskTypeServerRestart,
		Status:            domain.DaemonTaskStatusSuccess,
		Output:            new("find_output3"),
	}

	require.NoError(s.T(), s.repo.Save(ctx, task1))
	require.NoError(s.T(), s.repo.Save(ctx, task2))
	require.NoError(s.T(), s.repo.Save(ctx, task3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{task1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, task1.ID, results[0].ID)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{task1.ID, task3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, task1.ID)
		assert.Contains(t, ids, task3.ID)
	})

	s.T().Run("find_by_dedicated_server_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.Equal(t, uint(10), result.DedicatedServerID)
		}
	})

	s.T().Run("find_by_server_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			ServerIDs: []*uint{new(uint(101))},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, task1.ID, results[0].ID)
	})

	s.T().Run("find_by_task_type", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			Tasks: []domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)

		for _, result := range results {
			assert.Equal(t, domain.DaemonTaskTypeServerStart, result.Task)
		}
	})

	s.T().Run("find_by_status", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting, domain.DaemonTaskStatusWorking},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)

		for _, result := range results {
			assert.Contains(t, []domain.DaemonTaskStatus{
				domain.DaemonTaskStatusWaiting,
				domain.DaemonTaskStatusWorking,
			}, result.Status)
		}
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_with_empty_filter", func(t *testing.T) {
		filter := &filters.FindDaemonTask{}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{task1.ID, task2.ID, task3.ID},
		}
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{task1.ID, task2.ID, task3.ID},
		}
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, filter, order, nil)
		require.NoError(t, err)
		require.Len(t, results, 3)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})

	s.T().Run("find_by_dedicated_server_id_and_status", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10},
			Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, uint(10), results[0].DedicatedServerID)
		assert.Equal(t, domain.DaemonTaskStatusWaiting, results[0].Status)
	})

	s.T().Run("find_by_dedicated_server_id_and_task_type", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10},
			Tasks:              []domain.DaemonTaskType{domain.DaemonTaskTypeServerRestart},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, uint(10), results[0].DedicatedServerID)
		assert.Equal(t, domain.DaemonTaskTypeServerRestart, results[0].Task)
	})

	s.T().Run("find_by_status_and_task_type", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
			Tasks:    []domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 1)

		for _, result := range results {
			assert.Equal(t, domain.DaemonTaskStatusWaiting, result.Status)
			assert.Equal(t, domain.DaemonTaskTypeServerStart, result.Task)
		}
	})

	s.T().Run("find_by_ids_and_status", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs:      []uint{task1.ID, task2.ID, task3.ID},
			Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWorking},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, task2.ID, results[0].ID)
		assert.Equal(t, domain.DaemonTaskStatusWorking, results[0].Status)
	})

	s.T().Run("find_by_server_id_and_dedicated_server_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			ServerIDs:          []*uint{new(uint(101))},
			DedicatedServerIDs: []uint{10},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, task1.ID, results[0].ID)
	})

	s.T().Run("find_by_three_filters", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10},
			Tasks:              []domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
			Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, task1.ID, results[0].ID)
		assert.Equal(t, uint(10), results[0].DedicatedServerID)
		assert.Equal(t, domain.DaemonTaskTypeServerStart, results[0].Task)
		assert.Equal(t, domain.DaemonTaskStatusWaiting, results[0].Status)
	})

	s.T().Run("find_by_all_filters", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs:                []uint{task1.ID, task2.ID, task3.ID},
			DedicatedServerIDs: []uint{10},
			ServerIDs:          []*uint{new(uint(101))},
			Tasks:              []domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
			Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, task1.ID, results[0].ID)
	})

	s.T().Run("find_by_multiple_filters_no_match", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10},
			Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWorking},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_by_multiple_values_in_single_filter_fields", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10, 20},
			Statuses:           []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting, domain.DaemonTaskStatusWorking},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, task1.ID)
		assert.Contains(t, ids, task2.ID)
	})

	s.T().Run("find_with_multiple_filters_and_pagination", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10, 20},
			Statuses: []domain.DaemonTaskStatus{
				domain.DaemonTaskStatusWaiting,
				domain.DaemonTaskStatusWorking,
				domain.DaemonTaskStatusSuccess,
			},
		}
		pagination := &filters.Pagination{
			Limit:  1,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	s.T().Run("find_with_multiple_filters_and_order", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{10},
			Statuses: []domain.DaemonTaskStatus{
				domain.DaemonTaskStatusWaiting,
				domain.DaemonTaskStatusSuccess,
			},
		}
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, filter, order, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Greater(t, results[0].ID, results[1].ID)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryFindWithOutput() {
	ctx := context.Background()

	task := &domain.DaemonTask{
		DedicatedServerID: 5,
		ServerID:          new(uint(50)),
		Task:              domain.DaemonTaskTypeServerUpdate,
		Status:            domain.DaemonTaskStatusWorking,
		Output:            new("test output data"),
	}

	require.NoError(s.T(), s.repo.Save(ctx, task))

	s.T().Run("find_with_output_includes_output_field", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{task.ID},
		}

		results, err := s.repo.FindWithOutput(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.NotNil(t, results[0].Output)
		assert.Equal(t, "test output data", *results[0].Output)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_task", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 99,
			Task:              domain.DaemonTaskTypeServerDelete,
			Status:            domain.DaemonTaskStatusWaiting,
		}

		require.NoError(t, s.repo.Save(ctx, task))
		taskID := task.ID

		err := s.repo.Delete(ctx, taskID)
		require.NoError(t, err)

		filter := &filters.FindDaemonTask{
			IDs: []uint{taskID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent_task", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_task", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 88,
			Task:              domain.DaemonTaskTypeServerMove,
			Status:            domain.DaemonTaskStatusCanceled,
		}

		require.NoError(t, s.repo.Save(ctx, task))
		taskID := task.ID

		err := s.repo.Delete(ctx, taskID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, taskID)
		require.NoError(t, err)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryAppendOutput() {
	ctx := context.Background()

	s.T().Run("append_to_existing_output", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 7,
			Task:              domain.DaemonTaskTypeCmdExec,
			Status:            domain.DaemonTaskStatusWorking,
			Output:            new("Initial output\n"),
		}

		require.NoError(t, s.repo.Save(ctx, task))

		err := s.repo.AppendOutput(ctx, task.ID, "Additional line 1\n")
		require.NoError(t, err)

		err = s.repo.AppendOutput(ctx, task.ID, "Additional line 2\n")
		require.NoError(t, err)

		filter := &filters.FindDaemonTask{
			IDs: []uint{task.ID},
		}
		results, err := s.repo.FindWithOutput(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Initial output\nAdditional line 1\nAdditional line 2\n", *results[0].Output)
	})

	s.T().Run("append_to_nil_output", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 8,
			Task:              domain.DaemonTaskTypeCmdExec,
			Status:            domain.DaemonTaskStatusWorking,
			Output:            nil,
		}

		require.NoError(t, s.repo.Save(ctx, task))

		err := s.repo.AppendOutput(ctx, task.ID, "First line\n")
		require.NoError(t, err)

		filter := &filters.FindDaemonTask{
			IDs: []uint{task.ID},
		}
		results, err := s.repo.FindWithOutput(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "First line\n", *results[0].Output)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryCount() {
	ctx := context.Background()

	tasks := []*domain.DaemonTask{
		{
			DedicatedServerID: 15,
			Task:              domain.DaemonTaskTypeServerStart,
			Status:            domain.DaemonTaskStatusWaiting,
		},
		{
			DedicatedServerID: 15,
			Task:              domain.DaemonTaskTypeServerStop,
			Status:            domain.DaemonTaskStatusWaiting,
		},
		{
			DedicatedServerID: 16,
			Task:              domain.DaemonTaskTypeServerStart,
			Status:            domain.DaemonTaskStatusSuccess,
		},
	}

	for _, task := range tasks {
		require.NoError(s.T(), s.repo.Save(ctx, task))
	}

	s.T().Run("count_all_with_nil_filter", func(t *testing.T) {
		count, err := s.repo.Count(ctx, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 3)
	})

	s.T().Run("count_by_dedicated_server_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			DedicatedServerIDs: []uint{15},
		}

		count, err := s.repo.Count(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	s.T().Run("count_by_status", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
		}

		count, err := s.repo.Count(ctx, filter)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 2)
	})

	s.T().Run("count_non_existent", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{99999},
		}

		count, err := s.repo.Count(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryExists() {
	ctx := context.Background()

	task := &domain.DaemonTask{
		DedicatedServerID: 25,
		Task:              domain.DaemonTaskTypeServerInstall,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	require.NoError(s.T(), s.repo.Save(ctx, task))

	s.T().Run("exists_with_valid_filter", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{task.ID},
		}

		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("exists_with_non_existent_id", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			IDs: []uint{99999},
		}

		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("exists_with_nil_filter", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, nil)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("exists_by_status", func(t *testing.T) {
		filter := &filters.FindDaemonTask{
			Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting},
		}

		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		task := &domain.DaemonTask{
			DedicatedServerID: 30,
			ServerID:          new(uint(300)),
			Task:              domain.DaemonTaskTypeServerStart,
			Status:            domain.DaemonTaskStatusWaiting,
			Output:            new(""),
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.NotZero(t, task.ID)

		filter := &filters.FindDaemonTask{
			IDs: []uint{task.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, domain.DaemonTaskStatusWaiting, results[0].Status)

		task.Status = domain.DaemonTaskStatusWorking
		err = s.repo.Save(ctx, task)
		require.NoError(t, err)

		err = s.repo.AppendOutput(ctx, task.ID, "Starting server...\n")
		require.NoError(t, err)

		err = s.repo.AppendOutput(ctx, task.ID, "Server started successfully\n")
		require.NoError(t, err)

		resultsAfterAppend, err := s.repo.FindWithOutput(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, resultsAfterAppend, 1)
		task.Output = resultsAfterAppend[0].Output

		task.Status = domain.DaemonTaskStatusSuccess
		err = s.repo.Save(ctx, task)
		require.NoError(t, err)

		resultsWithOutput, err := s.repo.FindWithOutput(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, resultsWithOutput, 1)
		assert.Equal(t, domain.DaemonTaskStatusSuccess, resultsWithOutput[0].Status)
		assert.Contains(t, *resultsWithOutput[0].Output, "Starting server...")
		assert.Contains(t, *resultsWithOutput[0].Output, "Server started successfully")

		count, err := s.repo.Count(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		exists, err := s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.True(t, exists)

		err = s.repo.Delete(ctx, task.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)

		exists, err = s.repo.Exists(ctx, filter)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositorySorting() {
	ctx := context.Background()

	// ARRANGE — one fixture for the whole method: subtests share the repo.
	taskX := &domain.DaemonTask{
		DedicatedServerID: 101,
		ServerID:          new(uint(201)),
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}
	taskY := &domain.DaemonTask{
		DedicatedServerID: 102,
		ServerID:          new(uint(202)),
		Task:              domain.DaemonTaskTypeServerStop,
		Status:            domain.DaemonTaskStatusWorking,
	}
	taskZ := &domain.DaemonTask{
		DedicatedServerID: 103,
		ServerID:          new(uint(203)),
		Task:              domain.DaemonTaskTypeServerDelete,
		Status:            domain.DaemonTaskStatusSuccess,
	}
	require.NoError(s.T(), s.repo.Save(ctx, taskX))
	require.NoError(s.T(), s.repo.Save(ctx, taskY))
	require.NoError(s.T(), s.repo.Save(ctx, taskZ))

	filter := filters.FindDaemonTaskByIDs(taskX.ID, taskY.ID, taskZ.ID)

	tests := []struct {
		name  string
		order []filters.Sorting
		want  []uint
	}{
		{
			name: "sort_by_dedicated_server_id_asc",
			order: []filters.Sorting{
				{Field: "dedicated_server_id", Direction: filters.SortDirectionAsc},
			},
			want: []uint{taskX.ID, taskY.ID, taskZ.ID},
		},
		{
			name: "sort_by_dedicated_server_id_desc",
			order: []filters.Sorting{
				{Field: "dedicated_server_id", Direction: filters.SortDirectionDesc},
			},
			want: []uint{taskZ.ID, taskY.ID, taskX.ID},
		},
		{
			name: "sort_by_server_id_asc",
			order: []filters.Sorting{
				{Field: "server_id", Direction: filters.SortDirectionAsc},
			},
			want: []uint{taskX.ID, taskY.ID, taskZ.ID},
		},
		{
			name: "sort_by_server_id_desc",
			order: []filters.Sorting{
				{Field: "server_id", Direction: filters.SortDirectionDesc},
			},
			want: []uint{taskZ.ID, taskY.ID, taskX.ID},
		},
		{
			name: "sort_by_task_asc",
			order: []filters.Sorting{
				{Field: "task", Direction: filters.SortDirectionAsc},
			},
			want: []uint{taskZ.ID, taskX.ID, taskY.ID},
		},
		{
			name: "sort_by_task_desc",
			order: []filters.Sorting{
				{Field: "task", Direction: filters.SortDirectionDesc},
			},
			want: []uint{taskY.ID, taskX.ID, taskZ.ID},
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ACT
			results, err := s.repo.Find(ctx, filter, tt.order, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, results, 3)

			gotIDs := []uint{results[0].ID, results[1].ID, results[2].ID}
			assert.Equal(t, tt.want, gotIDs, "daemon tasks are in wrong order")
		})
	}
}

// TestDaemonTaskRepositorySortingStatus exercises status ordering with a
// waiting/working pair — the only statuses whose relative order coincides on
// every backend. MySQL stores the status as ENUM and sorts by the enum value
// index (waiting < working < error < success < canceled), while TEXT-backed
// backends (sqlite, postgres, inmemory) sort alphabetically; no three statuses
// are monotonic in both orders, so the shared 3-task fixture above cannot
// assert status ordering portably.
func (s *DaemonTaskRepositorySuite) TestDaemonTaskRepositorySortingStatus() {
	ctx := context.Background()

	// ARRANGE
	taskW1 := &domain.DaemonTask{
		DedicatedServerID: 111,
		ServerID:          new(uint(211)),
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}
	taskW2 := &domain.DaemonTask{
		DedicatedServerID: 112,
		ServerID:          new(uint(212)),
		Task:              domain.DaemonTaskTypeServerStop,
		Status:            domain.DaemonTaskStatusWorking,
	}
	require.NoError(s.T(), s.repo.Save(ctx, taskW1))
	require.NoError(s.T(), s.repo.Save(ctx, taskW2))

	filter := filters.FindDaemonTaskByIDs(taskW1.ID, taskW2.ID)

	tests := []struct {
		name  string
		order []filters.Sorting
		want  []uint
	}{
		{
			name: "sort_by_status_asc",
			order: []filters.Sorting{
				{Field: "status", Direction: filters.SortDirectionAsc},
			},
			want: []uint{taskW1.ID, taskW2.ID},
		},
		{
			name: "sort_by_status_desc",
			order: []filters.Sorting{
				{Field: "status", Direction: filters.SortDirectionDesc},
			},
			want: []uint{taskW2.ID, taskW1.ID},
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ACT
			results, err := s.repo.Find(ctx, filter, tt.order, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, results, 2)

			gotIDs := []uint{results[0].ID, results[1].ID}
			assert.Equal(t, tt.want, gotIDs, "daemon tasks are in wrong order")
		})
	}
}
