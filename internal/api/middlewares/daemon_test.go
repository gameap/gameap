package middlewares

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
	"github.com/gameap/gameap/pkg/auth"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonAuthMiddleware_Middleware(t *testing.T) {
	// Setup test node
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()
	validToken := "valid-test-token-123"
	invalidToken := "invalid-token-456"

	testNode := &domain.Node{
		ID:              1,
		Enabled:         true,
		Name:            "Test Node",
		OS:              "linux",
		Location:        "us-east-1",
		GdaemonHost:     "localhost",
		GdaemonPort:     8080,
		GdaemonAPIKey:   "test-key",
		GdaemonAPIToken: lo.ToPtr(validToken),
		WorkPath:        "/var/gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_ = nodeRepo.Save(context.Background(), testNode)

	tests := []struct {
		name           string
		authToken      string
		expectedStatus int
		expectNode     bool
		wantError      string
	}{
		{
			name:           "valid token in X-Auth-Token header",
			authToken:      validToken,
			expectedStatus: http.StatusOK,
			expectNode:     true,
		},
		{
			name:           "missing X-Auth-Token header",
			authToken:      "",
			expectedStatus: http.StatusUnauthorized,
			expectNode:     false,
			wantError:      "token not set",
		},
		{
			name:           "invalid token",
			authToken:      invalidToken,
			expectedStatus: http.StatusUnauthorized,
			expectNode:     false,
			wantError:      "invalid api token",
		},
		{
			name:           "non-existent token",
			authToken:      "non-existent-token",
			expectedStatus: http.StatusUnauthorized,
			expectNode:     false,
			wantError:      "invalid api token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup middleware
			responder := api.NewResponder()
			daemonMiddleware := NewDaemonAuthMiddleware(nodeRepo, responder)

			var daemonSession *auth.DaemonSession

			// Create test handler that will be protected
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				daemonSession = auth.DaemonSessionFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			// Apply middleware
			protectedHandler := daemonMiddleware.Middleware(testHandler)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authToken != "" {
				req.Header.Set("X-Auth-Token", tt.authToken)
			}

			// Execute request
			w := httptest.NewRecorder()
			protectedHandler.ServeHTTP(w, req)

			// Assert status
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectNode {
				require.NotNil(t, daemonSession)
				require.NotNil(t, daemonSession.Node)
				assert.Equal(t, testNode.ID, daemonSession.Node.ID)
				assert.Equal(t, testNode.Name, daemonSession.Node.Name)
				assert.Equal(t, testNode.GdaemonHost, daemonSession.Node.GdaemonHost)
			} else {
				assert.Nil(t, daemonSession)
			}

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				assert.Contains(t, response["error"], tt.wantError)
			}
		})
	}
}

func TestDaemonAuthMiddleware_MultipleNodes(t *testing.T) {
	// Setup multiple nodes with different tokens
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()

	node1Token := "token-node-1"
	node2Token := "token-node-2"

	node1 := &domain.Node{
		ID:              1,
		Name:            "Node 1",
		OS:              "linux",
		GdaemonHost:     "node1.example.com",
		GdaemonPort:     8080,
		GdaemonAPIToken: lo.ToPtr(node1Token),
		WorkPath:        "/var/gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_ = nodeRepo.Save(context.Background(), node1)

	node2 := &domain.Node{
		ID:              2,
		Name:            "Node 2",
		OS:              "windows",
		GdaemonHost:     "node2.example.com",
		GdaemonPort:     8081,
		GdaemonAPIToken: lo.ToPtr(node2Token),
		WorkPath:        "C:\\gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_ = nodeRepo.Save(context.Background(), node2)

	responder := api.NewResponder()
	daemonMiddleware := NewDaemonAuthMiddleware(nodeRepo, responder)

	tests := []struct {
		name         string
		token        string
		expectedNode *domain.Node
	}{
		{
			name:         "authenticate as node 1",
			token:        node1Token,
			expectedNode: node1,
		},
		{
			name:         "authenticate as node 2",
			token:        node2Token,
			expectedNode: node2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var daemonSession *auth.DaemonSession

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				daemonSession = auth.DaemonSessionFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			protectedHandler := daemonMiddleware.Middleware(testHandler)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("X-Auth-Token", tt.token)

			w := httptest.NewRecorder()
			protectedHandler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			require.NotNil(t, daemonSession)
			require.NotNil(t, daemonSession.Node)
			assert.Equal(t, tt.expectedNode.ID, daemonSession.Node.ID)
			assert.Equal(t, tt.expectedNode.Name, daemonSession.Node.Name)
			assert.Equal(t, tt.expectedNode.GdaemonHost, daemonSession.Node.GdaemonHost)
		})
	}
}

func TestDaemonAuthMiddleware_NodeWithNullToken(t *testing.T) {
	// Setup node with null GDaemonAPIToken
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()

	nodeWithNullToken := &domain.Node{
		ID:              1,
		Name:            "Node Without Token",
		OS:              "linux",
		GdaemonHost:     "localhost",
		GdaemonPort:     8080,
		GdaemonAPIToken: nil, // Null token
		WorkPath:        "/var/gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_ = nodeRepo.Save(context.Background(), nodeWithNullToken)

	responder := api.NewResponder()
	daemonMiddleware := NewDaemonAuthMiddleware(nodeRepo, responder)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protectedHandler := daemonMiddleware.Middleware(testHandler)

	// Try to authenticate with any token - should fail since node has no token
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-Auth-Token", "some-token")

	w := httptest.NewRecorder()
	protectedHandler.ServeHTTP(w, req)

	// Should return unauthorized since no node matches this token
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "invalid api token")
}
