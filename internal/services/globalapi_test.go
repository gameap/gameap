package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalAPIService_SendBug(t *testing.T) {
	tests := []struct {
		name           string
		report         BugReport
		mockStatusCode int
		wantErr        bool
		errContains    string
		validateReq    func(t *testing.T, r *http.Request)
	}{
		{
			name: "successful bug report",
			report: BugReport{
				Version:     "1.0.0",
				Summary:     "Test bug",
				Description: "This is a test bug",
				Environment: "Test environment\n",
			},
			mockStatusCode: http.StatusCreated,
			wantErr:        false,
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()

				assert.Equal(t, "/bugs", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var payload map[string]string
				err := json.NewDecoder(r.Body).Decode(&payload)
				require.NoError(t, err)

				assert.Equal(t, "1.0.0", payload["version"])
				assert.Equal(t, "Test bug", payload["summary"])
				assert.Equal(t, "This is a test bug", payload["description"])
				assert.Contains(t, payload["environment"], "Test environment")
				assert.Contains(t, payload["environment"], "Go version:")
				assert.Contains(t, payload["environment"], "OS/Arch:")
			},
		},
		{
			name: "HTTP error status",
			report: BugReport{
				Version:     "1.0.0",
				Summary:     "Test bug",
				Description: "This is a test bug",
			},
			mockStatusCode: http.StatusBadRequest,
			wantErr:        true,
			errContains:    "unexpected HTTP status code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.validateReq != nil {
					tt.validateReq(t, r)
				}

				w.WriteHeader(tt.mockStatusCode)
			}))
			defer server.Close()

			// Create service with test server URL
			cfg := &config.Config{}
			cfg.GlobalAPI.URL = server.URL

			service := NewGlobalAPIService(cfg)

			// Execute test
			err := service.SendBug(context.Background(), tt.report)

			// Validate results
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
