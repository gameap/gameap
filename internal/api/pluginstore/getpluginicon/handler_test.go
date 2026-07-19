package getpluginicon_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/api/pluginstore/getpluginicon"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pngData = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01, 0x02, 0x03}

func TestGetPluginIcon(t *testing.T) {
	tests := []struct {
		name            string
		upstreamStatus  int
		upstreamType    string
		upstreamBody    []byte
		wantStatus      int
		wantContentType string
		wantDisposition string
	}{
		{
			name:            "png_icon_served_inline",
			upstreamStatus:  http.StatusOK,
			upstreamType:    "image/png",
			upstreamBody:    pngData,
			wantStatus:      http.StatusOK,
			wantContentType: "image/png",
			wantDisposition: "inline",
		},
		{
			name:            "webp_icon_served_inline",
			upstreamStatus:  http.StatusOK,
			upstreamType:    "image/webp",
			upstreamBody:    []byte("RIFF-webp-bytes"),
			wantStatus:      http.StatusOK,
			wantContentType: "image/webp",
			wantDisposition: "inline",
		},
		{
			name:            "svg_forced_to_attachment",
			upstreamStatus:  http.StatusOK,
			upstreamType:    "image/svg+xml",
			upstreamBody:    []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"),
			wantStatus:      http.StatusOK,
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:            "non_image_content_forced_to_attachment",
			upstreamStatus:  http.StatusOK,
			upstreamType:    "text/html",
			upstreamBody:    []byte("<html></html>"),
			wantStatus:      http.StatusOK,
			wantContentType: "application/octet-stream",
			wantDisposition: "attachment",
		},
		{
			name:           "upstream_not_found",
			upstreamStatus: http.StatusNotFound,
			wantStatus:     http.StatusNotFound,
		},
		{
			name:           "upstream_error",
			upstreamStatus: http.StatusInternalServerError,
			wantStatus:     http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/plugins/hexeditor4jm2/icon", r.URL.Path)
				if tt.upstreamType != "" {
					w.Header().Set("Content-Type", tt.upstreamType)
				}
				w.WriteHeader(tt.upstreamStatus)
				_, _ = w.Write(tt.upstreamBody)
			}))
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
			h := getpluginicon.NewHandler(storeService, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/plugin-store/plugins/hexeditor4jm2/icon", nil)
			req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})

			h.ServeHTTP(recorder, req)

			require.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			assert.Equal(t, tt.upstreamBody, recorder.Body.Bytes())
			assert.Equal(t, tt.wantContentType, recorder.Header().Get("Content-Type"))
			assert.True(
				t,
				strings.HasPrefix(recorder.Header().Get("Content-Disposition"), tt.wantDisposition),
				"unexpected Content-Disposition: %s", recorder.Header().Get("Content-Disposition"),
			)
			assert.Contains(t, recorder.Header().Get("Content-Disposition"), "hexeditor4jm2-icon")
			assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, "sandbox", recorder.Header().Get("Content-Security-Policy"))
			assert.Equal(t, "private, max-age=3600", recorder.Header().Get("Cache-Control"))
		})
	}
}

func TestGetPluginIcon_cache_hit_avoids_second_upstream_call(t *testing.T) {
	// ARRANGE
	var iconCalls atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		iconCalls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngData)
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())
	h := getpluginicon.NewHandler(storeService, api.NewResponder())

	doRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/plugin-store/plugins/hexeditor4jm2/icon", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "hexeditor4jm2"})
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, req)

		return recorder
	}

	// ACT
	first := doRequest()
	second := doRequest()

	// ASSERT
	require.Equal(t, http.StatusOK, first.Code, "first request must succeed")
	require.Equal(t, http.StatusOK, second.Code, "second request must succeed")
	assert.Equal(t, int32(1), iconCalls.Load(), "second request must be served from cache, upstream hit only once")
	assert.Equal(t, pngData, second.Body.Bytes(), "cached icon bytes must match the original")
	assert.Equal(t, "image/png", second.Header().Get("Content-Type"), "cached content type must match the original")
}
