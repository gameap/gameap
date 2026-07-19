package getplugins_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/pluginstore/getplugins"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlugins(t *testing.T) {
	storeResp := pluginstore.PaginatedResponse[pluginstore.Plugin]{
		CurrentPage: 1,
		Data: []pluginstore.Plugin{
			{
				ID:            "hexeditor4jm2",
				URL:           "https://plugins.gameap.dev/plugins/hexeditor4jm2",
				Name:          "HEX Editor",
				Summary:       "Hex editor in filemanager",
				DownloadCount: 100,
				LatestVersion: "1.0.0",
				CreatedAt:     lo.Must(time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")),
				UpdatedAt:     lo.Must(time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")),
			},
		},
		From:     1,
		LastPage: 1,
		PerPage:  20,
		Total:    1,
	}

	tests := []struct {
		name             string
		queryParams      string
		installedPlugins []domain.Plugin
		wantBody         string
	}{
		{
			name:             "no_installed_plugins",
			installedPlugins: []domain.Plugin{},
			wantBody: `{
				"current_page": 1,
				"data": [{
					"id": "hexeditor4jm2",
					"url": "https://plugins.gameap.dev/plugins/hexeditor4jm2",
					"name": "HEX Editor",
					"summary": "Hex editor in filemanager",
					"icon_url": "",
					"category": {"id": 0, "slug": "", "name": ""},
					"labels": [],
					"download_count": 100,
					"rating_avg": 0,
					"rating_count": 0,
					"latest_version": "1.0.0",
					"requires_subscription": false,
					"created_at": "2026-01-01T00:00:00Z",
					"updated_at": "2026-01-01T00:00:00Z",
					"installed": false
				}],
				"from": 1,
				"last_page": 1,
				"per_page": 20,
				"total": 1
			}`,
		},
		{
			name: "with_installed_plugin",
			installedPlugins: []domain.Plugin{
				{
					ID:      pkgplugin.ParsePluginID("hexeditor4jm2"),
					Name:    "HEX Editor",
					Version: "1.0.0",
				},
			},
			wantBody: `{
				"current_page": 1,
				"data": [{
					"id": "hexeditor4jm2",
					"url": "https://plugins.gameap.dev/plugins/hexeditor4jm2",
					"name": "HEX Editor",
					"summary": "Hex editor in filemanager",
					"icon_url": "",
					"category": {"id": 0, "slug": "", "name": ""},
					"labels": [],
					"download_count": 100,
					"rating_avg": 0,
					"rating_count": 0,
					"latest_version": "1.0.0",
					"requires_subscription": false,
					"created_at": "2026-01-01T00:00:00Z",
					"updated_at": "2026-01-01T00:00:00Z",
					"installed": true,
					"installed_version": "1.0.0"
				}],
				"from": 1,
				"last_page": 1,
				"per_page": 20,
				"total": 1
			}`,
		},
		{
			name:        "with_pagination_params",
			queryParams: "?page[number]=2&page[size]=10",
			wantBody: `{
				"current_page": 1,
				"data": [{
					"id": "hexeditor4jm2",
					"url": "https://plugins.gameap.dev/plugins/hexeditor4jm2",
					"name": "HEX Editor",
					"summary": "Hex editor in filemanager",
					"icon_url": "",
					"category": {"id": 0, "slug": "", "name": ""},
					"labels": [],
					"download_count": 100,
					"rating_avg": 0,
					"rating_count": 0,
					"latest_version": "1.0.0",
					"requires_subscription": false,
					"created_at": "2026-01-01T00:00:00Z",
					"updated_at": "2026-01-01T00:00:00Z",
					"installed": false
				}],
				"from": 1,
				"last_page": 1,
				"per_page": 20,
				"total": 1
			}`,
		},
		{
			name:        "with_sort_params",
			queryParams: "?sort=-download_count",
			wantBody: `{
				"current_page": 1,
				"data": [{
					"id": "hexeditor4jm2",
					"url": "https://plugins.gameap.dev/plugins/hexeditor4jm2",
					"name": "HEX Editor",
					"summary": "Hex editor in filemanager",
					"icon_url": "",
					"category": {"id": 0, "slug": "", "name": ""},
					"labels": [],
					"download_count": 100,
					"rating_avg": 0,
					"rating_count": 0,
					"latest_version": "1.0.0",
					"requires_subscription": false,
					"created_at": "2026-01-01T00:00:00Z",
					"updated_at": "2026-01-01T00:00:00Z",
					"installed": false
				}],
				"from": 1,
				"last_page": 1,
				"per_page": 20,
				"total": 1
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(storeResp)
			}))
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
			pluginRepo := inmemory.NewPluginRepository()

			for _, p := range tt.installedPlugins {
				plugin := p
				err := pluginRepo.Save(context.Background(), &plugin)
				require.NoError(t, err)
			}

			h := getplugins.NewHandler(storeService, pluginRepo, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins"+tt.queryParams, nil)

			h.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)

			var resp map[string]any
			err := json.Unmarshal(recorder.Body.Bytes(), &resp)
			require.NoError(t, err)

			if tt.wantBody != "" {
				assert.JSONEq(t, tt.wantBody, recorder.Body.String())
			}
		})
	}
}

func TestGetPlugins_icon_url_rewritten_to_panel_endpoint(t *testing.T) {
	// ARRANGE
	storeResp := pluginstore.PaginatedResponse[pluginstore.Plugin]{
		CurrentPage: 1,
		Data: []pluginstore.Plugin{
			{
				ID:            "hexeditor4jm2",
				Name:          "HEX Editor",
				IconURL:       "https://plugins.gameap.dev/api/plugins/hexeditor4jm2/icon",
				LatestVersion: "1.0.0",
			},
		},
		From:     1,
		LastPage: 1,
		PerPage:  20,
		Total:    1,
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(storeResp)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	h := getplugins.NewHandler(storeService, pluginRepo, api.NewResponder())
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins", nil)

	// ACT
	h.ServeHTTP(recorder, req)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Data []struct {
			IconURL string `json:"icon_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	assert.Equal(
		t,
		"/api/plugin-store/plugins/hexeditor4jm2/icon",
		resp.Data[0].IconURL,
		"external store icon URL must be rewritten to the panel proxy endpoint",
	)
}
