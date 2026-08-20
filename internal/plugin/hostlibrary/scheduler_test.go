package hostlibrary

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTaskScheduler struct {
	addTaskFunc    func(ctx context.Context, task domain.PluginScheduledTask) error
	removeTaskFunc func(ctx context.Context, pluginID domain.Uint64ID, name string) error
	listTasksFunc  func(ctx context.Context, pluginID domain.Uint64ID) ([]domain.PluginScheduledTask, error)

	addedTasks []domain.PluginScheduledTask
	removed    []string
}

func (s *stubTaskScheduler) AddTask(ctx context.Context, task domain.PluginScheduledTask) error {
	s.addedTasks = append(s.addedTasks, task)

	if s.addTaskFunc != nil {
		return s.addTaskFunc(ctx, task)
	}

	return nil
}

func (s *stubTaskScheduler) RemoveTask(ctx context.Context, pluginID domain.Uint64ID, name string) error {
	s.removed = append(s.removed, name)

	if s.removeTaskFunc != nil {
		return s.removeTaskFunc(ctx, pluginID, name)
	}

	return nil
}

func (s *stubTaskScheduler) ListTasks(
	ctx context.Context,
	pluginID domain.Uint64ID,
) ([]domain.PluginScheduledTask, error) {
	if s.listTasksFunc != nil {
		return s.listTasksFunc(ctx, pluginID)
	}

	return nil, nil
}

func TestSchedulerService_AddTask(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		pluginID   uint64
		req        *scheduler.AddTaskRequest
		addErr     error
		wantOK     bool
		wantError  string
		wantAdded  int
		verifyTask func(t *testing.T, task domain.PluginScheduledTask)
	}{
		{
			name:     "maps_all_fields_to_domain",
			pluginID: 42,
			req: &scheduler.AddTaskRequest{
				Name:       "stats-report",
				IntervalMs: 60_000,
				ErrorPolicy: &scheduler.ErrorPolicy{
					Policy:       scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_RETRY,
					MaxRetries:   3,
					RetryDelayMs: 1_500,
					MaxJitterMs:  500,
				},
				TimeoutMs: 10_000,
			},
			wantOK:    true,
			wantAdded: 1,
			verifyTask: func(t *testing.T, task domain.PluginScheduledTask) {
				t.Helper()
				assert.Equal(t, domain.Uint64ID(42), task.PluginID)
				assert.Equal(t, "stats-report", task.Name)
				assert.Equal(t, time.Minute, task.Interval)
				assert.Equal(t, domain.PluginScheduledTaskErrorPolicyRetry, task.ErrorPolicy)
				assert.Equal(t, uint(3), task.MaxRetries)
				assert.Equal(t, 1500*time.Millisecond, task.RetryDelay)
				assert.Equal(t, 500*time.Millisecond, task.MaxJitter)
				assert.Equal(t, 10*time.Second, task.Timeout)
			},
		},
		{
			name:     "missing_policy_leaves_domain_defaults",
			pluginID: 42,
			req: &scheduler.AddTaskRequest{
				Name:       "no-policy",
				IntervalMs: 60_000,
			},
			wantOK:    true,
			wantAdded: 1,
			verifyTask: func(t *testing.T, task domain.PluginScheduledTask) {
				t.Helper()
				assert.Equal(t, domain.PluginScheduledTaskErrorPolicy(""), task.ErrorPolicy)
				assert.Equal(t, uint(0), task.MaxRetries)
				assert.Equal(t, time.Duration(0), task.Timeout)
			},
		},
		{
			name:      "scheduler_error_goes_into_response",
			pluginID:  42,
			req:       &scheduler.AddTaskRequest{Name: "bad", IntervalMs: 10},
			addErr:    errors.New("interval 10ms is below the minimum 1s"),
			wantOK:    false,
			wantError: "below the minimum",
			wantAdded: 1,
		},
		{
			name:      "zero_plugin_id_rejected_without_scheduler_call",
			pluginID:  0,
			req:       &scheduler.AddTaskRequest{Name: "dry-run", IntervalMs: 60_000},
			wantOK:    false,
			wantError: "plugin id is not known",
			wantAdded: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stub := &stubTaskScheduler{}
			if tt.addErr != nil {
				stub.addTaskFunc = func(context.Context, domain.PluginScheduledTask) error {
					return tt.addErr
				}
			}

			svc := NewSchedulerService(tt.pluginID, stub)

			resp, err := svc.AddTask(context.Background(), tt.req)

			require.NoError(t, err, "expected failures must be reported in the response, not as a trap")
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantOK, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
			}

			require.Len(t, stub.addedTasks, tt.wantAdded)
			if tt.verifyTask != nil {
				tt.verifyTask(t, stub.addedTasks[0])
			}
		})
	}
}

func TestSchedulerService_RemoveTask(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		pluginID    uint64
		removeErr   error
		wantOK      bool
		wantError   string
		wantRemoved int
	}{
		{name: "removes_task", pluginID: 42, wantOK: true, wantRemoved: 1},
		{
			name:        "scheduler_error_goes_into_response",
			pluginID:    42,
			removeErr:   errors.New("db is down"),
			wantOK:      false,
			wantError:   "db is down",
			wantRemoved: 1,
		},
		{
			name:      "zero_plugin_id_rejected_without_scheduler_call",
			pluginID:  0,
			wantOK:    false,
			wantError: "plugin id is not known",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stub := &stubTaskScheduler{}
			if tt.removeErr != nil {
				stub.removeTaskFunc = func(context.Context, domain.Uint64ID, string) error {
					return tt.removeErr
				}
			}

			svc := NewSchedulerService(tt.pluginID, stub)

			resp, err := svc.RemoveTask(context.Background(), &scheduler.RemoveTaskRequest{Name: "stats-report"})

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantOK, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			}

			require.Len(t, stub.removed, tt.wantRemoved)
		})
	}
}

func TestSchedulerService_ListTasks(t *testing.T) {
	t.Parallel()
	t.Run("maps_domain_tasks_to_proto", func(t *testing.T) {
		t.Parallel()
		stub := &stubTaskScheduler{
			listTasksFunc: func(_ context.Context, pluginID domain.Uint64ID) ([]domain.PluginScheduledTask, error) {
				assert.Equal(t, domain.Uint64ID(42), pluginID)

				return []domain.PluginScheduledTask{
					{
						PluginID:    42,
						Name:        "stats-report",
						Interval:    time.Minute,
						ErrorPolicy: domain.PluginScheduledTaskErrorPolicyRetry,
						MaxRetries:  3,
						RetryDelay:  1500 * time.Millisecond,
						MaxJitter:   500 * time.Millisecond,
						Timeout:     10 * time.Second,
					},
				}, nil
			},
		}
		svc := NewSchedulerService(42, stub)

		resp, err := svc.ListTasks(context.Background(), &scheduler.ListTasksRequest{})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Tasks, 1)
		task := resp.Tasks[0]
		assert.Equal(t, "stats-report", task.Name)
		assert.Equal(t, int64(60_000), task.IntervalMs)
		require.NotNil(t, task.ErrorPolicy)
		assert.Equal(t, scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_RETRY, task.ErrorPolicy.Policy)
		assert.Equal(t, uint32(3), task.ErrorPolicy.MaxRetries)
		assert.Equal(t, int64(1_500), task.ErrorPolicy.RetryDelayMs)
		assert.Equal(t, int64(500), task.ErrorPolicy.MaxJitterMs)
		assert.Equal(t, int64(10_000), task.TimeoutMs)
	})

	t.Run("zero_plugin_id_returns_empty_list", func(t *testing.T) {
		t.Parallel()
		svc := NewSchedulerService(0, &stubTaskScheduler{
			listTasksFunc: func(context.Context, domain.Uint64ID) ([]domain.PluginScheduledTask, error) {
				t.Fatal("scheduler must not be called for transient loads")

				return nil, nil
			},
		})

		resp, err := svc.ListTasks(context.Background(), &scheduler.ListTasksRequest{})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Tasks, 0)
	})

	t.Run("scheduler_error_traps_the_call", func(t *testing.T) {
		t.Parallel()
		svc := NewSchedulerService(42, &stubTaskScheduler{
			listTasksFunc: func(context.Context, domain.Uint64ID) ([]domain.PluginScheduledTask, error) {
				return nil, errors.New("db is down")
			},
		})

		resp, err := svc.ListTasks(context.Background(), &scheduler.ListTasksRequest{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "db is down")
		assert.Nil(t, resp)
	})
}

func TestSchedulerHostLibraryFactory_Create(t *testing.T) {
	t.Parallel()
	factory := NewSchedulerHostLibraryFactory(&stubTaskScheduler{})

	lib := factory.Create(42)

	require.NotNil(t, lib)
	hostLib, ok := lib.(*SchedulerHostLibrary)
	require.True(t, ok)
	assert.Equal(t, uint64(42), hostLib.impl.pluginID)
}
