// OWASP API Top 10:2023 — API2:2023 Broken Authentication.
// The daemon authenticates with a long-lived API key that is stored only as
// a SHA-256 digest. These tests assert the handler matches on the digest, so
// a legacy plaintext key sitting in the database cannot authenticate and the
// issued token is itself persisted hashed (never the plaintext).
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
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConnectionChecker struct {
	connected bool
}

func (f *fakeConnectionChecker) IsConnectedAnywhere(_ uint64) bool {
	return f.connected
}

// TestHandler_ServeHTTP — OWASP API Top 10:2023 API2:2023 Broken
// Authentication. Includes a case proving a legacy plaintext API key at rest
// cannot authenticate because the handler matches on the SHA-256 digest.
func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		setupRepo      func(*inmemory.NodeRepository) *domain.Node
		grpcConnected  bool
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
					GdaemonAPIKey:       pkgstrings.SHA256("test-api-key"),
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
					GdaemonAPIKey:       pkgstrings.SHA256("test-api-key"),
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
		{
			name:       "legacy_plaintext_key_at_rest_is_rejected",
			authHeader: "Bearer test-api-key",
			setupRepo: func(nodesRepo *inmemory.NodeRepository) *domain.Node {
				now := time.Now()
				node := &domain.Node{
					ID:          1,
					Enabled:     true,
					Name:        "test-node",
					OS:          "linux",
					Location:    "Montenegro",
					IPs:         []string{"172.18.0.5"},
					WorkPath:    "/srv/gameap",
					GdaemonHost: "172.18.0.5",
					GdaemonPort: 31717,
					// Stored as PLAINTEXT (legacy, pre hash-at-rest migration),
					// not pkgstrings.SHA256("test-api-key").
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
		{
			name:       "daemon_connected_via_grpc_returns_conflict",
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
					GdaemonAPIKey:       pkgstrings.SHA256("test-api-key"),
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))

				return node
			},
			grpcConnected:  true,
			expectedStatus: http.StatusConflict,
			wantError:      "HTTP API is disabled",
			expectToken:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodesRepo := inmemory.NewNodeRepository()
			responder := api.NewResponder()
			connChecker := &fakeConnectionChecker{connected: tt.grpcConnected}
			handler := NewHandler(nodesRepo, connChecker, responder)

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
				assert.NotEqual(t, response.Token, *nodes[0].GdaemonAPIToken,
					"persisted token must not be the plaintext returned to the daemon")
				assert.Equal(t, pkgstrings.SHA256(response.Token), *nodes[0].GdaemonAPIToken,
					"persisted token must equal SHA-256 of the plaintext")
				assert.NotNil(t, nodes[0].UpdatedAt)
			}
		})
	}
}

func TestHandler_TokenGeneration(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, responder)

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
		GdaemonAPIKey:       pkgstrings.SHA256("test-api-key"),
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
	connChecker := &fakeConnectionChecker{}

	handler := NewHandler(nodesRepo, connChecker, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodesRepo, handler.nodeRepo)
	assert.Equal(t, connChecker, handler.connChecker)
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
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, responder)

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
		GdaemonAPIKey:       pkgstrings.SHA256("test-api-key"),
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
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, responder)

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
		GdaemonAPIKey:       pkgstrings.SHA256("test-api-key"),
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
