package pluginstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_GetCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		lang           string
		mockResponse   any
		mockStatusCode int
		cachedValue    any
		wantErr        bool
		errContains    string
		validate       func(t *testing.T, categories []Category)
	}{
		{
			name:           "successful_response_with_categories",
			lang:           "en",
			mockStatusCode: http.StatusOK,
			mockResponse: []Category{
				{
					ID:          1,
					Slug:        "server-management",
					Name:        "Server Management",
					Description: "",
					Icon:        "",
					SortOrder:   1000,
				},
				{
					ID:          2,
					Slug:        "integrations",
					Name:        "Integrations",
					Description: "",
					Icon:        "",
					SortOrder:   3000,
				},
			},
			wantErr: false,
			validate: func(t *testing.T, categories []Category) {
				t.Helper()
				require.Len(t, categories, 2)
				assert.Equal(t, "server-management", categories[0].Slug)
				assert.Equal(t, "Server Management", categories[0].Name)
				assert.Equal(t, "integrations", categories[1].Slug)
			},
		},
		{
			name:           "returns_cached_categories",
			lang:           "ru",
			mockStatusCode: http.StatusOK,
			mockResponse:   []Category{},
			cachedValue: []Category{
				{ID: 1, Slug: "cached", Name: "Cached Category"},
			},
			wantErr: false,
			validate: func(t *testing.T, categories []Category) {
				t.Helper()
				require.Len(t, categories, 1)
				assert.Equal(t, "cached", categories[0].Slug)
			},
		},
		{
			name:           "HTTP_error_status_500",
			lang:           "en",
			mockStatusCode: http.StatusInternalServerError,
			mockResponse:   nil,
			wantErr:        true,
			errContains:    "plugin store API error: HTTP 500",
		},
		{
			name:           "invalid_JSON_response",
			lang:           "en",
			mockStatusCode: http.StatusOK,
			mockResponse:   "invalid json",
			wantErr:        true,
			errContains:    "failed to decode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/categories", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				if tt.lang != "" && tt.cachedValue == nil {
					assert.Equal(t, tt.lang, r.Header.Get("Accept-Language"))
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)

				if tt.mockResponse != nil {
					if str, ok := tt.mockResponse.(string); ok {
						_, _ = w.Write([]byte(str))
					} else {
						_ = json.NewEncoder(w).Encode(tt.mockResponse)
					}
				}
			}))
			defer server.Close()

			testCache := cache.NewInMemory()
			if tt.cachedValue != nil {
				key := "pluginstore:categories:" + tt.lang
				err := testCache.Set(context.Background(), key, tt.cachedValue)
				require.NoError(t, err)
			}

			service := NewService(server.URL, "", testCache)

			categories, err := service.GetCategories(context.Background(), tt.lang)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, categories)
				}
			}
		})
	}
}

func TestService_GetLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		lang           string
		mockResponse   any
		mockStatusCode int
		cachedValue    any
		wantErr        bool
		errContains    string
		validate       func(t *testing.T, labels []Label)
	}{
		{
			name:           "successful_response_with_labels",
			lang:           "en",
			mockStatusCode: http.StatusOK,
			mockResponse: []Label{
				{ID: 1, Slug: "file-editors", Name: "file-editor", Color: "#737373"},
				{ID: 2, Slug: "minecraft", Name: "minecraft", Color: "#3c8527"},
			},
			wantErr: false,
			validate: func(t *testing.T, labels []Label) {
				t.Helper()
				require.Len(t, labels, 2)
				assert.Equal(t, "file-editors", labels[0].Slug)
				assert.Equal(t, "#737373", labels[0].Color)
			},
		},
		{
			name:           "returns_cached_labels",
			lang:           "ru",
			mockStatusCode: http.StatusOK,
			mockResponse:   []Label{},
			cachedValue: []Label{
				{ID: 5, Slug: "cached", Name: "Cached Label", Color: "#000000"},
			},
			wantErr: false,
			validate: func(t *testing.T, labels []Label) {
				t.Helper()
				require.Len(t, labels, 1)
				assert.Equal(t, "cached", labels[0].Slug)
			},
		},
		{
			name:           "HTTP_error_status_500",
			lang:           "en",
			mockStatusCode: http.StatusInternalServerError,
			mockResponse:   nil,
			wantErr:        true,
			errContains:    "plugin store API error: HTTP 500",
		},
		{
			name:           "invalid_JSON_response",
			lang:           "en",
			mockStatusCode: http.StatusOK,
			mockResponse:   "invalid json",
			wantErr:        true,
			errContains:    "failed to decode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/labels", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != nil {
					if str, ok := tt.mockResponse.(string); ok {
						_, _ = w.Write([]byte(str))
					} else {
						_ = json.NewEncoder(w).Encode(tt.mockResponse)
					}
				}
			}))
			defer server.Close()

			testCache := cache.NewInMemory()
			if tt.cachedValue != nil {
				key := "pluginstore:labels:" + tt.lang
				err := testCache.Set(context.Background(), key, tt.cachedValue)
				require.NoError(t, err)
			}

			service := NewService(server.URL, "", testCache)

			labels, err := service.GetLabels(context.Background(), tt.lang)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, labels)
				}
			}
		})
	}
}

func TestService_GetPlugins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		params         GetPluginsParams
		mockResponse   any
		mockStatusCode int
		wantErr        bool
		validateReq    func(t *testing.T, r *http.Request)
		validate       func(t *testing.T, resp *PaginatedResponse[Plugin])
	}{
		{
			name: "successful_response_with_plugins",
			params: GetPluginsParams{
				Page:      1,
				PerPage:   20,
				SortBy:    "download_count",
				SortOrder: "desc",
			},
			mockStatusCode: http.StatusOK,
			mockResponse: PaginatedResponse[Plugin]{
				CurrentPage: 1,
				Data: []Plugin{
					{
						ID:            "hexeditor4jm2",
						Name:          "HEX Editor",
						Summary:       "Hex editor in filemanager",
						DownloadCount: 100,
						LatestVersion: "1.0.0",
					},
				},
				From:     1,
				LastPage: 1,
				PerPage:  20,
				Total:    1,
			},
			wantErr: false,
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "1", r.URL.Query().Get("page"))
				assert.Equal(t, "20", r.URL.Query().Get("per_page"))
				assert.Equal(t, "download_count", r.URL.Query().Get("sort_by"))
				assert.Equal(t, "desc", r.URL.Query().Get("sort_order"))
			},
			validate: func(t *testing.T, resp *PaginatedResponse[Plugin]) {
				t.Helper()
				require.Len(t, resp.Data, 1)
				assert.Equal(t, "hexeditor4jm2", resp.Data[0].ID)
				assert.Equal(t, "HEX Editor", resp.Data[0].Name)
				assert.Equal(t, 1, resp.Total)
			},
		},
		{
			name:           "empty_params",
			params:         GetPluginsParams{},
			mockStatusCode: http.StatusOK,
			mockResponse: PaginatedResponse[Plugin]{
				Data:  []Plugin{},
				Total: 0,
			},
			wantErr: false,
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Empty(t, r.URL.Query().Get("page"))
				assert.Empty(t, r.URL.Query().Get("per_page"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/plugins", r.URL.Path)
				if tt.validateReq != nil {
					tt.validateReq(t, r)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				_ = json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			service := NewService(server.URL, "", cache.NewInMemory())

			resp, err := service.GetPlugins(context.Background(), "en", tt.params)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, resp)
				}
			}
		})
	}
}

func TestService_GetPlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pluginID       string
		mockResponse   any
		mockStatusCode int
		wantErr        bool
		validate       func(t *testing.T, plugin *PluginDetails)
	}{
		{
			name:           "successful_response_with_plugin_details",
			pluginID:       "hexeditor4jm2",
			mockStatusCode: http.StatusOK,
			mockResponse: PluginDetails{
				ID:            "hexeditor4jm2",
				Name:          "HEX Editor",
				Summary:       "Hex editor in filemanager",
				Description:   "Full description here",
				License:       "MIT",
				RepositoryURL: "https://github.com/gameap/plugin-hex-editor",
				Author:        Author{ID: 2, Username: "GameAP"},
				Category:      Category{ID: 3, Slug: "files", Name: "Files"},
				LatestVersion: "1.0.0",
			},
			wantErr: false,
			validate: func(t *testing.T, plugin *PluginDetails) {
				t.Helper()
				assert.Equal(t, "hexeditor4jm2", plugin.ID)
				assert.Equal(t, "HEX Editor", plugin.Name)
				assert.Equal(t, "MIT", plugin.License)
				assert.Equal(t, "GameAP", plugin.Author.Username)
				assert.Equal(t, "files", plugin.Category.Slug)
			},
		},
		{
			name:           "plugin_not_found",
			pluginID:       "nonexistent",
			mockStatusCode: http.StatusNotFound,
			mockResponse:   nil,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/plugins/"+tt.pluginID, r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != nil {
					_ = json.NewEncoder(w).Encode(tt.mockResponse)
				}
			}))
			defer server.Close()

			service := NewService(server.URL, "", cache.NewInMemory())

			plugin, err := service.GetPlugin(context.Background(), tt.pluginID, "en")

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, plugin)
				}
			}
		})
	}
}

func TestService_GetPluginVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pluginID       string
		params         GetPluginVersionsParams
		mockResponse   any
		mockStatusCode int
		wantErr        bool
		validate       func(t *testing.T, resp *PaginatedResponse[PluginVersion])
	}{
		{
			name:           "successful_response_with_versions",
			pluginID:       "hexeditor4jm2",
			params:         GetPluginVersionsParams{Page: 1, PerPage: 10},
			mockStatusCode: http.StatusOK,
			mockResponse: PaginatedResponse[PluginVersion]{
				CurrentPage: 1,
				Data: []PluginVersion{
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
			},
			wantErr: false,
			validate: func(t *testing.T, resp *PaginatedResponse[PluginVersion]) {
				t.Helper()
				require.Len(t, resp.Data, 1)
				assert.Equal(t, "1.0.0", resp.Data[0].Version)
				assert.Equal(t, "First Release", resp.Data[0].Changelog)
				assert.True(t, resp.Data[0].IsStable)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/plugins/"+tt.pluginID+"/versions", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				_ = json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			service := NewService(server.URL, "", cache.NewInMemory())

			resp, err := service.GetPluginVersions(context.Background(), tt.pluginID, tt.params)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, resp)
				}
			}
		})
	}
}

func TestService_DownloadPlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pluginID       string
		version        string
		mockResponse   []byte
		mockStatusCode int
		wantErr        bool
		errContains    string
		wantHTTPStatus int
	}{
		{
			name:           "successful_download",
			pluginID:       "hexeditor4jm2",
			version:        "1.0.0",
			mockResponse:   []byte("fake wasm content"),
			mockStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "download_not_found_with_json_message",
			pluginID:       "nonexistent",
			version:        "1.0.0",
			mockStatusCode: http.StatusNotFound,
			mockResponse:   []byte(`{"message": "Plugin not found", "error": "", "http_code": 404}`),
			wantErr:        true,
			errContains:    "Plugin not found",
			wantHTTPStatus: http.StatusNotFound,
		},
		{
			name:           "download_402_payment_required",
			pluginID:       "paid-plugin",
			version:        "1.0.0",
			mockStatusCode: http.StatusPaymentRequired,
			mockResponse:   []byte(`{"message": "Payment required for this plugin", "error": "", "http_code": 402}`),
			wantErr:        true,
			errContains:    "Payment required for this plugin",
			wantHTTPStatus: http.StatusPaymentRequired,
		},
		{
			name:           "download_403_forbidden",
			pluginID:       "restricted-plugin",
			version:        "1.0.0",
			mockStatusCode: http.StatusForbidden,
			mockResponse:   []byte(`{"message": "", "error": "Access denied", "http_code": 403}`),
			wantErr:        true,
			errContains:    "Access denied",
			wantHTTPStatus: http.StatusForbidden,
		},
		{
			name:           "download_4XX_with_empty_body",
			pluginID:       "some-plugin",
			version:        "1.0.0",
			mockStatusCode: http.StatusBadRequest,
			mockResponse:   nil,
			wantErr:        true,
			errContains:    "plugin store API error: HTTP 400",
			wantHTTPStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := "/plugins/" + tt.pluginID + "/versions/" + tt.version + "/download"
				assert.Equal(t, expectedPath, r.URL.Path)

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != nil {
					_, _ = w.Write(tt.mockResponse)
				}
			}))
			defer server.Close()

			service := NewService(server.URL, "", nil)

			data, err := service.DownloadPlugin(context.Background(), tt.pluginID, tt.version)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				if tt.wantHTTPStatus != 0 {
					var apiErr *APIError
					require.True(t, errors.As(err, &apiErr), "error should be *APIError")
					assert.Equal(t, tt.wantHTTPStatus, apiErr.HTTPStatus())
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.mockResponse, data)
			}
		})
	}
}

func TestVerifyHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		data         []byte
		expectedHash string
		want         bool
	}{
		{
			name:         "valid_hash",
			data:         []byte("test data"),
			expectedHash: "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9",
			want:         true,
		},
		{
			name:         "invalid_hash",
			data:         []byte("test data"),
			expectedHash: "wronghash",
			want:         false,
		},
		{
			name:         "empty_data",
			data:         []byte{},
			expectedHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := VerifyHash(tt.data, tt.expectedHash)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestExtractLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		queryParam string
		header     string
		want       string
	}{
		{
			name:       "query_param_takes_precedence",
			queryParam: "ru",
			header:     "en",
			want:       "ru",
		},
		{
			name:       "header_fallback",
			queryParam: "",
			header:     "de",
			want:       "de",
		},
		{
			name:       "empty_when_both_missing",
			queryParam: "",
			header:     "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			url := "/test"
			if tt.queryParam != "" {
				url += "?lang=" + tt.queryParam
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.header != "" {
				req.Header.Set("Accept-Language", tt.header)
			}

			result := ExtractLanguage(req)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestService_CacheExpiration(t *testing.T) {
	t.Parallel()

	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]Category{{ID: callCount, Slug: "test"}})
	}))
	defer server.Close()

	testCache := cache.NewInMemory()
	service := NewService(server.URL, "", testCache)

	// First call should hit the API
	categories1, err := service.GetCategories(context.Background(), "en")
	require.NoError(t, err)
	assert.Equal(t, 1, categories1[0].ID)
	assert.Equal(t, 1, callCount)

	// Second call should use cache
	categories2, err := service.GetCategories(context.Background(), "en")
	require.NoError(t, err)
	assert.Equal(t, 1, categories2[0].ID)
	assert.Equal(t, 1, callCount)
}

func TestService_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	service := NewService(server.URL, "", nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.GetCategories(ctx, "en")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestService_WithNilCache(t *testing.T) {
	t.Parallel()

	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]Category{{ID: 1, Slug: "test"}})
	}))
	defer server.Close()

	service := NewService(server.URL, "", nil)

	// First call
	_, err := service.GetCategories(context.Background(), "en")
	require.NoError(t, err)

	// Second call - should still hit API since cache is nil
	_, err = service.GetCategories(context.Background(), "en")
	require.NoError(t, err)

	assert.Equal(t, 2, callCount)
}

func TestService_LanguageAwareCaching(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]Category{{ID: 1, Slug: "test", Name: "Name-" + lang}})
	}))
	defer server.Close()

	testCache := cache.NewInMemory()
	service := NewService(server.URL, "", testCache)

	categoriesEN, err := service.GetCategories(context.Background(), "en")
	require.NoError(t, err)
	assert.Equal(t, "Name-en", categoriesEN[0].Name)

	categoriesRU, err := service.GetCategories(context.Background(), "ru")
	require.NoError(t, err)
	assert.Equal(t, "Name-ru", categoriesRU[0].Name)

	// Verify EN is still cached correctly
	categoriesEN2, err := service.GetCategories(context.Background(), "en")
	require.NoError(t, err)
	assert.Equal(t, "Name-en", categoriesEN2[0].Name)
}

func TestService_BaseURL(t *testing.T) {
	t.Parallel()

	service := NewService("https://custom.api.com", "", nil)
	assert.Equal(t, "https://custom.api.com", service.BaseURL())
}

func TestService_DefaultBaseURL(t *testing.T) {
	t.Parallel()

	service := NewService("", "", nil)
	assert.Equal(t, defaultBaseURL, service.BaseURL())
}

func TestNewService(t *testing.T) {
	t.Parallel()

	testCache := cache.NewInMemory()
	service := NewService("https://test.com", "", testCache)

	require.NotNil(t, service)
	assert.Equal(t, "https://test.com", service.baseURL)
	assert.NotNil(t, service.httpClient)
	assert.Equal(t, 30*time.Second, service.httpClient.Timeout)
}

func TestAPIError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statusCode     int
		message        string
		wantError      string
		wantHTTPStatus int
	}{
		{
			name:           "error_with_message",
			statusCode:     http.StatusPaymentRequired,
			message:        "Payment required",
			wantError:      "Payment required",
			wantHTTPStatus: http.StatusPaymentRequired,
		},
		{
			name:           "error_without_message",
			statusCode:     http.StatusForbidden,
			message:        "",
			wantError:      "plugin store API error: HTTP 403",
			wantHTTPStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			apiErr := &APIError{
				StatusCode: tt.statusCode,
				Message:    tt.message,
			}

			assert.Equal(t, tt.wantError, apiErr.Error())
			assert.Equal(t, tt.wantHTTPStatus, apiErr.HTTPStatus())
		})
	}
}
