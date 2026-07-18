package installplugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/pluginstore/installplugin"
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

func TestInstallPlugin(t *testing.T) {
	pluginDetails := pluginstore.PluginDetails{
		ID:            "testplugin123",
		Name:          "Test Plugin",
		Summary:       "A test plugin",
		Description:   "Full description",
		Author:        pluginstore.Author{ID: 1, Username: "TestAuthor"},
		LatestVersion: "1.0.0",
	}

	versions := pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
		Data: []pluginstore.PluginVersion{
			{
				ID:       1,
				Version:  "1.0.0",
				FileHash: "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9",
				IsStable: true,
			},
		},
		Total: 1,
	}

	wasmContent := []byte("test data")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "successful_install",
			body:       "",
			wantStatus: http.StatusOK,
			wantBody: `{
				"name": "Test Plugin",
				"version": "1.0.0",
				"description": "Full description",
				"author": "TestAuthor",
				"status": "active"
			}`,
		},
		{
			name:       "install_with_version",
			body:       `{"version": "1.0.0"}`,
			wantStatus: http.StatusOK,
			wantBody: `{
				"name": "Test Plugin",
				"version": "1.0.0",
				"description": "Full description",
				"author": "TestAuthor",
				"status": "active"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/plugins/testplugin123":
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(pluginDetails)
				case "/plugins/testplugin123/versions":
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(versions)
				case "/plugins/testplugin123/versions/1.0.0/download":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(wasmContent)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
			pluginRepo := inmemory.NewPluginRepository()
			fileManager := files.NewInMemoryFileManager()

			h := installplugin.NewHandler(
				storeService,
				pluginRepo,
				fileManager,
				nil,
				nil,
				"plugins",
				api.NewResponder(),
			)
			recorder := httptest.NewRecorder()

			var body *bytes.Reader
			if tt.body != "" {
				body = bytes.NewReader([]byte(tt.body))
			} else {
				body = bytes.NewReader([]byte{})
			}

			req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/store/plugins/testplugin123/install", body)
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": "testplugin123"})

			h.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				err := json.Unmarshal(recorder.Body.Bytes(), &resp)
				require.NoError(t, err)

				assert.NotNil(t, resp["id"])
				assert.NotNil(t, resp["installed_at"])
				delete(resp, "id")
				delete(resp, "installed_at")

				if tt.wantBody != "" {
					respWithoutDynamic, err := json.Marshal(resp)
					require.NoError(t, err)
					assert.JSONEq(t, tt.wantBody, string(respWithoutDynamic))
				}

				installed, err := pluginRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)
			}
		})
	}
}

func TestInstallPlugin_already_installed(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()

	existingPlugin := domain.Plugin{
		ID:      pkgplugin.ParsePluginID("testplugin"),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}
	err := pluginRepo.Save(context.Background(), &existingPlugin)
	require.NoError(t, err)

	h := installplugin.NewHandler(
		storeService,
		pluginRepo,
		fileManager,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/store/plugins/testplugin/install", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "testplugin"})

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusConflict, recorder.Code)
}

const (
	testPluginID    = "testplugin123"
	testPluginPath  = "plugins/testplugin123.wasm"
	testWasmHashHex = "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"
)

var testWasmContent = []byte("test data")

type upstreamConfig struct {
	pluginDetails    *pluginstore.PluginDetails
	versions         *pluginstore.PaginatedResponse[pluginstore.PluginVersion]
	versionsStatus   int
	licenseValidate  *pluginstore.LicenseValidation
	licenseStatus    int
	downloadStatus   int
	downloadBody     []byte
	failOnUnexpected bool
}

func newUpstreamServer(t *testing.T, cfg upstreamConfig) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/plugins/" + testPluginID:
			if cfg.pluginDetails == nil {
				w.WriteHeader(http.StatusInternalServerError)

				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(cfg.pluginDetails)
		case "/plugins/" + testPluginID + "/versions":
			status := cfg.versionsStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			if cfg.versions != nil {
				_ = json.NewEncoder(w).Encode(cfg.versions)
			}
		case "/licenses/validate":
			status := cfg.licenseStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			if cfg.licenseValidate != nil {
				_ = json.NewEncoder(w).Encode(cfg.licenseValidate)
			}
		default:
			if isDownloadPath(r.URL.Path) {
				status := cfg.downloadStatus
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				if cfg.downloadBody != nil {
					_, _ = w.Write(cfg.downloadBody)
				}

				return
			}
			if cfg.failOnUnexpected {
				t.Errorf("unexpected upstream request: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func isDownloadPath(path string) bool {
	const prefix = "/plugins/" + testPluginID + "/versions/"

	return strings.HasPrefix(path, prefix) && strings.HasSuffix(path, "/download")
}

type fakeFileManager struct {
	inner        *files.InMemoryFileManager
	writeErr     error
	deleteCalls  atomic.Int32
	deletedPaths []string
}

func newFakeFileManager() *fakeFileManager {
	return &fakeFileManager{inner: files.NewInMemoryFileManager()}
}

func (f *fakeFileManager) Read(ctx context.Context, path string) ([]byte, error) {
	return f.inner.Read(ctx, path)
}

func (f *fakeFileManager) Write(ctx context.Context, path string, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}

	return f.inner.Write(ctx, path, data)
}

func (f *fakeFileManager) Delete(ctx context.Context, path string) error {
	f.deleteCalls.Add(1)
	f.deletedPaths = append(f.deletedPaths, path)

	return f.inner.Delete(ctx, path)
}

func (f *fakeFileManager) Exists(ctx context.Context, path string) bool {
	return f.inner.Exists(ctx, path)
}

func (f *fakeFileManager) List(ctx context.Context, dir string) ([]string, error) {
	return f.inner.List(ctx, dir)
}

type errPluginRepo struct {
	inner   *inmemory.PluginRepository
	saveErr error
}

func (r *errPluginRepo) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	return r.inner.FindAll(ctx, order, pagination)
}

func (r *errPluginRepo) Find(
	ctx context.Context,
	filter *filters.FindPlugin,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	return r.inner.Find(ctx, filter, order, pagination)
}

func (r *errPluginRepo) Save(ctx context.Context, p *domain.Plugin) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.inner.Save(ctx, p)
}

func (r *errPluginRepo) Delete(ctx context.Context, id domain.Uint64ID) error {
	return r.inner.Delete(ctx, id)
}

func (r *errPluginRepo) Exists(ctx context.Context, filter *filters.FindPlugin) (bool, error) {
	return r.inner.Exists(ctx, filter)
}

var _ repositories.PluginRepository = (*errPluginRepo)(nil)

type errLoaderManager struct {
	loadErr error
}

func (m *errLoaderManager) Load(
	_ context.Context,
	_ []byte,
	_ map[string]string,
	_ uint64,
) (*pkgplugin.LoadedPlugin, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}

	return &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{Id: testPluginID, Name: "Test Plugin", Version: "1.0.0"},
	}, nil
}

func (m *errLoaderManager) LoadTransient(
	ctx context.Context,
	wasmBytes []byte,
	config map[string]string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return m.Load(ctx, wasmBytes, config, pluginID)
}

func (m *errLoaderManager) Unload(_ context.Context, _ string) error           { return nil }
func (m *errLoaderManager) GetPlugin(_ string) (*pkgplugin.LoadedPlugin, bool) { return nil, false }
func (m *errLoaderManager) GetPlugins() []*pkgplugin.LoadedPlugin              { return nil }
func (m *errLoaderManager) Shutdown(_ context.Context) error                   { return nil }

func defaultPluginDetails(requiresSubscription bool) *pluginstore.PluginDetails {
	return &pluginstore.PluginDetails{
		ID:                   testPluginID,
		Name:                 "Test Plugin",
		Description:          "Full description",
		Author:               pluginstore.Author{ID: 1, Username: "TestAuthor"},
		LatestVersion:        "1.0.0",
		RequiresSubscription: requiresSubscription,
	}
}

func defaultVersions() *pluginstore.PaginatedResponse[pluginstore.PluginVersion] {
	return &pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
		Data: []pluginstore.PluginVersion{
			{
				ID:       1,
				Version:  "1.0.0",
				FileHash: testWasmHashHex,
				IsStable: true,
			},
		},
		Total: 1,
	}
}

func TestInstallPlugin_errors(t *testing.T) {
	tests := []struct {
		name          string
		licenseKey    string
		body          string
		buildUpstream func() upstreamConfig
		buildFiles    func() *fakeFileManager
		buildRepo     func() repositories.PluginRepository
		buildLoader   func(repo repositories.PluginRepository, fm files.FileManager) *plugin.Loader
		wantStatus    int
		wantError     string
		assertSideFx  func(t *testing.T, fm *fakeFileManager, repo repositories.PluginRepository)
	}{
		{
			name:       "subscription_required_no_license_key",
			licenseKey: "",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{pluginDetails: defaultPluginDetails(true)}
			},
			wantStatus: http.StatusPaymentRequired,
			wantError:  "this plugin requires a subscription",
		},
		{
			name:       "subscription_required_validate_returns_error",
			licenseKey: "test-key",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(true),
					licenseStatus: http.StatusInternalServerError,
				}
			},
			wantStatus: http.StatusPaymentRequired,
			wantError:  "this plugin requires a subscription",
		},
		{
			name:       "subscription_required_invalid_license",
			licenseKey: "test-key",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails:   defaultPluginDetails(true),
					licenseValidate: &pluginstore.LicenseValidation{Valid: false},
				}
			},
			wantStatus: http.StatusPaymentRequired,
			wantError:  "this plugin requires a subscription",
		},
		{
			name:       "no_subscription_for_plugin",
			licenseKey: "test-key",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(true),
					licenseValidate: &pluginstore.LicenseValidation{
						Valid: true,
						Subscriptions: []pluginstore.LicenseSubscription{
							{PluginID: "different-plugin", PluginName: "Other"},
						},
					},
				}
			},
			wantStatus: http.StatusPaymentRequired,
			wantError:  "no active subscription for this plugin",
		},
		{
			name: "no_versions_available",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(false),
					versions: &pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
						Data: []pluginstore.PluginVersion{},
					},
				}
			},
			wantStatus: http.StatusNotFound,
			wantError:  "no versions available for this plugin",
		},
		{
			name: "version_not_found",
			body: `{"version":"9.9.9"}`,
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(false),
					versions:      defaultVersions(),
				}
			},
			wantStatus: http.StatusNotFound,
			wantError:  "specified version not found",
		},
		{
			name: "download_returns_error",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails:  defaultPluginDetails(false),
					versions:       defaultVersions(),
					downloadStatus: http.StatusInternalServerError,
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  http.StatusText(http.StatusInternalServerError),
		},
		{
			name: "download_hash_mismatch",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(false),
					versions:      defaultVersions(),
					downloadBody:  []byte("totally different bytes"),
				}
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "plugin file hash mismatch",
		},
		{
			name: "file_write_error",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(false),
					versions:      defaultVersions(),
					downloadBody:  testWasmContent,
				}
			},
			buildFiles: func() *fakeFileManager {
				fm := newFakeFileManager()
				fm.writeErr = errors.New("disk full")

				return fm
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  http.StatusText(http.StatusInternalServerError),
			assertSideFx: func(t *testing.T, fm *fakeFileManager, _ repositories.PluginRepository) {
				t.Helper()
				assert.Equal(t, int32(0), fm.deleteCalls.Load(), "delete must not run when write fails")
			},
		},
		{
			name: "repo_save_error_triggers_cleanup",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(false),
					versions:      defaultVersions(),
					downloadBody:  testWasmContent,
				}
			},
			buildRepo: func() repositories.PluginRepository {
				return &errPluginRepo{
					inner:   inmemory.NewPluginRepository(),
					saveErr: errors.New("repo save failed"),
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  http.StatusText(http.StatusInternalServerError),
			assertSideFx: func(t *testing.T, fm *fakeFileManager, repo repositories.PluginRepository) {
				t.Helper()
				assert.Equal(t, int32(1), fm.deleteCalls.Load(), "wasm file must be cleaned up on save failure")
				require.Len(t, fm.deletedPaths, 1)
				assert.Equal(t, testPluginPath, fm.deletedPaths[0])

				stored, err := repo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				assert.Empty(t, stored, "no plugin record must be persisted on save failure")
			},
		},
		{
			name: "plugin_load_error_records_status_error",
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{
					pluginDetails: defaultPluginDetails(false),
					versions:      defaultVersions(),
					downloadBody:  testWasmContent,
				}
			},
			buildLoader: func(repo repositories.PluginRepository, fm files.FileManager) *plugin.Loader {
				return plugin.NewLoader(
					&errLoaderManager{loadErr: errors.New("wasm runtime broken")},
					fm,
					repo,
					nil,
					"plugins",
				)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "plugin installed but failed to load",
			assertSideFx: func(t *testing.T, fm *fakeFileManager, repo repositories.PluginRepository) {
				t.Helper()
				stored, err := repo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, stored, 1, "plugin record must remain persisted after load failure")
				assert.Equal(t, domain.PluginStatusError, stored[0].Status, "status must be flipped to error")
				assert.Equal(t, "Test Plugin", stored[0].Name)
				assert.Equal(t, int32(0), fm.deleteCalls.Load(), "wasm file must be kept on load failure")
			},
		},
		{
			name: "invalid_json_body",
			body: `{broken`,
			buildUpstream: func() upstreamConfig {
				return upstreamConfig{}
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			cfg := tt.buildUpstream()
			mockServer := newUpstreamServer(t, cfg)
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, tt.licenseKey, cache.NewInMemory())

			var fm *fakeFileManager
			if tt.buildFiles != nil {
				fm = tt.buildFiles()
			} else {
				fm = newFakeFileManager()
			}

			var repo repositories.PluginRepository
			if tt.buildRepo != nil {
				repo = tt.buildRepo()
			} else {
				repo = inmemory.NewPluginRepository()
			}

			var loader *plugin.Loader
			if tt.buildLoader != nil {
				loader = tt.buildLoader(repo, fm)
			}

			h := installplugin.NewHandler(
				storeService,
				repo,
				fm,
				loader,
				nil,
				"plugins",
				api.NewResponder(),
			)

			var body *bytes.Reader
			if tt.body != "" {
				body = bytes.NewReader([]byte(tt.body))
			} else {
				body = bytes.NewReader([]byte{})
			}
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/admin/plugins/store/plugins/"+testPluginID+"/install",
				body,
			)
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": testPluginID})

			recorder := httptest.NewRecorder()

			// ACT
			h.ServeHTTP(recorder, req)

			// ASSERT
			assert.Equal(t, tt.wantStatus, recorder.Code, "unexpected status code")

			var resp map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), "response must be JSON")

			errMsg, ok := resp["error"].(string)
			require.True(t, ok, "response must include error field")
			assert.Contains(t, errMsg, tt.wantError, "error message mismatch")

			if tt.assertSideFx != nil {
				tt.assertSideFx(t, fm, repo)
			}
		})
	}
}

func TestInstallPlugin_load_error_keeps_record_with_correct_timestamps(t *testing.T) {
	// ARRANGE
	cfg := upstreamConfig{
		pluginDetails: defaultPluginDetails(false),
		versions:      defaultVersions(),
		downloadBody:  testWasmContent,
	}
	mockServer := newUpstreamServer(t, cfg)
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	fm := newFakeFileManager()
	repo := inmemory.NewPluginRepository()
	loader := plugin.NewLoader(
		&errLoaderManager{loadErr: errors.New("manager rejected")},
		fm,
		repo,
		nil,
		"plugins",
	)

	h := installplugin.NewHandler(storeService, repo, fm, loader, nil, "plugins", api.NewResponder())

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/plugins/store/plugins/"+testPluginID+"/install",
		bytes.NewReader([]byte{}),
	)
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": testPluginID})

	beforeInstall := time.Now()

	// ACT
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)

	// ASSERT
	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)

	stored, err := repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, stored, 1)

	got := stored[0]
	assert.Equal(t, domain.PluginStatusError, got.Status)
	require.NotNil(t, got.InstalledAt, "installed_at must remain set after load failure")
	assert.False(t, got.InstalledAt.Before(beforeInstall), "installed_at must be from this install")
	require.NotNil(t, got.Filename)
	assert.Equal(t, testPluginID+".wasm", *got.Filename)
	require.NotNil(t, got.Source)
	assert.Equal(t, mockServer.URL+"/plugins/"+testPluginID, *got.Source)
}
