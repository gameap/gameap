package plugin

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecovery_end_to_end_with_real_runtime drives the whole chain on a real
// wazero runtime: the guest terminates its module, the manager's disable
// hook reaches the supervisor, the row is marked, and the plugin is reloaded
// and enabled again without anyone touching it.
func TestRecovery_end_to_end_with_real_runtime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	wasmBytes, err := os.ReadFile("../../pkg/plugin/testdata/misbehaving.wasm")
	require.NoError(t, err)

	var supervisor *Supervisor
	manager := pkgplugin.NewManager(pkgplugin.ManagerConfig{
		GuestLogger: slog.New(slog.DiscardHandler),
		OnPluginDisabled: func(pluginID string, dbID uint64, reason string) {
			supervisor.OnPluginDisabled(pluginID, dbID, reason)
		},
	})
	t.Cleanup(func() { _ = manager.Shutdown(ctx) })

	fileManager := files.NewInMemoryFileManager()
	require.NoError(t, fileManager.Write(ctx, "plugins/misbehaving.wasm", wasmBytes))

	repo := inmemory.NewPluginRepository()
	const dbID domain.Uint64ID = 4242
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID: dbID, Name: "misbehaving", Version: "1.0.0", Filename: new("misbehaving.wasm"),
		Status: domain.PluginStatusActive,
	}))

	dispatcher := pkgplugin.NewDispatcher(manager, slog.New(slog.DiscardHandler))
	recorder := &auditCapture{}
	loader := NewLoader(manager, fileManager, repo, nil, "plugins", WithSubscriptionRefresher(dispatcher))
	supervisor = NewSupervisor(loader, repo, recorder, RecoveryOptions{
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		MaxAttempts:  3,
	}, slog.New(slog.DiscardHandler))
	t.Cleanup(supervisor.Stop)

	require.NoError(t, loader.LoadAll(ctx))

	loaded, ok := manager.GetPlugin("misbehaving")
	require.True(t, ok)
	assert.Equal(t, uint64(dbID), loaded.DBID)

	// The guest prints a line and exits its module.
	_, err = loaded.Instance.HandleEvent(ctx, &proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START})
	require.Error(t, err)
	assert.False(t, loaded.IsEnabled())

	// The fresh instance is registered before the audit record is written,
	// so wait for both side effects of the reload.
	require.Eventually(t, func() bool {
		current, exists := manager.GetPlugin("misbehaving")

		return exists && current != loaded && current.IsEnabled() &&
			len(recorder.ofType(audit.EventPluginReloaded)) == 1
	}, 5*time.Second, 10*time.Millisecond, "the supervisor must reload the plugin into a fresh enabled instance")

	row := findPlugin(ctx, t, repo, dbID)
	assert.Equal(t, domain.PluginStatusActive, row.Status)
	assert.Nil(t, row.LastError)
	assert.NotNil(t, row.LastLoadedAt)

	disabled := recorder.ofType(audit.EventPluginDisabled)
	require.Len(t, disabled, 1)
	assert.Equal(t, "guest_exited", disabled[0].Reason)

	reloaded := recorder.ofType(audit.EventPluginReloaded)
	require.Len(t, reloaded, 1)
	assert.Equal(t, audit.OutcomeSuccess, reloaded[0].Outcome)

	// The fresh instance answers calls again.
	reloadedPlugin, _ := manager.GetPlugin("misbehaving")
	info, err := reloadedPlugin.Instance.GetInfo(ctx, &proto.GetInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, "misbehaving", info.Id)
}
