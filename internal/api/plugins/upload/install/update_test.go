package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/plugins/upload/install"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPluginID = "testplugin"

func testPluginDBID() domain.Uint64ID {
	return pkgplugin.ParsePluginID(testPluginID)
}

// resolveTestFilename is the name the upload path gives a freshly installed
// plugin: "<database id>.wasm".
func resolveTestFilename() string {
	return strconv.FormatUint(uint64(testPluginDBID()), 10) + ".wasm"
}

func createUploadRequest(t *testing.T, content []byte, fields map[string]string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "plugin.wasm")
	require.NoError(t, err)

	_, err = io.Copy(part, bytes.NewReader(content))
	require.NoError(t, err)

	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}

	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/install", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

// uploadedPlugin is the manifest of the build being uploaded.
func uploadedPlugin(version string, permissions ...string) *mockLoaderManager {
	return &mockLoaderManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:                  testPluginID,
					Name:                "Test Plugin",
					Version:             version,
					Description:         "A test plugin",
					Author:              "Test Author",
					ApiVersion:          "v1",
					RequiredPermissions: permissions,
				},
			}, nil
		},
	}
}

func newUpdatedWASM() []byte {
	return append(validWASMBytes(), 0x01, 0x02, 0x03)
}

func TestInstall_update(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	installedAt := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)

	storeSource := "https://store.gameap.com/plugins/testplugin"
	storeFilename := "testplugin.wasm"

	tests := []struct {
		name        string
		setupRepo   func(*inmemory.PluginRepository)
		setupFiles  func(*files.InMemoryFileManager)
		manager     *mockLoaderManager
		fields      map[string]string
		wantStatus  int
		wantError   string
		assertState func(*testing.T, *inmemory.PluginRepository, *files.InMemoryFileManager, map[string]any)
	}{
		{
			name: "update_replaces_version_and_file",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:      testPluginDBID(),
					Name:    "Test Plugin",
					Version: "1.0.0",
					Status:  domain.PluginStatusActive,
				})
			},
			manager:    uploadedPlugin("2.0.0"),
			fields:     map[string]string{"update": "true"},
			wantStatus: http.StatusOK,
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				fm *files.InMemoryFileManager,
				resp map[string]any,
			) {
				t.Helper()

				assert.Equal(t, true, resp["updated"])
				assert.Equal(t, "1.0.0", resp["previous_version"])
				assert.Equal(t, "2.0.0", resp["version"])

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1, "update must not create a second record")
				assert.Equal(t, "2.0.0", installed[0].Version)
				assert.Equal(t, domain.PluginStatusActive, installed[0].Status)

				data, err := fm.Read(ctx, "plugins/"+resolveTestFilename())
				require.NoError(t, err)
				assert.Equal(t, newUpdatedWASM(), data, "the uploaded build must land on disk")
			},
		},
		{
			name: "update_preserves_operator_managed_fields",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:          testPluginDBID(),
					Name:        "Test Plugin",
					Version:     "1.0.0",
					Source:      &storeSource,
					Filename:    &storeFilename,
					Priority:    5,
					Config:      map[string]any{"api_key": "kept"},
					Category:    new("tools"),
					Status:      domain.PluginStatusActive,
					InstalledAt: &installedAt,
				})
			},
			setupFiles: func(fm *files.InMemoryFileManager) {
				_ = fm.Write(ctx, "plugins/"+storeFilename, validWASMBytes())
			},
			manager:    uploadedPlugin("2.0.0"),
			fields:     map[string]string{"update": "true"},
			wantStatus: http.StatusOK,
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				fm *files.InMemoryFileManager,
				_ map[string]any,
			) {
				t.Helper()

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)

				require.NotNil(t, installed[0].Source)
				assert.Equal(t, storeSource, *installed[0].Source,
					"a store plugin must stay updatable from the store")
				require.NotNil(t, installed[0].InstalledAt)
				assert.Equal(t, installedAt, *installed[0].InstalledAt)
				assert.Equal(t, 5, installed[0].Priority)
				assert.Equal(t, map[string]any{"api_key": "kept"}, installed[0].Config)
				require.NotNil(t, installed[0].Category)
				assert.Equal(t, "tools", *installed[0].Category)

				require.NotNil(t, installed[0].Filename)
				assert.Equal(t, storeFilename, *installed[0].Filename,
					"the plugin keeps the file it already uses")

				data, err := fm.Read(ctx, "plugins/"+storeFilename)
				require.NoError(t, err)
				assert.Equal(t, newUpdatedWASM(), data)

				assert.False(t, fm.Exists(ctx, "plugins/"+resolveTestFilename()),
					"no orphan file under the upload naming convention")
			},
		},
		{
			name: "update_unions_permissions",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:      testPluginDBID(),
					Name:    "Test Plugin",
					Version: "1.0.0",
					Status:  domain.PluginStatusActive,
					RequiredPermissions: []domain.PluginPermission{
						domain.PluginPermissionListenEvents,
					},
					AllowedPermissions: []domain.PluginPermission{
						domain.PluginPermissionListenEvents,
						domain.PluginPermissionFiles,
					},
				})
			},
			manager:    uploadedPlugin("2.0.0", "listen_events", "secrets"),
			fields:     map[string]string{"update": "true"},
			wantStatus: http.StatusOK,
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				_ *files.InMemoryFileManager,
				_ map[string]any,
			) {
				t.Helper()

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)

				assert.Equal(t, []domain.PluginPermission{
					domain.PluginPermissionListenEvents,
					domain.PluginPermissionSecrets,
				}, installed[0].RequiredPermissions)

				assert.Equal(t, []domain.PluginPermission{
					domain.PluginPermissionListenEvents,
					domain.PluginPermissionFiles,
					domain.PluginPermissionSecrets,
				}, installed[0].AllowedPermissions,
					"the grandfathered files grant survives the update")
			},
		},
		{
			name:       "update_of_a_plugin_that_is_not_installed_installs_it",
			setupRepo:  func(_ *inmemory.PluginRepository) {},
			manager:    uploadedPlugin("2.0.0"),
			fields:     map[string]string{"update": "true"},
			wantStatus: http.StatusOK,
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				_ *files.InMemoryFileManager,
				resp map[string]any,
			) {
				t.Helper()

				assert.Equal(t, false, resp["updated"])
				assert.NotContains(t, resp, "previous_version")

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)
				require.NotNil(t, installed[0].Source)
				assert.Contains(t, *installed[0].Source, "file://")
			},
		},
		{
			name: "downgrade_is_allowed",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:      testPluginDBID(),
					Name:    "Test Plugin",
					Version: "2.0.0",
					Status:  domain.PluginStatusActive,
				})
			},
			manager:    uploadedPlugin("1.0.0"),
			fields:     map[string]string{"update": "true"},
			wantStatus: http.StatusOK,
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				_ *files.InMemoryFileManager,
				resp map[string]any,
			) {
				t.Helper()

				assert.Equal(t, "2.0.0", resp["previous_version"])

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)
				assert.Equal(t, "1.0.0", installed[0].Version)
			},
		},
		{
			name: "update_false_is_treated_as_no_confirmation",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:      testPluginDBID(),
					Name:    "Test Plugin",
					Version: "1.0.0",
					Status:  domain.PluginStatusActive,
				})
			},
			manager:    uploadedPlugin("2.0.0"),
			fields:     map[string]string{"update": "false"},
			wantStatus: http.StatusConflict,
			wantError:  "plugin already installed",
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				_ *files.InMemoryFileManager,
				_ map[string]any,
			) {
				t.Helper()

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)
				assert.Equal(t, "1.0.0", installed[0].Version, "the installed plugin must be untouched")
			},
		},
		{
			name:       "unparsable_update_flag_is_rejected",
			setupRepo:  func(_ *inmemory.PluginRepository) {},
			manager:    uploadedPlugin("2.0.0"),
			fields:     map[string]string{"update": "maybe"},
			wantStatus: http.StatusBadRequest,
			wantError:  "value is invalid",
		},
		{
			name: "name_owned_by_another_plugin_is_rejected",
			setupRepo: func(repo *inmemory.PluginRepository) {
				_ = repo.Save(ctx, &domain.Plugin{
					ID:      999,
					Name:    "Test Plugin",
					Version: "1.0.0",
					Status:  domain.PluginStatusActive,
				})
			},
			manager:    uploadedPlugin("2.0.0"),
			wantStatus: http.StatusConflict,
			wantError:  "another plugin is already installed under this name",
			assertState: func(
				t *testing.T,
				repo *inmemory.PluginRepository,
				fm *files.InMemoryFileManager,
				_ map[string]any,
			) {
				t.Helper()

				installed, err := repo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1, "the conflicting upload must not be recorded")
				assert.False(t, fm.Exists(ctx, "plugins/"+resolveTestFilename()))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewPluginRepository()
			tt.setupRepo(repo)

			fileManager := files.NewInMemoryFileManager()
			if tt.setupFiles != nil {
				tt.setupFiles(fileManager)
			} else if tt.fields["update"] == "true" {
				_ = fileManager.Write(ctx, "plugins/"+resolveTestFilename(), validWASMBytes())
			}

			h := install.NewHandler(
				tt.manager, repo, fileManager, nil, nil, "plugins", api.NewResponder(), nil,
			)
			recorder := httptest.NewRecorder()

			h.ServeHTTP(recorder, createUploadRequest(t, newUpdatedWASM(), tt.fields))

			require.Equal(t, tt.wantStatus, recorder.Code, "body=%s", recorder.Body.String())

			if tt.wantError != "" {
				assert.Contains(t, recorder.Body.String(), tt.wantError)
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

			if tt.assertState != nil {
				tt.assertState(t, repo, fileManager, resp)
			}
		})
	}
}

// rollbackManager fails to load one specific build, which is how a broken
// upload is simulated: the transient validation pass succeeds (the manifest
// reads fine) and the real load afterwards trips.
type rollbackManager struct {
	failingWASM []byte
	loadedWASM  [][]byte
	unloaded    []string
}

func (m *rollbackManager) LoadTransient(
	_ context.Context,
	_ []byte,
	_ map[string]string,
	_ uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{
			Id:         testPluginID,
			Name:       "Test Plugin",
			Version:    "2.0.0",
			ApiVersion: "v1",
		},
	}, nil
}

func (m *rollbackManager) Load(
	_ context.Context,
	wasmBytes []byte,
	_ map[string]string,
	_ uint64,
) (*pkgplugin.LoadedPlugin, error) {
	m.loadedWASM = append(m.loadedWASM, wasmBytes)

	if bytes.Equal(wasmBytes, m.failingWASM) {
		return nil, errors.New("wasm start function trapped")
	}

	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{
			Id:         testPluginID,
			Name:       "Test Plugin",
			Version:    "1.0.0",
			ApiVersion: "v1",
		},
	}, nil
}

func (m *rollbackManager) Unload(_ context.Context, pluginID string) error {
	m.unloaded = append(m.unloaded, pluginID)

	return nil
}

func (m *rollbackManager) GetPlugin(_ string) (*pkgplugin.LoadedPlugin, bool) { return nil, false }

func (m *rollbackManager) GetPlugins() []*pkgplugin.LoadedPlugin { return nil }

func (m *rollbackManager) Shutdown(_ context.Context) error { return nil }

func TestInstall_update_restores_the_previous_version_when_the_new_build_does_not_load(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pluginPath := "plugins/" + resolveTestFilename()

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:      testPluginDBID(),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}))

	fileManager := files.NewInMemoryFileManager()
	require.NoError(t, fileManager.Write(ctx, pluginPath, validWASMBytes()))

	manager := &rollbackManager{failingWASM: newUpdatedWASM()}
	loader := plugin.NewLoader(manager, fileManager, repo, nil, "plugins")

	refresher := &fakeRefresher{}
	h := install.NewHandler(
		manager, repo, fileManager, loader, refresher, "plugins", api.NewResponder(), nil,
	)
	recorder := httptest.NewRecorder()

	h.ServeHTTP(recorder, createUploadRequest(t, newUpdatedWASM(), map[string]string{"update": "true"}))

	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code, "body=%s", recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "plugin update failed, previous version restored")

	installed, err := repo.Find(ctx, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, installed, 1)
	assert.Equal(t, "1.0.0", installed[0].Version, "the record must be rolled back")
	assert.Equal(t, domain.PluginStatusActive, installed[0].Status,
		"a restored plugin is running again, not errored")

	data, err := fileManager.Read(ctx, pluginPath)
	require.NoError(t, err)
	assert.Equal(t, validWASMBytes(), data, "the previous build must be back on disk")

	require.Len(t, manager.loadedWASM, 2)
	assert.Equal(t, newUpdatedWASM(), manager.loadedWASM[0])
	assert.Equal(t, validWASMBytes(), manager.loadedWASM[1])

	assert.Equal(t, []string{pkgplugin.CompactPluginID(testPluginDBID())}, manager.unloaded)
}

func TestInstall_update_reports_no_rollback_when_the_previous_file_is_gone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:      testPluginDBID(),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusError,
	}))

	fileManager := files.NewInMemoryFileManager()
	manager := &rollbackManager{failingWASM: newUpdatedWASM()}
	loader := plugin.NewLoader(manager, fileManager, repo, nil, "plugins")

	h := install.NewHandler(
		manager, repo, fileManager, loader, nil, "plugins", api.NewResponder(), nil,
	)
	recorder := httptest.NewRecorder()

	h.ServeHTTP(recorder, createUploadRequest(t, newUpdatedWASM(), map[string]string{"update": "true"}))

	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code, "body=%s", recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "plugin updated but failed to load")

	installed, err := repo.Find(ctx, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, installed, 1)
	assert.Equal(t, domain.PluginStatusError, installed[0].Status)
}

func TestInstall_update_refreshes_subscriptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:      testPluginDBID(),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}))

	refresher := &fakeRefresher{}
	h := install.NewHandler(
		uploadedPlugin("2.0.0"),
		repo,
		files.NewInMemoryFileManager(),
		nil,
		refresher,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	h.ServeHTTP(recorder, createUploadRequest(t, newUpdatedWASM(), map[string]string{"update": "true"}))

	require.Equal(t, http.StatusOK, recorder.Code, "body=%s", recorder.Body.String())
	assert.Equal(t, 1, refresher.calls, "subscriptions must be refreshed after a runtime update")
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — replacing a plugin swaps the
//     executable code the platform runs, so it must be recorded (OWASP ASVS
//     §7.2.1) with the version it replaced.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

func TestInstall_Audit_UpdateIsRecorded(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:      testPluginDBID(),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}))

	recorder := &auditCapture{}
	h := install.NewHandler(
		uploadedPlugin("2.0.0"),
		repo,
		files.NewInMemoryFileManager(),
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		recorder,
	)
	w := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(w, createUploadRequest(t, newUpdatedWASM(), map[string]string{"update": "true"}))

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	events := recorder.snapshot()
	assert.Equal(t, 1, countEvents(events, audit.EventPluginUpdate))
	assert.Equal(t, 0, countEvents(events, audit.EventPluginInstall),
		"replacing a plugin is an update, not an install")

	event, ok := findEvent(events, audit.EventPluginUpdate)
	require.True(t, ok)
	assert.Equal(t, audit.CategoryPluginOp, event.Category)
	assert.Equal(t, audit.OutcomeSuccess, event.Outcome)
	assert.Equal(t, "plugin", event.ResourceType)
	assert.Equal(t, strconv.FormatUint(uint64(testPluginDBID()), 10), event.ResourceID)
	assert.Equal(t, "update", event.Action)

	previous, ok := extraString(event, "previous_version")
	require.True(t, ok, "the replaced version must be recorded for provenance")
	assert.Equal(t, "1.0.0", previous)

	version, ok := extraString(event, "version")
	require.True(t, ok)
	assert.Equal(t, "2.0.0", version)
}

func TestInstall_Audit_RolledBackUpdateIsNotRecorded(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()

	repo := inmemory.NewPluginRepository()
	require.NoError(t, repo.Save(ctx, &domain.Plugin{
		ID:      testPluginDBID(),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}))

	fileManager := files.NewInMemoryFileManager()
	require.NoError(t, fileManager.Write(ctx, "plugins/"+resolveTestFilename(), validWASMBytes()))

	manager := &rollbackManager{failingWASM: newUpdatedWASM()}
	auditRecorder := &auditCapture{}
	h := install.NewHandler(
		manager,
		repo,
		fileManager,
		plugin.NewLoader(manager, fileManager, repo, nil, "plugins"),
		nil,
		"plugins",
		api.NewResponder(),
		auditRecorder,
	)
	w := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(w, createUploadRequest(t, newUpdatedWASM(), map[string]string{"update": "true"}))

	// ASSERT
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Empty(t, auditRecorder.snapshot(),
		"an update that was rolled back changed nothing and must not be recorded as done")
}
