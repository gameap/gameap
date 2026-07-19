package install_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/upload/install"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

func extraString(e audit.Event, key string) (string, bool) {
	for _, a := range e.Extra {
		if a.Key == key {
			return a.Value.String(), true
		}
	}

	return "", false
}

type mockLoaderManager struct {
	loadFunc func(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
}

func (m *mockLoaderManager) LoadTransient(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ctx, wasmBytes, config, pluginID)
	}

	return nil, nil
}

//nolint:unparam // filename is fixed today but kept as a parameter for clarity at call sites
func createMultipartRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)

	_, err = io.Copy(part, bytes.NewReader(content))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/install", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func validWASMBytes() []byte {
	return []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
}

func TestInstall(t *testing.T) {
	tests := []struct {
		name           string
		wasmContent    []byte
		mockManager    *mockLoaderManager
		wantStatus     int
		wantName       string
		wantVersion    string
		wantErrorMatch string
	}{
		{
			name:        "successful_install",
			wasmContent: validWASMBytes(),
			mockManager: &mockLoaderManager{
				loadFunc: func(_ context.Context, _ []byte, _ map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
					assert.Equal(t, uint64(0), pluginID)

					return &pkgplugin.LoadedPlugin{
						Info: &proto.PluginInfo{
							Id:          "testplugin",
							Name:        "Test Plugin",
							Version:     "1.0.0",
							Description: "A test plugin",
							Author:      "Test Author",
							ApiVersion:  "v1",
						},
					}, nil
				},
			},
			wantStatus:  http.StatusOK,
			wantName:    "Test Plugin",
			wantVersion: "1.0.0",
		},
		{
			name:        "invalid_wasm_magic",
			wasmContent: []byte{0x01, 0x02, 0x03, 0x04},
			mockManager: &mockLoaderManager{},
			wantStatus:  http.StatusInternalServerError,
		},
		{
			name:        "load_returns_error",
			wasmContent: validWASMBytes(),
			mockManager: &mockLoaderManager{
				loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
					return nil, errors.New("failed to compile WASM")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pluginRepo := inmemory.NewPluginRepository()
			fileManager := files.NewInMemoryFileManager()

			h := install.NewHandler(
				tt.mockManager,
				pluginRepo,
				fileManager,
				nil,
				nil,
				"plugins",
				api.NewResponder(),
				nil,
			)
			recorder := httptest.NewRecorder()

			req := createMultipartRequest(t, "plugin.wasm", tt.wasmContent)
			h.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				err := json.Unmarshal(recorder.Body.Bytes(), &resp)
				require.NoError(t, err)

				assert.NotNil(t, resp["id"])
				assert.Equal(t, tt.wantName, resp["name"])
				assert.Equal(t, tt.wantVersion, resp["version"])
				assert.Equal(t, "active", resp["status"])
				assert.NotNil(t, resp["installed_at"])

				installed, err := pluginRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, installed, 1)
				assert.Equal(t, tt.wantName, installed[0].Name)
				assert.Equal(t, tt.wantVersion, installed[0].Version)
				assert.NotNil(t, installed[0].Source)
				assert.Contains(t, *installed[0].Source, "file://")
			}

			if tt.wantErrorMatch != "" {
				assert.Contains(t, recorder.Body.String(), tt.wantErrorMatch)
			}
		})
	}
}

func TestInstall_already_installed_returns_409(t *testing.T) {
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

	mockManager := &mockLoaderManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:          "testplugin",
					Name:        "Test Plugin",
					Version:     "1.0.0",
					Description: "A test plugin",
					Author:      "Test Author",
					ApiVersion:  "v1",
				},
			}, nil
		},
	}

	h := install.NewHandler(
		mockManager,
		pluginRepo,
		fileManager,
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := createMultipartRequest(t, "plugin.wasm", validWASMBytes())
	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "plugin already installed")
}

func TestInstall_no_file_uploaded(t *testing.T) {
	h := install.NewHandler(
		&mockLoaderManager{},
		inmemory.NewPluginRepository(),
		files.NewInMemoryFileManager(),
		nil,
		nil,
		"plugins",
		api.NewResponder(),
		nil,
	)
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/upload/install", nil)
	req.Header.Set("Content-Type", "multipart/form-data")

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — installing a plugin loads
//     attacker-supplied executable code into the platform; it must be
//     recorded (OWASP ASVS §7.2.1) so code provenance is auditable.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestInstall_Audit_SuccessfulInstallIsRecorded covers OWASP API8:2023. A
// successful plugin install must emit exactly one plugin.install event with
// outcome success, category plugin_op, the plugin id as the resource, and the
// plugin identifier recorded in Extra for provenance.
func TestInstall_Audit_SuccessfulInstallIsRecorded(t *testing.T) {
	// ARRANGE
	pluginRepo := inmemory.NewPluginRepository()
	fileManager := files.NewInMemoryFileManager()
	mockManager := &mockLoaderManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:          "testplugin",
					Name:        "Test Plugin",
					Version:     "1.0.0",
					Description: "A test plugin",
					Author:      "Test Author",
					ApiVersion:  "v1",
				},
			}, nil
		},
	}

	recorder := &auditCapture{}
	h := install.NewHandler(
		mockManager, pluginRepo, fileManager, nil, nil, "plugins", api.NewResponder(), recorder,
	)
	w := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(w, createMultipartRequest(t, "plugin.wasm", validWASMBytes()))

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "install must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventPluginInstall),
		"exactly one plugin.install event must be emitted per successful install")

	ev, ok := findEvent(events, audit.EventPluginInstall)
	require.True(t, ok, "a successful plugin install must leave a plugin.install audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryPluginOp, ev.Category)
	assert.Equal(t, "plugin", ev.ResourceType)
	assert.NotEmpty(t, ev.ResourceID, "the installed plugin id must be recorded as resource_id")
	assert.Equal(t, "install", ev.Action)
	pluginID, hasPlugin := extraString(ev, "plugin")
	require.True(t, hasPlugin, "the plugin identifier must be recorded for provenance")
	assert.Equal(t, "testplugin", pluginID)
}

// TestInstall_Audit_AlreadyInstalledIsNotRecorded covers OWASP API8:2023. An
// install rejected because the plugin is already present must NOT emit a
// plugin.install event (no executable code was newly loaded).
func TestInstall_Audit_AlreadyInstalledIsNotRecorded(t *testing.T) {
	// ARRANGE
	pluginRepo := inmemory.NewPluginRepository()
	require.NoError(t, pluginRepo.Save(context.Background(), &domain.Plugin{
		ID:      pkgplugin.ParsePluginID("testplugin"),
		Name:    "Test Plugin",
		Version: "1.0.0",
		Status:  domain.PluginStatusActive,
	}))

	mockManager := &mockLoaderManager{
		loadFunc: func(_ context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
			return &pkgplugin.LoadedPlugin{
				Info: &proto.PluginInfo{
					Id:         "testplugin",
					Name:       "Test Plugin",
					Version:    "1.0.0",
					ApiVersion: "v1",
				},
			}, nil
		},
	}

	recorder := &auditCapture{}
	h := install.NewHandler(
		mockManager, pluginRepo, files.NewInMemoryFileManager(), nil, nil, "plugins",
		api.NewResponder(), recorder,
	)
	w := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(w, createMultipartRequest(t, "plugin.wasm", validWASMBytes()))

	// ASSERT
	require.Equal(t, http.StatusConflict, w.Code,
		"an already-installed plugin must be rejected; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventPluginInstall),
		"a rejected install must not be recorded as a successful plugin.install")
}
