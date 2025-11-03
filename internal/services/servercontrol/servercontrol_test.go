package servercontrol

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerControlService_Start(t *testing.T) {
	tests := []struct {
		name          string
		server        *domain.Server
		setupSettings func(*inmemory.ServerSettingRepository)
		setupTasks    func(*inmemory.DaemonTaskRepository)
		wantErr       bool
		errContains   string
		validate      func(t *testing.T, taskID uint, settingRepo *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository)
	}{
		{
			name: "successful start without autostart",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: lo.ToPtr("./start.sh"),
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks:    func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:       false,
			validate: func(t *testing.T, taskID uint, _ *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				task := tasks[0]
				assert.Equal(t, domain.DaemonTaskTypeServerStart, task.Task)
				assert.Equal(t, domain.DaemonTaskStatusWaiting, task.Status)
				assert.Equal(t, uint(1), *task.ServerID)
				assert.Equal(t, uint(10), task.DedicatedServerID)
			},
		},
		{
			name: "successful start with autostart enabled",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: lo.ToPtr("./start.sh"),
			},
			setupSettings: func(repo *inmemory.ServerSettingRepository) {
				_ = repo.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     autostartSettingKey,
					Value:    domain.NewServerSettingValue(true),
				})
			},
			setupTasks: func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:    false,
			validate: func(t *testing.T, taskID uint, settingRepo *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				// Check task was created
				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				// Check autostart_current was set to true
				settings, err := settingRepo.Find(context.Background(), &filters.FindServerSetting{
					ServerIDs: []uint{1},
					Names:     []string{autostartCurrentSettingKey},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, settings, 1)

				autostartCurrent, ok := settings[0].Value.Bool()
				require.True(t, ok)
				assert.True(t, autostartCurrent)
			},
		},
		{
			name: "error when start command is empty",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: nil,
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks:    func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:       true,
			errContains:   "empty server start command",
		},
		{
			name: "error when start task already exists",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: lo.ToPtr("./start.sh"),
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks: func(repo *inmemory.DaemonTaskRepository) {
				serverID := uint(1)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ServerID:          &serverID,
					DedicatedServerID: 10,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWaiting,
				})
			},
			wantErr:     true,
			errContains: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()

			tt.setupSettings(settingRepo)
			tt.setupTasks(taskRepo)

			service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())

			taskID, err := service.Start(context.Background(), tt.server)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotZero(t, taskID)
				if tt.validate != nil {
					tt.validate(t, taskID, settingRepo, taskRepo)
				}
			}
		})
	}
}

func TestServerControlService_Stop(t *testing.T) {
	tests := []struct {
		name          string
		server        *domain.Server
		setupSettings func(*inmemory.ServerSettingRepository)
		setupTasks    func(*inmemory.DaemonTaskRepository)
		wantErr       bool
		errContains   string
		validate      func(t *testing.T, taskID uint, settingRepo *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository)
	}{
		{
			name: "successful stop",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks:    func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:       false,
			validate: func(t *testing.T, taskID uint, settingRepo *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				// Check task was created
				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				task := tasks[0]
				assert.Equal(t, domain.DaemonTaskTypeServerStop, task.Task)
				assert.Equal(t, domain.DaemonTaskStatusWaiting, task.Status)

				// Check autostart_current was set to false
				settings, err := settingRepo.Find(context.Background(), &filters.FindServerSetting{
					ServerIDs: []uint{1},
					Names:     []string{autostartCurrentSettingKey},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, settings, 1)

				autostartCurrent, ok := settings[0].Value.Bool()
				require.True(t, ok)
				assert.False(t, autostartCurrent)
			},
		},
		{
			name: "error when stop task already exists",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks: func(repo *inmemory.DaemonTaskRepository) {
				serverID := uint(1)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ServerID:          &serverID,
					DedicatedServerID: 10,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWorking,
				})
			},
			wantErr:     true,
			errContains: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()

			tt.setupSettings(settingRepo)
			tt.setupTasks(taskRepo)

			service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())

			taskID, err := service.Stop(context.Background(), tt.server)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotZero(t, taskID)
				if tt.validate != nil {
					tt.validate(t, taskID, settingRepo, taskRepo)
				}
			}
		})
	}
}

func TestServerControlService_Restart(t *testing.T) {
	tests := []struct {
		name          string
		server        *domain.Server
		setupSettings func(*inmemory.ServerSettingRepository)
		setupTasks    func(*inmemory.DaemonTaskRepository)
		wantErr       bool
		errContains   string
		validate      func(t *testing.T, taskID uint, settingRepo *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository)
	}{
		{
			name: "successful restart without autostart",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: lo.ToPtr("./start.sh"),
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks:    func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:       false,
			validate: func(t *testing.T, taskID uint, _ *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				task := tasks[0]
				assert.Equal(t, domain.DaemonTaskTypeServerRestart, task.Task)
				assert.Equal(t, domain.DaemonTaskStatusWaiting, task.Status)
			},
		},
		{
			name: "successful restart with autostart enabled",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: lo.ToPtr("./start.sh"),
			},
			setupSettings: func(repo *inmemory.ServerSettingRepository) {
				_ = repo.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     autostartSettingKey,
					Value:    domain.NewServerSettingValue(true),
				})
			},
			setupTasks: func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:    false,
			validate: func(t *testing.T, taskID uint, settingRepo *inmemory.ServerSettingRepository, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				// Check task was created
				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				// Check autostart_current was set to true
				settings, err := settingRepo.Find(context.Background(), &filters.FindServerSetting{
					ServerIDs: []uint{1},
					Names:     []string{autostartCurrentSettingKey},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, settings, 1)

				autostartCurrent, ok := settings[0].Value.Bool()
				require.True(t, ok)
				assert.True(t, autostartCurrent)
			},
		},
		{
			name: "error when start command is empty",
			server: &domain.Server{
				ID:           1,
				DSID:         10,
				StartCommand: nil,
			},
			setupSettings: func(_ *inmemory.ServerSettingRepository) {},
			setupTasks:    func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:       true,
			errContains:   "empty server start command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()

			tt.setupSettings(settingRepo)
			tt.setupTasks(taskRepo)

			service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())

			taskID, err := service.Restart(context.Background(), tt.server)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotZero(t, taskID)
				if tt.validate != nil {
					tt.validate(t, taskID, settingRepo, taskRepo)
				}
			}
		})
	}
}

func TestServerControlService_Update(t *testing.T) {
	tests := []struct {
		name        string
		server      *domain.Server
		setupTasks  func(*inmemory.DaemonTaskRepository)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, taskID uint, taskRepo *inmemory.DaemonTaskRepository)
	}{
		{
			name: "successful update",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:    false,
			validate: func(t *testing.T, taskID uint, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				task := tasks[0]
				assert.Equal(t, domain.DaemonTaskTypeServerUpdate, task.Task)
				assert.Equal(t, domain.DaemonTaskStatusWaiting, task.Status)
			},
		},
		{
			name: "error when update task already exists",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(repo *inmemory.DaemonTaskRepository) {
				serverID := uint(1)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ServerID:          &serverID,
					DedicatedServerID: 10,
					Task:              domain.DaemonTaskTypeServerUpdate,
					Status:            domain.DaemonTaskStatusWaiting,
				})
			},
			wantErr:     true,
			errContains: "update/install task is already in progress",
		},
		{
			name: "error when install task already exists",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(repo *inmemory.DaemonTaskRepository) {
				serverID := uint(1)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ServerID:          &serverID,
					DedicatedServerID: 10,
					Task:              domain.DaemonTaskTypeServerInstall,
					Status:            domain.DaemonTaskStatusWorking,
				})
			},
			wantErr:     true,
			errContains: "update/install task is already in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()

			tt.setupTasks(taskRepo)

			service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())

			taskID, err := service.Update(context.Background(), tt.server)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotZero(t, taskID)
				if tt.validate != nil {
					tt.validate(t, taskID, taskRepo)
				}
			}
		})
	}
}

func TestServerControlService_Install(t *testing.T) {
	tests := []struct {
		name        string
		server      *domain.Server
		setupTasks  func(*inmemory.DaemonTaskRepository)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, taskID uint, taskRepo *inmemory.DaemonTaskRepository)
	}{
		{
			name: "successful install",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:    false,
			validate: func(t *testing.T, taskID uint, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					IDs: []uint{taskID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				task := tasks[0]
				assert.Equal(t, domain.DaemonTaskTypeServerInstall, task.Task)
				assert.Equal(t, domain.DaemonTaskStatusWaiting, task.Status)
			},
		},
		{
			name: "error when install task already exists",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(repo *inmemory.DaemonTaskRepository) {
				serverID := uint(1)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ServerID:          &serverID,
					DedicatedServerID: 10,
					Task:              domain.DaemonTaskTypeServerInstall,
					Status:            domain.DaemonTaskStatusWaiting,
				})
			},
			wantErr:     true,
			errContains: "update/install task is already in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()

			tt.setupTasks(taskRepo)

			service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())

			taskID, err := service.Install(context.Background(), tt.server)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotZero(t, taskID)
				if tt.validate != nil {
					tt.validate(t, taskID, taskRepo)
				}
			}
		})
	}
}

func TestServerControlService_Reinstall(t *testing.T) {
	tests := []struct {
		name        string
		server      *domain.Server
		setupTasks  func(*inmemory.DaemonTaskRepository)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, taskID uint, taskRepo *inmemory.DaemonTaskRepository)
	}{
		{
			name: "successful reinstall",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(_ *inmemory.DaemonTaskRepository) {},
			wantErr:    false,
			validate: func(t *testing.T, taskID uint, taskRepo *inmemory.DaemonTaskRepository) {
				t.Helper()

				// Get all tasks for this server
				serverID := uint(1)
				tasks, err := taskRepo.Find(context.Background(), &filters.FindDaemonTask{
					ServerIDs: []*uint{&serverID},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 3) // stop, delete, install

				// Find each task type
				var stopTask, deleteTask, installTask *domain.DaemonTask
				for i := range tasks {
					switch tasks[i].Task {
					case domain.DaemonTaskTypeServerStop:
						stopTask = &tasks[i]
					case domain.DaemonTaskTypeServerDelete:
						deleteTask = &tasks[i]
					case domain.DaemonTaskTypeServerInstall:
						installTask = &tasks[i]
					}
				}

				require.NotNil(t, stopTask, "stop task should exist")
				require.NotNil(t, deleteTask, "delete task should exist")
				require.NotNil(t, installTask, "install task should exist")

				// Verify task dependencies
				assert.Nil(t, stopTask.RunAftID, "stop task should not depend on any task")
				assert.NotNil(t, deleteTask.RunAftID, "delete task should depend on stop task")
				assert.Equal(t, stopTask.ID, *deleteTask.RunAftID, "delete task should run after stop task")
				assert.NotNil(t, installTask.RunAftID, "install task should depend on delete task")
				assert.Equal(t, deleteTask.ID, *installTask.RunAftID, "install task should run after delete task")

				// The returned taskID should be the install task ID
				assert.Equal(t, installTask.ID, taskID)
			},
		},
		{
			name: "error when server has working tasks",
			server: &domain.Server{
				ID:   1,
				DSID: 10,
			},
			setupTasks: func(repo *inmemory.DaemonTaskRepository) {
				serverID := uint(1)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ServerID:          &serverID,
					DedicatedServerID: 10,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWorking,
				})
			},
			wantErr:     true,
			errContains: "task already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()

			tt.setupTasks(taskRepo)

			service := NewService(taskRepo, settingRepo, services.NewNilTransactionManager())

			taskID, err := service.Reinstall(context.Background(), tt.server)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotZero(t, taskID)
				if tt.validate != nil {
					tt.validate(t, taskID, taskRepo)
				}
			}
		})
	}
}
