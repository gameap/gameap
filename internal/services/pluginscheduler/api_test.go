package pluginscheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTask_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(task *domain.PluginScheduledTask)
		wantError string
	}{
		{
			name:      "plugin_id_zero_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.PluginID = 0 },
			wantError: "plugin id is not known",
		},
		{
			name:      "empty_name_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.Name = "" },
			wantError: "task name must match",
		},
		{
			name:      "name_with_dot_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.Name = "bad.name" },
			wantError: "task name must match",
		},
		{
			name:      "name_starting_with_dash_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.Name = "-bad" },
			wantError: "task name must match",
		},
		{
			name:      "name_longer_than_64_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.Name = strings.Repeat("a", 65) },
			wantError: "task name must match",
		},
		{
			name:      "interval_below_minimum_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.Interval = 100 * time.Millisecond },
			wantError: "below the minimum",
		},
		{
			name:      "unknown_policy_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.ErrorPolicy = "explode" },
			wantError: "unknown error policy",
		},
		{
			name: "retries_above_cap_rejected",
			mutate: func(task *domain.PluginScheduledTask) {
				task.ErrorPolicy = domain.PluginScheduledTaskErrorPolicyRetry
				task.MaxRetries = 100
			},
			wantError: "exceeds the limit",
		},
		{
			name:      "negative_retry_delay_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.RetryDelay = -time.Second },
			wantError: "retry delay must be within",
		},
		{
			name:      "retry_delay_above_cap_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.RetryDelay = time.Hour },
			wantError: "retry delay must be within",
		},
		{
			name:      "jitter_above_cap_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.MaxJitter = time.Hour },
			wantError: "retry jitter must be within",
		},
		{
			name:      "negative_timeout_rejected",
			mutate:    func(task *domain.PluginScheduledTask) { task.Timeout = -time.Second },
			wantError: "timeout must not be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := newTestEnv(t, nil)
			task := testTask(time.Minute)
			tt.mutate(&task)

			err := env.service.AddTask(context.Background(), task)

			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidTask))
			assert.Contains(t, err.Error(), tt.wantError)

			stored, repoErr := env.repo.FindAll(context.Background())
			require.NoError(t, repoErr)
			require.Len(t, stored, 0, "rejected tasks must not be persisted")
		})
	}
}

func TestAddTask_PersistsArmsAndDefaults(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newTestEnv(t, nil)
	task := testTask(time.Minute)
	task.ErrorPolicy = ""
	task.Timeout = time.Hour

	// ACT
	require.NoError(t, env.service.AddTask(context.Background(), task))

	// ASSERT
	stored, err := env.repo.FindByPlugin(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, domain.PluginScheduledTaskErrorPolicyIgnore, stored[0].ErrorPolicy,
		"empty policy defaults to ignore")
	assert.Equal(t, defaultMaxCallTimeout, stored[0].Timeout,
		"timeout above the ceiling is clamped, not rejected")

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	st, ok := env.service.tasks[taskKey{pluginID: 1, name: "stats-report"}]
	require.True(t, ok)
	assert.Equal(t, nextSlot(env.clock.Now(), time.Minute), st.nextAt)
}

func TestAddTask_LimitPerPlugin(t *testing.T) {
	t.Parallel()

	const firstTaskName = "first"

	env := newTestEnv(t, func(opts *Options) { opts.MaxTasksPerPlugin = 2 })
	ctx := context.Background()

	first := testTask(time.Minute)
	first.Name = firstTaskName
	require.NoError(t, env.service.AddTask(ctx, first))

	second := testTask(time.Minute)
	second.Name = "second"
	require.NoError(t, env.service.AddTask(ctx, second))

	third := testTask(time.Minute)
	third.Name = "third"
	err := env.service.AddTask(ctx, third)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTaskLimitReached))

	updated := testTask(30 * time.Minute)
	updated.Name = firstTaskName
	require.NoError(t, env.service.AddTask(ctx, updated),
		"an upsert of an existing name passes the limit check")
}

func TestAddTask_UpsertPreservesRunningFlag(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	ctx := context.Background()
	key := taskKey{pluginID: 1, name: "stats-report"}

	require.NoError(t, env.service.AddTask(ctx, testTask(time.Minute)))

	env.service.mu.Lock()
	env.service.tasks[key].running = true
	env.service.mu.Unlock()

	require.NoError(t, env.service.AddTask(ctx, testTask(2*time.Minute)))

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.True(t, env.service.tasks[key].running,
		"re-registration must not drop the local overlap guard")
	assert.Equal(t, 2*time.Minute, env.service.tasks[key].task.Interval)
}

func TestRemoveTask(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	ctx := context.Background()

	require.NoError(t, env.service.AddTask(ctx, testTask(time.Minute)))
	require.NoError(t, env.service.RemoveTask(ctx, 1, "stats-report"))

	stored, err := env.repo.FindByPlugin(ctx, 1)
	require.NoError(t, err)
	require.Len(t, stored, 0)

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.Len(t, env.service.tasks, 0)
}

func TestRemovePluginTasks(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	ctx := context.Background()

	first := testTask(time.Minute)
	first.Name = "first"
	require.NoError(t, env.service.AddTask(ctx, first))

	second := testTask(time.Minute)
	second.Name = "second"
	require.NoError(t, env.service.AddTask(ctx, second))

	foreign := testTask(time.Minute)
	foreign.PluginID = 2
	foreign.Name = "foreign"
	require.NoError(t, env.repo.Upsert(ctx, &foreign))

	deleted, err := env.service.RemovePluginTasks(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	stored, err := env.repo.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "foreign", stored[0].Name)

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.Len(t, env.service.tasks, 0)
}

func TestListTasks_ReadsFromRepository(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	ctx := context.Background()

	// A task written by another instance exists only in the DB.
	foreign := testTask(time.Minute)
	foreign.Name = "from-other-instance"
	require.NoError(t, env.repo.Upsert(ctx, &foreign))

	tasks, err := env.service.ListTasks(ctx, 1)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "from-other-instance", tasks[0].Name)
}
