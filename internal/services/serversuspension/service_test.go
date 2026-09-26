package serversuspension

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testNow     = time.Date(2026, 9, 26, 10, 15, 0, 0, time.UTC)
	errSaveFail = errors.New("simulated save error")
	errFindFail = errors.New("simulated find error")
)

type pushRecorder struct {
	mu        sync.Mutex
	serverIDs []uint
}

func (p *pushRecorder) PushServerConfig(_ context.Context, serverID uint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.serverIDs = append(p.serverIDs, serverID)
}

func (p *pushRecorder) pushed() []uint {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]uint(nil), p.serverIDs...)
}

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = append(a.events, e)
}

func (a *auditCapture) types() []audit.EventType {
	a.mu.Lock()
	defer a.mu.Unlock()

	types := make([]audit.EventType, 0, len(a.events))
	for _, e := range a.events {
		types = append(types, e.Type)
	}

	return types
}

func (a *auditCapture) attr(t *testing.T, key string) (slog.Value, bool) {
	t.Helper()

	a.mu.Lock()
	defer a.mu.Unlock()

	require.Len(t, a.events, 1)

	for _, attr := range a.events[0].Extra {
		if attr.Key == key {
			return attr.Value, true
		}
	}

	return slog.Value{}, false
}

type cancellingPlugins struct{}

func (cancellingPlugins) DispatchServerEvent(
	context.Context, servercontrol.PluginEventType, *domain.Server, map[string]string,
) *servercontrol.PluginDispatchResult {
	return &servercontrol.PluginDispatchResult{Cancelled: true, CancelledBy: "guard"}
}

func (cancellingPlugins) DispatchServerEventAsync(
	context.Context, servercontrol.PluginEventType, *domain.Server, map[string]string,
) {
}

type countingPlugins struct {
	mu     sync.Mutex
	events int
}

func (c *countingPlugins) DispatchServerEvent(
	context.Context, servercontrol.PluginEventType, *domain.Server, map[string]string,
) *servercontrol.PluginDispatchResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events++

	return nil
}

func (c *countingPlugins) DispatchServerEventAsync(
	context.Context, servercontrol.PluginEventType, *domain.Server, map[string]string,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events++
}

func (c *countingPlugins) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.events
}

type failingSaveRepo struct {
	repositories.ServerRepository
}

func (failingSaveRepo) Save(context.Context, *domain.Server) error {
	return errSaveFail
}

type fixture struct {
	service     *Service
	serverRepo  *inmemory.ServerRepository
	taskRepo    *inmemory.DaemonTaskRepository
	settingRepo *inmemory.ServerSettingRepository
	pusher      *pushRecorder
	audit       *auditCapture
}

func newFixture(t *testing.T, opts ...servercontrol.ServiceOption) *fixture {
	t.Helper()

	f := &fixture{
		serverRepo:  inmemory.NewServerRepository(),
		taskRepo:    inmemory.NewDaemonTaskRepository(),
		settingRepo: inmemory.NewServerSettingRepository(),
		pusher:      &pushRecorder{},
		audit:       &auditCapture{},
	}

	control := servercontrol.NewService(f.taskRepo, f.settingRepo, services.NewNilTransactionManager(), opts...)
	f.service = NewService(f.serverRepo, f.taskRepo, control, f.pusher, f.audit, nil)
	f.service.now = func() time.Time { return testNow }

	return f
}

func (f *fixture) saveServer(t *testing.T, server *domain.Server) *domain.Server {
	t.Helper()

	require.NoError(t, f.serverRepo.Save(context.Background(), server))

	return server
}

func (f *fixture) storedServer(t *testing.T, id uint) domain.Server {
	t.Helper()

	servers, err := f.serverRepo.Find(context.Background(), filters.FindServerByIDs(id), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	return servers[0]
}

func (f *fixture) tasks(t *testing.T) []domain.DaemonTask {
	t.Helper()

	tasks, err := f.taskRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)

	return tasks
}

func installedServer(processActive bool, lastCheck *time.Time) *domain.Server {
	return &domain.Server{
		ID:               1,
		DSID:             7,
		Installed:        domain.ServerInstalledStatusInstalled,
		StartCommand:     new("./start.sh"),
		ProcessActive:    processActive,
		LastProcessCheck: lastCheck,
	}
}

func TestService_Suspend(t *testing.T) {
	t.Parallel()

	fresh := new(time.Now().Add(-20 * time.Second))
	stale := new(time.Now().Add(-10 * time.Minute))

	tests := []struct {
		name     string
		server   func() *domain.Server
		wantStop bool
	}{
		{
			name:     "running_server_is_stopped",
			server:   func() *domain.Server { return installedServer(true, fresh) },
			wantStop: true,
		},
		{
			name:     "server_in_unknown_state_is_stopped",
			server:   func() *domain.Server { return installedServer(false, nil) },
			wantStop: true,
		},
		{
			name:     "server_with_stale_report_of_down_is_stopped",
			server:   func() *domain.Server { return installedServer(false, stale) },
			wantStop: true,
		},
		{
			name:     "server_freshly_reported_down_is_not_stopped",
			server:   func() *domain.Server { return installedServer(false, fresh) },
			wantStop: false,
		},
		{
			name: "not_installed_server_is_not_stopped",
			server: func() *domain.Server {
				server := installedServer(true, fresh)
				server.Installed = domain.ServerInstalledStatusNotInstalled

				return server
			},
			wantStop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newFixture(t)
			server := f.saveServer(t, tt.server())

			// ACT
			stopTaskID, err := f.service.Suspend(context.Background(), server, new("unpaid"))

			// ASSERT
			require.NoError(t, err)

			stored := f.storedServer(t, server.ID)
			assert.True(t, stored.Blocked)
			assert.Equal(t, &domain.ServerSuspension{Since: new(testNow), Reason: "unpaid"}, stored.Suspension())
			assert.Equal(t, []uint{server.ID}, f.pusher.pushed(), "the daemon must learn about the block at once")
			assert.Equal(t, []audit.EventType{audit.EventServerSuspend}, f.audit.types())

			tasks := f.tasks(t)
			if !tt.wantStop {
				assert.Zero(t, stopTaskID)
				assert.Empty(t, tasks)

				return
			}

			require.Len(t, tasks, 1)
			assert.Equal(t, tasks[0].ID, stopTaskID)
			assert.Equal(t, domain.DaemonTaskTypeServerStop, tasks[0].Task)
			assert.Equal(t, uint(7), tasks[0].DedicatedServerID)

			stopTaskAttr, ok := f.audit.attr(t, "stop_task_id")
			require.True(t, ok)
			assert.Equal(t, uint64(stopTaskID), stopTaskAttr.Uint64())
		})
	}
}

func TestService_Suspend_ReusesStopInFlight(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting, domain.DaemonTaskStatusWorking} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			plugins := &countingPlugins{}
			f := newFixture(t, servercontrol.WithPluginDispatcher(plugins))
			server := f.saveServer(t, installedServer(true, new(time.Now())))
			inFlight := &domain.DaemonTask{
				DedicatedServerID: 7,
				ServerID:          new(server.ID),
				Task:              domain.DaemonTaskTypeServerStop,
				Status:            status,
			}
			require.NoError(t, f.taskRepo.Save(context.Background(), inFlight))

			// ACT
			stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, inFlight.ID, stopTaskID)
			assert.Len(t, f.tasks(t), 1, "no second stop must be queued")
			assert.Zero(t, plugins.count(), "plugins must not hear of a stop that is not queued")
		})
	}
}

func TestService_Suspend_FinishedStopIsNotReused(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t)
	server := f.saveServer(t, installedServer(true, new(time.Now())))
	finished := &domain.DaemonTask{
		DedicatedServerID: 7,
		ServerID:          new(server.ID),
		Task:              domain.DaemonTaskTypeServerStop,
		Status:            domain.DaemonTaskStatusSuccess,
	}
	require.NoError(t, f.taskRepo.Save(context.Background(), finished))

	// ACT
	stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

	// ASSERT
	require.NoError(t, err)
	assert.NotZero(t, stopTaskID)
	assert.NotEqual(t, finished.ID, stopTaskID)
	assert.Len(t, f.tasks(t), 2)
}

func TestService_Suspend_Repeated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   func() *domain.Server
		wantStop bool
	}{
		{
			name:     "server_reported_running_is_stopped_again",
			server:   func() *domain.Server { return installedServer(true, new(time.Now())) },
			wantStop: true,
		},
		{
			name:     "server_in_unknown_state_is_left_to_the_daemon",
			server:   func() *domain.Server { return installedServer(false, nil) },
			wantStop: false,
		},
		{
			name:     "server_reported_down_is_not_stopped",
			server:   func() *domain.Server { return installedServer(false, new(time.Now())) },
			wantStop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newFixture(t)
			server := tt.server()
			server.Suspend(testNow.Add(-72*time.Hour), new("unpaid"))
			f.saveServer(t, server)

			// ACT
			stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantStop, stopTaskID != 0)
			assert.Len(t, f.tasks(t), map[bool]int{true: 1, false: 0}[tt.wantStop])

			stored := f.storedServer(t, server.ID)
			assert.Equal(t, &domain.ServerSuspension{
				Since:  new(testNow.Add(-72 * time.Hour)),
				Reason: "unpaid",
			}, stored.Suspension(), "a repeated suspension keeps the date and the reason")
		})
	}
}

func TestService_Suspend_StopCancelledByPluginKeepsTheSuspension(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t, servercontrol.WithPluginDispatcher(cancellingPlugins{}))
	server := f.saveServer(t, installedServer(true, new(time.Now())))

	// ACT
	stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

	// ASSERT
	require.NoError(t, err)
	assert.Zero(t, stopTaskID)
	assert.Empty(t, f.tasks(t))
	assert.True(t, f.storedServer(t, server.ID).Blocked)
	assert.Equal(t, []uint{server.ID}, f.pusher.pushed())
}

func TestService_Suspend_SaveError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t)
	f.service.serverRepo = failingSaveRepo{ServerRepository: f.serverRepo}
	server := installedServer(true, new(time.Now()))

	// ACT
	stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, errSaveFail)
	assert.Contains(t, err.Error(), "failed to save server")
	assert.Zero(t, stopTaskID)
	assert.Empty(t, f.tasks(t), "nothing is stopped when the suspension was not saved")
	assert.Empty(t, f.pusher.pushed())
	assert.Empty(t, f.audit.types())
}

func TestService_Unsuspend(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t)
	server := installedServer(false, new(time.Now()))
	server.Metadata = domain.Metadata{"public_ip": "203.0.113.10"}
	server.Suspend(testNow, new("unpaid"))
	f.saveServer(t, server)

	// ACT
	err := f.service.Unsuspend(context.Background(), server)

	// ASSERT
	require.NoError(t, err)

	stored := f.storedServer(t, server.ID)
	assert.False(t, stored.Blocked)
	assert.Nil(t, stored.Suspension())
	assert.Equal(t, domain.Metadata{"public_ip": "203.0.113.10"}, stored.Metadata)
	assert.Equal(t, []uint{server.ID}, f.pusher.pushed())
	assert.Equal(t, []audit.EventType{audit.EventServerUnsuspend}, f.audit.types())
	assert.Empty(t, f.tasks(t), "unsuspending starts nothing")
}

func TestService_Unsuspend_NotSuspended(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t)
	f.service.serverRepo = failingSaveRepo{ServerRepository: f.serverRepo}
	server := installedServer(false, nil)

	// ACT
	err := f.service.Unsuspend(context.Background(), server)

	// ASSERT
	require.NoError(t, err, "nothing to save, so the failing repository is never reached")
	assert.Empty(t, f.pusher.pushed())
	assert.Empty(t, f.audit.types())
}

func TestService_Unsuspend_SaveError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t)
	f.service.serverRepo = failingSaveRepo{ServerRepository: f.serverRepo}
	server := installedServer(false, nil)
	server.Blocked = true

	// ACT
	err := f.service.Unsuspend(context.Background(), server)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, errSaveFail)
	assert.Empty(t, f.pusher.pushed())
	assert.Empty(t, f.audit.types())
}

func TestService_AfterUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wasSuspended bool
		suspended    bool
		wantStop     bool
		wantEvents   []audit.EventType
	}{
		{
			name:       "new_suspension_stops_the_server",
			suspended:  true,
			wantStop:   true,
			wantEvents: []audit.EventType{audit.EventServerSuspend},
		},
		{
			name:         "lifted_suspension_is_recorded",
			wasSuspended: true,
			wantEvents:   []audit.EventType{audit.EventServerUnsuspend},
		},
		{
			name:         "suspension_kept",
			wasSuspended: true,
			suspended:    true,
			wantEvents:   []audit.EventType{},
		},
		{
			name:       "no_suspension",
			wantEvents: []audit.EventType{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newFixture(t)
			server := installedServer(true, new(time.Now()))
			server.Blocked = tt.suspended
			f.saveServer(t, server)

			// ACT
			stopTaskID := f.service.AfterUpdate(context.Background(), server, tt.wasSuspended)

			// ASSERT
			assert.Equal(t, tt.wantEvents, f.audit.types())
			assert.Empty(t, f.pusher.pushed(), "the update pushes the config itself")

			tasks := f.tasks(t)
			if !tt.wantStop {
				assert.Zero(t, stopTaskID)
				assert.Empty(t, tasks)

				return
			}

			require.Len(t, tasks, 1)
			assert.Equal(t, tasks[0].ID, stopTaskID)
			assert.Equal(t, domain.DaemonTaskTypeServerStop, tasks[0].Task)
		})
	}
}

type failingFindTaskRepo struct {
	repositories.DaemonTaskRepository
}

func (failingFindTaskRepo) Find(
	context.Context, *filters.FindDaemonTask, []filters.Sorting, *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return nil, errFindFail
}

func TestService_Suspend_StartSinceTheLastReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		startStatus domain.DaemonTaskStatus
		reportAfter bool
		wantStop    bool
	}{
		{
			name:        "start_finished_after_the_report_of_down",
			startStatus: domain.DaemonTaskStatusSuccess,
			wantStop:    true,
		},
		{
			name:        "start_still_queued",
			startStatus: domain.DaemonTaskStatusWaiting,
			reportAfter: true,
			wantStop:    true,
		},
		{
			name:        "start_still_running",
			startStatus: domain.DaemonTaskStatusWorking,
			reportAfter: true,
			wantStop:    true,
		},
		{
			name:        "start_the_report_already_accounts_for",
			startStatus: domain.DaemonTaskStatusSuccess,
			reportAfter: true,
			wantStop:    false,
		},
		{
			name:        "failed_start",
			startStatus: domain.DaemonTaskStatusError,
			wantStop:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := newFixture(t)
			server := installedServer(false, new(time.Now().Add(-20*time.Second)))
			f.saveServer(t, server)

			start := &domain.DaemonTask{
				DedicatedServerID: 7,
				ServerID:          new(server.ID),
				Task:              domain.DaemonTaskTypeServerStart,
				Status:            tt.startStatus,
			}
			require.NoError(t, f.taskRepo.Save(context.Background(), start))

			if tt.reportAfter {
				server.LastProcessCheck = new(time.Now().Add(time.Second))
			}

			// ACT
			stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantStop, stopTaskID != 0)

			stops, err := f.taskRepo.Find(context.Background(), &filters.FindDaemonTask{
				Tasks: []domain.DaemonTaskType{domain.DaemonTaskTypeServerStop},
			}, nil, nil)
			require.NoError(t, err)
			assert.Len(t, stops, map[bool]int{true: 1, false: 0}[tt.wantStop])
		})
	}
}

func TestService_Suspend_UnreadableTasksErrOnTheSideOfStopping(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newFixture(t)
	f.service.taskRepo = failingFindTaskRepo{DaemonTaskRepository: f.taskRepo}
	server := f.saveServer(t, installedServer(false, new(time.Now())))

	// ACT
	stopTaskID, err := f.service.Suspend(context.Background(), server, nil)

	// ASSERT
	require.NoError(t, err)
	assert.NotZero(t, stopTaskID)
	require.Len(t, f.tasks(t), 1)
	assert.Equal(t, domain.DaemonTaskTypeServerStop, f.tasks(t)[0].Task)
}
