package getcategories_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/pluginstore/getcategories"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCategories(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mockServer func() *httptest.Server
		wantStatus int
		wantBody   string
	}{
		{
			name: "successful_response",
			mockServer: func() *httptest.Server {
				return mockAPIServer(
					t,
					http.StatusOK,
					string(lo.Must(json.Marshal([]pluginstore.Category{
						{ID: 1, Slug: "server-management", Name: "Server Management", SortOrder: 1000},
						{ID: 2, Slug: "files", Name: "Files", Description: "File management", SortOrder: 2000},
					}))),
				)
			},
			wantStatus: http.StatusOK,
			wantBody: `[
				{"id":1,"slug":"server-management","name":"Server Management","description":"","sort_order":1000},
				{"id":2,"slug":"files","name":"Files","description":"File management","sort_order":2000}
			]`,
		},
		{
			name: "empty_categories",
			mockServer: func() *httptest.Server {
				return mockAPIServer(
					t,
					http.StatusOK,
					"[]",
				)
			},
			wantStatus: http.StatusOK,
			wantBody:   `[]`,
		},
		{
			name: "store_error",
			mockServer: func() *httptest.Server {
				return mockAPIServer(
					t,
					http.StatusInternalServerError,
					"",
				)
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockServer := tt.mockServer()
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())

			h := getcategories.NewHandler(storeService, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/categories", nil)
			req.Header.Set("Accept-Language", "en")

			h.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)

			if tt.wantStatus == http.StatusOK {
				var resp []map[string]any
				err := json.Unmarshal(recorder.Body.Bytes(), &resp)
				require.NoError(t, err)

				if tt.wantBody != "" {
					assert.JSONEq(t, tt.wantBody, recorder.Body.String())
				}
			}
		})
	}
}

func TestGetCategories_with_language_query_param(t *testing.T) {
	t.Parallel()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "ru", r.Header.Get("Accept-Language"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]pluginstore.Category{
			{ID: 1, Slug: "test", Name: "Тест"},
		})
	}))
	defer mockServer.Close()

	storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())

	h := getcategories.NewHandler(storeService, api.NewResponder())
	recorder := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/categories?lang=ru", nil)
	req.Header.Set("Accept-Language", "en")

	h.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func mockAPIServer(t *testing.T, statusCode int, result string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(statusCode)

		if result != "" {
			_, _ = w.Write([]byte(result))
		}
	}))
}
