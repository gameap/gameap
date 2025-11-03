package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalAPIService_Games(t *testing.T) {
	tests := []struct {
		name           string
		mockResponse   any
		mockStatusCode int
		wantErr        bool
		errContains    string
		validate       func(t *testing.T, games []domain.GlobalAPIGame)
	}{
		{
			name:           "successful response",
			mockStatusCode: http.StatusOK,
			mockResponse: domain.GlobalAPIResponse[[]domain.GlobalAPIGame]{
				Data: []domain.GlobalAPIGame{
					{
						Code:              "cstrike",
						StartCode:         "cstrike",
						Name:              "Counter-Strike 1.6",
						Engine:            "GoldSource",
						EngineVersion:     "1",
						SteamAppIDLinux:   90,
						SteamAppIDWindows: 0,
						Mods: []domain.GlobalAPIGameMod{
							{
								ID:       3,
								GameCode: "cstrike",
								Name:     "Classic (Standart)",
							},
						},
					},
				},
				Message: "Games retrieved successfully",
				Success: true,
			},
			wantErr: false,
			validate: func(t *testing.T, games []domain.GlobalAPIGame) {
				t.Helper()

				require.Len(t, games, 1)
				assert.Equal(t, "cstrike", games[0].Code)
				assert.Equal(t, "Counter-Strike 1.6", games[0].Name)
				assert.Len(t, games[0].Mods, 1)
				assert.Equal(t, "Classic (Standart)", games[0].Mods[0].Name)
			},
		},
		{
			name:           "API error - success=false",
			mockStatusCode: http.StatusOK,
			mockResponse: domain.GlobalAPIResponse[[]domain.GlobalAPIGame]{
				Data:    nil,
				Message: "Internal server error",
				Success: false,
			},
			wantErr:     true,
			errContains: "API error",
		},
		{
			name:           "HTTP error status",
			mockStatusCode: http.StatusInternalServerError,
			mockResponse:   nil,
			wantErr:        true,
			errContains:    "unexpected HTTP status code",
		},
		{
			name:           "invalid JSON response",
			mockStatusCode: http.StatusOK,
			mockResponse:   "invalid json",
			wantErr:        true,
			errContains:    "failed to decode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/games", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Accept"))

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

			// Create service with test server URL
			cfg := &config.Config{}
			cfg.GlobalAPI.URL = server.URL

			service := NewGlobalAPIService(cfg)

			// Execute test
			games, err := service.Games(context.Background())

			// Validate results
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, games)
				}
			}
		})
	}
}

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

func TestGlobalAPIService_Games_ContextCancellation(t *testing.T) {
	// Create a server that delays the response
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	cfg := &config.Config{}
	cfg.GlobalAPI.URL = server.URL

	service := NewGlobalAPIService(cfg)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Execute test
	_, err := service.Games(ctx)

	// Should fail with context error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}
