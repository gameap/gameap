package pluginstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errTestCacheSet = errors.New("simulated cache set failure")
	errTestClose    = errors.New("simulated body close failure")
)

func TestService_HasLicenseKey(t *testing.T) {
	tests := []struct {
		name       string
		licenseKey string
		want       bool
	}{
		{name: "with_license_key", licenseKey: "secret-key", want: true},
		{name: "without_license_key", licenseKey: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			service := NewService("https://example.com", tt.licenseKey, nil)

			// ACT & ASSERT
			assert.Equal(t, tt.want, service.HasLicenseKey())
		})
	}
}

func TestService_ValidateLicense(t *testing.T) {
	t.Run("no_license_key_returns_nil_without_http_call", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		service := NewService(server.URL, "", cache.NewInMemory())

		// ACT
		validation, err := service.ValidateLicense(context.Background())

		// ASSERT
		require.NoError(t, err)
		assert.Nil(t, validation, "without a license key there is nothing to validate")
		assert.Equal(t, int32(0), callCount.Load(), "no HTTP call must be made without a license key")
	})

	t.Run("validates_license_and_caches_result", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount.Add(1)
			assert.Equal(t, "/licenses/validate", r.URL.Path)
			assert.Equal(t, "test-license-key", r.Header.Get("X-License-Key"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(LicenseValidation{
				Valid:   true,
				Message: "license is active",
			})
		}))
		defer server.Close()

		service := NewService(server.URL, "test-license-key", cache.NewInMemory())

		// ACT
		validation, err := service.ValidateLicense(context.Background())
		require.NoError(t, err)

		cachedValidation, cachedErr := service.ValidateLicense(context.Background())

		// ASSERT
		require.NoError(t, cachedErr)
		require.NotNil(t, validation)
		assert.True(t, validation.Valid)
		assert.Equal(t, "license is active", validation.Message)
		require.NotNil(t, cachedValidation)
		assert.True(t, cachedValidation.Valid)
		assert.Equal(t, int32(1), callCount.Load(), "the second call must be served from cache")
	})

	t.Run("returns_cached_validation", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		testCache := cache.NewInMemory()
		require.NoError(t, testCache.Set(
			context.Background(),
			"pluginstore:license",
			&LicenseValidation{Valid: true, Message: "cached"},
		))

		service := NewService(server.URL, "test-license-key", testCache)

		// ACT
		validation, err := service.ValidateLicense(context.Background())

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, validation)
		assert.True(t, validation.Valid)
		assert.Equal(t, "cached", validation.Message)
		assert.Equal(t, int32(0), callCount.Load(), "cached validation must not trigger an HTTP call")
	})

	t.Run("api_error_is_returned", func(t *testing.T) {
		// ARRANGE
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "Invalid license key", "error": "", "http_code": 403}`))
		}))
		defer server.Close()

		service := NewService(server.URL, "bad-key", cache.NewInMemory())

		// ACT
		validation, err := service.ValidateLicense(context.Background())

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid license key")
		assert.Nil(t, validation)
	})
}

func TestService_GetPluginIcon(t *testing.T) {
	t.Run("downloads_icon_and_caches_it", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount.Add(1)
			assert.Equal(t, "/plugins/hexeditor4jm2/icon", r.URL.Path)

			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fake-png-bytes"))
		}))
		defer server.Close()

		service := NewService(server.URL, "", cache.NewInMemory())

		// ACT
		icon, err := service.GetPluginIcon(context.Background(), "hexeditor4jm2")
		require.NoError(t, err)

		cachedIcon, cachedErr := service.GetPluginIcon(context.Background(), "hexeditor4jm2")

		// ASSERT
		require.NoError(t, cachedErr)
		require.NotNil(t, icon)
		assert.Equal(t, []byte("fake-png-bytes"), icon.Data)
		assert.Equal(t, "image/png", icon.ContentType)
		require.NotNil(t, cachedIcon)
		assert.Equal(t, []byte("fake-png-bytes"), cachedIcon.Data)
		assert.Equal(t, int32(1), callCount.Load(), "the second call must be served from cache")
	})

	t.Run("returns_cached_icon", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		testCache := cache.NewInMemory()
		require.NoError(t, testCache.Set(
			context.Background(),
			"pluginstore:plugin:hexeditor4jm2:icon",
			PluginIcon{Data: []byte("cached-icon"), ContentType: "image/jpeg"},
		))

		service := NewService(server.URL, "", testCache)

		// ACT
		icon, err := service.GetPluginIcon(context.Background(), "hexeditor4jm2")

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, icon)
		assert.Equal(t, []byte("cached-icon"), icon.Data)
		assert.Equal(t, "image/jpeg", icon.ContentType)
		assert.Equal(t, int32(0), callCount.Load(), "cached icon must not trigger an HTTP call")
	})

	t.Run("http_error_status_404", func(t *testing.T) {
		// ARRANGE
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		service := NewService(server.URL, "", cache.NewInMemory())

		// ACT
		icon, err := service.GetPluginIcon(context.Background(), "nonexistent")

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin store API error: HTTP 404")
		assert.Nil(t, icon)
	})

	t.Run("works_without_cache", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount.Add(1)
			w.Header().Set("Content-Type", "image/svg+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<svg></svg>"))
		}))
		defer server.Close()

		service := NewService(server.URL, "", nil)

		// ACT
		icon, err := service.GetPluginIcon(context.Background(), "editor")
		require.NoError(t, err)
		_, secondErr := service.GetPluginIcon(context.Background(), "editor")

		// ASSERT
		require.NoError(t, secondErr)
		require.NotNil(t, icon)
		assert.Equal(t, []byte("<svg></svg>"), icon.Data)
		assert.Equal(t, int32(2), callCount.Load(), "without cache every call must hit the API")
	})
}

func TestService_DownloadPlugin_LicenseKeyHeader(t *testing.T) {
	// ARRANGE
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-license-key", r.Header.Get("X-License-Key"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("wasm-bytes"))
	}))
	defer server.Close()

	service := NewService(server.URL, "test-license-key", nil)

	// ACT
	data, err := service.DownloadPlugin(context.Background(), "hexeditor4jm2", "1.0.0")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, []byte("wasm-bytes"), data)
}

func TestService_GetPlugins_CacheAndFilters(t *testing.T) {
	t.Run("returns_cached_plugins", func(t *testing.T) {
		// ARRANGE
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		testCache := cache.NewInMemory()
		require.NoError(t, testCache.Set(
			context.Background(),
			"pluginstore:plugins:en:page=1",
			&PaginatedResponse[Plugin]{
				Data:  []Plugin{{ID: "cached-plugin", Name: "Cached"}},
				Total: 1,
			},
		))

		service := NewService(server.URL, "", testCache)

		// ACT
		resp, err := service.GetPlugins(context.Background(), "en", GetPluginsParams{Page: 1})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, "cached-plugin", resp.Data[0].ID)
		assert.Equal(t, int32(0), callCount.Load(), "cached plugins must not trigger an HTTP call")
	})

	t.Run("sends_category_and_label_filters", func(t *testing.T) {
		// ARRANGE
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "files", r.URL.Query().Get("category"))
			assert.Equal(t, "minecraft", r.URL.Query().Get("label"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(PaginatedResponse[Plugin]{Data: []Plugin{}})
		}))
		defer server.Close()

		service := NewService(server.URL, "", cache.NewInMemory())

		// ACT
		resp, err := service.GetPlugins(context.Background(), "en", GetPluginsParams{
			Category: "files",
			Label:    "minecraft",
		})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("http_error_status_500", func(t *testing.T) {
		// ARRANGE
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		service := NewService(server.URL, "", cache.NewInMemory())

		// ACT
		resp, err := service.GetPlugins(context.Background(), "en", GetPluginsParams{})

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin store API error: HTTP 500")
		assert.Nil(t, resp)
	})
}

func TestService_GetPlugin_Cached(t *testing.T) {
	// ARRANGE
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testCache := cache.NewInMemory()
	require.NoError(t, testCache.Set(
		context.Background(),
		"pluginstore:plugin:hexeditor4jm2:en",
		&PluginDetails{ID: "hexeditor4jm2", Name: "Cached Details"},
	))

	service := NewService(server.URL, "", testCache)

	// ACT
	plugin, err := service.GetPlugin(context.Background(), "hexeditor4jm2", "en")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, plugin)
	assert.Equal(t, "Cached Details", plugin.Name)
	assert.Equal(t, int32(0), callCount.Load(), "cached plugin details must not trigger an HTTP call")
}

func TestService_GetPluginVersions_Cached(t *testing.T) {
	// ARRANGE
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testCache := cache.NewInMemory()
	require.NoError(t, testCache.Set(
		context.Background(),
		"pluginstore:plugin:hex:versions:page=2",
		&PaginatedResponse[PluginVersion]{
			Data:  []PluginVersion{{ID: 7, Version: "2.0.0"}},
			Total: 1,
		},
	))

	service := NewService(server.URL, "", testCache)

	// ACT
	resp, err := service.GetPluginVersions(context.Background(), "hex", GetPluginVersionsParams{Page: 2})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "2.0.0", resp.Data[0].Version)
	assert.Equal(t, int32(0), callCount.Load(), "cached versions must not trigger an HTTP call")
}

func TestService_InvalidBaseURL(t *testing.T) {
	tests := []struct {
		name        string
		call        func(s *Service) error
		wantMessage string
	}{
		{
			name: "fetch_json_request_creation",
			call: func(s *Service) error {
				_, err := s.GetCategories(context.Background(), "en")

				return err
			},
			wantMessage: "failed to create request",
		},
		{
			name: "download_request_creation",
			call: func(s *Service) error {
				_, err := s.DownloadPlugin(context.Background(), "plugin", "1.0.0")

				return err
			},
			wantMessage: "failed to create download request",
		},
		{
			name: "icon_request_creation",
			call: func(s *Service) error {
				_, err := s.GetPluginIcon(context.Background(), "plugin")

				return err
			},
			wantMessage: "failed to create icon request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE — a space in the host makes url.Parse reject the request URL.
			service := NewService("http://invalid host", "", nil)

			// ACT
			err := tt.call(service)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

// failingSetCache wraps a real cache and fails only Set.
type failingSetCache struct {
	cache.Cache

	setErr error
}

func (c *failingSetCache) Set(_ context.Context, _ string, _ any, _ ...cache.Option) error {
	return c.setErr
}

func TestService_SetCache_CacheErrorIsLogged(t *testing.T) {
	// ARRANGE
	service := NewService("", "", &failingSetCache{
		Cache:  cache.NewInMemory(),
		setErr: errTestCacheSet,
	})

	// ACT & ASSERT
	assert.NotPanics(t, func() {
		service.setCache(context.Background(), "key", "value")
	}, "a cache write error must only be logged, never propagated")
}

type errorCloser struct {
	io.Reader
}

func (errorCloser) Close() error {
	return errTestClose
}

func TestCloseBody_CloseErrorIsLogged(t *testing.T) {
	// ACT & ASSERT
	assert.NotPanics(t, func() {
		closeBody(errorCloser{Reader: io.LimitReader(nil, 0)})
	}, "a body close error must only be logged")
}
