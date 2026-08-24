package hostlibrary

import (
	"context"
	"errors"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/servercontrol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errControllerError = errors.New("controller error")

type mockServerController struct {
	startFunc     func(ctx context.Context, server *domain.Server) (uint, error)
	stopFunc      func(ctx context.Context, server *domain.Server) (uint, error)
	restartFunc   func(ctx context.Context, server *domain.Server) (uint, error)
	updateFunc    func(ctx context.Context, server *domain.Server) (uint, error)
	installFunc   func(ctx context.Context, server *domain.Server) (uint, error)
	reinstallFunc func(ctx context.Context, server *domain.Server) (uint, error)
}

func (m *mockServerController) Start(ctx context.Context, server *domain.Server) (uint, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, server)
	}

	return 0, nil
}

func (m *mockServerController) Stop(ctx context.Context, server *domain.Server) (uint, error) {
	if m.stopFunc != nil {
		return m.stopFunc(ctx, server)
	}

	return 0, nil
}

func (m *mockServerController) Restart(ctx context.Context, server *domain.Server) (uint, error) {
	if m.restartFunc != nil {
		return m.restartFunc(ctx, server)
	}

	return 0, nil
}

func (m *mockServerController) Update(ctx context.Context, server *domain.Server) (uint, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, server)
	}

	return 0, nil
}

func (m *mockServerController) Install(ctx context.Context, server *domain.Server) (uint, error) {
	if m.installFunc != nil {
		return m.installFunc(ctx, server)
	}

	return 0, nil
}

func (m *mockServerController) Reinstall(ctx context.Context, server *domain.Server) (uint, error) {
	if m.reinstallFunc != nil {
		return m.reinstallFunc(ctx, server)
	}

	return 0, nil
}

func TestServerControlService_StartServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.ServerRepository)
		setupCtrl   func() *mockServerController
		serverID    uint64
		wantError   string
		wantSuccess bool
		wantTaskID  uint64
	}{
		{
			name:      "server_not_found_returns_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupCtrl: func() *mockServerController {
				return &mockServerController{}
			},
			serverID:    999,
			wantError:   "server not found",
			wantSuccess: false,
		},
		{
			name: "controller_error_returns_error",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					startFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 0, errControllerError
					},
				}
			},
			serverID:    1,
			wantError:   "controller error",
			wantSuccess: false,
		},
		{
			name: "success_returns_task_id",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					startFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 42, nil
					},
				}
			},
			serverID:    1,
			wantSuccess: true,
			wantTaskID:  42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)
			ctrl := tt.setupCtrl()

			svc := NewServerControlService(repo, ctrl, allowAllGuard(testPluginID))
			resp, err := svc.StartServer(context.Background(), &servercontrol.ServerControlRequest{ServerId: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
				require.NotNil(t, resp.TaskId)
				assert.Equal(t, tt.wantTaskID, *resp.TaskId)
			}
		})
	}
}

func TestServerControlService_StopServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.ServerRepository)
		setupCtrl   func() *mockServerController
		serverID    uint64
		wantError   string
		wantSuccess bool
		wantTaskID  uint64
	}{
		{
			name:      "server_not_found_returns_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupCtrl: func() *mockServerController {
				return &mockServerController{}
			},
			serverID:    999,
			wantError:   "server not found",
			wantSuccess: false,
		},
		{
			name: "success_returns_task_id",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					stopFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 100, nil
					},
				}
			},
			serverID:    1,
			wantSuccess: true,
			wantTaskID:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)
			ctrl := tt.setupCtrl()

			svc := NewServerControlService(repo, ctrl, allowAllGuard(testPluginID))
			resp, err := svc.StopServer(context.Background(), &servercontrol.ServerControlRequest{ServerId: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
				require.NotNil(t, resp.TaskId)
				assert.Equal(t, tt.wantTaskID, *resp.TaskId)
			}
		})
	}
}

func TestServerControlService_RestartServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.ServerRepository)
		setupCtrl   func() *mockServerController
		serverID    uint64
		wantError   string
		wantSuccess bool
		wantTaskID  uint64
	}{
		{
			name:      "server_not_found_returns_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupCtrl: func() *mockServerController {
				return &mockServerController{}
			},
			serverID:    999,
			wantError:   "server not found",
			wantSuccess: false,
		},
		{
			name: "success_returns_task_id",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					restartFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 200, nil
					},
				}
			},
			serverID:    1,
			wantSuccess: true,
			wantTaskID:  200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)
			ctrl := tt.setupCtrl()

			svc := NewServerControlService(repo, ctrl, allowAllGuard(testPluginID))
			resp, err := svc.RestartServer(context.Background(), &servercontrol.ServerControlRequest{ServerId: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
				require.NotNil(t, resp.TaskId)
				assert.Equal(t, tt.wantTaskID, *resp.TaskId)
			}
		})
	}
}

func TestServerControlService_UpdateServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.ServerRepository)
		setupCtrl   func() *mockServerController
		serverID    uint64
		wantError   string
		wantSuccess bool
		wantTaskID  uint64
	}{
		{
			name:      "server_not_found_returns_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupCtrl: func() *mockServerController {
				return &mockServerController{}
			},
			serverID:    999,
			wantError:   "server not found",
			wantSuccess: false,
		},
		{
			name: "success_returns_task_id",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					updateFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 300, nil
					},
				}
			},
			serverID:    1,
			wantSuccess: true,
			wantTaskID:  300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)
			ctrl := tt.setupCtrl()

			svc := NewServerControlService(repo, ctrl, allowAllGuard(testPluginID))
			resp, err := svc.UpdateServer(context.Background(), &servercontrol.ServerControlRequest{ServerId: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
				require.NotNil(t, resp.TaskId)
				assert.Equal(t, tt.wantTaskID, *resp.TaskId)
			}
		})
	}
}

func TestServerControlService_InstallServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.ServerRepository)
		setupCtrl   func() *mockServerController
		serverID    uint64
		wantError   string
		wantSuccess bool
		wantTaskID  uint64
	}{
		{
			name:      "server_not_found_returns_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupCtrl: func() *mockServerController {
				return &mockServerController{}
			},
			serverID:    999,
			wantError:   "server not found",
			wantSuccess: false,
		},
		{
			name: "success_returns_task_id",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					installFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 400, nil
					},
				}
			},
			serverID:    1,
			wantSuccess: true,
			wantTaskID:  400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)
			ctrl := tt.setupCtrl()

			svc := NewServerControlService(repo, ctrl, allowAllGuard(testPluginID))
			resp, err := svc.InstallServer(context.Background(), &servercontrol.ServerControlRequest{ServerId: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
				require.NotNil(t, resp.TaskId)
				assert.Equal(t, tt.wantTaskID, *resp.TaskId)
			}
		})
	}
}

func TestServerControlService_ReinstallServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.ServerRepository)
		setupCtrl   func() *mockServerController
		serverID    uint64
		wantError   string
		wantSuccess bool
		wantTaskID  uint64
	}{
		{
			name:      "server_not_found_returns_error",
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupCtrl: func() *mockServerController {
				return &mockServerController{}
			},
			serverID:    999,
			wantError:   "server not found",
			wantSuccess: false,
		},
		{
			name: "success_returns_task_id",
			setupRepo: func(r *inmemory.ServerRepository) {
				_ = r.Save(context.Background(), &domain.Server{Name: "Server1", GameID: "cs", DSID: 1})
			},
			setupCtrl: func() *mockServerController {
				return &mockServerController{
					reinstallFunc: func(_ context.Context, _ *domain.Server) (uint, error) {
						return 500, nil
					},
				}
			},
			serverID:    1,
			wantSuccess: true,
			wantTaskID:  500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerRepository()
			tt.setupRepo(repo)
			ctrl := tt.setupCtrl()

			svc := NewServerControlService(repo, ctrl, allowAllGuard(testPluginID))
			resp, err := svc.ReinstallServer(context.Background(), &servercontrol.ServerControlRequest{ServerId: tt.serverID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
				require.NotNil(t, resp.TaskId)
				assert.Equal(t, tt.wantTaskID, *resp.TaskId)
			}
		})
	}
}

func TestNewServerControlHostLibraryFactory(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewServerRepository()
	ctrl := &mockServerController{}
	factory := NewServerControlHostLibraryFactory(repo, ctrl, NewGuard(stubPermissionChecker{allowed: true}))

	lib, ok := factory.Create(42).(*ServerControlHostLibrary)
	require.True(t, ok)
	require.NotNil(t, lib.impl)
	assert.Equal(t, uint64(42), lib.impl.guard.PluginID(), "factory must bind the plugin id")
}
