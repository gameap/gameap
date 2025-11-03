package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ServerTaskRepositorySuite struct {
	suite.Suite

	repo repositories.ServerTaskRepository

	fn func(t *testing.T) repositories.ServerTaskRepository
}

func NewServerTaskRepositorySuite(fn func(t *testing.T) repositories.ServerTaskRepository) *ServerTaskRepositorySuite {
	return &ServerTaskRepositorySuite{
		fn: fn,
	}
}

func (s *ServerTaskRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *ServerTaskRepositorySuite) TestServerTaskRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_task", func(t *testing.T) {
		executeDate := time.Now().Add(1 * time.Hour)
		task := &domain.ServerTask{
			Command:      domain.ServerTaskCommandStart,
			ServerID:     1,
			Repeat:       0,
			RepeatPeriod: 0,
			Counter:      0,
			ExecuteDate:  executeDate,
			Payload:      lo.ToPtr("test payload"),
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.NotZero(t, task.ID)
		assert.NotNil(t, task.CreatedAt)
		assert.NotNil(t, task.UpdatedAt)
	})

	s.T().Run("update_existing_task", func(t *testing.T) {
		executeDate := time.Now().Add(2 * time.Hour)
		task := &domain.ServerTask{
			Command:      domain.ServerTaskCommandStop,
			ServerID:     2,
			Repeat:       1,
			RepeatPeriod: 3600 * time.Second,
			Counter:      5,
			ExecuteDate:  executeDate,
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		originalID := task.ID

		task.Command = domain.ServerTaskCommandRestart
		task.Counter = 10

		err = s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.Equal(t, originalID, task.ID)

		filter := &filters.FindServerTask{IDs: []uint{task.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, domain.ServerTaskCommandRestart, results[0].Command)
		assert.Equal(t, uint(10), results[0].Counter)
	})

	s.T().Run("auto_set_timestamps", func(t *testing.T) {
		executeDate := time.Now().Add(3 * time.Hour)
		task := &domain.ServerTask{
			Command:      domain.ServerTaskCommandUpdate,
			ServerID:     3,
			Repeat:       0,
			RepeatPeriod: 0,
			Counter:      0,
			ExecuteDate:  executeDate,
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.NotNil(t, task.CreatedAt)
		assert.NotNil(t, task.UpdatedAt)
		assert.False(t, task.CreatedAt.IsZero())
		assert.False(t, task.UpdatedAt.IsZero())
	})
}

func (s *ServerTaskRepositorySuite) TestServerTaskRepositoryFindAll() {
	ctx := context.Background()

	executeDate := time.Now().Add(1 * time.Hour)

	task1 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStart,
		ServerID:    10,
		ExecuteDate: executeDate,
	}
	task2 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStop,
		ServerID:    11,
		ExecuteDate: executeDate,
	}

	require.NoError(s.T(), s.repo.Save(ctx, task1))
	require.NoError(s.T(), s.repo.Save(ctx, task2))

	s.T().Run("find_all_tasks", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 1, Offset: 0}

		results, err := s.repo.FindAll(ctx, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
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

func (s *ServerTaskRepositorySuite) TestServerTaskRepositoryFind() {
	ctx := context.Background()

	executeDate := time.Now().Add(1 * time.Hour)

	task1 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStart,
		ServerID:    100,
		ExecuteDate: executeDate,
	}
	task2 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStop,
		ServerID:    100,
		ExecuteDate: executeDate,
	}
	task3 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandRestart,
		ServerID:    200,
		ExecuteDate: executeDate,
	}

	require.NoError(s.T(), s.repo.Save(ctx, task1))
	require.NoError(s.T(), s.repo.Save(ctx, task2))
	require.NoError(s.T(), s.repo.Save(ctx, task3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindServerTask{IDs: []uint{task1.ID}}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, task1.ID, results[0].ID)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindServerTask{IDs: []uint{task1.ID, task3.ID}}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, task1.ID)
		assert.Contains(t, ids, task3.ID)
	})

	s.T().Run("find_by_server_id", func(t *testing.T) {
		filter := &filters.FindServerTask{ServersIDs: []uint{100}}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.Equal(t, uint(100), result.ServerID)
		}
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		filter := &filters.FindServerTask{IDs: []uint{99999}}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindServerTask{ServersIDs: []uint{100}}
		pagination := &filters.Pagination{Limit: 1, Offset: 0}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, nil, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *ServerTaskRepositorySuite) TestServerTaskRepositoryDelete() {
	ctx := context.Background()

	executeDate := time.Now().Add(1 * time.Hour)

	s.T().Run("delete_existing_task", func(t *testing.T) {
		task := &domain.ServerTask{
			Command:     domain.ServerTaskCommandStart,
			ServerID:    1000,
			ExecuteDate: executeDate,
		}

		require.NoError(t, s.repo.Save(ctx, task))
		taskID := task.ID

		err := s.repo.Delete(ctx, taskID)
		require.NoError(t, err)

		filter := &filters.FindServerTask{IDs: []uint{taskID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent_task", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})
}

func (s *ServerTaskRepositorySuite) TestServerTaskRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		executeDate := time.Now().Add(1 * time.Hour)
		task := &domain.ServerTask{
			Command:      domain.ServerTaskCommandStart,
			ServerID:     5000,
			Repeat:       1,
			RepeatPeriod: 7200 * time.Second,
			Counter:      0,
			ExecuteDate:  executeDate,
			Payload:      lo.ToPtr("test payload"),
		}

		err := s.repo.Save(ctx, task)
		require.NoError(t, err)
		assert.NotZero(t, task.ID)

		filter := &filters.FindServerTask{IDs: []uint{task.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, domain.ServerTaskCommandStart, results[0].Command)
		assert.Equal(t, uint8(1), results[0].Repeat)
		assert.Equal(t, 7200*time.Second, results[0].RepeatPeriod)

		task.Counter = 5
		task.Command = domain.ServerTaskCommandRestart
		err = s.repo.Save(ctx, task)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, domain.ServerTaskCommandRestart, results[0].Command)
		assert.Equal(t, uint(5), results[0].Counter)

		err = s.repo.Delete(ctx, task.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_tasks_for_same_server", func(t *testing.T) {
		serverID := uint(6000)
		executeDate := time.Now().Add(1 * time.Hour)

		tasks := []*domain.ServerTask{
			{
				Command:     domain.ServerTaskCommandStart,
				ServerID:    serverID,
				ExecuteDate: executeDate,
			},
			{
				Command:     domain.ServerTaskCommandStop,
				ServerID:    serverID,
				ExecuteDate: executeDate.Add(1 * time.Hour),
			},
			{
				Command:     domain.ServerTaskCommandRestart,
				ServerID:    serverID,
				ExecuteDate: executeDate.Add(2 * time.Hour),
			},
		}

		for _, task := range tasks {
			require.NoError(t, s.repo.Save(ctx, task))
		}

		filter := &filters.FindServerTask{ServersIDs: []uint{serverID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		err = s.repo.Delete(ctx, tasks[1].ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		commands := make(map[domain.ServerTaskCommand]bool)
		for _, result := range results {
			commands[result.Command] = true
		}
		assert.True(t, commands[domain.ServerTaskCommandStart])
		assert.False(t, commands[domain.ServerTaskCommandStop])
		assert.True(t, commands[domain.ServerTaskCommandRestart])
	})
}
