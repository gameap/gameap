package hostlibrary

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/tetratelabs/wazero"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

var domainToProtoErrorPolicy = map[domain.PluginScheduledTaskErrorPolicy]scheduler.ErrorPolicyType{
	domain.PluginScheduledTaskErrorPolicyIgnore: scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_IGNORE,
	domain.PluginScheduledTaskErrorPolicyRetry:  scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_RETRY,
}

var protoToDomainErrorPolicy = map[scheduler.ErrorPolicyType]domain.PluginScheduledTaskErrorPolicy{
	scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_IGNORE: domain.PluginScheduledTaskErrorPolicyIgnore,
	scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_RETRY:  domain.PluginScheduledTaskErrorPolicyRetry,
}

type SchedulerServiceImpl struct {
	pluginID  uint64
	scheduler TaskScheduler
}

func NewSchedulerService(pluginID uint64, taskScheduler TaskScheduler) *SchedulerServiceImpl {
	return &SchedulerServiceImpl{
		pluginID:  pluginID,
		scheduler: taskScheduler,
	}
}

// AddTask registers a task. Expected failures go into the response fields:
// returning an error from a host function panics into a wazero trap that
// fails the whole guest call.
func (s *SchedulerServiceImpl) AddTask(
	ctx context.Context,
	req *scheduler.AddTaskRequest,
) (*scheduler.AddTaskResponse, error) {
	if s.pluginID == 0 {
		// Transient loads (dry-run, info discovery) must not mutate state.
		return &scheduler.AddTaskResponse{
			Success: false,
			Error:   new("plugin id is not known"),
		}, nil
	}

	task := domain.PluginScheduledTask{
		PluginID: domain.Uint64ID(s.pluginID),
		Name:     req.Name,
		Interval: time.Duration(req.IntervalMs) * time.Millisecond,
		Timeout:  time.Duration(req.TimeoutMs) * time.Millisecond,
	}

	if req.ErrorPolicy != nil {
		task.ErrorPolicy = protoToDomainErrorPolicy[req.ErrorPolicy.Policy]
		task.MaxRetries = uint(req.ErrorPolicy.MaxRetries)
		task.RetryDelay = time.Duration(req.ErrorPolicy.RetryDelayMs) * time.Millisecond
		task.MaxJitter = time.Duration(req.ErrorPolicy.MaxJitterMs) * time.Millisecond
	}

	if err := s.scheduler.AddTask(ctx, task); err != nil {
		return &scheduler.AddTaskResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &scheduler.AddTaskResponse{Success: true}, nil
}

func (s *SchedulerServiceImpl) RemoveTask(
	ctx context.Context,
	req *scheduler.RemoveTaskRequest,
) (*scheduler.RemoveTaskResponse, error) {
	if s.pluginID == 0 {
		return &scheduler.RemoveTaskResponse{
			Success: false,
			Error:   new("plugin id is not known"),
		}, nil
	}

	if err := s.scheduler.RemoveTask(ctx, domain.Uint64ID(s.pluginID), req.Name); err != nil {
		return &scheduler.RemoveTaskResponse{
			Success: false,
			Error:   new(err.Error()),
		}, nil
	}

	return &scheduler.RemoveTaskResponse{Success: true}, nil
}

func (s *SchedulerServiceImpl) ListTasks(
	ctx context.Context,
	_ *scheduler.ListTasksRequest,
) (*scheduler.ListTasksResponse, error) {
	if s.pluginID == 0 {
		return &scheduler.ListTasksResponse{}, nil
	}

	tasks, err := s.scheduler.ListTasks(ctx, domain.Uint64ID(s.pluginID))
	if err != nil {
		return nil, err
	}

	infos := make([]*scheduler.TaskInfo, 0, len(tasks))
	for _, task := range tasks {
		infos = append(infos, &scheduler.TaskInfo{
			Name:       task.Name,
			IntervalMs: task.Interval.Milliseconds(),
			ErrorPolicy: &scheduler.ErrorPolicy{
				Policy:       domainToProtoErrorPolicy[task.ErrorPolicy],
				MaxRetries:   uint32(task.MaxRetries), //nolint:gosec // G115: capped by scheduler validation
				RetryDelayMs: task.RetryDelay.Milliseconds(),
				MaxJitterMs:  task.MaxJitter.Milliseconds(),
			},
			TimeoutMs: task.Timeout.Milliseconds(),
		})
	}

	return &scheduler.ListTasksResponse{Tasks: infos}, nil
}

type SchedulerHostLibrary struct {
	impl *SchedulerServiceImpl
}

func NewSchedulerHostLibrary(pluginID uint64, taskScheduler TaskScheduler) *SchedulerHostLibrary {
	return &SchedulerHostLibrary{
		impl: NewSchedulerService(pluginID, taskScheduler),
	}
}

func (l *SchedulerHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return scheduler.Instantiate(ctx, r, l.impl)
}

type SchedulerHostLibraryFactory struct {
	scheduler TaskScheduler
}

func NewSchedulerHostLibraryFactory(taskScheduler TaskScheduler) *SchedulerHostLibraryFactory {
	return &SchedulerHostLibraryFactory{scheduler: taskScheduler}
}

func (f *SchedulerHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return NewSchedulerHostLibrary(pluginID, f.scheduler)
}
