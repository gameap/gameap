package servercontrol

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerControlService_SuspendedServerIsRefused(t *testing.T) {
	t.Parallel()

	refused := map[string]controlOperation{
		"start":     (*Service).Start,
		"restart":   (*Service).Restart,
		"update":    (*Service).Update,
		"install":   (*Service).Install,
		"reinstall": (*Service).Reinstall,
	}

	for name, op := range refused {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			settingRepo := inmemory.NewServerSettingRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()
			pluginDispatcher := &recordingPluginDispatcher{}
			taskDispatcher := &stubTaskDispatcher{}
			service := NewService(
				taskRepo, settingRepo, services.NewNilTransactionManager(),
				WithPluginDispatcher(pluginDispatcher),
				WithTaskDispatcher(taskDispatcher),
			)
			require.NoError(t, settingRepo.Save(context.Background(), &domain.ServerSetting{
				ServerID: 1,
				Name:     autostartSettingKey,
				Value:    domain.NewServerSettingValue(true),
			}))
			server := &domain.Server{ID: 1, DSID: 10, Blocked: true, StartCommand: new("./start.sh")}

			// ACT
			taskID, err := op(service, context.Background(), server)

			// ASSERT
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrServerBlocked)
			assert.Equal(t, "server is blocked", err.Error())

			var withStatus interface{ HTTPStatus() int }
			require.True(t, errors.As(err, &withStatus), "the refusal must carry an HTTP status")
			assert.Equal(t, http.StatusForbidden, withStatus.HTTPStatus())

			assert.Zero(t, taskID)
			assert.Empty(t, findAllTasks(t, taskRepo), "a refused operation must not create tasks")
			assert.Empty(t, taskDispatcher.dispatchedTypes(), "a refused operation must not reach the daemon")
			assert.Empty(t, pluginDispatcher.syncEvents, "a refused operation must not reach plugins")
			assert.Empty(t, pluginDispatcher.asyncEvents, "a refused operation must not reach plugins")

			settings, err := settingRepo.Find(context.Background(), &filters.FindServerSetting{ServerIDs: []uint{1}}, nil, nil)
			require.NoError(t, err)
			require.Len(t, settings, 1, "autostart_current must not be touched")
			assert.Equal(t, autostartSettingKey, settings[0].Name)
		})
	}
}

func TestServerControlService_SuspendedServerCanBeStopped(t *testing.T) {
	t.Parallel()

	// ARRANGE
	settingRepo := inmemory.NewServerSettingRepository()
	taskRepo := inmemory.NewDaemonTaskRepository()
	taskDispatcher := &stubTaskDispatcher{}
	service := NewService(
		taskRepo, settingRepo, services.NewNilTransactionManager(),
		WithTaskDispatcher(taskDispatcher),
	)
	server := &domain.Server{ID: 1, DSID: 10, Blocked: true, StartCommand: new("./start.sh")}

	// ACT
	_, err := service.Stop(context.Background(), server)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, []domain.DaemonTaskType{domain.DaemonTaskTypeServerStop}, taskDispatcher.dispatchedTypes())
}
