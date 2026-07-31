package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PluginScheduledTaskRepositorySuite struct {
	suite.Suite

	repo repositories.PluginScheduledTaskRepository
	fn   func(t *testing.T) repositories.PluginScheduledTaskRepository
}

func NewPluginScheduledTaskRepositorySuite(
	fn func(t *testing.T) repositories.PluginScheduledTaskRepository,
) *PluginScheduledTaskRepositorySuite {
	return &PluginScheduledTaskRepositorySuite{
		fn: fn,
	}
}

func (s *PluginScheduledTaskRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func newScheduledTask(pluginID domain.Uint64ID, name string) *domain.PluginScheduledTask {
	return &domain.PluginScheduledTask{
		PluginID:    pluginID,
		Name:        name,
		Interval:    5 * time.Minute,
		ErrorPolicy: domain.PluginScheduledTaskErrorPolicyRetry,
		MaxRetries:  3,
		RetryDelay:  1500 * time.Millisecond,
		MaxJitter:   500 * time.Millisecond,
		Timeout:     10 * time.Second,
	}
}

func (s *PluginScheduledTaskRepositorySuite) TestPluginScheduledTaskRepositoryUpsert() {
	ctx := context.Background()

	s.T().Run("inserts_new_task_and_assigns_id", func(t *testing.T) {
		task := newScheduledTask(1, "stats-report")

		err := s.repo.Upsert(ctx, task)
		require.NoError(t, err)
		assert.NotZero(t, task.ID)
		assert.NotNil(t, task.CreatedAt)
		assert.NotNil(t, task.UpdatedAt)

		found, err := s.repo.FindByPlugin(ctx, 1)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, task.ID, found[0].ID)
		assert.Equal(t, domain.Uint64ID(1), found[0].PluginID)
		assert.Equal(t, "stats-report", found[0].Name)
		assert.Equal(t, 5*time.Minute, found[0].Interval)
		assert.Equal(t, domain.PluginScheduledTaskErrorPolicyRetry, found[0].ErrorPolicy)
		assert.Equal(t, uint(3), found[0].MaxRetries)
		assert.Equal(t, 1500*time.Millisecond, found[0].RetryDelay)
		assert.Equal(t, 500*time.Millisecond, found[0].MaxJitter)
		assert.Equal(t, 10*time.Second, found[0].Timeout)
		assert.NotNil(t, found[0].CreatedAt)
		assert.NotNil(t, found[0].UpdatedAt)
	})

	s.T().Run("updates_existing_task_on_same_plugin_and_name", func(t *testing.T) {
		first := newScheduledTask(2, "cleanup")
		require.NoError(t, s.repo.Upsert(ctx, first))
		firstCreatedAt := *first.CreatedAt

		updated := newScheduledTask(2, "cleanup")
		updated.Interval = 30 * time.Second
		updated.ErrorPolicy = domain.PluginScheduledTaskErrorPolicyIgnore
		updated.MaxRetries = 0
		updated.RetryDelay = 0
		updated.MaxJitter = 0
		updated.Timeout = time.Minute

		require.NoError(t, s.repo.Upsert(ctx, updated))
		assert.Equal(t, first.ID, updated.ID)
		require.NotNil(t, updated.CreatedAt)
		assert.WithinDuration(t, firstCreatedAt, *updated.CreatedAt, 2*time.Second,
			"upsert must write the persisted created_at back to the task")

		found, err := s.repo.FindByPlugin(ctx, 2)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, first.ID, found[0].ID)
		assert.Equal(t, 30*time.Second, found[0].Interval)
		assert.Equal(t, domain.PluginScheduledTaskErrorPolicyIgnore, found[0].ErrorPolicy)
		assert.Equal(t, uint(0), found[0].MaxRetries)
		assert.Equal(t, time.Minute, found[0].Timeout)
		require.NotNil(t, found[0].CreatedAt)
		assert.WithinDuration(t, firstCreatedAt, *found[0].CreatedAt, 2*time.Second)
		assert.NotNil(t, found[0].UpdatedAt)
	})

	s.T().Run("same_name_for_different_plugins_creates_separate_rows", func(t *testing.T) {
		taskA := newScheduledTask(3, "shared-name")
		taskB := newScheduledTask(4, "shared-name")

		require.NoError(t, s.repo.Upsert(ctx, taskA))
		require.NoError(t, s.repo.Upsert(ctx, taskB))
		assert.NotEqual(t, taskA.ID, taskB.ID)

		foundA, err := s.repo.FindByPlugin(ctx, 3)
		require.NoError(t, err)
		require.Len(t, foundA, 1)

		foundB, err := s.repo.FindByPlugin(ctx, 4)
		require.NoError(t, err)
		require.Len(t, foundB, 1)
	})

	s.T().Run("zero_timeout_round_trips", func(t *testing.T) {
		task := newScheduledTask(5, "default-timeout")
		task.Timeout = 0

		require.NoError(t, s.repo.Upsert(ctx, task))

		found, err := s.repo.FindByPlugin(ctx, 5)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, time.Duration(0), found[0].Timeout)
	})
}

func (s *PluginScheduledTaskRepositorySuite) TestPluginScheduledTaskRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("deletes_task_by_plugin_and_name", func(t *testing.T) {
		task := newScheduledTask(10, "to-delete")
		require.NoError(t, s.repo.Upsert(ctx, task))

		require.NoError(t, s.repo.Delete(ctx, 10, "to-delete"))

		found, err := s.repo.FindByPlugin(ctx, 10)
		require.NoError(t, err)
		require.Len(t, found, 0)
	})

	s.T().Run("keeps_other_tasks_of_same_plugin", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(11, "first")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(11, "second")))

		require.NoError(t, s.repo.Delete(ctx, 11, "first"))

		found, err := s.repo.FindByPlugin(ctx, 11)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, "second", found[0].Name)
	})

	s.T().Run("keeps_same_name_task_of_other_plugin", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(12, "shared")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(13, "shared")))

		require.NoError(t, s.repo.Delete(ctx, 12, "shared"))

		found, err := s.repo.FindByPlugin(ctx, 13)
		require.NoError(t, err)
		require.Len(t, found, 1)
	})

	s.T().Run("missing_task_is_noop", func(t *testing.T) {
		require.NoError(t, s.repo.Delete(ctx, 14, "does-not-exist"))
	})
}

func (s *PluginScheduledTaskRepositorySuite) TestPluginScheduledTaskRepositoryDeleteByPlugin() {
	ctx := context.Background()

	s.T().Run("deletes_all_tasks_and_returns_count", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(20, "one")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(20, "two")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(20, "three")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(21, "untouched")))

		deleted, err := s.repo.DeleteByPlugin(ctx, 20)
		require.NoError(t, err)
		assert.Equal(t, 3, deleted)

		found, err := s.repo.FindByPlugin(ctx, 20)
		require.NoError(t, err)
		require.Len(t, found, 0)

		other, err := s.repo.FindByPlugin(ctx, 21)
		require.NoError(t, err)
		require.Len(t, other, 1)
	})

	s.T().Run("returns_zero_for_plugin_without_tasks", func(t *testing.T) {
		deleted, err := s.repo.DeleteByPlugin(ctx, 22)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})
}

func (s *PluginScheduledTaskRepositorySuite) TestPluginScheduledTaskRepositoryFindAll() {
	ctx := context.Background()

	s.T().Run("returns_all_tasks_ordered_by_plugin_and_name", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(31, "beta")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(30, "zulu")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(31, "alpha")))

		found, err := s.repo.FindAll(ctx)
		require.NoError(t, err)
		require.Len(t, found, 3)
		assert.Equal(t, domain.Uint64ID(30), found[0].PluginID)
		assert.Equal(t, "zulu", found[0].Name)
		assert.Equal(t, domain.Uint64ID(31), found[1].PluginID)
		assert.Equal(t, "alpha", found[1].Name)
		assert.Equal(t, domain.Uint64ID(31), found[2].PluginID)
		assert.Equal(t, "beta", found[2].Name)
	})
}

func (s *PluginScheduledTaskRepositorySuite) TestPluginScheduledTaskRepositoryFindAllEmpty() {
	ctx := context.Background()

	found, err := s.repo.FindAll(ctx)
	s.Require().NoError(err)
	s.Require().Len(found, 0)
}

func (s *PluginScheduledTaskRepositorySuite) TestPluginScheduledTaskRepositoryFindByPlugin() {
	ctx := context.Background()

	s.T().Run("returns_only_tasks_of_requested_plugin", func(t *testing.T) {
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(40, "mine")))
		require.NoError(t, s.repo.Upsert(ctx, newScheduledTask(41, "other")))

		found, err := s.repo.FindByPlugin(ctx, 40)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, "mine", found[0].Name)
	})

	s.T().Run("returns_empty_for_unknown_plugin", func(t *testing.T) {
		found, err := s.repo.FindByPlugin(ctx, 42)
		require.NoError(t, err)
		require.Len(t, found, 0)
	})
}
