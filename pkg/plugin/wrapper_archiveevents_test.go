package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

func TestPluginServiceWrapper_HandleArchiveEvents(t *testing.T) {
	// ARRANGE: the current fixture is built with the extended sdk/nodefs
	// module and registers an ArchiveEventsHandler in init().
	plugin := loadSharedServerLoggerWASM(t)

	require.True(t, plugin.HasArchiveEventsHandler())

	handler, ok := plugin.Instance.(nodefs.ArchiveEventsHandler)
	require.True(t, ok)

	// ACT + ASSERT: both callbacks round-trip through the guest.
	progressResp, err := handler.HandleArchiveProgress(context.Background(), &nodefs.HandleArchiveProgressRequest{
		OperationId:    "op-1",
		NodeId:         1,
		FilesProcessed: 2,
		FilesTotal:     5,
		BytesProcessed: 100,
		BytesTotal:     1000,
		CurrentEntry:   "maps/de_dust2.bsp",
	})
	require.NoError(t, err)
	require.NotNil(t, progressResp)

	completedResp, err := handler.HandleArchiveCompleted(context.Background(), &nodefs.HandleArchiveCompletedRequest{
		OperationId:    "op-1",
		NodeId:         1,
		Success:        true,
		FilesProcessed: 5,
		BytesProcessed: 1000,
		ArchiveSize:    512,
		Format:         nodefs.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
	})
	require.NoError(t, err)
	require.NotNil(t, completedResp)
}

func TestPluginServiceWrapper_HandleArchiveEvents_NilFunctionsReturnError(t *testing.T) {
	// ARRANGE
	w := &pluginServiceWrapper{}

	// ACT + ASSERT
	progressResp, err := w.HandleArchiveProgress(context.Background(), &nodefs.HandleArchiveProgressRequest{
		OperationId: "op-1",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExportNotFound))
	assert.Contains(t, err.Error(), "archive_events_handler_handle_archive_progress")
	assert.Nil(t, progressResp)

	completedResp, err := w.HandleArchiveCompleted(context.Background(), &nodefs.HandleArchiveCompletedRequest{
		OperationId: "op-1",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExportNotFound))
	assert.Contains(t, err.Error(), "archive_events_handler_handle_archive_completed")
	assert.Nil(t, completedResp)
}

func TestPluginServiceWrapper_HasArchiveEventsHandler(t *testing.T) {
	tests := []struct {
		name     string
		progress bool
		complete bool
		want     bool
	}{
		{name: "both_exports_present", progress: true, complete: true, want: true},
		{name: "progress_only_is_not_enough", progress: true, complete: false, want: false},
		{name: "complete_only_is_not_enough", progress: false, complete: true, want: false},
		{name: "no_exports", progress: false, complete: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &pluginServiceWrapper{}
			if tt.progress {
				w.handlearchiveprogress = unusedExport{}
			}
			if tt.complete {
				w.handlearchivecompleted = unusedExport{}
			}

			assert.Equal(t, tt.want, w.HasArchiveEventsHandler())
		})
	}
}

type archiveCapablePluginService struct {
	mockPluginService

	hasHandler bool
}

func (s *archiveCapablePluginService) HasArchiveEventsHandler() bool {
	return s.hasHandler
}

func TestLoadedPlugin_HasArchiveEventsHandler(t *testing.T) {
	t.Run("instance_without_capability_reports_false", func(t *testing.T) {
		lp := &LoadedPlugin{Instance: &mockPluginService{}}

		assert.False(t, lp.HasArchiveEventsHandler())
	})

	t.Run("capability_reports_true", func(t *testing.T) {
		lp := &LoadedPlugin{Instance: &archiveCapablePluginService{hasHandler: true}}

		assert.True(t, lp.HasArchiveEventsHandler())
	})

	t.Run("capability_reports_false", func(t *testing.T) {
		lp := &LoadedPlugin{Instance: &archiveCapablePluginService{hasHandler: false}}

		assert.False(t, lp.HasArchiveEventsHandler())
	})
}

func TestManager_Load_PluginWithoutArchiveEventsExports(t *testing.T) {
	// ARRANGE: the frozen pre-scheduler fixture also predates the archive
	// events service — it proves old binaries load with the capability off.
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

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, plugin)
	assert.False(t, plugin.HasArchiveEventsHandler(),
		"binaries built before the archive events service must load with the capability off")
}
