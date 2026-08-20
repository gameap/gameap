package plugin

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"io"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// The preserved pre-scheduler build of the example plugin: it lacks the
// scheduled_task_handler_* exports and stands in for every plugin compiled
// against an SDK without the scheduler module.
//
//go:embed testdata/server-logger-noscheduler.wasm.gz
var serverLoggerNoSchedulerWASMGz []byte

func decompressNoSchedulerWASM(t *testing.T) []byte {
	t.Helper()

	gr, err := gzip.NewReader(bytes.NewReader(serverLoggerNoSchedulerWASMGz))
	require.NoError(t, err)
	defer func() { _ = gr.Close() }()

	wasm, err := io.ReadAll(gr)
	require.NoError(t, err)

	return wasm
}

func TestPluginServiceWrapper_HandleScheduledTask(t *testing.T) {
	t.Parallel()
	// ARRANGE: the current fixture is built with the sdk/scheduler module and
	// registers a handler in init().
	plugin := loadSharedServerLoggerWASM(t)

	require.True(t, plugin.HasScheduledTaskHandler())

	handler, ok := plugin.Instance.(scheduler.ScheduledTaskHandler)
	require.True(t, ok)

	// ACT
	resp, err := handler.HandleScheduledTask(context.Background(), &scheduler.HandleScheduledTaskRequest{
		TaskName:    "stats-report",
		ScheduledAt: 1_700_000_000_000,
		Attempt:     1,
	})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestPluginServiceWrapper_HandleScheduledTask_NilFunctionReturnsError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	w := &pluginServiceWrapper{handlescheduledtask: nil}

	// ACT
	resp, err := w.HandleScheduledTask(context.Background(), &scheduler.HandleScheduledTaskRequest{
		TaskName: "stats-report",
	})

	// ASSERT
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExportNotFound))
	assert.Contains(t, err.Error(), "scheduled_task_handler_handle_scheduled_task")
	assert.Nil(t, resp)
}

func TestPluginServiceWrapper_HasScheduledTaskHandler_NilFunction(t *testing.T) {
	t.Parallel()
	w := &pluginServiceWrapper{}

	assert.False(t, w.HasScheduledTaskHandler())
}

func TestLoadedPlugin_HasScheduledTaskHandler_InstanceWithoutCapability(t *testing.T) {
	t.Parallel()
	// mockPluginService implements proto.PluginService only; the capability
	// type assertion must miss and report false.
	lp := &LoadedPlugin{Instance: &mockPluginService{}}

	assert.False(t, lp.HasScheduledTaskHandler())
}

type schedulerCapablePluginService struct {
	mockPluginService

	hasHandler bool
}

func (s *schedulerCapablePluginService) HasScheduledTaskHandler() bool {
	return s.hasHandler
}

func TestLoadedPlugin_HasScheduledTaskHandler_InstanceWithCapability(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		hasHandler bool
		want       bool
	}{
		{name: "capability_reports_true", hasHandler: true, want: true},
		{name: "capability_reports_false", hasHandler: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lp := &LoadedPlugin{Instance: &schedulerCapablePluginService{hasHandler: tt.hasHandler}}

			assert.Equal(t, tt.want, lp.HasScheduledTaskHandler())
		})
	}
}

func TestManager_Load_PluginWithoutSchedulerExport(t *testing.T) {
	t.Parallel()
	// ARRANGE
	wasmBytes := decompressNoSchedulerWASM(t)

	cfg := ManagerConfig{
		Libraries: []HostLibrary{
			hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
				return log.Instantiate(ctx, r, &stubLogService{})
			}),
			hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
				return games.Instantiate(ctx, r, &stubGamesService{})
			}),
			hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
				return gamemods.Instantiate(ctx, r, &stubGameModsService{})
			}),
			hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
				return servers.Instantiate(ctx, r, &stubServersService{})
			}),
		},
	}
	m := NewManager(cfg)
	t.Cleanup(func() {
		_ = m.Shutdown(context.Background())
	})

	// ACT
	plugin, err := m.Load(context.Background(), wasmBytes, map[string]string{}, 0)

	// ASSERT: pre-scheduler binaries keep loading unchanged
	require.NoError(t, err)
	require.NotNil(t, plugin)
	assert.False(t, plugin.HasScheduledTaskHandler())

	handler, ok := plugin.Instance.(scheduler.ScheduledTaskHandler)
	require.True(t, ok, "the wrapper always satisfies the handler interface; capability is a separate check")

	resp, err := handler.HandleScheduledTask(context.Background(), &scheduler.HandleScheduledTaskRequest{
		TaskName: "stats-report",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExportNotFound))
	assert.Nil(t, resp)

	info, err := plugin.Instance.GetInfo(context.Background(), &proto.GetInfoRequest{})
	require.NoError(t, err, "a missing scheduler export must not break the plugin")
	assert.NotEmpty(t, info.Id)
}
