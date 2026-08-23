package uninstall_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/uninstall"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStorageCleaner struct {
	mu      sync.Mutex
	removed []uint64
	err     error
}

func (f *fakeStorageCleaner) DeleteByPlugin(_ context.Context, pluginID uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, pluginID)

	return f.err
}

func (f *fakeStorageCleaner) snapshot() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]uint64(nil), f.removed...)
}

type fakeSecretCleaner struct {
	mu      sync.Mutex
	removed []domain.Uint64ID
	err     error
}

func (f *fakeSecretCleaner) DeleteByPlugin(_ context.Context, pluginID domain.Uint64ID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, pluginID)

	return 2, f.err
}

func (f *fakeSecretCleaner) snapshot() []domain.Uint64ID {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]domain.Uint64ID(nil), f.removed...)
}

// fakeResolverWithRecovery resolves manager IDs and records recovery cancels.
type fakeResolverWithRecovery struct {
	mu        sync.Mutex
	forgotten []domain.Uint64ID
}

func (f *fakeResolverWithRecovery) GetPluginManagerID(domain.Uint64ID) (string, bool) {
	return "", false
}

func (f *fakeResolverWithRecovery) Forget(dbID domain.Uint64ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgotten = append(f.forgotten, dbID)
}

func (f *fakeResolverWithRecovery) snapshot() []domain.Uint64ID {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]domain.Uint64ID(nil), f.forgotten...)
}

func installedPlugin(t *testing.T, pluginRepo *inmemory.PluginRepository) domain.Plugin {
	t.Helper()

	existing := domain.Plugin{
		ID:       pkgplugin.ParsePluginID("testplugin123"),
		Name:     "Test Plugin",
		Version:  "1.0.0",
		Filename: new("testplugin123.wasm"),
		Status:   domain.PluginStatusActive,
	}
	require.NoError(t, pluginRepo.Save(context.Background(), &existing))

	return existing
}

func uninstallRequest() *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plugins/testplugin123", nil)

	return mux.SetURLVars(req, map[string]string{"id": "testplugin123"})
}

func TestUninstall_RemovesStorageAndSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pluginRepo := inmemory.NewPluginRepository()
	storageRepo := inmemory.NewPluginStorageRepository()
	secretRepo := inmemory.NewPluginSecretRepository()
	existing := installedPlugin(t, pluginRepo)
	other := domain.Uint64ID(999)

	for _, entry := range []domain.PluginStorageEntry{
		{PluginID: uint64(existing.ID), Key: "settings", Payload: []byte("a")},
		{PluginID: uint64(existing.ID), Key: "state", Payload: []byte("b")},
		{PluginID: uint64(other), Key: "settings", Payload: []byte("c")},
	} {
		require.NoError(t, storageRepo.Save(ctx, &entry))
	}

	for _, secret := range []domain.PluginSecret{
		{PluginID: existing.ID, Key: "token", Value: "enc:1"},
		{PluginID: other, Key: "token", Value: "enc:2"},
	} {
		require.NoError(t, secretRepo.Upsert(ctx, &secret))
	}

	recorder := &auditCapture{}
	h := uninstall.NewHandler(
		pluginRepo,
		files.NewInMemoryFileManager(),
		nil,
		nil,
		nil,
		nil,
		nil,
		storageRepo,
		secretRepo,
		"plugins",
		api.NewResponder(),
		recorder,
	)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, uninstallRequest())

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	remaining, err := storageRepo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{uint64(existing.ID)}}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, remaining)

	kept, err := storageRepo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{uint64(other)}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, kept, 1)

	remainingSecrets, err := secretRepo.Find(ctx, &filters.FindPluginSecret{PluginIDs: []domain.Uint64ID{existing.ID}}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, remainingSecrets)

	keptSecrets, err := secretRepo.Find(ctx, &filters.FindPluginSecret{PluginIDs: []domain.Uint64ID{other}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, keptSecrets, 1)

	event, ok := findEvent(recorder.snapshot(), audit.EventPluginUninstall)
	require.True(t, ok)
	require.Len(t, event.Extra, 1)
	assert.Equal(t, "secrets_removed", event.Extra[0].Key)
	assert.Equal(t, int64(1), event.Extra[0].Value.Int64())
}

func TestUninstall_CleanupErrorKeepsPluginInstalled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		storage     *fakeStorageCleaner
		secrets     *fakeSecretCleaner
		wantSecrets bool
	}{
		{
			name:    "storage_cleanup_error",
			storage: &fakeStorageCleaner{err: errors.New("storage unavailable")},
			secrets: &fakeSecretCleaner{},
		},
		{
			name:        "secrets_cleanup_error",
			storage:     &fakeStorageCleaner{},
			secrets:     &fakeSecretCleaner{err: errors.New("secrets unavailable")},
			wantSecrets: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			pluginRepo := inmemory.NewPluginRepository()
			fileManager := files.NewInMemoryFileManager()
			existing := installedPlugin(t, pluginRepo)
			require.NoError(t, fileManager.Write(ctx, "plugins/testplugin123.wasm", []byte("wasm")))
			recorder := &auditCapture{}

			h := uninstall.NewHandler(
				pluginRepo,
				fileManager,
				nil,
				nil,
				nil,
				nil,
				nil,
				tt.storage,
				tt.secrets,
				"plugins",
				api.NewResponder(),
				recorder,
			)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, uninstallRequest())

			// Internal failures are not echoed to the client (responder contract);
			// the cause is logged.
			require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
			assert.Equal(t, []uint64{uint64(existing.ID)}, tt.storage.snapshot())
			assert.Equal(t, tt.wantSecrets, len(tt.secrets.snapshot()) == 1)

			// Nothing was torn down: the operator can retry the uninstall.
			plugins, err := pluginRepo.FindAll(ctx, nil, nil)
			require.NoError(t, err)
			require.Len(t, plugins, 1)
			assert.True(t, fileManager.Exists(ctx, "plugins/testplugin123.wasm"))
			assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventPluginUninstall))
		})
	}
}

func TestUninstall_NotInstalledSkipsCleanup(t *testing.T) {
	t.Parallel()
	storage := &fakeStorageCleaner{}
	secrets := &fakeSecretCleaner{}
	resolver := &fakeResolverWithRecovery{}

	h := uninstall.NewHandler(
		inmemory.NewPluginRepository(),
		files.NewInMemoryFileManager(),
		newMockPluginManager(),
		resolver,
		nil,
		nil,
		nil,
		storage,
		secrets,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, uninstallRequest())

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, storage.snapshot())
	assert.Empty(t, secrets.snapshot())
	assert.Empty(t, resolver.snapshot())
}

func TestUninstall_CancelsPendingRecovery(t *testing.T) {
	t.Parallel()
	pluginRepo := inmemory.NewPluginRepository()
	existing := installedPlugin(t, pluginRepo)
	resolver := &fakeResolverWithRecovery{}

	// The plugin is not loaded (its last reload failed); the supervisor may
	// still hold a timer for it.
	h := uninstall.NewHandler(
		pluginRepo,
		files.NewInMemoryFileManager(),
		newMockPluginManager(),
		resolver,
		nil,
		nil,
		nil,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, uninstallRequest())

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, []domain.Uint64ID{existing.ID}, resolver.snapshot())
}
