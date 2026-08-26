package getplugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/pluginstore/getplugin"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFindFailed = errors.New("find failed")

func TestGetPlugin(t *testing.T) {
	t.Parallel()
	storeResp := pluginstore.PluginDetails{
		ID:            "hexeditor4jm2",
		URL:           "https://plugins.gameap.dev/plugins/hexeditor4jm2",
		Name:          "HEX Editor",
		Summary:       "Hex editor in filemanager",
		Description:   "Full description here",
		License:       "MIT",
		RepositoryURL: "https://github.com/gameap/plugin-hex-editor",
		Author:        pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:      pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
		LatestVersion: "1.0.0",
		PublishedAt:   lo.Must(time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")),
		CreatedAt:     lo.Must(time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")),
		UpdatedAt:     lo.Must(time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")),
	}

	tests := []struct {
		name       string
		statusCode int
		wantStatus int
		wantBody   string
	}{
		{
			name:       "successful_response",
			statusCode: http.StatusOK,
			wantStatus: http.StatusOK,
			wantBody: `{
				"id": "hexeditor4jm2",
				"url": "https://plugins.gameap.dev/plugins/hexeditor4jm2",
				"name": "HEX Editor",
				"summary": "Hex editor in filemanager",
				"description": "Full description here",
				"icon_url": "",
				"license": "MIT",
				"repository_url": "https://github.com/gameap/plugin-hex-editor",
				"min_gameap_version": "",
				"min_plugin_api_version": "",
				"author": {"id": 2, "username": "GameAP"},
				"category": {"id": 3, "slug": "files", "name": "Files", "description": "", "icon": ""},
				"labels": [],
				"download_count": 0,
				"rating_avg": 0,
				"rating_count": 0,
				"latest_version": "1.0.0",
				"requires_subscription": false,
				"published_at": "2026-01-01T00:00:00Z",
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-01T00:00:00Z",
				"installed": false
			}`,
		},
		{
			name:       "store_error",
			statusCode: http.StatusInternalServerError,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(storeResp)
				}
			}))
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
			pluginRepo := inmemory.NewPluginRepository()

			h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
			req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})

			h.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				err := json.Unmarshal(recorder.Body.Bytes(), &resp)
				require.NoError(t, err)

				if tt.wantBody != "" {
					assert.JSONEq(t, tt.wantBody, recorder.Body.String())
				}
			}
		})
	}
}

func TestGetPlugin_cache_hit_avoids_second_upstream_call(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storeResp := pluginstore.PluginDetails{
		ID:            "hexeditor4jm2",
		Name:          "HEX Editor",
		LatestVersion: "1.0.0",
		Author:        pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:      pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
	}

	var pluginCalls atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/plugins/") {
			pluginCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(storeResp)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())

	doRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, req)

		return recorder
	}

	// ACT
	first := doRequest()
	second := doRequest()

	// ASSERT
	assert.Equal(t, http.StatusOK, first.Code, "first request must succeed")
	assert.Equal(t, http.StatusOK, second.Code, "second request must succeed")
	assert.Equal(t, int32(1), pluginCalls.Load(), "second request must be served from cache, upstream hit only once")
	assert.JSONEq(t, first.Body.String(), second.Body.String(), "cached response must match the original")
}

func TestGetPlugin_plugin_repo_find_error_is_swallowed(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storeResp := pluginstore.PluginDetails{
		ID:            "hexeditor4jm2",
		Name:          "HEX Editor",
		LatestVersion: "1.0.0",
		Author:        pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:      pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(storeResp)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	pluginRepo := &errPluginRepo{PluginRepository: inmemory.NewPluginRepository(), findErr: errFindFailed}
	h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
	recorder := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(recorder, req)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, "repo error must not break the upstream response")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, "hexeditor4jm2", resp["id"])
	assert.Equal(t, false, resp["installed"], "repo error must yield installed=false")
	_, hasInstalledVersion := resp["installed_version"]
	assert.False(t, hasInstalledVersion, "installed_version must be omitted when repo lookup fails")
}

func TestGetPlugin_license_validation_error_is_swallowed(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storeResp := pluginstore.PluginDetails{
		ID:                   "hexeditor4jm2",
		Name:                 "HEX Editor",
		LatestVersion:        "1.0.0",
		RequiresSubscription: true,
		Author:               pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:             pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/plugins/"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(storeResp)
		case r.URL.Path == "/licenses/validate":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "test-license-key", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
	recorder := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(recorder, req)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code, "license validation error must not fail the request")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, "hexeditor4jm2", resp["id"])
	assert.Equal(t, true, resp["requires_subscription"])
	_, hasSubField := resp["has_subscription"]
	assert.False(t, hasSubField, "has_subscription must be absent when license validation fails")
}

func TestGetPlugin_installed_plugin_response_contains_version(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storeResp := pluginstore.PluginDetails{
		ID:            "hexeditor4jm2",
		Name:          "HEX Editor",
		LatestVersion: "2.0.0",
		Author:        pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:      pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(storeResp)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	require.NoError(t, pluginRepo.Save(t.Context(), &domain.Plugin{
		ID:      pkgplugin.ParsePluginID("hexeditor4jm2"),
		Name:    "HEX Editor",
		Version: "1.2.3",
		Status:  domain.PluginStatusActive,
	}))
	h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
	recorder := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(recorder, req)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["installed"], "installed flag must reflect repo state")
	assert.Equal(t, "1.2.3", resp["installed_version"], "installed_version must come from local repo, not store")
	assert.Equal(t, "2.0.0", resp["latest_version"], "latest_version must come from store, not repo")
}

func TestGetPlugin_subscription_info_populated_from_valid_license(t *testing.T) {
	t.Parallel()
	// ARRANGE
	expiresAt := lo.Must(time.Parse(time.RFC3339, "2027-12-31T23:59:59Z"))
	storeResp := pluginstore.PluginDetails{
		ID:                   "hexeditor4jm2",
		Name:                 "HEX Editor",
		LatestVersion:        "1.0.0",
		RequiresSubscription: true,
		Author:               pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:             pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
	}
	licenseResp := pluginstore.LicenseValidation{
		Valid: true,
		Subscriptions: []pluginstore.LicenseSubscription{
			{PluginID: "hexeditor4jm2", PluginName: "HEX Editor", ExpiresAt: expiresAt},
		},
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasPrefix(r.URL.Path, "/plugins/"):
			_ = json.NewEncoder(w).Encode(storeResp)
		case r.URL.Path == "/licenses/validate":
			_ = json.NewEncoder(w).Encode(licenseResp)
		}
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "test-license-key", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
	recorder := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(recorder, req)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["has_subscription"], "has_subscription must reflect valid license with matching plugin")
	require.Contains(t, resp, "subscription_expires_at")
	assert.Equal(t, "2027-12-31T23:59:59Z", resp["subscription_expires_at"])
}

func TestGetPlugin_icon_url_rewritten_to_panel_endpoint(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storeResp := pluginstore.PluginDetails{
		ID:            "hexeditor4jm2",
		Name:          "HEX Editor",
		IconURL:       "https://plugins.gameap.dev/api/plugins/hexeditor4jm2/icon",
		LatestVersion: "1.0.0",
		Author:        pluginstore.Author{ID: 2, Username: "GameAP"},
		Category:      pluginstore.Category{ID: 3, Slug: "files", Name: "Files"},
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(storeResp)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	pluginRepo := inmemory.NewPluginRepository()
	h := getplugin.NewHandler(storeService, pluginRepo, api.NewResponder())

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
	recorder := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(recorder, req)

	// ASSERT
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(
		t,
		"/api/plugin-store/plugins/hexeditor4jm2/icon",
		resp["icon_url"],
		"external store icon URL must be rewritten to the panel proxy endpoint",
	)
}

type errPluginRepo struct {
	repositories.PluginRepository

	findErr error
}

func (r *errPluginRepo) Find(
	_ context.Context,
	_ *filters.FindPlugin,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Plugin, error) {
	return nil, r.findErr
}
