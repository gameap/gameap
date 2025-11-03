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

type ServerTaskFailRepositorySuite struct {
	suite.Suite

	repo repositories.ServerTaskFailRepository

	fn func(t *testing.T) repositories.ServerTaskFailRepository
}

func NewServerTaskFailRepositorySuite(fn func(t *testing.T) repositories.ServerTaskFailRepository) *ServerTaskFailRepositorySuite {
	return &ServerTaskFailRepositorySuite{
		fn: fn,
	}
}

func (s *ServerTaskFailRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *ServerTaskFailRepositorySuite) TestServerTaskFailRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_task", func(t *testing.T) {
		taskFail := &domain.ServerTaskFail{
			ServerTaskID: 1,
			Output:       "Error: Task failed",
		}

		err := s.repo.Save(ctx, taskFail)
		require.NoError(t, err)
		assert.NotZero(t, taskFail.ID)
	})

	s.T().Run("update_existing_task", func(t *testing.T) {
		taskFail := &domain.ServerTaskFail{
			ServerTaskID: 2,
			Output:       "Initial output",
		}

		err := s.repo.Save(ctx, taskFail)
		require.NoError(t, err)
		originalID := taskFail.ID

		taskFail.Output = "Updated output"

		err = s.repo.Save(ctx, taskFail)
		require.NoError(t, err)
		assert.Equal(t, originalID, taskFail.ID)

		filter := &filters.FindServerTaskFail{IDs: []uint{taskFail.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated output", results[0].Output)
	})

	s.T().Run("auto_set_timestamps", func(t *testing.T) {
		taskFail := &domain.ServerTaskFail{
			ServerTaskID: 3,
			Output:       "Test output",
		}

		err := s.repo.Save(ctx, taskFail)
		require.NoError(t, err)
		assert.NotNil(t, taskFail.CreatedAt)
		assert.NotNil(t, taskFail.UpdatedAt)
	})
}

func (s *ServerTaskFailRepositorySuite) TestServerTaskFailRepositoryFindAll() {
	ctx := context.Background()

	taskFail1 := &domain.ServerTaskFail{
		ServerTaskID: 10,
		Output:       "Output 1",
	}
	taskFail2 := &domain.ServerTaskFail{
		ServerTaskID: 11,
		Output:       "Output 2",
	}

	require.NoError(s.T(), s.repo.Save(ctx, taskFail1))
	require.NoError(s.T(), s.repo.Save(ctx, taskFail2))

	s.T().Run("find_all_task_issues", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  1,
			Offset: 0,
		}

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

func (s *ServerTaskFailRepositorySuite) TestServerTaskFailRepositoryFind() {
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	taskFail1 := &domain.ServerTaskFail{
		ServerTaskID: 100,
		Output:       "Task fail 1",
		CreatedAt:    lo.ToPtr(now.Add(-24 * time.Hour)),
		UpdatedAt:    lo.ToPtr(now.Add(-24 * time.Hour)),
	}
	require.NoError(s.T(), s.repo.Save(ctx, taskFail1))

	timeBetweenTaskFail1and2 := now.Add(-2 * time.Hour)

	taskFail2 := &domain.ServerTaskFail{
		ServerTaskID: 100,
		Output:       "Task fail 2",
		CreatedAt:    lo.ToPtr(timeBetweenTaskFail1and2.Add(1 * time.Minute)),
		UpdatedAt:    lo.ToPtr(timeBetweenTaskFail1and2.Add(1 * time.Minute)),
	}
	taskFail3 := &domain.ServerTaskFail{
		ServerTaskID: 200,
		Output:       "Task fail 3",
		CreatedAt:    lo.ToPtr(timeBetweenTaskFail1and2.Add(30 * time.Minute)),
		UpdatedAt:    lo.ToPtr(timeBetweenTaskFail1and2.Add(30 * time.Minute)),
	}
	require.NoError(s.T(), s.repo.Save(ctx, taskFail2))
	require.NoError(s.T(), s.repo.Save(ctx, taskFail3))

	endTime := time.Now().Add(1 * time.Hour)

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			IDs: []uint{taskFail1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, taskFail1.ID, results[0].ID)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			IDs: []uint{taskFail1.ID, taskFail3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, taskFail1.ID)
		assert.Contains(t, ids, taskFail3.ID)
	})

	s.T().Run("find_by_server_task_id", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			ServerTaskIDs: []uint{100},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.Equal(t, uint(100), result.ServerTaskID)
		}
	})

	s.T().Run("find_by_created_after", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			CreatedAfter: &timeBetweenTaskFail1and2,
		}

		results, err := s.repo.Find(ctx, filter, []filters.Sorting{
			{Field: "created_at", Direction: filters.SortDirectionAsc},
		}, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.GreaterOrEqual(t, len(results), 2)
		assert.Equal(t, taskFail2.ID, results[0].ID)
		assert.Equal(t, taskFail3.ID, results[1].ID)

		for _, result := range results {
			assert.False(t, result.CreatedAt.Before(timeBetweenTaskFail1and2), "CreatedAt %v should not be before %v", result.CreatedAt, timeBetweenTaskFail1and2)
		}
	})

	s.T().Run("find_by_created_before", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			CreatedBefore: &endTime,
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)

		for _, result := range results {
			assert.True(t, result.CreatedAt.Before(endTime) || result.CreatedAt.Equal(endTime))
		}
	})

	s.T().Run("find_with_combined_filters", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			ServerTaskIDs: []uint{100},
			CreatedAfter:  &timeBetweenTaskFail1and2,
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)

		for _, result := range results {
			assert.Equal(t, uint(100), result.ServerTaskID)
			assert.False(t, result.CreatedAt.Before(timeBetweenTaskFail1and2), "CreatedAt %v should not be before %v", result.CreatedAt, timeBetweenTaskFail1and2)
		}
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			IDs: []uint{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindServerTaskFail{
			ServerTaskIDs: []uint{100},
		}
		pagination := &filters.Pagination{
			Limit:  1,
			Offset: 0,
		}

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

func (s *ServerTaskFailRepositorySuite) TestServerTaskFailRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		serverTaskID := uint(7000)

		taskFail := &domain.ServerTaskFail{
			ServerTaskID: serverTaskID,
			Output:       "Initial output",
		}

		err := s.repo.Save(ctx, taskFail)
		require.NoError(t, err)
		assert.NotZero(t, taskFail.ID)

		filter := &filters.FindServerTaskFail{
			IDs: []uint{taskFail.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Initial output", results[0].Output)

		taskFail.Output = "Updated output"
		err = s.repo.Save(ctx, taskFail)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated output", results[0].Output)
	})

	s.T().Run("multiple_issues_for_same_task", func(t *testing.T) {
		serverTaskID := uint(8000)

		taskFails := []*domain.ServerTaskFail{
			{ServerTaskID: serverTaskID, Output: "Output 1"},
			{ServerTaskID: serverTaskID, Output: "Output 2"},
			{ServerTaskID: serverTaskID, Output: "Output 3"},
		}

		for _, tf := range taskFails {
			require.NoError(t, s.repo.Save(ctx, tf))
		}

		filter := &filters.FindServerTaskFail{
			ServerTaskIDs: []uint{serverTaskID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})
}
