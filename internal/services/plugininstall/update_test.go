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
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindInstalled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name        string
		setupRepo   func(*inmemory.PluginRepository)
		dbID        domain.Uint64ID
		wantFound   bool
		wantVersion string
	}{
		{
			name:      "plugin_not_installed_returns_nil",
			setupRepo: func(_ *inmemory.PluginRepository) {},
			dbID:      12345,
			wantFound: false,
		},
		{
			name: "installed_plugin_is_returned",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:      12345,
					Name:    "Test Plugin",
					Version: "1.0.0",
					Status:  domain.PluginStatusActive,
				})
			},
			dbID:        12345,
			wantFound:   true,
			wantVersion: "1.0.0",
		},
		{
			name: "another_plugin_installed_returns_nil",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:     999,
					Name:   "Other Plugin",
					Status: domain.PluginStatusActive,
				})
			},
			dbID:      12345,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewPluginRepository()
			tt.setupRepo(repo)

			installed, err := plugininstall.FindInstalled(ctx, repo, tt.dbID)

			require.NoError(t, err)

			if !tt.wantFound {
				assert.Nil(t, installed)

				return
			}

			require.NotNil(t, installed)
			assert.Equal(t, tt.dbID, installed.ID)
			assert.Equal(t, tt.wantVersion, installed.Version)
		})
	}
}

func TestCheckNameAvailable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name       string
		setupRepo  func(*inmemory.PluginRepository)
		dbID       domain.Uint64ID
		pluginName string
		wantError  string
	}{
		{
			name:       "free_name_is_available",
			setupRepo:  func(_ *inmemory.PluginRepository) {},
			dbID:       12345,
			pluginName: "Test Plugin",
			wantError:  "",
		},
		{
			name: "own_name_is_available_on_update",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{ID: 12345, Name: "Test Plugin"})
			},
			dbID:       12345,
			pluginName: "Test Plugin",
			wantError:  "",
		},
		{
			name: "name_owned_by_another_plugin_is_rejected",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{ID: 999, Name: "Test Plugin"})
			},
			dbID:       12345,
			pluginName: "Test Plugin",
			wantError:  "another plugin is already installed under this name",
		},
		{
			name:       "empty_name_is_not_checked",
			setupRepo:  func(_ *inmemory.PluginRepository) {},
			dbID:       12345,
			pluginName: "",
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewPluginRepository()
			tt.setupRepo(repo)

			err := plugininstall.CheckNameAvailable(ctx, repo, tt.dbID, tt.pluginName)

			if tt.wantError == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)

			var httpErr interface{ HTTPStatus() int }
			if assert.ErrorAs(t, err, &httpErr) {
				assert.Equal(t, http.StatusConflict, httpErr.HTTPStatus())
			}
		})
	}
}

// unloadRecorder is a LoaderManager whose Unload outcome is scripted, so the
// helper's tolerance of a plugin that is not loaded can be exercised.
type unloadRecorder struct {
	fakeLoaderManager

	unloadErr error
	unloadedX []string
}

func (u *unloadRecorder) Unload(_ context.Context, pluginID string) error {
	u.unloadedX = append(u.unloadedX, pluginID)

	return u.unloadErr
}

func TestUnloadPlugin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name         string
		nilLoader    bool
		registerID   string
		unloadErr    error
		wantError    string
		wantUnloaded []string
	}{
		{
			name:         "unloads_under_the_registered_manager_id",
			registerID:   "wasm-internal-id",
			wantUnloaded: []string{"wasm-internal-id"},
		},
		{
			name:         "falls_back_to_the_compact_id",
			wantUnloaded: []string{pkgplugin.CompactPluginID(12345)},
		},
		{
			name:         "plugin_not_loaded_is_not_an_error",
			unloadErr:    errors.Wrap(pkgplugin.ErrPluginNotFound, "plugin: x"),
			wantUnloaded: []string{pkgplugin.CompactPluginID(12345)},
		},
		{
			name:         "other_unload_errors_are_reported",
			unloadErr:    errors.New("runtime is stuck"),
			wantError:    "failed to unload plugin",
			wantUnloaded: []string{pkgplugin.CompactPluginID(12345)},
		},
		{
			name:      "nil_loader_is_a_no_op",
			nilLoader: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr := &unloadRecorder{unloadErr: tt.unloadErr}

			var loader *plugin.Loader
			if !tt.nilLoader {
				loader = plugin.NewLoader(
					mgr, files.NewInMemoryFileManager(), inmemory.NewPluginRepository(), nil, "plugins",
				)
				if tt.registerID != "" {
					loader.RegisterPluginID(12345, tt.registerID)
				}
			}

			err := plugininstall.UnloadPlugin(ctx, loader, 12345)

			if tt.wantError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}

			assert.Equal(t, tt.wantUnloaded, mgr.unloadedX)
		})
	}
}

func TestResolvePluginFilename(t *testing.T) {
	t.Parallel()

	storeFilename := "storeplugin.wasm"
	empty := ""

	tests := []struct {
		name      string
		installed *domain.Plugin
		want      string
	}{
		{
			name:      "new_plugin_gets_the_upload_convention",
			installed: nil,
			want:      "12345.wasm",
		},
		{
			name:      "installed_plugin_keeps_its_own_file",
			installed: &domain.Plugin{ID: 12345, Filename: &storeFilename},
			want:      storeFilename,
		},
		{
			name:      "record_without_filename_falls_back",
			installed: &domain.Plugin{ID: 12345},
			want:      "12345.wasm",
		},
		{
			name:      "record_with_empty_filename_falls_back",
			installed: &domain.Plugin{ID: 12345, Filename: &empty},
			want:      "12345.wasm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, plugininstall.ResolvePluginFilename(tt.installed, 12345))
		})
	}
}

func TestApplyManifest(t *testing.T) {
	t.Parallel()

	oldFilename := "12345.wasm"
	source := "https://store.gameap.com/plugins/storeplugin"
	installedAt := domain.Plugin{}.InstalledAt

	installed := &domain.Plugin{
		ID:                  12345,
		Name:                "Old Name",
		Version:             "1.0.0",
		Description:         "old description",
		Author:              "Old Author",
		APIVersion:          "v1",
		Filename:            &oldFilename,
		Source:              &source,
		Priority:            7,
		Category:            new("tools"),
		Config:              map[string]any{"api_key": "kept"},
		RequiredPermissions: []domain.PluginPermission{domain.PluginPermissionListenEvents},
		AllowedPermissions: []domain.PluginPermission{
			domain.PluginPermissionListenEvents,
			domain.PluginPermissionFiles,
		},
		Status:      domain.PluginStatusError,
		InstalledAt: installedAt,
	}

	loaded := &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{
			Id:                  "testplugin",
			Name:                "New Name",
			Version:             "2.0.0",
			Description:         "new description",
			Author:              "New Author",
			ApiVersion:          "v2",
			RequiredPermissions: []string{"listen_events", "secrets"},
		},
	}

	plugininstall.ApplyManifest(installed, loaded, "12345.wasm")

	assert.Equal(t, "New Name", installed.Name)
	assert.Equal(t, "2.0.0", installed.Version)
	assert.Equal(t, "new description", installed.Description)
	assert.Equal(t, "New Author", installed.Author)
	assert.Equal(t, "v2", installed.APIVersion)
	assert.Equal(t, domain.PluginStatusActive, installed.Status)

	assert.Equal(t, []domain.PluginPermission{
		domain.PluginPermissionListenEvents,
		domain.PluginPermissionSecrets,
	}, installed.RequiredPermissions, "required permissions mirror the new manifest")

	assert.Equal(t, []domain.PluginPermission{
		domain.PluginPermissionListenEvents,
		domain.PluginPermissionFiles,
		domain.PluginPermissionSecrets,
	}, installed.AllowedPermissions,
		"the grandfathered files grant survives and the new requirement is added")

	require.NotNil(t, installed.Source)
	assert.Equal(t, source, *installed.Source, "a store plugin stays updatable from the store")
	assert.Equal(t, 7, installed.Priority)
	require.NotNil(t, installed.Category)
	assert.Equal(t, "tools", *installed.Category)
	assert.Equal(t, map[string]any{"api_key": "kept"}, installed.Config)
}

func TestApplyManifest_nil_arguments_are_ignored(t *testing.T) {
	t.Parallel()

	record := &domain.Plugin{ID: 1, Version: "1.0.0"}

	plugininstall.ApplyManifest(record, nil, "1.wasm")
	plugininstall.ApplyManifest(record, &pkgplugin.LoadedPlugin{}, "1.wasm")
	plugininstall.ApplyManifest(nil, &pkgplugin.LoadedPlugin{Info: &proto.PluginInfo{}}, "1.wasm")

	assert.Equal(t, "1.0.0", record.Version)
}

func TestMergePermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		granted  []domain.PluginPermission
		required []domain.PluginPermission
		want     []domain.PluginPermission
	}{
		{
			name:     "no_grants_yet_takes_the_requirements",
			granted:  nil,
			required: []domain.PluginPermission{domain.PluginPermissionSecrets},
			want:     []domain.PluginPermission{domain.PluginPermissionSecrets},
		},
		{
			name:     "grants_are_never_revoked",
			granted:  []domain.PluginPermission{domain.PluginPermissionFiles},
			required: nil,
			want:     []domain.PluginPermission{domain.PluginPermissionFiles},
		},
		{
			name:     "duplicates_are_not_added_twice",
			granted:  []domain.PluginPermission{domain.PluginPermissionFiles},
			required: []domain.PluginPermission{domain.PluginPermissionFiles},
			want:     []domain.PluginPermission{domain.PluginPermissionFiles},
		},
		{
			name:    "new_requirements_are_appended",
			granted: []domain.PluginPermission{domain.PluginPermissionFiles},
			required: []domain.PluginPermission{
				domain.PluginPermissionSecrets,
				domain.PluginPermissionManageRBAC,
			},
			want: []domain.PluginPermission{
				domain.PluginPermissionFiles,
				domain.PluginPermissionSecrets,
				domain.PluginPermissionManageRBAC,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			granted := append([]domain.PluginPermission(nil), tt.granted...)

			got := plugininstall.MergePermissions(granted, tt.required)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.granted, granted, "the caller's slice must not be mutated")
		})
	}
}

func TestMissingPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		installed *domain.Plugin
		required  []string
		want      []string
	}{
		{
			name:      "not_installed_reports_nothing",
			installed: nil,
			required:  []string{"secrets"},
			want:      nil,
		},
		{
			name: "already_granted_permissions_are_not_listed",
			installed: &domain.Plugin{
				AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionSecrets},
			},
			required: []string{"secrets"},
			want:     nil,
		},
		{
			name: "ungranted_permissions_are_listed",
			installed: &domain.Plugin{
				AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionFiles},
			},
			required: []string{"files", "secrets", "manage_rbac"},
			want:     []string{"secrets", "manage_rbac"},
		},
		{
			name: "unknown_permission_names_are_dropped",
			installed: &domain.Plugin{
				AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionFiles},
			},
			required: []string{"become_root"},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, plugininstall.MissingPermissions(tt.installed, tt.required))
		})
	}
}
