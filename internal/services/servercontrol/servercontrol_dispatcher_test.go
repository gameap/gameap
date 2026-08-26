package servercontrol

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errTestDispatch    = errors.New("simulated dispatch failure")
	errTestTaskExists  = errors.New("simulated task exists failure")
	errTestSettingFind = errors.New("simulated setting find failure")
	errTestSettingSave = errors.New("simulated setting save failure")
)

type recordedPluginEvent struct {
	eventType PluginEventType
	serverID  uint
}

type recordingPluginDispatcher struct {
	mu          sync.Mutex
	result      *PluginDispatchResult
	syncEvents  []recordedPluginEvent
	asyncEvents []recordedPluginEvent
}

func (d *recordingPluginDispatcher) DispatchServerEvent(
	_ context.Context,
	eventType PluginEventType,
	server *domain.Server,
	_ map[string]string,
) *PluginDispatchResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.syncEvents = append(d.syncEvents, recordedPluginEvent{eventType: eventType, serverID: server.ID})

	return d.result
}

func (d *recordingPluginDispatcher) DispatchServerEventAsync(
	_ context.Context,
	eventType PluginEventType,
	server *domain.Server,
	_ map[string]string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.asyncEvents = append(d.asyncEvents, recordedPluginEvent{eventType: eventType, serverID: server.ID})
}

type stubTaskDispatcher struct {
	mu     sync.Mutex
	tasks  []domain.DaemonTask
	failOn map[domain.DaemonTaskType]error
}

func (d *stubTaskDispatcher) Dispatch(_ context.Context, task *domain.DaemonTask) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err, ok := d.failOn[task.Task]; ok {
		return err
	}

	d.tasks = append(d.tasks, *task)

	return nil
}

func (d *stubTaskDispatcher) dispatchedTypes() []domain.DaemonTaskType {
	d.mu.Lock()
	defer d.mu.Unlock()

	types := make([]domain.DaemonTaskType, 0, len(d.tasks))
	for _, task := range d.tasks {
		types = append(types, task.Task)
	}

	return types
}

// errorDaemonTaskRepo wraps a real repository and fails only Exists.
type errorDaemonTaskRepo struct {
	repositories.DaemonTaskRepository

	existsErr error
}

func (r *errorDaemonTaskRepo) Exists(ctx context.Context, filter *filters.FindDaemonTask) (bool, error) {
	if r.existsErr != nil {
		return false, r.existsErr
	}

	return r.DaemonTaskRepository.Exists(ctx, filter)
}

// errorServerSettingRepo wraps a real repository and fails Find/Save on demand.
type errorServerSettingRepo struct {
	repositories.ServerSettingRepository

	findErr error
	saveErr error
}

func (r *errorServerSettingRepo) Find(
	ctx context.Context,
	filter *filters.FindServerSetting,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerSetting, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}

	return r.ServerSettingRepository.Find(ctx, filter, order, pagination)
}

func (r *errorServerSettingRepo) Save(ctx context.Context, setting *domain.ServerSetting) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.ServerSettingRepository.Save(ctx, setting)
}

type controlOperation func(s *Service, ctx context.Context, server *domain.Server) (uint, error)

func allControlOperations() map[string]controlOperation {
	return map[string]controlOperation{
		"start":     (*Service).Start,
		"stop":      (*Service).Stop,
		"restart":   (*Service).Restart,
		"update":    (*Service).Update,
		"install":   (*Service).Install,
		"reinstall": (*Service).Reinstall,
	}
}

func findAllTasks(t *testing.T, repo *inmemory.DaemonTaskRepository) []domain.DaemonTask {
	t.Helper()

	tasks, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)

	return tasks
}

func TestServerControlService_PluginEventCancelsOperation(t *testing.T) {
	t.Parallel()

	for name, op := range allControlOperations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()
			dispatcher := &recordingPluginDispatcher{
				result: &PluginDispatchResult{
					Cancelled:     true,
					CancelledBy:   "test-plugin",
					CancelMessage: "not allowed now",
				},
			}
			service := NewService(
				taskRepo, settingRepo, services.NewNilTransactionManager(),
				WithPluginDispatcher(dispatcher),
			)
			server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

			// ACT
			taskID, err := op(service, context.Background(), server)

			// ASSERT
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrCancelledByPlugin, "error must wrap ErrCancelledByPlugin")
			assert.Contains(t, err.Error(), "cancelled by test-plugin: not allowed now")
			assert.Zero(t, taskID)
			assert.Empty(t, findAllTasks(t, taskRepo), "cancelled operation must not create tasks")
			require.Len(t, dispatcher.syncEvents, 1, "exactly one pre-event must be dispatched")
			assert.Empty(t, dispatcher.asyncEvents, "cancelled operation must not dispatch post-events")
		})
	}
}

func TestServerControlService_PluginCancelMessageFallback(t *testing.T) {
	t.Parallel()

	// ARRANGE — without CancelMessage the CancelledBy value is used as the message.
	settingRepo := inmemory.NewServerSettingRepository()
	taskRepo := inmemory.NewDaemonTaskRepository()
	dispatcher := &recordingPluginDispatcher{
		result: &PluginDispatchResult{
			Cancelled:   true,
			CancelledBy: "guard-plugin",
		},
	}
	service := NewService(
		taskRepo, settingRepo, services.NewNilTransactionManager(),
		WithPluginDispatcher(dispatcher),
	)
	server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

	// ACT
	_, err := service.Start(context.Background(), server)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled by guard-plugin: guard-plugin")
}

func TestServerControlService_PluginEventsDispatched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    *PluginDispatchResult
		wantSync  []PluginEventType
		wantAsync []PluginEventType
	}{
		{
			name:      "allowed_result_dispatches_pre_and_post_events",
			result:    &PluginDispatchResult{Cancelled: false},
			wantSync:  []PluginEventType{PluginEventServerPreStart},
			wantAsync: []PluginEventType{PluginEventServerPostStart},
		},
		{
			name:      "nil_result_is_treated_as_allowed",
			result:    nil,
			wantSync:  []PluginEventType{PluginEventServerPreStart},
			wantAsync: []PluginEventType{PluginEventServerPostStart},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()
			dispatcher := &recordingPluginDispatcher{result: tt.result}
			service := NewService(
				taskRepo, settingRepo, services.NewNilTransactionManager(),
				WithPluginDispatcher(dispatcher),
			)
			server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

			// ACT
			taskID, err := service.Start(context.Background(), server)

			// ASSERT
			require.NoError(t, err)
			assert.NotZero(t, taskID)

			syncTypes := make([]PluginEventType, 0, len(dispatcher.syncEvents))
			for _, e := range dispatcher.syncEvents {
				syncTypes = append(syncTypes, e.eventType)
				assert.Equal(t, uint(1), e.serverID)
			}
			assert.Equal(t, tt.wantSync, syncTypes)

			asyncTypes := make([]PluginEventType, 0, len(dispatcher.asyncEvents))
			for _, e := range dispatcher.asyncEvents {
				asyncTypes = append(asyncTypes, e.eventType)
				assert.Equal(t, uint(1), e.serverID)
			}
			assert.Equal(t, tt.wantAsync, asyncTypes)
		})
	}
}

func TestServerControlService_TaskDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("dispatches_task_via_grpc_instead_of_saving", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := inmemory.NewServerSettingRepository()
		taskRepo := inmemory.NewDaemonTaskRepository()
		dispatcher := &stubTaskDispatcher{}
		service := NewService(
			taskRepo, settingRepo, services.NewNilTransactionManager(),
			WithTaskDispatcher(dispatcher),
		)
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		taskID, err := service.Start(context.Background(), server)

		// ASSERT
		require.NoError(t, err)
		assert.Zero(t, taskID, "with gRPC dispatch the task is not persisted locally, so no ID is assigned")
		assert.Equal(t,
			[]domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
			dispatcher.dispatchedTypes(),
		)
		require.Len(t, dispatcher.tasks, 1)
		assert.Equal(t, domain.DaemonTaskStatusWaiting, dispatcher.tasks[0].Status)
		assert.Equal(t, uint(10), dispatcher.tasks[0].DedicatedServerID)
		assert.Empty(t, findAllTasks(t, taskRepo), "dispatched task must not land in the repository")
	})

	t.Run("dispatch_error_is_wrapped", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := inmemory.NewServerSettingRepository()
		taskRepo := inmemory.NewDaemonTaskRepository()
		dispatcher := &stubTaskDispatcher{
			failOn: map[domain.DaemonTaskType]error{
				domain.DaemonTaskTypeServerStart: errTestDispatch,
			},
		}
		service := NewService(
			taskRepo, settingRepo, services.NewNilTransactionManager(),
			WithTaskDispatcher(dispatcher),
		)
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		taskID, err := service.Start(context.Background(), server)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to dispatch daemon task")
		assert.ErrorIs(t, err, errTestDispatch)
		assert.Zero(t, taskID)
		assert.Empty(t, findAllTasks(t, taskRepo))
	})
}

func TestServerControlService_ReinstallDispatchErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		failOn      domain.DaemonTaskType
		errContains string
	}{
		{
			name:        "stop_task_dispatch_failure",
			failOn:      domain.DaemonTaskTypeServerStop,
			errContains: "failed to create stop task",
		},
		{
			name:        "delete_task_dispatch_failure",
			failOn:      domain.DaemonTaskTypeServerDelete,
			errContains: "failed to create delete task",
		},
		{
			name:        "install_task_dispatch_failure",
			failOn:      domain.DaemonTaskTypeServerInstall,
			errContains: "failed to create install task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()
			dispatcher := &stubTaskDispatcher{
				failOn: map[domain.DaemonTaskType]error{tt.failOn: errTestDispatch},
			}
			service := NewService(
				taskRepo, settingRepo, services.NewNilTransactionManager(),
				WithTaskDispatcher(dispatcher),
			)
			server := &domain.Server{ID: 1, DSID: 10}

			// ACT
			taskID, err := service.Reinstall(context.Background(), server)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
			assert.ErrorIs(t, err, errTestDispatch)
			assert.Zero(t, taskID)
		})
	}
}

func TestServerControlService_RepositoryErrors(t *testing.T) {
	t.Parallel()

	t.Run("task_existence_check_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := inmemory.NewServerSettingRepository()
		taskRepo := &errorDaemonTaskRepo{
			DaemonTaskRepository: inmemory.NewDaemonTaskRepository(),
			existsErr:            errTestTaskExists,
		}
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		taskID, err := service.Start(context.Background(), server)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check daemon task existence")
		assert.ErrorIs(t, err, errTestTaskExists)
		assert.Zero(t, taskID)
	})

	t.Run("autostart_setting_find_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := &errorServerSettingRepo{
			ServerSettingRepository: inmemory.NewServerSettingRepository(),
			findErr:                 errTestSettingFind,
		}
		taskRepo := inmemory.NewDaemonTaskRepository()
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		taskID, err := service.Start(context.Background(), server)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get autostart setting")
		assert.ErrorIs(t, err, errTestSettingFind)
		assert.Zero(t, taskID)
	})

	t.Run("autostart_current_setting_find_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := &errorServerSettingRepo{
			ServerSettingRepository: inmemory.NewServerSettingRepository(),
			findErr:                 errTestSettingFind,
		}
		taskRepo := inmemory.NewDaemonTaskRepository()
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10}

		// ACT
		taskID, err := service.Stop(context.Background(), server)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get autostart_current setting")
		assert.ErrorIs(t, err, errTestSettingFind)
		assert.Zero(t, taskID)
	})

	t.Run("autostart_current_setting_save_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := &errorServerSettingRepo{
			ServerSettingRepository: inmemory.NewServerSettingRepository(),
			saveErr:                 errTestSettingSave,
		}
		taskRepo := inmemory.NewDaemonTaskRepository()
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10}

		// ACT
		taskID, err := service.Stop(context.Background(), server)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save autostart_current setting")
		assert.ErrorIs(t, err, errTestSettingSave)
		assert.Zero(t, taskID)
	})
}

func TestServerControlService_AutostartCurrentBranches(t *testing.T) {
	t.Parallel()

	t.Run("autostart_disabled_skips_autostart_current", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := inmemory.NewServerSettingRepository()
		require.NoError(t, settingRepo.Save(context.Background(), &domain.ServerSetting{
			ServerID: 1,
			Name:     autostartSettingKey,
			Value:    domain.NewServerSettingValue(false),
		}))
		taskRepo := inmemory.NewDaemonTaskRepository()
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		_, err := service.Start(context.Background(), server)

		// ASSERT
		require.NoError(t, err)
		settings, findErr := settingRepo.Find(context.Background(), &filters.FindServerSetting{
			ServerIDs: []uint{1},
			Names:     []string{autostartCurrentSettingKey},
		}, nil, nil)
		require.NoError(t, findErr)
		assert.Empty(t, settings, "autostart_current must stay unset when autostart is disabled")
	})

	t.Run("non_boolean_autostart_value_skips_autostart_current", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := inmemory.NewServerSettingRepository()
		require.NoError(t, settingRepo.Save(context.Background(), &domain.ServerSetting{
			ServerID: 1,
			Name:     autostartSettingKey,
			Value:    domain.NewServerSettingValue("yes"),
		}))
		taskRepo := inmemory.NewDaemonTaskRepository()
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		_, err := service.Start(context.Background(), server)

		// ASSERT
		require.NoError(t, err)
		settings, findErr := settingRepo.Find(context.Background(), &filters.FindServerSetting{
			ServerIDs: []uint{1},
			Names:     []string{autostartCurrentSettingKey},
		}, nil, nil)
		require.NoError(t, findErr)
		assert.Empty(t, settings, "a non-boolean autostart value must not enable autostart_current")
	})

	t.Run("existing_autostart_current_is_updated", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		settingRepo := inmemory.NewServerSettingRepository()
		require.NoError(t, settingRepo.Save(context.Background(), &domain.ServerSetting{
			ServerID: 1,
			Name:     autostartSettingKey,
			Value:    domain.NewServerSettingValue(true),
		}))
		require.NoError(t, settingRepo.Save(context.Background(), &domain.ServerSetting{
			ServerID: 1,
			Name:     autostartCurrentSettingKey,
			Value:    domain.NewServerSettingValue(false),
		}))
		taskRepo := inmemory.NewDaemonTaskRepository()
		service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())
		server := &domain.Server{ID: 1, DSID: 10, StartCommand: new("./start.sh")}

		// ACT
		_, err := service.Start(context.Background(), server)

		// ASSERT
		require.NoError(t, err)
		settings, findErr := settingRepo.Find(context.Background(), &filters.FindServerSetting{
			ServerIDs: []uint{1},
			Names:     []string{autostartCurrentSettingKey},
		}, nil, nil)
		require.NoError(t, findErr)
		require.Len(t, settings, 1, "the existing setting must be updated, not duplicated")

		value, ok := settings[0].Value.Bool()
		require.True(t, ok)
		assert.True(t, value, "autostart_current must be flipped to true on start")
	})
}
