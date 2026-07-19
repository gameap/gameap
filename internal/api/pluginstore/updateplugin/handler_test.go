package updateplugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/api/pluginstore/updateplugin"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPluginID = "testplugin123"

// sha256("test data") — keep in sync with wasmContent in the tests below.
const testWasmHash = "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"

type fakeLoaderManager struct {
	loadFunc   func(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	unloadFunc func(ctx context.Context, pluginID string) error
}

func (m *fakeLoaderManager) Load(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, wasmBytes, config, pluginID)
	}

	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{
			Id:      "test-plugin-id",
			Name:    "test-plugin",
			Version: "1.0.0",
		},
		Enabled: true,
	}, nil
}

func (m *fakeLoaderManager) LoadTransient(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return m.Load(ctx, wasmBytes, config, pluginID)
}

func (m *fakeLoaderManager) Unload(ctx context.Context, pluginID string) error {
	if m.unloadFunc != nil {
		return m.unloadFunc(ctx, pluginID)
	}

	return nil
}

func (m *fakeLoaderManager) GetPlugin(_ string) (*pkgplugin.LoadedPlugin, bool) {
	return nil, false
}

func (m *fakeLoaderManager) GetPlugins() []*pkgplugin.LoadedPlugin {
	return nil
}

func (m *fakeLoaderManager) Shutdown(_ context.Context) error {
	return nil
}

type errPluginRepo struct {
	*inmemory.PluginRepository

	findErr error
	saveErr error
}

func (r *errPluginRepo) Find(
	ctx context.Context,
	filter *filters.FindPlugin,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}

	return r.PluginRepository.Find(ctx, filter, order, pagination)
}

func (r *errPluginRepo) Save(ctx context.Context, plug *domain.Plugin) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.PluginRepository.Save(ctx, plug)
}

type pluginstoreHandler func(w http.ResponseWriter, r *http.Request)

func newMockServer(t *testing.T, h pluginstoreHandler) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(server.Close)

	return server
}

func defaultVersionsResponse() pluginstore.PaginatedResponse[pluginstore.PluginVersion] {
	return pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
		Data: []pluginstore.PluginVersion{
			{
				ID:       2,
				Version:  "2.0.0",
				FileHash: testWasmHash,
				IsStable: true,
			},
		},
		Total: 1,
	}
}

func defaultMockHandler(t *testing.T) pluginstoreHandler {
	t.Helper()

	versions := defaultVersionsResponse()

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/plugins/" + testPluginID + "/versions":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(versions)
		case "/plugins/" + testPluginID + "/versions/2.0.0/download":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test data"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func saveExistingPlugin(t *testing.T, repo repositories.PluginRepository) *domain.Plugin {
	t.Helper()

	plug := &domain.Plugin{
		ID:      pkgplugin.ParsePluginID(testPluginID),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}
	require.NoError(t, repo.Save(context.Background(), plug))

	return plug
}

func executeUpdate(
	t *testing.T,
	pluginRepo repositories.PluginRepository,
	fileManager files.FileManager,
	loader *plugin.Loader,
	storeService *pluginstore.Service,
) *httptest.ResponseRecorder {
	t.Helper()

	h := updateplugin.NewHandler(
		storeService,
		pluginRepo,
		fileManager,
		loader,
		nil,
		"plugins",
		api.NewResponder(),
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/store/plugins/"+testPluginID+"/update", nil)
	req = mux.SetURLVars(req, map[string]string{"id": testPluginID})

	h.ServeHTTP(recorder, req)

	return recorder
}

func TestUpdatePlugin(t *testing.T) {
	server := newMockServer(t, defaultMockHandler(t))

	storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	saveExistingPlugin(t, pluginRepo)

	recorder := executeUpdate(t, pluginRepo, fileManager, nil, storeService)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

	assert.Equal(t, "Test Plugin", resp["name"])
	assert.Equal(t, "2.0.0", resp["version"])
	assert.Equal(t, "active", resp["status"])
	assert.NotNil(t, resp["updated_at"])

	updated, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "2.0.0", updated[0].Version)
	require.NotNil(t, updated[0].Filename)
	assert.Equal(t, testPluginID+".wasm", *updated[0].Filename)
	assert.True(t, fileManager.Exists(context.Background(), "plugins/"+testPluginID+".wasm"))
}

func TestUpdatePlugin_picks_first_stable_version_when_unspecified(t *testing.T) {
	// ARRANGE
	versions := pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
		Data: []pluginstore.PluginVersion{
			{ID: 1, Version: "3.0.0-beta", FileHash: "wronghash", IsStable: false},
			{ID: 2, Version: "2.0.0", FileHash: testWasmHash, IsStable: true},
		},
		Total: 2,
	}
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/plugins/" + testPluginID + "/versions":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(versions)
		case "/plugins/" + testPluginID + "/versions/2.0.0/download":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test data"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	saveExistingPlugin(t, pluginRepo)

	// ACT
	recorder := executeUpdate(t, pluginRepo, fileManager, nil, storeService)

	// ASSERT
	assert.Equal(t, http.StatusOK, recorder.Code)

	updated, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "2.0.0", updated[0].Version,
		"stable version must be preferred even when listed after an unstable one")
}

func TestUpdatePlugin_falls_back_to_first_when_no_stable_version_exists(t *testing.T) {
	// ARRANGE
	versions := pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
		Data: []pluginstore.PluginVersion{
			{ID: 1, Version: "2.0.0", FileHash: testWasmHash, IsStable: false},
			{ID: 2, Version: "3.0.0-beta", FileHash: "another", IsStable: false},
		},
		Total: 2,
	}
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/plugins/" + testPluginID + "/versions":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(versions)
		case "/plugins/" + testPluginID + "/versions/2.0.0/download":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test data"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	saveExistingPlugin(t, pluginRepo)

	// ACT
	recorder := executeUpdate(t, pluginRepo, fileManager, nil, storeService)

	// ASSERT
	assert.Equal(t, http.StatusOK, recorder.Code)

	updated, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "2.0.0", updated[0].Version,
		"first version must be selected when no stable version exists")
}

func TestUpdatePlugin_with_loader_unloads_old_and_loads_new(t *testing.T) {
	// ARRANGE
	server := newMockServer(t, defaultMockHandler(t))
	storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	plug := saveExistingPlugin(t, pluginRepo)

	var unloadedID string
	manager := &fakeLoaderManager{
		unloadFunc: func(_ context.Context, pluginID string) error {
			unloadedID = pluginID

			return nil
		},
	}
	loader := plugin.NewLoader(manager, fileManager, pluginRepo, nil, "plugins")
	loader.RegisterPluginID(plug.ID, "old-manager-id")

	// ACT
	recorder := executeUpdate(t, pluginRepo, fileManager, loader, storeService)

	// ASSERT
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "old-manager-id", unloadedID, "previous plugin instance must be unloaded")

	updated, err := pluginRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, domain.PluginStatusActive, updated[0].Status)
	assert.Equal(t, "2.0.0", updated[0].Version)
}

func TestUpdatePlugin_with_loader_no_registered_manager_id_falls_back_to_compact_id(t *testing.T) {
	// ARRANGE
	server := newMockServer(t, defaultMockHandler(t))
	storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	saveExistingPlugin(t, pluginRepo)

	unloadedID := ""
	manager := &fakeLoaderManager{
		unloadFunc: func(_ context.Context, pluginID string) error {
			unloadedID = pluginID

			return pkgplugin.ErrPluginNotFound
		},
	}
	loader := plugin.NewLoader(manager, fileManager, pluginRepo, nil, "plugins")

	// ACT
	recorder := executeUpdate(t, pluginRepo, fileManager, loader, storeService)

	// ASSERT: without a registered mapping the handler must try the compact
	// form of the DB ID and tolerate the plugin not being loaded.
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t,
		pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(testPluginID)), unloadedID,
		"unload must be attempted with the compact DB id when no manager ID is registered")
}

func TestUpdatePlugin_not_installed(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	h := updateplugin.NewHandler(
		storeService,
		pluginRepo,
		fileManager,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/store/plugins/nonexistent/update", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "nonexistent"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNotFound, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Contains(t, errorMessage(t, resp), "plugin not installed")
}

func errorMessage(t *testing.T, resp map[string]any) string {
	t.Helper()

	if msg, ok := resp["error"].(string); ok {
		return msg
	}
	if msg, ok := resp["message"].(string); ok {
		return msg
	}

	return ""
}

func TestUpdatePlugin_pipeline_failures(t *testing.T) {
	tests := []struct {
		name             string
		setupRepo        func(t *testing.T) repositories.PluginRepository
		setupFileManager func(t *testing.T) files.FileManager
		setupLoader      func(t *testing.T, repo repositories.PluginRepository, fm files.FileManager, plug *domain.Plugin) *plugin.Loader
		afterSetup       func(t *testing.T, repo repositories.PluginRepository, fm files.FileManager)
		mockHandler      func(t *testing.T) pluginstoreHandler
		body             string
		wantStatus       int
		wantError        string
		assertState      func(t *testing.T, repo repositories.PluginRepository, fm files.FileManager)
	}{
		{
			name: "invalid_request_body",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: defaultMockHandler,
			body:        `{not valid json`,
			wantStatus:  http.StatusBadRequest,
			wantError:   "invalid request body",
		},
		{
			name: "plugin_repo_find_returns_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return &errPluginRepo{
					PluginRepository: inmemory.NewPluginRepository(),
					findErr:          errors.New("db unavailable"),
				}
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: defaultMockHandler,
			wantStatus:  http.StatusInternalServerError,
		},
		{
			name: "unload_returns_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			setupLoader: func(t *testing.T, repo repositories.PluginRepository, fm files.FileManager, plug *domain.Plugin) *plugin.Loader {
				t.Helper()

				manager := &fakeLoaderManager{
					unloadFunc: func(_ context.Context, _ string) error {
						return errors.New("unload failed")
					},
				}
				loader := plugin.NewLoader(manager, fm, repo, nil, "plugins")
				loader.RegisterPluginID(plug.ID, "test-plugin-id")

				return loader
			},
			mockHandler: defaultMockHandler,
			wantStatus:  http.StatusInternalServerError,
		},
		{
			name: "get_versions_upstream_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: func(t *testing.T) pluginstoreHandler {
				t.Helper()

				return func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/plugins/"+testPluginID+"/versions" {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"message":"upstream broke"}`))

						return
					}
					w.WriteHeader(http.StatusNotFound)
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "no_versions_available",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: func(t *testing.T) pluginstoreHandler {
				t.Helper()

				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/plugins/"+testPluginID+"/versions" {
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
							Data: nil, Total: 0,
						})

						return
					}
					w.WriteHeader(http.StatusNotFound)
				}
			},
			wantStatus: http.StatusNotFound,
			wantError:  "no versions available",
		},
		{
			name: "specified_version_not_found",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: defaultMockHandler,
			body:        `{"version":"9.9.9"}`,
			wantStatus:  http.StatusNotFound,
			wantError:   "specified version not found",
		},
		{
			name: "download_returns_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: func(t *testing.T) pluginstoreHandler {
				t.Helper()

				versions := defaultVersionsResponse()

				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/plugins/" + testPluginID + "/versions":
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(versions)
					case "/plugins/" + testPluginID + "/versions/2.0.0/download":
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte(`{"message":"download crashed"}`))
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}
			},
			wantStatus: http.StatusInternalServerError,
			assertState: func(t *testing.T, _ repositories.PluginRepository, fm files.FileManager) {
				t.Helper()
				assert.False(t, fm.Exists(context.Background(), "plugins/"+testPluginID+".wasm"),
					"plugin file must not be written when download fails")
			},
		},
		{
			name: "download_hash_mismatch",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			mockHandler: func(t *testing.T) pluginstoreHandler {
				t.Helper()

				versions := defaultVersionsResponse()

				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/plugins/" + testPluginID + "/versions":
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(versions)
					case "/plugins/" + testPluginID + "/versions/2.0.0/download":
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("tampered content"))
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "plugin file hash mismatch",
			assertState: func(t *testing.T, _ repositories.PluginRepository, fm files.FileManager) {
				t.Helper()
				assert.False(t, fm.Exists(context.Background(), "plugins/"+testPluginID+".wasm"),
					"plugin file must not be written when hash mismatches")
			},
		},
		{
			name: "file_write_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return &files.MockFileManager{
					WriteFunc: func(_ context.Context, _ string, _ []byte) error {
						return errors.New("disk full")
					},
				}
			},
			mockHandler: defaultMockHandler,
			wantStatus:  http.StatusInternalServerError,
			assertState: func(t *testing.T, repo repositories.PluginRepository, _ files.FileManager) {
				t.Helper()
				plugins, err := repo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, plugins, 1)
				assert.Equal(t, "1.0.0", plugins[0].Version,
					"plugin record must keep the previous version if file write fails")
			},
		},
		{
			name: "repo_save_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return &errPluginRepo{
					PluginRepository: inmemory.NewPluginRepository(),
				}
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			afterSetup: func(t *testing.T, repo repositories.PluginRepository, _ files.FileManager) {
				t.Helper()
				errRepo, ok := repo.(*errPluginRepo)
				require.True(t, ok)
				errRepo.saveErr = errors.New("save failed")
			},
			mockHandler: defaultMockHandler,
			wantStatus:  http.StatusInternalServerError,
			assertState: func(t *testing.T, _ repositories.PluginRepository, fm files.FileManager) {
				t.Helper()
				assert.True(t, fm.Exists(context.Background(), "plugins/"+testPluginID+".wasm"),
					"plugin file is written before save and is not rolled back by update handler")
			},
		},
		{
			name: "plugin_load_error",
			setupRepo: func(t *testing.T) repositories.PluginRepository {
				t.Helper()

				return inmemory.NewPluginRepository()
			},
			setupFileManager: func(t *testing.T) files.FileManager {
				t.Helper()

				return files.NewInMemoryFileManager()
			},
			setupLoader: func(t *testing.T, repo repositories.PluginRepository, fm files.FileManager, _ *domain.Plugin) *plugin.Loader {
				t.Helper()

				manager := &fakeLoaderManager{
					loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
						return nil, errors.New("wasm broken")
					},
				}

				return plugin.NewLoader(manager, fm, repo, nil, "plugins")
			},
			mockHandler: defaultMockHandler,
			wantStatus:  http.StatusUnprocessableEntity,
			wantError:   "plugin installed but failed to load",
			assertState: func(t *testing.T, repo repositories.PluginRepository, fm files.FileManager) {
				t.Helper()
				plugins, err := repo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, plugins, 1)
				assert.Equal(t, domain.PluginStatusError, plugins[0].Status,
					"plugin status must be marked as error when loading fails")
				assert.Equal(t, "2.0.0", plugins[0].Version,
					"plugin version must already be updated to the new one")
				assert.True(t, fm.Exists(context.Background(), "plugins/"+testPluginID+".wasm"),
					"plugin file must be written even if loading fails")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			server := newMockServer(t, tt.mockHandler(t))
			storeService := pluginstore.NewService(server.URL, "", cache.NewInMemory())

			repo := tt.setupRepo(t)
			fileManager := tt.setupFileManager(t)
			plug := saveExistingPlugin(t, repo)

			var loader *plugin.Loader
			if tt.setupLoader != nil {
				loader = tt.setupLoader(t, repo, fileManager, plug)
			}

			if tt.afterSetup != nil {
				tt.afterSetup(t, repo, fileManager)
			}

			h := updateplugin.NewHandler(
				storeService,
				repo,
				fileManager,
				loader,
				nil,
				"plugins",
				api.NewResponder(),
			)
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/admin/plugins/store/plugins/"+testPluginID+"/update",
				http.NoBody,
			)
			if tt.body != "" {
				req = httptest.NewRequest(
					http.MethodPost,
					"/api/admin/plugins/store/plugins/"+testPluginID+"/update",
					strings.NewReader(tt.body),
				)
				req.Header.Set("Content-Type", "application/json")
			}
			req = mux.SetURLVars(req, map[string]string{"id": testPluginID})

			// ACT
			h.ServeHTTP(recorder, req)

			// ASSERT
			assert.Equal(t, tt.wantStatus, recorder.Code, "unexpected HTTP status")

			var resp map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))

			if tt.wantError != "" {
				assert.Equal(t, "error", resp["status"])
				assert.Contains(t, errorMessage(t, resp), tt.wantError, "error message mismatch")
			}

			if tt.assertState != nil {
				tt.assertState(t, repo, fileManager)
			}
		})
	}
}
