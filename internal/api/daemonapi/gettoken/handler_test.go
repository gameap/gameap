package gettoken

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		setupRepo      func(*inmemory.NodeRepository) *domain.Node
		expectedStatus int
		wantError      string
		expectToken    bool
	}{
		{
			name:       "successful token generation",
			authHeader: "Bearer test-api-key",
			setupRepo: func(nodesRepo *inmemory.NodeRepository) *domain.Node {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))

				return node
			},
			expectedStatus: http.StatusOK,
			expectToken:    true,
		},
		{
			name:           "missing authorization header",
			authHeader:     "",
			setupRepo:      func(_ *inmemory.NodeRepository) *domain.Node { return nil },
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid api key",
			expectToken:    false,
		},
		{
			name:           "empty bearer token",
			authHeader:     "Bearer ",
			setupRepo:      func(_ *inmemory.NodeRepository) *domain.Node { return nil },
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid api key",
			expectToken:    false,
		},
		{
			name:       "invalid api key",
			authHeader: "Bearer invalid-key",
			setupRepo: func(nodesRepo *inmemory.NodeRepository) *domain.Node {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))

				return node
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid api key",
			expectToken:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodesRepo := inmemory.NewNodeRepository()
			responder := api.NewResponder()
			handler := NewHandler(nodesRepo, responder)

			var node *domain.Node
			if tt.setupRepo != nil {
				node = tt.setupRepo(nodesRepo)
			}

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/init", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.expectToken {
				var response tokenResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Token)
				assert.Len(t, response.Token, tokenLength)
				assert.NotZero(t, response.Timestamp)

				// Verify token was saved to node
				require.NotNil(t, node)
				nodes, err := nodesRepo.Find(
					context.Background(),
					nil,
					nil,
					nil,
				)
				require.NoError(t, err)
				require.Len(t, nodes, 1)
				require.NotNil(t, nodes[0].GdaemonAPIToken)
				assert.Equal(t, response.Token, *nodes[0].GdaemonAPIToken)
				assert.NotNil(t, nodes[0].UpdatedAt)
			}
		})
	}
}

func TestHandler_TokenGeneration(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, responder)

	now := time.Now()
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}
	require.NoError(t, nodesRepo.Save(context.Background(), node))

	tokens := make(map[string]bool)

	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/init", nil)
		req.Header.Set("Authorization", "Bearer test-api-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response tokenResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Len(t, response.Token, tokenLength)

		tokens[response.Token] = true
	}

	assert.Len(t, tokens, 10)
}

func TestHandler_NewHandler(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()

	handler := NewHandler(nodesRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodesRepo, handler.nodeRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestNewTokenResponse(t *testing.T) {
	token := "test-token-12345"
	timestamp := int64(1234567890)

	response := newTokenResponse(token, timestamp)

	assert.Equal(t, token, response.Token)
	assert.Equal(t, timestamp, response.Timestamp)
}

func TestHandler_TokenResponseJSON(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, responder)

	now := time.Now()
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}
	require.NoError(t, nodesRepo.Save(context.Background(), node))

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/init", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var rawResponse map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rawResponse))

	token, tokenExists := rawResponse["token"]
	assert.True(t, tokenExists)
	assert.NotEmpty(t, token)

	timestamp, timestampExists := rawResponse["timestamp"]
	assert.True(t, timestampExists)
	assert.NotZero(t, timestamp)
}

func TestHandler_UpdatesNodeTimestamp(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, responder)

	originalTime := time.Now().Add(-1 * time.Hour)
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &originalTime,
		UpdatedAt:           &originalTime,
	}
	require.NoError(t, nodesRepo.Save(context.Background(), node))

	beforeRequest := time.Now()

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/init", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	nodes, err := nodesRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	updatedNode := nodes[0]
	require.NotNil(t, updatedNode.UpdatedAt)
	assert.True(t, updatedNode.UpdatedAt.After(beforeRequest) || updatedNode.UpdatedAt.Equal(beforeRequest))
	assert.True(t, updatedNode.UpdatedAt.After(originalTime))
}
