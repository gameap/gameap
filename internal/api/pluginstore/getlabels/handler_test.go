package getlabels_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/pluginstore/getlabels"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLabels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		storeResp  []pluginstore.Label
		statusCode int
		wantStatus int
		wantBody   string
	}{
		{
			name:       "successful_response",
			statusCode: http.StatusOK,
			storeResp: []pluginstore.Label{
				{ID: 1, Slug: "file-editors", Name: "file-editor", Color: "#737373"},
				{ID: 2, Slug: "minecraft", Name: "minecraft", Color: "#3c8527"},
			},
			wantStatus: http.StatusOK,
			wantBody: `[
				{"id":1,"slug":"file-editors","name":"file-editor","color":"#737373"},
				{"id":2,"slug":"minecraft","name":"minecraft","color":"#3c8527"}
			]`,
		},
		{
			name:       "empty_labels",
			statusCode: http.StatusOK,
			storeResp:  []pluginstore.Label{},
			wantStatus: http.StatusOK,
			wantBody:   `[]`,
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
				if tt.storeResp != nil {
					_ = json.NewEncoder(w).Encode(tt.storeResp)
				}
			}))
			defer mockServer.Close()

			storeService := pluginstore.NewService(mockServer.URL, "", cache.NewInMemory())

			h := getlabels.NewHandler(storeService, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/admin/plugins/store/labels", nil)

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
