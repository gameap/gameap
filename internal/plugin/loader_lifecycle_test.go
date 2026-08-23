package plugin

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type refreshRecorder struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (r *refreshRecorder) RefreshSubscriptions(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++

	return r.err
}

func (r *refreshRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls
}

func loadedPluginNamed(id string) *pkgplugin.LoadedPlugin {
	return &pkgplugin.LoadedPlugin{
		Info:    &proto.PluginInfo{Id: id, Name: id, Version: "1.0.0"},
		Enabled: true,
	}
}

// failingManager fails Load for the wasm files whose content is listed in
// broken and loads everything else.
func failingManager(broken ...string) *mockPluginManager {
	return &mockPluginManager{
		loadFunc: func(_ context.Context, wasmBytes []byte, _ map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
			for _, content := range broken {
				if string(wasmBytes) == content {
					return nil, errors.New("simulated load failure for " + content)
				}
			}

			return loadedPluginNamed("loaded-" + string(wasmBytes) + "-" + pluginIDString(pluginID)), nil
		},
	}
}

func pluginIDString(id uint64) string {
	return pkgplugin.CompactPluginID(domain.Uint64ID(id))
}

func seedPlugin(ctx context.Context, t *testing.T, repo *inmemory.PluginRepository, id domain.Uint64ID, status domain.PluginStatus) *domain.Plugin {
	t.Helper()

	plugin := &domain.Plugin{
		ID:       id,
		Name:     "plugin-" + pluginIDString(uint64(id)),
		Version:  "1.0.0",
		Filename: new(pluginIDString(uint64(id)) + ".wasm"),
		Status:   status,
	}
	require.NoError(t, repo.Save(ctx, plugin))

	return plugin
}

func findPlugin(ctx context.Context, t *testing.T, repo *inmemory.PluginRepository, id domain.Uint64ID) domain.Plugin {
	t.Helper()

	plugins, err := repo.Find(ctx, filters.FindPluginByIDs(id), nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1)

	return plugins[0]
}

func TestLoader_LoadAll_continues_past_failing_plugin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	broken := seedPlugin(ctx, t, repo, 101, domain.PluginStatusActive)
	healthy := seedPlugin(ctx, t, repo, 102, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*broken.Filename, []byte("broken")))
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*healthy.Filename, []byte("healthy")))

	manager := failingManager("broken")
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	err := loader.LoadAll(ctx)
	require.NoError(t, err, "non-strict mode starts the panel with the healthy subset")

	assert.Equal(t, 2, manager.loadedCount, "every plugin is attempted")

	_, ok := loader.GetPluginManagerID(healthy.ID)
	assert.True(t, ok)
	_, ok = loader.GetPluginManagerID(broken.ID)
	assert.False(t, ok)

	brokenRow := findPlugin(ctx, t, repo, broken.ID)
	assert.Equal(t, domain.PluginStatusError, brokenRow.Status)
	require.NotNil(t, brokenRow.LastError)
	assert.Contains(t, *brokenRow.LastError, "simulated load failure for broken")
	assert.NotNil(t, brokenRow.LastErrorAt)
	assert.Nil(t, brokenRow.LastLoadedAt)

	healthyRow := findPlugin(ctx, t, repo, healthy.ID)
	assert.Equal(t, domain.PluginStatusActive, healthyRow.Status)
	assert.Nil(t, healthyRow.LastError)
	assert.NotNil(t, healthyRow.LastLoadedAt)
}

func TestLoader_LoadAll_strict_reports_failures_after_trying_everything(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	broken := seedPlugin(ctx, t, repo, 201, domain.PluginStatusActive)
	healthy := seedPlugin(ctx, t, repo, 202, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*broken.Filename, []byte("broken")))
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*healthy.Filename, []byte("healthy")))

	manager := failingManager("broken")
	loader := NewLoader(manager, fileManager, repo, nil, "plugins", WithStrictLoad(true))

	err := loader.LoadAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load plugin "+broken.Name)
	assert.Contains(t, err.Error(), "simulated load failure for broken")

	assert.Equal(t, 2, manager.loadedCount, "strict mode still attempts every plugin")
	assert.Equal(t, domain.PluginStatusError, findPlugin(ctx, t, repo, broken.ID).Status)
	assert.Equal(t, domain.PluginStatusActive, findPlugin(ctx, t, repo, healthy.ID).Status)
}

func TestLoader_LoadAll_retries_error_rows_and_clears_the_error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	plugin := seedPlugin(ctx, t, repo, 301, domain.PluginStatusError)
	plugin.LastError = new("boom")
	plugin.LastErrorAt = new(time.Now())
	require.NoError(t, repo.Save(ctx, plugin))
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fixed")))

	manager := failingManager()
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	require.NoError(t, loader.LoadAll(ctx))

	assert.Equal(t, 1, manager.loadedCount)

	row := findPlugin(ctx, t, repo, plugin.ID)
	assert.Equal(t, domain.PluginStatusActive, row.Status)
	assert.Nil(t, row.LastError)
	assert.Nil(t, row.LastErrorAt)
	assert.NotNil(t, row.LastLoadedAt)
}

func TestLoader_LoadAll_skips_disabled_and_updating_rows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	disabled := seedPlugin(ctx, t, repo, 401, domain.PluginStatusDisabled)
	updating := seedPlugin(ctx, t, repo, 402, domain.PluginStatusUpdating)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*disabled.Filename, []byte("disabled")))
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*updating.Filename, []byte("updating")))

	manager := failingManager()
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	require.NoError(t, loader.LoadAll(ctx))

	assert.Equal(t, 0, manager.loadedCount)
	assert.Equal(t, domain.PluginStatusDisabled, findPlugin(ctx, t, repo, disabled.ID).Status)
	assert.Equal(t, domain.PluginStatusUpdating, findPlugin(ctx, t, repo, updating.ID).Status)
}

func TestLoader_LoadAll_autoload_missing_file(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		strict    bool
		wantError string
	}{
		{name: "non_strict_continues", strict: false, wantError: ""},
		{name: "strict_reports", strict: true, wantError: "autoload plugin file not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fileManager := files.NewInMemoryFileManager()
			repo := inmemory.NewPluginRepository()

			healthy := seedPlugin(ctx, t, repo, 501, domain.PluginStatusActive)
			require.NoError(t, fileManager.Write(ctx, "plugins/"+*healthy.Filename, []byte("healthy")))

			manager := failingManager()
			loader := NewLoader(manager, fileManager, repo, []string{"missing.wasm"}, "plugins",
				WithStrictLoad(tt.strict))

			err := loader.LoadAll(ctx)
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}

			assert.Equal(t, 1, manager.loadedCount, "installed plugins load regardless of the autoload failure")
		})
	}
}

func TestLoader_ProcessAutoLoad_attempts_every_entry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()
	require.NoError(t, fileManager.Write(ctx, "plugins/second.wasm", []byte("second")))

	manager := failingManager()
	loader := NewLoader(manager, fileManager, repo, []string{"first.wasm", "second.wasm"}, "plugins")

	err := loader.processAutoLoad(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "autoload first.wasm")

	plugins, err := repo.FindAll(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, plugins, 1, "the second entry is registered despite the first failing")
}

func TestLoadErrorText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input error
		want  string
	}{
		{name: "nil", input: nil, want: ""},
		{name: "plain_text_kept", input: errors.New("plugin file not found: plugins/1.wasm"), want: "plugin file not found: plugins/1.wasm"},
		{
			name: "go_stack_trace_removed",
			//nolint:revive // mimics the wazero runtime error format
			input: errors.New("failed to call api_version\nwasm stack trace:\n\tfunc1()\nGo runtime stack trace:\ngoroutine 1 [running]:"),
			want:  "failed to call api_version\nwasm stack trace:\n\tfunc1()",
		},
		{name: "whitespace_trimmed", input: errors.New("  spaced  \n"), want: "spaced"}, //nolint:revive // deliberate padding
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, LoadErrorText(tt.input))
		})
	}

	t.Run("long_text_is_capped_on_a_rune_boundary", func(t *testing.T) {
		t.Parallel()
		text := strings.Repeat("я", maxLoadErrorLen)

		got := LoadErrorText(errors.New(text))

		assert.True(t, strings.HasSuffix(got, "…"))
		assert.LessOrEqual(t, len(got), maxLoadErrorLen+len("…"))
		assert.Equal(t, strings.Repeat("я", maxLoadErrorLen/2), strings.TrimSuffix(got, "…"))
	})
}

func TestLoader_Reload(t *testing.T) {
	t.Parallel()

	t.Run("reloads_installed_plugin_and_records_success", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		fileManager := files.NewInMemoryFileManager()
		repo := inmemory.NewPluginRepository()
		refresher := &refreshRecorder{}

		plugin := seedPlugin(ctx, t, repo, 601, domain.PluginStatusError)
		plugin.LastError = new("http handler timed out")
		require.NoError(t, repo.Save(ctx, plugin))
		require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

		var unloaded []string
		manager := failingManager()
		manager.unloadFunc = func(_ context.Context, pluginID string) error {
			unloaded = append(unloaded, pluginID)

			return nil
		}

		loader := NewLoader(manager, fileManager, repo, nil, "plugins", WithSubscriptionRefresher(refresher))
		loader.RegisterPluginID(plugin.ID, "old-instance")

		row, loaded, err := loader.Reload(ctx, plugin.ID)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		require.NotNil(t, row)

		assert.Equal(t, []string{"old-instance"}, unloaded)
		assert.Equal(t, domain.PluginStatusActive, row.Status)
		assert.Nil(t, row.LastError)
		assert.Equal(t, 1, refresher.count())

		managerID, ok := loader.GetPluginManagerID(plugin.ID)
		require.True(t, ok)
		assert.Equal(t, loaded.Info.Id, managerID)

		stored := findPlugin(ctx, t, repo, plugin.ID)
		assert.Equal(t, domain.PluginStatusActive, stored.Status)
		assert.Nil(t, stored.LastError)
	})

	t.Run("plugin_not_loaded_yet_is_tolerated", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		fileManager := files.NewInMemoryFileManager()
		repo := inmemory.NewPluginRepository()

		plugin := seedPlugin(ctx, t, repo, 602, domain.PluginStatusError)
		require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

		manager := failingManager()
		manager.unloadFunc = func(_ context.Context, pluginID string) error {
			return errors.Wrapf(pkgplugin.ErrPluginNotFound, "plugin: %s", pluginID)
		}

		loader := NewLoader(manager, fileManager, repo, nil, "plugins")

		_, loaded, err := loader.Reload(ctx, plugin.ID)
		require.NoError(t, err)
		assert.NotNil(t, loaded)
	})

	t.Run("not_installed", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(failingManager(), files.NewInMemoryFileManager(), inmemory.NewPluginRepository(), nil, "plugins")

		_, _, err := loader.Reload(context.Background(), 603)
		require.ErrorIs(t, err, ErrPluginNotInstalled)
	})

	t.Run("operator_states_are_refused", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			status    domain.PluginStatus
			wantError error
		}{
			{status: domain.PluginStatusDisabled, wantError: ErrPluginDisabled},
			{status: domain.PluginStatusUpdating, wantError: ErrPluginUpdating},
		}

		for _, tt := range tests {
			ctx := context.Background()
			repo := inmemory.NewPluginRepository()
			plugin := seedPlugin(ctx, t, repo, 604, tt.status)

			manager := failingManager()
			loader := NewLoader(manager, files.NewInMemoryFileManager(), repo, nil, "plugins")

			row, _, err := loader.Reload(ctx, plugin.ID)
			require.ErrorIs(t, err, tt.wantError)
			require.NotNil(t, row)
			assert.Equal(t, 0, manager.loadedCount)
		}
	})

	t.Run("load_failure_marks_error", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		fileManager := files.NewInMemoryFileManager()
		repo := inmemory.NewPluginRepository()
		refresher := &refreshRecorder{}

		plugin := seedPlugin(ctx, t, repo, 605, domain.PluginStatusActive)
		require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("broken")))

		loader := NewLoader(failingManager("broken"), fileManager, repo, nil, "plugins", WithSubscriptionRefresher(refresher))

		row, loaded, err := loader.Reload(ctx, plugin.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "simulated load failure")
		assert.Nil(t, loaded)
		require.NotNil(t, row)
		assert.Equal(t, domain.PluginStatusError, row.Status)
		require.NotNil(t, row.LastError)
		assert.Contains(t, *row.LastError, "simulated load failure for broken")
		assert.Equal(t, 0, refresher.count())

		stored := findPlugin(ctx, t, repo, plugin.ID)
		assert.Equal(t, domain.PluginStatusError, stored.Status)
	})

	t.Run("unload_failure_is_reported_without_loading", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		fileManager := files.NewInMemoryFileManager()
		repo := inmemory.NewPluginRepository()

		plugin := seedPlugin(ctx, t, repo, 606, domain.PluginStatusActive)
		require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

		manager := failingManager()
		manager.unloadFunc = func(context.Context, string) error {
			return errors.New("runtime close failed")
		}
		loader := NewLoader(manager, fileManager, repo, nil, "plugins")

		_, _, err := loader.Reload(ctx, plugin.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unload plugin")
		assert.Contains(t, err.Error(), "runtime close failed")
		assert.Equal(t, 0, manager.loadedCount)
	})
}

func TestLoader_GetDBPluginID_accepts_compact_form(t *testing.T) {
	t.Parallel()
	loader := NewLoader(failingManager(), files.NewInMemoryFileManager(), inmemory.NewPluginRepository(), nil, "plugins")
	loader.RegisterPluginID(777, "some-plugin")

	compact := pkgplugin.CompactPluginID(pkgplugin.ParsePluginID("some-plugin"))

	dbID, ok := loader.GetDBPluginID(compact)
	require.True(t, ok)
	assert.Equal(t, domain.Uint64ID(777), dbID)

	dbID, ok = loader.GetDBPluginID("some-plugin")
	require.True(t, ok)
	assert.Equal(t, domain.Uint64ID(777), dbID)

	_, ok = loader.GetDBPluginID("unknown")
	assert.False(t, ok)
}

func TestLoader_Forget_without_supervisor_is_safe(t *testing.T) {
	t.Parallel()
	loader := NewLoader(failingManager(), files.NewInMemoryFileManager(), inmemory.NewPluginRepository(), nil, "plugins")

	loader.Forget(1)
}

func TestLoader_reload_honours_cancellation_without_marking_error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	plugin := seedPlugin(ctx, t, repo, 607, domain.PluginStatusError)
	plugin.LastError = new("http handler timed out")
	require.NoError(t, repo.Save(ctx, plugin))
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

	manager := failingManager()
	manager.loadFunc = func(ctx context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
		return nil, ctx.Err()
	}
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	_, _, err := loader.reload(cancelled, plugin.ID)
	require.ErrorIs(t, err, context.Canceled)

	row := findPlugin(ctx, t, repo, plugin.ID)
	assert.Equal(t, domain.PluginStatusError, row.Status)
	require.NotNil(t, row.LastError)
	assert.Equal(t, "http handler timed out", *row.LastError, "the previous reason is kept")
}

// deadlineCheckingRepo refuses writes on a done context, as a database
// driver would.
type deadlineCheckingRepo struct {
	*inmemory.PluginRepository
}

func (r deadlineCheckingRepo) Save(ctx context.Context, plugin *domain.Plugin) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return r.PluginRepository.Save(ctx, plugin)
}

func TestLoader_reload_records_expired_deadline_as_error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	plugin := seedPlugin(ctx, t, repo, 609, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("slow")))

	manager := failingManager()
	manager.loadFunc = func(ctx context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
		return nil, errors.Wrap(ctx.Err(), "module start")
	}
	loader := NewLoader(manager, fileManager, deadlineCheckingRepo{repo}, nil, "plugins")

	// The supervisor's budget runs out before the loader's own: the reload
	// sees a deadline, not a cancellation.
	expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()

	_, _, err := loader.reload(expired, plugin.ID)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	row := findPlugin(ctx, t, repo, plugin.ID)
	assert.Equal(t, domain.PluginStatusError, row.Status, "a reload that ran out of time is the plugin's failure")
	require.NotNil(t, row.LastError)
	assert.Contains(t, *row.LastError, "module start: context deadline exceeded")
}

func TestLoader_Reload_is_detached_from_caller_cancellation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileManager := files.NewInMemoryFileManager()
	repo := inmemory.NewPluginRepository()

	plugin := seedPlugin(ctx, t, repo, 608, domain.PluginStatusActive)
	require.NoError(t, fileManager.Write(ctx, "plugins/"+*plugin.Filename, []byte("fine")))

	manager := failingManager()
	manager.loadFunc = func(ctx context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return loadedPluginNamed("fresh"), nil
	}
	loader := NewLoader(manager, fileManager, repo, nil, "plugins")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	_, loaded, err := loader.Reload(cancelled, plugin.ID)
	require.NoError(t, err, "an operator's dropped request must not abort the reload")
	assert.NotNil(t, loaded)
}
