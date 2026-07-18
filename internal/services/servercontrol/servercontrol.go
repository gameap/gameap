package servercontrol

import (
	"context"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

const (
	autostartSettingKey        = "autostart"
	autostartCurrentSettingKey = "autostart_current"
)

var (
	ErrAnotherTaskAlreadyExists      = errors.New("another task already exists, please wait until it is completed")
	ErrEmptyServerStartCommand       = errors.New("empty server start command")
	ErrServerUpdateInstallInProgress = errors.New("server update/install task is already in progress")
	ErrCancelledByPlugin             = errors.New("operation cancelled by plugin")
)

// PluginEventType represents the type of plugin event.
type PluginEventType int

const (
	PluginEventServerPreStart PluginEventType = iota
	PluginEventServerPostStart
	PluginEventServerPreStop
	PluginEventServerPostStop
	PluginEventServerPreRestart
	PluginEventServerPostRestart
	PluginEventServerPreInstall
	PluginEventServerPostInstall
	PluginEventServerPreUpdate
	PluginEventServerPostUpdate
	PluginEventServerPreReinstall
	PluginEventServerPostReinstall
	PluginEventServerPreDelete
	PluginEventServerPostDelete
	PluginEventServerCreated
	PluginEventServerUpdated
	PluginEventServerDeleted
)

// PluginDispatchResult contains the result of dispatching an event.
type PluginDispatchResult struct {
	Cancelled     bool
	CancelledBy   string
	CancelMessage string
}

// PluginDispatcher is an interface for dispatching plugin events.
type PluginDispatcher interface {
	// DispatchServerEvent dispatches synchronously; used for cancellable
	// pre-events where the result matters.
	DispatchServerEvent(
		ctx context.Context,
		eventType PluginEventType,
		server *domain.Server,
		extraData map[string]string,
	) *PluginDispatchResult

	// DispatchServerEventAsync dispatches in the background (fire-and-forget);
	// used for post-events so plugins cannot delay the caller.
	DispatchServerEventAsync(
		ctx context.Context,
		eventType PluginEventType,
		server *domain.Server,
		extraData map[string]string,
	)
}

// TaskDispatcher is an interface for dispatching daemon tasks via gRPC.
type TaskDispatcher interface {
	Dispatch(ctx context.Context, task *domain.DaemonTask) error
}

type TaskAlreadyExistsError struct {
	taskName string
}

func (e *TaskAlreadyExistsError) Error() string {
	sb := strings.Builder{}
	sb.Grow(64)
	sb.WriteString("task '")
	sb.WriteString(e.taskName)
	sb.WriteString("' already exists")

	return sb.String()
}

// Service provides methods for controlling game servers.
type Service struct {
	daemonTaskRepo    repositories.DaemonTaskRepository
	serverSettingRepo repositories.ServerSettingRepository
	tm                base.TransactionManager
	pluginDispatcher  PluginDispatcher
	taskDispatcher    TaskDispatcher
}

// ServiceOption is a functional option for configuring the Service.
type ServiceOption func(*Service)

// WithPluginDispatcher sets the plugin dispatcher for the service.
func WithPluginDispatcher(dispatcher PluginDispatcher) ServiceOption {
	return func(s *Service) {
		s.pluginDispatcher = dispatcher
	}
}

// WithTaskDispatcher sets the task dispatcher for the service.
// When set, tasks are dispatched via gRPC instead of just being saved to the database.
func WithTaskDispatcher(dispatcher TaskDispatcher) ServiceOption {
	return func(s *Service) {
		s.taskDispatcher = dispatcher
	}
}

func NewService(
	daemonTaskRepo repositories.DaemonTaskRepository,
	serverSettingRepo repositories.ServerSettingRepository,
	tm base.TransactionManager,
	opts ...ServiceOption,
) *Service {
	s := &Service{
		daemonTaskRepo:    daemonTaskRepo,
		serverSettingRepo: serverSettingRepo,
		tm:                tm,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// dispatchOrSaveTask dispatches a task via gRPC if taskDispatcher is available,
// otherwise falls back to saving directly to the repository.
func (s *Service) dispatchOrSaveTask(ctx context.Context, task *domain.DaemonTask) error {
	if s.taskDispatcher != nil {
		return s.taskDispatcher.Dispatch(ctx, task)
	}

	return s.daemonTaskRepo.Save(ctx, task)
}

// dispatchPreEvent dispatches a pre-event and returns an error if cancelled.
func (s *Service) dispatchPreEvent(
	ctx context.Context,
	eventType PluginEventType,
	server *domain.Server,
) error {
	if s.pluginDispatcher == nil {
		return nil
	}

	result := s.pluginDispatcher.DispatchServerEvent(ctx, eventType, server, nil)
	if result != nil && result.Cancelled {
		msg := result.CancelMessage
		if msg == "" {
			msg = result.CancelledBy
		}

		return errors.Wrapf(ErrCancelledByPlugin, "cancelled by %s: %s", result.CancelledBy, msg)
	}

	return nil
}

// dispatchPostEvent dispatches a post-event in the background so plugins
// cannot delay the server operation.
func (s *Service) dispatchPostEvent(
	ctx context.Context,
	eventType PluginEventType,
	server *domain.Server,
) {
	if s.pluginDispatcher == nil {
		return
	}

	s.pluginDispatcher.DispatchServerEventAsync(ctx, eventType, server, nil)
}

// Start creates a server start task.
// If the server has autostart enabled, it will also enable autostart_current.
func (s *Service) Start(ctx context.Context, server *domain.Server) (uint, error) {
	if err := s.dispatchPreEvent(ctx, PluginEventServerPreStart, server); err != nil {
		return 0, err
	}

	// If autostart is enabled, set autostart_current to true
	if err := s.updateAutostartCurrentIfEnabled(ctx, server.ID, true); err != nil {
		return 0, err
	}

	// Create the start task
	taskID, err := s.addServerStart(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	s.dispatchPostEvent(ctx, PluginEventServerPostStart, server)

	return taskID, nil
}

// Stop creates a server stop task.
// This method also disables autostart_current.
func (s *Service) Stop(ctx context.Context, server *domain.Server) (uint, error) {
	if err := s.dispatchPreEvent(ctx, PluginEventServerPreStop, server); err != nil {
		return 0, err
	}

	// Set autostart_current to false
	if err := s.updateAutostartCurrent(ctx, server.ID, false); err != nil {
		return 0, err
	}

	// Create the stop task
	taskID, err := s.addServerStop(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	s.dispatchPostEvent(ctx, PluginEventServerPostStop, server)

	return taskID, nil
}

// Restart creates a server restart task.
// If the server has autostart enabled, it will also enable autostart_current.
func (s *Service) Restart(ctx context.Context, server *domain.Server) (uint, error) {
	if err := s.dispatchPreEvent(ctx, PluginEventServerPreRestart, server); err != nil {
		return 0, err
	}

	// If autostart is enabled, set autostart_current to true
	if err := s.updateAutostartCurrentIfEnabled(ctx, server.ID, true); err != nil {
		return 0, err
	}

	// Create the restart task
	taskID, err := s.addServerRestart(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	s.dispatchPostEvent(ctx, PluginEventServerPostRestart, server)

	return taskID, nil
}

// Update creates a server update task.
func (s *Service) Update(ctx context.Context, server *domain.Server) (uint, error) {
	if err := s.dispatchPreEvent(ctx, PluginEventServerPreUpdate, server); err != nil {
		return 0, err
	}

	taskID, err := s.addServerUpdate(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	s.dispatchPostEvent(ctx, PluginEventServerPostUpdate, server)

	return taskID, nil
}

// Install creates a server install task.
func (s *Service) Install(ctx context.Context, server *domain.Server) (uint, error) {
	if err := s.dispatchPreEvent(ctx, PluginEventServerPreInstall, server); err != nil {
		return 0, err
	}

	taskID, err := s.addServerInstall(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	s.dispatchPostEvent(ctx, PluginEventServerPostInstall, server)

	return taskID, nil
}

// Reinstall creates a server reinstall task.
// This is a combination of stop, delete, and install tasks.
func (s *Service) Reinstall(ctx context.Context, server *domain.Server) (uint, error) {
	if err := s.dispatchPreEvent(ctx, PluginEventServerPreReinstall, server); err != nil {
		return 0, err
	}

	// First, ensure no working tasks exist
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{
			domain.DaemonTaskTypeServerStart,
			domain.DaemonTaskTypeServerStop,
			domain.DaemonTaskTypeServerRestart,
			domain.DaemonTaskTypeServerUpdate,
			domain.DaemonTaskTypeServerInstall,
			domain.DaemonTaskTypeServerDelete,
		},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrAnotherTaskAlreadyExists
	}

	var installTaskID uint
	err = s.tm.Do(ctx, func(ctx context.Context) error {
		// Create a stop task
		stopTaskID, err := s.addServerStop(ctx, server, 0)
		if err != nil {
			return errors.WithMessage(err, "failed to create stop task")
		}

		// Create a delete task that runs after stop
		deleteTaskID, err := s.addServerDelete(ctx, server, stopTaskID)
		if err != nil {
			return errors.WithMessage(err, "failed to create delete task")
		}

		// Create an installation task that runs after delete
		installTaskID, err = s.addServerInstall(ctx, server, deleteTaskID)
		if err != nil {
			return errors.WithMessage(err, "failed to create install task")
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	s.dispatchPostEvent(ctx, PluginEventServerPostReinstall, server)

	return installTaskID, nil
}

// addServerStart creates a new starting of game server task.
func (s *Service) addServerStart(
	ctx context.Context,
	server *domain.Server,
	runAftID uint,
) (uint, error) {
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, &TaskAlreadyExistsError{taskName: "server start"}
	}

	if err := s.serverCommandCorrectOrFail(server); err != nil {
		return 0, err
	}

	task := &domain.DaemonTask{
		RunAftID:          new(runAftID),
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         new(time.Now()),
		UpdatedAt:         new(time.Now()),
	}

	if err := s.dispatchOrSaveTask(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

// addServerStop creates a new stopping of game server task.
func (s *Service) addServerStop(
	ctx context.Context,
	server *domain.Server,
	runAftID uint,
) (uint, error) {
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{domain.DaemonTaskTypeServerStop},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, &TaskAlreadyExistsError{taskName: "server stop"}
	}

	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerStop,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         new(time.Now()),
		UpdatedAt:         new(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = new(runAftID)
	}

	if err := s.dispatchOrSaveTask(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

// addServerRestart creates a new restarting of game server task.
func (s *Service) addServerRestart(
	ctx context.Context,
	server *domain.Server,
	runAftID uint,
) (uint, error) {
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{domain.DaemonTaskTypeServerRestart},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, &TaskAlreadyExistsError{taskName: "server restart"}
	}

	if err := s.serverCommandCorrectOrFail(server); err != nil {
		return 0, err
	}

	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerRestart,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         new(time.Now()),
		UpdatedAt:         new(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = new(runAftID)
	}

	if err := s.dispatchOrSaveTask(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

// addServerUpdate creates a new server update task.
func (s *Service) addServerUpdate(
	ctx context.Context,
	server *domain.Server,
	runAftID uint,
) (uint, error) {
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{
			domain.DaemonTaskTypeServerUpdate,
			domain.DaemonTaskTypeServerInstall,
		},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrServerUpdateInstallInProgress
	}

	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerUpdate,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         new(time.Now()),
		UpdatedAt:         new(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = new(runAftID)
	}

	if err := s.dispatchOrSaveTask(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

// addServerInstall creates a new server install task.
func (s *Service) addServerInstall(
	ctx context.Context,
	server *domain.Server,
	runAftID uint,
) (uint, error) {
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{
			domain.DaemonTaskTypeServerUpdate,
			domain.DaemonTaskTypeServerInstall,
		},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrServerUpdateInstallInProgress
	}

	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerInstall,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         new(time.Now()),
		UpdatedAt:         new(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = new(runAftID)
	}

	if err := s.dispatchOrSaveTask(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

// addServerDelete creates a new server delete task.
func (s *Service) addServerDelete(
	ctx context.Context,
	server *domain.Server,
	runAftID uint,
) (uint, error) {
	exists, err := s.workingTasksExist(
		ctx,
		server,
		[]domain.DaemonTaskType{domain.DaemonTaskTypeServerDelete},
	)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, &TaskAlreadyExistsError{taskName: "server delete"}
	}

	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerDelete,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         new(time.Now()),
		UpdatedAt:         new(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = new(runAftID)
	}

	if err := s.dispatchOrSaveTask(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

// workingTasksExist checks if there are any working or waiting tasks
// for the given server and task types.
func (s *Service) workingTasksExist(
	ctx context.Context,
	server *domain.Server,
	taskTypes []domain.DaemonTaskType,
) (bool, error) {
	serverID := server.ID
	exists, err := s.daemonTaskRepo.Exists(ctx, &filters.FindDaemonTask{
		ServerIDs: []*uint{&serverID},
		Tasks:     taskTypes,
		Statuses: []domain.DaemonTaskStatus{
			domain.DaemonTaskStatusWaiting,
			domain.DaemonTaskStatusWorking,
		},
	})
	if err != nil {
		return false, errors.WithMessage(err, "failed to check daemon task existence")
	}

	return exists, nil
}

// serverCommandCorrectOrFail validates that the server has a start command.
func (s *Service) serverCommandCorrectOrFail(server *domain.Server) error {
	if server.StartCommand == nil || *server.StartCommand == "" {
		return ErrEmptyServerStartCommand
	}

	return nil
}

// getSetting retrieves a server setting by name.
func (s *Service) getSetting(
	ctx context.Context,
	serverID uint,
	settingName string,
) (*domain.ServerSetting, error) {
	settings, err := s.serverSettingRepo.Find(ctx, &filters.FindServerSetting{
		ServerIDs: []uint{serverID},
		Names:     []string{settingName},
	}, nil, nil)
	if err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		return nil, nil
	}

	return &settings[0], nil
}

// updateAutostartCurrentIfEnabled updates the autostart_current setting
// if autostart is enabled for the given server.
func (s *Service) updateAutostartCurrentIfEnabled(
	ctx context.Context,
	serverID uint,
	value bool,
) error {
	autostartSetting, err := s.getSetting(ctx, serverID, autostartSettingKey)
	if err != nil {
		return errors.WithMessage(err, "failed to get autostart setting")
	}

	if autostartSetting == nil {
		return nil
	}

	autostartValue, ok := autostartSetting.Value.Bool()
	if !ok || !autostartValue {
		return nil
	}

	return s.updateAutostartCurrent(ctx, serverID, value)
}

// updateAutostartCurrent updates or creates the autostart_current setting.
func (s *Service) updateAutostartCurrent(
	ctx context.Context,
	serverID uint,
	value bool,
) error {
	autostartCurrentSetting, err := s.getSetting(ctx, serverID, autostartCurrentSettingKey)
	if err != nil {
		return errors.WithMessage(err, "failed to get autostart_current setting")
	}

	if autostartCurrentSetting == nil {
		autostartCurrentSetting = &domain.ServerSetting{
			Name:     autostartCurrentSettingKey,
			ServerID: serverID,
			Value:    domain.NewServerSettingValue(value),
		}
	} else {
		autostartCurrentSetting.Value = domain.NewServerSettingValue(value)
	}

	if err := s.serverSettingRepo.Save(ctx, autostartCurrentSetting); err != nil {
		return errors.WithMessage(err, "failed to save autostart_current setting")
	}

	return nil
}
