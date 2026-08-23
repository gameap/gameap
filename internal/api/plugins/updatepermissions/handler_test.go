// Tests for PUT /api/admin/plugins/{id}/permissions.
//
// OWASP API Security Top 10:2023 — API5:2023 Broken Function Level
// Authorization: the grants an operator sets here are what every privileged
// host library enforces, so the endpoint must only ever persist known
// permission names and must record who changed what.
package updatepermissions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/updatepermissions"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gorilla/mux"
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

type fakeManager struct {
	plugins map[string]*pkgplugin.LoadedPlugin
}

func (f *fakeManager) GetPlugin(id string) (*pkgplugin.LoadedPlugin, bool) {
	plugin, ok := f.plugins[id]

	return plugin, ok
}

type fakeRefresher struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeRefresher) RefreshSubscriptions(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	return nil
}

func (f *fakeRefresher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

func permissionsRequest(id, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/api/admin/plugins/"+id+"/permissions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return mux.SetURLVars(req, map[string]string{"id": id})
}

func setupRepo(t *testing.T, plugins ...*domain.Plugin) *inmemory.PluginRepository {
	t.Helper()

	repo := inmemory.NewPluginRepository()
	for _, plugin := range plugins {
		require.NoError(t, repo.Save(context.Background(), plugin))
	}

	return repo
}

func TestUpdatePermissions(t *testing.T) {
	t.Parallel()

	dbID := pkgplugin.ParsePluginID("testplugin123")
	compactID := pkgplugin.CompactPluginID(dbID)

	installed := func() *domain.Plugin {
		return &domain.Plugin{
			ID:                  dbID,
			Name:                "Test Plugin",
			Version:             "1.0.0",
			Status:              domain.PluginStatusActive,
			RequiredPermissions: []domain.PluginPermission{domain.PluginPermissionFiles},
			AllowedPermissions:  []domain.PluginPermission{domain.PluginPermissionFiles, domain.PluginPermissionListenEvents},
		}
	}

	loaded := &pkgplugin.LoadedPlugin{
		Info: &proto.PluginInfo{Id: compactID},
		DBID: uint64(dbID),
		HostImports: []pkgplugin.HostImport{
			{Module: "gameap-nodecmd", Function: "execute_command"},
			{Module: "gameap-nodefs", Function: "read_dir"},
		},
		SubscribedEvents: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
	}

	tests := []struct {
		name         string
		id           string
		body         string
		plugins      []*domain.Plugin
		loaded       map[string]*pkgplugin.LoadedPlugin
		wantStatus   int
		wantError    string
		wantAllowed  []string
		wantMissing  []string
		wantUsed     []string
		wantGranted  []string
		wantRevoked  []string
		wantRefresh  int
		wantAuditLen int
	}{
		{
			name:        "grants_and_revokes",
			id:          compactID,
			body:        `{"allowed_permissions": ["files", "node_commands"]}`,
			plugins:     []*domain.Plugin{installed()},
			loaded:      map[string]*pkgplugin.LoadedPlugin{compactID: loaded},
			wantStatus:  http.StatusOK,
			wantAllowed: []string{"files", "node_commands"},
			// The fixture only reads through gameap-nodefs, so it uses
			// files_read; the granted "files" includes it.
			wantUsed:     []string{"files_read", "listen_events", "node_commands"},
			wantMissing:  []string{"listen_events"},
			wantGranted:  []string{"node_commands"},
			wantRevoked:  []string{"listen_events"},
			wantRefresh:  1,
			wantAuditLen: 1,
		},
		{
			name:         "unchanged_listen_events_skips_the_refresh",
			id:           compactID,
			body:         `{"allowed_permissions": ["files", "listen_events", "manage_servers"]}`,
			plugins:      []*domain.Plugin{installed()},
			wantStatus:   http.StatusOK,
			wantAllowed:  []string{"files", "listen_events", "manage_servers"},
			wantGranted:  []string{"manage_servers"},
			wantRevoked:  []string{},
			wantRefresh:  0,
			wantAuditLen: 1,
		},
		{
			name:         "duplicates_collapse",
			id:           compactID,
			body:         `{"allowed_permissions": ["files", "files", "listen_events"]}`,
			plugins:      []*domain.Plugin{installed()},
			wantStatus:   http.StatusOK,
			wantAllowed:  []string{"files", "listen_events"},
			wantGranted:  []string{},
			wantRevoked:  []string{},
			wantAuditLen: 1,
		},
		{
			name:         "empty_list_revokes_everything",
			id:           compactID,
			body:         `{"allowed_permissions": []}`,
			plugins:      []*domain.Plugin{installed()},
			wantStatus:   http.StatusOK,
			wantAllowed:  []string{},
			wantGranted:  []string{},
			wantRevoked:  []string{"files", "listen_events"},
			wantRefresh:  1,
			wantAuditLen: 1,
		},
		{
			name:       "unknown_permission_is_rejected",
			id:         compactID,
			body:       `{"allowed_permissions": ["files", "root", "everything"]}`,
			plugins:    []*domain.Plugin{installed()},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "unknown permissions: root, everything",
		},
		{
			name:       "plugin_not_installed",
			id:         compactID,
			body:       `{"allowed_permissions": ["files"]}`,
			wantStatus: http.StatusNotFound,
			wantError:  "plugin is not installed",
		},
		{
			name:       "malformed_body",
			id:         compactID,
			body:       `{"allowed_permissions": "files"}`,
			plugins:    []*domain.Plugin{installed()},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := setupRepo(t, tt.plugins...)
			recorder := &auditCapture{}
			refresher := &fakeRefresher{}
			handler := updatepermissions.NewHandler(repo, &fakeManager{plugins: tt.loaded}, nil, refresher, nil,
				api.NewResponder(), recorder)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, permissionsRequest(tt.id, tt.body))

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())

			if tt.wantError != "" {
				assert.Contains(t, rec.Body.String(), tt.wantError)
				assert.Empty(t, recorder.snapshot(), "a refused update is not a permissions change")

				return
			}

			var resp struct {
				ID                  string   `json:"id"`
				RequiredPermissions []string `json:"required_permissions"`
				AllowedPermissions  []string `json:"allowed_permissions"`
				UsedPermissions     []string `json:"used_permissions"`
				MissingPermissions  []string `json:"missing_permissions"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			assert.Equal(t, compactID, resp.ID)
			assert.Equal(t, []string{"files"}, resp.RequiredPermissions, "declared permissions are untouched")
			assert.Equal(t, tt.wantAllowed, resp.AllowedPermissions)
			assert.Equal(t, tt.wantUsed, resp.UsedPermissions)
			assert.Equal(t, tt.wantMissing, resp.MissingPermissions)

			stored, err := repo.Find(context.Background(), filters.FindPluginByIDs(dbID), nil, nil)
			require.NoError(t, err)
			require.Len(t, stored, 1)

			storedNames := make([]string, 0, len(stored[0].AllowedPermissions))
			for _, permission := range stored[0].AllowedPermissions {
				storedNames = append(storedNames, string(permission))
			}
			assert.Equal(t, tt.wantAllowed, storedNames, "the grants must be persisted")

			assert.Equal(t, tt.wantRefresh, refresher.count())

			events := recorder.snapshot()
			require.Len(t, events, tt.wantAuditLen)
			assert.Equal(t, audit.EventPluginPermissionsUpdate, events[0].Type)
			assert.Equal(t, audit.CategoryPluginOp, events[0].Category)
			assert.Equal(t, audit.OutcomeSuccess, events[0].Outcome)
			assert.Equal(t, "plugin", events[0].ResourceType)
			assert.Equal(t, "update", events[0].Action)
			assert.Equal(t, tt.wantGranted, attrStrings(events[0], "granted"))
			assert.Equal(t, tt.wantRevoked, attrStrings(events[0], "revoked"))
		})
	}
}

func attrStrings(event audit.Event, key string) []string {
	for _, attr := range event.Extra {
		if attr.Key == key {
			if values, ok := attr.Value.Any().([]string); ok {
				return values
			}
		}
	}

	return nil
}
