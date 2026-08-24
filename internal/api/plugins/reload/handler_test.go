package reload_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/plugins/reload"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

type fakeReloader struct {
	mu     sync.Mutex
	calls  []domain.Uint64ID
	plugin *domain.Plugin
	loaded *pkgplugin.LoadedPlugin
	err    error
}

func (f *fakeReloader) Reload(_ context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, dbID)

	return f.plugin, f.loaded, f.err
}

func reloadRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugins/"+id+"/reload", nil)

	return mux.SetURLVars(req, map[string]string{"id": id})
}

func TestReload(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	dbID := pkgplugin.ParsePluginID("testplugin123")
	activeRow := &domain.Plugin{
		ID:           dbID,
		Name:         "Test Plugin",
		Version:      "1.2.0",
		Status:       domain.PluginStatusActive,
		LastLoadedAt: &now,
	}
	erroredRow := &domain.Plugin{
		ID:          dbID,
		Name:        "Test Plugin",
		Version:     "1.2.0",
		Status:      domain.PluginStatusError,
		LastError:   new("plugin file not found: plugins/testplugin123.wasm"),
		LastErrorAt: &now,
	}

	tests := []struct {
		name           string
		reloader       *fakeReloader
		wantStatus     int
		wantError      string
		wantTitle      string
		wantAuditType  audit.EventType
		wantOutcome    audit.Outcome
		wantReason     string
		checkResponse  func(t *testing.T, body map[string]any)
		wantNoAudit    bool
		wantCalledWith domain.Uint64ID
	}{
		{
			name: "success_returns_plugin_state",
			reloader: &fakeReloader{
				plugin: activeRow,
				loaded: &pkgplugin.LoadedPlugin{Info: &proto.PluginInfo{Id: "testplugin123"}},
			},
			wantStatus:     http.StatusOK,
			wantAuditType:  audit.EventPluginReloaded,
			wantOutcome:    audit.OutcomeSuccess,
			wantCalledWith: dbID,
			checkResponse: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, pkgplugin.CompactPluginID(dbID), body["id"])
				assert.Equal(t, "Test Plugin", body["name"])
				assert.Equal(t, "1.2.0", body["version"])
				assert.Equal(t, "active", body["status"])
				assert.Nil(t, body["error"])
				assert.Nil(t, body["error_at"])
				assert.Equal(t, true, body["loaded"])
				assert.Equal(t, now.Format(time.RFC3339), body["last_loaded_at"])
			},
		},
		{
			name:           "not_installed_returns_404",
			reloader:       &fakeReloader{err: internalplugin.ErrPluginNotInstalled},
			wantStatus:     http.StatusNotFound,
			wantError:      "plugin not installed",
			wantNoAudit:    true,
			wantCalledWith: dbID,
		},
		{
			name:           "disabled_by_operator_returns_409",
			reloader:       &fakeReloader{plugin: activeRow, err: internalplugin.ErrPluginDisabled},
			wantStatus:     http.StatusConflict,
			wantError:      "plugin is disabled",
			wantNoAudit:    true,
			wantCalledWith: dbID,
		},
		{
			name:           "updating_returns_409",
			reloader:       &fakeReloader{plugin: activeRow, err: internalplugin.ErrPluginUpdating},
			wantStatus:     http.StatusConflict,
			wantError:      "plugin is being updated",
			wantNoAudit:    true,
			wantCalledWith: dbID,
		},
		{
			name: "load_failure_returns_422_with_sanitized_message",
			reloader: &fakeReloader{
				plugin: erroredRow,
				//nolint:revive // mimics the wazero runtime error format
				err: errors.New("failed to load plugin: runtime error\nwasm stack trace:\n\tfunc1()\nGo runtime stack trace:\ngoroutine 1 [running]:"),
			},
			wantStatus:     http.StatusUnprocessableEntity,
			wantError:      "failed to load plugin: runtime error\nwasm stack trace:\n\tfunc1(): plugin failed to load",
			wantTitle:      "plugins.reload_failed_title",
			wantAuditType:  audit.EventPluginReloaded,
			wantOutcome:    audit.OutcomeFailure,
			wantReason:     "load_failed",
			wantCalledWith: dbID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := &auditCapture{}
			handler := reload.NewHandler(tt.reloader, nil, api.NewResponder(), recorder)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, reloadRequest("testplugin123"))

			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, []domain.Uint64ID{tt.wantCalledWith}, tt.reloader.calls)

			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

			if tt.wantError != "" {
				assert.Contains(t, body["error"], tt.wantError)
				assert.NotContains(t, body["error"], "Go runtime stack trace")
			}

			if tt.wantTitle != "" {
				assert.Equal(t, tt.wantTitle, body["title"])
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, body)
			}

			events := recorder.snapshot()
			if tt.wantNoAudit {
				assert.Empty(t, events)

				return
			}

			require.Len(t, events, 1)
			assert.Equal(t, tt.wantAuditType, events[0].Type)
			assert.Equal(t, tt.wantOutcome, events[0].Outcome)
			assert.Equal(t, audit.CategoryPluginOp, events[0].Category)
			assert.Equal(t, "plugin", events[0].ResourceType)
			assert.Equal(t, "reload", events[0].Action)
			assert.Equal(t, tt.wantReason, events[0].Reason)
			require.Len(t, events[0].Extra, 1)
			assert.Equal(t, "manual", events[0].Extra[0].Value.String())
		})
	}
}

func TestReload_MissingID(t *testing.T) {
	t.Parallel()
	reloader := &fakeReloader{}
	handler := reload.NewHandler(reloader, nil, api.NewResponder(), nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/plugins//reload", nil))

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "plugin ID is required")
	assert.Empty(t, reloader.calls)
}

type resolvingReloader struct {
	fakeReloader

	mapping map[string]domain.Uint64ID
}

func (r *resolvingReloader) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	dbID, ok := r.mapping[managerID]

	return dbID, ok
}

func TestReload_ResolvesDatabaseIDThroughLoader(t *testing.T) {
	t.Parallel()
	// A store plugin whose wasm declares another ID is registered in the
	// manager under that ID; the loader maps it back to the database row.
	reloader := &resolvingReloader{
		fakeReloader: fakeReloader{plugin: &domain.Plugin{ID: 777, Name: "Mapped", Version: "1.0.0", Status: domain.PluginStatusActive}},
		mapping:      map[string]domain.Uint64ID{"declaredid": 777},
	}
	handler := reload.NewHandler(reloader, nil, api.NewResponder(), nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, reloadRequest("declaredid"))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []domain.Uint64ID{777}, reloader.calls)
}
