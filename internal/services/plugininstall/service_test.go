package plugininstall_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckNotInstalled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		setupRepo  func(*inmemory.PluginRepository)
		dbID       domain.Uint64ID
		wantError  string
		wantStatus int
	}{
		{
			name:       "plugin_not_installed",
			setupRepo:  func(_ *inmemory.PluginRepository) {},
			dbID:       12345,
			wantError:  "",
			wantStatus: 0,
		},
		{
			name: "plugin_already_installed_returns_409",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:     12345,
					Name:   "Test Plugin",
					Status: domain.PluginStatusActive,
				})
			},
			dbID:       12345,
			wantError:  "plugin already installed",
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewPluginRepository()
			tt.setupRepo(repo)

			err := plugininstall.CheckNotInstalled(ctx, repo, tt.dbID)

			if tt.wantError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				var httpErr interface{ HTTPStatus() int }
				if assert.ErrorAs(t, err, &httpErr) {
					assert.Equal(t, tt.wantStatus, httpErr.HTTPStatus())
				}
			}
		})
	}
}

func TestBuildPluginRecord(t *testing.T) {
	tests := []struct {
		name       string
		dbID       domain.Uint64ID
		loaded     *pkgplugin.LoadedPlugin
		filename   string
		source     string
		wantRecord *domain.Plugin
	}{
		{
			name: "builds_correct_record",
			dbID: 12345,
			loaded: &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:          "testplugin",
					Name:        "Test Plugin",
					Version:     "1.0.0",
					Description: "A test plugin",
					Author:      "Test Author",
					ApiVersion:  "v1",
				},
			},
			filename: "12345.wasm",
			source:   "file://12345.wasm",
		},
		{
			name: "builds_record_from_store",
			dbID: 67890,
			loaded: &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:          "storeplugin",
					Name:        "Store Plugin",
					Version:     "2.0.0",
					Description: "A store plugin",
					Author:      "Store Author",
					ApiVersion:  "v2",
				},
			},
			filename: "storeplugin.wasm",
			source:   "https://store.gameap.com/plugins/storeplugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := plugininstall.BuildPluginRecord(tt.dbID, tt.loaded, tt.filename, tt.source)

			assert.Equal(t, tt.dbID, record.ID)
			assert.Equal(t, tt.loaded.Info.Name, record.Name)
			assert.Equal(t, tt.loaded.Info.Version, record.Version)
			assert.Equal(t, tt.loaded.Info.Description, record.Description)
			assert.Equal(t, tt.loaded.Info.Author, record.Author)
			assert.Equal(t, tt.loaded.Info.ApiVersion, record.APIVersion)
			require.NotNil(t, record.Filename)
			assert.Equal(t, tt.filename, *record.Filename)
			require.NotNil(t, record.Source)
			assert.Equal(t, tt.source, *record.Source)
			assert.Equal(t, domain.PluginStatusActive, record.Status)
			require.NotNil(t, record.InstalledAt)
		})
	}
}

func TestBuildPluginRecord_permissions(t *testing.T) {
	tests := []struct {
		name     string
		declared []string
		want     []domain.PluginPermission
	}{
		{
			name:     "no_permissions_declared",
			declared: nil,
			want:     nil,
		},
		{
			name:     "known_permissions_are_carried_over",
			declared: []string{"manage_rbac", "listen_events"},
			want: []domain.PluginPermission{
				domain.PluginPermissionManageRBAC,
				domain.PluginPermissionListenEvents,
			},
		},
		{
			name:     "unknown_permissions_are_dropped",
			declared: []string{"manage_rbac", "become_root"},
			want:     []domain.PluginPermission{domain.PluginPermissionManageRBAC},
		},
		{
			name:     "only_unknown_permissions_grants_nothing",
			declared: []string{"become_root"},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := plugininstall.BuildPluginRecord(1, &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Name:                "Test Plugin",
					RequiredPermissions: tt.declared,
				},
			}, "1.wasm", "file://1.wasm")

			assert.Equal(t, tt.want, record.RequiredPermissions)
			assert.Equal(t, tt.want, record.AllowedPermissions,
				"confirming the install is the grant")
		})
	}
}

type fakeLoaderManager struct {
	loadErr     error
	loadedID    string
	loadedCount int
	gotPluginID uint64
}

func (f *fakeLoaderManager) Load(
	_ context.Context,
	_ []byte,
	_ map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	f.loadedCount++
	f.gotPluginID = pluginID
	if f.loadErr != nil {
		return nil, f.loadErr
	}

	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{
			Id:      f.loadedID,
			Name:    "Test Plugin",
			Version: "1.0.0",
		},
	}, nil
}

func (f *fakeLoaderManager) Unload(_ context.Context, _ string) error { return nil }

func (f *fakeLoaderManager) LoadTransient(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return f.Load(ctx, wasmBytes, config, pluginID)
}

func (f *fakeLoaderManager) GetPlugin(_ string) (*pkgplugin.LoadedPlugin, bool) {
	return nil, false
}

func (f *fakeLoaderManager) GetPlugins() []*pkgplugin.LoadedPlugin { return nil }

func (f *fakeLoaderManager) Shutdown(_ context.Context) error { return nil }

func TestTryLoadPlugin(t *testing.T) {
	ctx := context.Background()
	const pluginsDir = "plugins"
	const wasmFilename = "test-plugin.wasm"
	const fullWasmPath = pluginsDir + "/" + wasmFilename

	tests := []struct {
		name           string
		nilLoader      bool
		loaderLoadErr  error
		loaderLoadedID string
		pluginRecord   *domain.Plugin
		writeWASMFile  bool
		wantError      string
		assertState    func(
			t *testing.T,
			mgr *fakeLoaderManager,
			loader *plugin.Loader,
			repo *inmemory.PluginRepository,
			rec *domain.Plugin,
		)
	}{
		{
			name:           "successful_load",
			nilLoader:      false,
			loaderLoadedID: "wasm-internal-id",
			pluginRecord: &domain.Plugin{
				ID:     12345,
				Name:   "Test Plugin",
				Status: domain.PluginStatusActive,
			},
			writeWASMFile: true,
			wantError:     "",
			assertState: func(
				t *testing.T,
				mgr *fakeLoaderManager,
				loader *plugin.Loader,
				_ *inmemory.PluginRepository,
				rec *domain.Plugin,
			) {
				t.Helper()

				assert.Equal(t, 1, mgr.loadedCount, "manager.Load must be called exactly once")
				assert.Equal(t, uint64(rec.ID), mgr.gotPluginID, "plugin DB ID must be passed to manager.Load")

				registered, ok := loader.GetPluginManagerID(rec.ID)
				assert.True(t, ok, "RegisterPluginID must record the DB ID after successful load")
				assert.Equal(t, "wasm-internal-id", registered, "registered manager ID must come from the loaded plugin info")

				assert.Equal(t, domain.PluginStatusActive, rec.Status, "status must remain unchanged on successful load")
			},
		},
		{
			name:          "loader_returns_error_wraps_and_marks_status_error",
			nilLoader:     false,
			loaderLoadErr: errors.New("wasm parse failed"),
			pluginRecord: &domain.Plugin{
				ID:     54321,
				Name:   "Bad Plugin",
				Status: domain.PluginStatusActive,
			},
			writeWASMFile: true,
			wantError:     "failed to load plugin",
			assertState: func(
				t *testing.T,
				mgr *fakeLoaderManager,
				loader *plugin.Loader,
				repo *inmemory.PluginRepository,
				rec *domain.Plugin,
			) {
				t.Helper()

				assert.Equal(t, 1, mgr.loadedCount)
				assert.Equal(t, domain.PluginStatusError, rec.Status, "status must be set to error after a failed load")

				_, ok := loader.GetPluginManagerID(rec.ID)
				assert.False(t, ok, "RegisterPluginID must NOT be called when load fails")

				saved, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, saved, 1, "repo.Save must be called with the error-status record after a failed load")
				assert.Equal(t, domain.PluginStatusError, saved[0].Status, "persisted record must reflect the error status")
				assert.Equal(t, rec.ID, saved[0].ID)
			},
		},
		{
			name:           "nil_loader_skips_load",
			nilLoader:      true,
			loaderLoadedID: "",
			pluginRecord: &domain.Plugin{
				ID:     99999,
				Name:   "Skipped Plugin",
				Status: domain.PluginStatusActive,
			},
			writeWASMFile: false,
			wantError:     "",
			assertState: func(
				t *testing.T,
				mgr *fakeLoaderManager,
				_ *plugin.Loader,
				repo *inmemory.PluginRepository,
				rec *domain.Plugin,
			) {
				t.Helper()

				assert.Equal(t, 0, mgr.loadedCount, "manager.Load must not be called when loader is nil")
				assert.Equal(t, domain.PluginStatusActive, rec.Status, "status must remain unchanged when loader is nil")

				saved, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				assert.Empty(t, saved, "repo must not be touched when loader is nil")
			},
		},
		{
			name:          "load_returns_error_when_repo_save_also_fails",
			nilLoader:     false,
			loaderLoadErr: errors.New("wasm boom"),
			pluginRecord: &domain.Plugin{
				ID:     0,
				Name:   "Unsavable Plugin",
				Status: domain.PluginStatusActive,
			},
			writeWASMFile: true,
			wantError:     "failed to load plugin",
			assertState: func(
				t *testing.T,
				mgr *fakeLoaderManager,
				loader *plugin.Loader,
				repo *inmemory.PluginRepository,
				rec *domain.Plugin,
			) {
				t.Helper()

				assert.Equal(t, 1, mgr.loadedCount)
				assert.Equal(t, domain.PluginStatusError, rec.Status, "status must still be set to error even if repo.Save then fails")

				_, ok := loader.GetPluginManagerID(rec.ID)
				assert.False(t, ok, "RegisterPluginID must NOT be called when load fails")

				saved, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				assert.Empty(t, saved, "repo must remain empty when Save itself fails (ID == 0)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewPluginRepository()
			fileManager := files.NewInMemoryFileManager()
			if tt.writeWASMFile {
				require.NoError(t, fileManager.Write(ctx, fullWasmPath, []byte("\x00asm\x01\x00\x00\x00")))
			}

			mgr := &fakeLoaderManager{
				loadErr:  tt.loaderLoadErr,
				loadedID: tt.loaderLoadedID,
			}

			var loader *plugin.Loader
			if !tt.nilLoader {
				loader = plugin.NewLoader(mgr, fileManager, repo, nil, pluginsDir)
			}

			err := plugininstall.TryLoadPlugin(ctx, loader, repo, tt.pluginRecord, wasmFilename)

			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message must wrap the underlying loader error")
			}

			if tt.assertState != nil {
				tt.assertState(t, mgr, loader, repo, tt.pluginRecord)
			}
		})
	}
}

func TestCheckNotInstalled_returns_409_status(t *testing.T) {
	ctx := context.Background()
	repo := inmemory.NewPluginRepository()

	_ = repo.Save(ctx, &domain.Plugin{
		ID:     pkgplugin.ParsePluginID("testplugin"),
		Name:   "Test Plugin",
		Status: domain.PluginStatusActive,
	})

	err := plugininstall.CheckNotInstalled(ctx, repo, pkgplugin.ParsePluginID("testplugin"))

	require.Error(t, err)

	var wrappedErr *api.WrappedError
	require.ErrorAs(t, err, &wrappedErr)
	assert.Equal(t, http.StatusConflict, wrappedErr.HTTPStatus())
}
