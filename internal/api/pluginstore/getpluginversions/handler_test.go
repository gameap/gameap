package getpluginversions_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/pluginstore/getpluginversions"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPluginVersions(t *testing.T) {
	t.Parallel()
	storeResp := pluginstore.PaginatedResponse[pluginstore.PluginVersion]{
		CurrentPage: 1,
		Data: []pluginstore.PluginVersion{
			{
				ID:        3,
				Version:   "1.0.0",
				Changelog: "First Release",
				FileSize:  2870739,
				FileHash:  "c864ca4a5261028ade4f05ab5c63e4945d6affeff3ed9ff0e0bb17ec95bddfa3",
				IsStable:  true,
			},
		},
		Total: 1,
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
				"current_page": 1,
				"data": [{
					"id": 3,
					"version": "1.0.0",
					"changelog": "First Release",
					"file_size": 2870739,
					"file_hash": "c864ca4a5261028ade4f05ab5c63e4945d6affeff3ed9ff0e0bb17ec95bddfa3",
					"sign_url": "",
					"min_gameap_version": "",
					"min_plugin_api_version": "",
					"is_stable": true,
					"screenshots": [],
					"download_count": 0,
					"created_at": "0001-01-01T00:00:00Z"
				}],
				"from": 0,
				"last_page": 0,
				"per_page": 0,
				"total": 1
			}`,
		},
		{
			name:       "not_found",
			statusCode: http.StatusNotFound,
			wantStatus: http.StatusNotFound,
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

			h := getpluginversions.NewHandler(storeService, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/plugins/hexeditor4jm2/versions", nil)

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
