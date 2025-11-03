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
	"github.com/samber/lo"
)

const (
	autostartSettingKey        = "autostart"
	autostartCurrentSettingKey = "autostart_current"
)

var (
	ErrAnotherTaskAlreadyExists      = errors.New("another task already exists, please wait until it is completed")
	ErrEmptyServerStartCommand       = errors.New("empty server start command")
	ErrServerUpdateInstallInProgress = errors.New("server update/install task is already in progress")
)

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
}

func NewService(
	daemonTaskRepo repositories.DaemonTaskRepository,
	serverSettingRepo repositories.ServerSettingRepository,
	tm base.TransactionManager,
) *Service {
	return &Service{
		daemonTaskRepo:    daemonTaskRepo,
		serverSettingRepo: serverSettingRepo,
		tm:                tm,
	}
}

// Start creates a server start task.
// If the server has autostart enabled, it will also enable autostart_current.
func (s *Service) Start(ctx context.Context, server *domain.Server) (uint, error) {
	// If autostart is enabled, set autostart_current to true
	if err := s.updateAutostartCurrentIfEnabled(ctx, server.ID, true); err != nil {
		return 0, err
	}

	// Create the start task
	taskID, err := s.addServerStart(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	return taskID, nil
}

// Stop creates a server stop task.
// This method also disables autostart_current.
func (s *Service) Stop(ctx context.Context, server *domain.Server) (uint, error) {
	// Set autostart_current to false
	if err := s.updateAutostartCurrent(ctx, server.ID, false); err != nil {
		return 0, err
	}

	// Create the stop task
	taskID, err := s.addServerStop(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	return taskID, nil
}

// Restart creates a server restart task.
// If the server has autostart enabled, it will also enable autostart_current.
func (s *Service) Restart(ctx context.Context, server *domain.Server) (uint, error) {
	// If autostart is enabled, set autostart_current to true
	if err := s.updateAutostartCurrentIfEnabled(ctx, server.ID, true); err != nil {
		return 0, err
	}

	// Create the restart task
	taskID, err := s.addServerRestart(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	return taskID, nil
}

// Update creates a server update task.
func (s *Service) Update(ctx context.Context, server *domain.Server) (uint, error) {
	taskID, err := s.addServerUpdate(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	return taskID, nil
}

// Install creates a server install task.
func (s *Service) Install(ctx context.Context, server *domain.Server) (uint, error) {
	taskID, err := s.addServerInstall(ctx, server, 0)
	if err != nil {
		return 0, err
	}

	return taskID, nil
}

// Reinstall creates a server reinstall task.
// This is a combination of stop, delete, and install tasks.
func (s *Service) Reinstall(ctx context.Context, server *domain.Server) (uint, error) {
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
		RunAftID:          lo.ToPtr(runAftID),
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if err := s.daemonTaskRepo.Save(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
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
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = lo.ToPtr(runAftID)
	}

	if err := s.daemonTaskRepo.Save(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
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
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = lo.ToPtr(runAftID)
	}

	if err := s.daemonTaskRepo.Save(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
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
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = lo.ToPtr(runAftID)
	}

	if err := s.daemonTaskRepo.Save(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
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
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = lo.ToPtr(runAftID)
	}

	if err := s.daemonTaskRepo.Save(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
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
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if runAftID > 0 {
		task.RunAftID = lo.ToPtr(runAftID)
	}

	if err := s.daemonTaskRepo.Save(ctx, task); err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
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
