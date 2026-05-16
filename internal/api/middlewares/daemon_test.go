package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonAuthMiddleware_Middleware(t *testing.T) {
	// Setup test node. The middleware now compares SHA-256 of the presented
	// X-Auth-Token to the stored value, so the fixture stores the hash.
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()
	validToken := "valid-test-token-123"
	invalidToken := "invalid-token-456"
	storedToken := pkgstrings.SHA256(validToken)

	testNode := &domain.Node{
		ID:              1,
		Enabled:         true,
		Name:            "Test Node",
		OS:              "linux",
		Location:        "us-east-1",
		GdaemonHost:     "localhost",
		GdaemonPort:     8080,
		GdaemonAPIKey:   "test-key",
		GdaemonAPIToken: &storedToken,
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
			daemonMiddleware := NewDaemonAuthMiddleware(nodeRepo, responder, nil)

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
	// Setup multiple nodes with different tokens. Stored values are hashes;
	// the middleware hashes the presented X-Auth-Token before lookup.
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()

	node1Token := "token-node-1"
	node2Token := "token-node-2"
	node1Hash := pkgstrings.SHA256(node1Token)
	node2Hash := pkgstrings.SHA256(node2Token)

	node1 := &domain.Node{
		ID:              1,
		Name:            "Node 1",
		OS:              "linux",
		GdaemonHost:     "node1.example.com",
		GdaemonPort:     8080,
		GdaemonAPIToken: &node1Hash,
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
		GdaemonAPIToken: &node2Hash,
		WorkPath:        "C:\\gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_ = nodeRepo.Save(context.Background(), node2)

	responder := api.NewResponder()
	daemonMiddleware := NewDaemonAuthMiddleware(nodeRepo, responder, nil)

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
	daemonMiddleware := NewDaemonAuthMiddleware(nodeRepo, responder, nil)

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

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — every rejected daemon (node)
//     authentication attempt must leave an auth.daemon.rejected audit event
//     with outcome failure and a stable, non-sensitive reason so an operator
//     can detect a node-token brute force or a misconfigured agent
//     (OWASP ASVS §7.1.3). A silent rejection is a detective-control gap.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestDaemonAuthMiddleware_Audit_RejectionEmitsEvent covers OWASP API2:2023.
// Both rejection branches (no X-Auth-Token at all, and a token that matches
// no node) must emit exactly one auth.daemon.rejected event with outcome
// failure and the branch-specific stable reason.
func TestDaemonAuthMiddleware_Audit_RejectionEmitsEvent(t *testing.T) {
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()
	storedToken := pkgstrings.SHA256("valid-test-token-123")
	require.NoError(t, nodeRepo.Save(context.Background(), &domain.Node{
		ID:              1,
		Enabled:         true,
		Name:            "Test Node",
		OS:              "linux",
		GdaemonHost:     "localhost",
		GdaemonPort:     8080,
		GdaemonAPIToken: &storedToken,
		WorkPath:        "/var/gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}))

	tests := []struct {
		name       string
		authToken  string
		wantReason string
	}{
		{
			name:       "missing_header_has_token_not_set_reason",
			authToken:  "",
			wantReason: "token_not_set",
		},
		{
			name:       "unknown_token_has_invalid_api_token_reason",
			authToken:  "this-token-matches-no-node",
			wantReason: "invalid_api_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			recorder := &auditCapture{}
			mw := NewDaemonAuthMiddleware(nodeRepo, api.NewResponder(), recorder)
			handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authToken != "" {
				req.Header.Set("X-Auth-Token", tt.authToken)
			}
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusUnauthorized, w.Code,
				"a rejected daemon request must not reach the protected handler; body=%s", w.Body.String())

			events := recorder.snapshot()
			require.Equal(t, 1, countEvents(events, audit.EventAuthDaemonRejected),
				"exactly one daemon-rejected event must be emitted per rejected request")

			ev, ok := findEvent(events, audit.EventAuthDaemonRejected)
			require.True(t, ok, "a daemon-rejected audit event must be emitted")
			assert.Equal(t, audit.OutcomeFailure, ev.Outcome, "a rejected daemon auth attempt is a failure")
			assert.Equal(t, audit.CategoryAuthentication, ev.Category)
			assert.Equal(t, tt.wantReason, ev.Reason,
				"the failure reason must be the branch-specific stable token")
			assert.Equal(t, audit.AuthMethodAnonymous, ev.AuthMethod,
				"a node that never authenticated must be attributed anonymous")
		})
	}
}

// TestDaemonAuthMiddleware_Audit_ValidTokenIsNotRejected covers OWASP
// API2:2023. A valid node token must pass authentication and must NOT leave
// a daemon-rejected event (no false-positive failures in the audit trail).
func TestDaemonAuthMiddleware_Audit_ValidTokenIsNotRejected(t *testing.T) {
	// ARRANGE
	nodeRepo := inmemory.NewNodeRepository()
	now := time.Now()
	validToken := "valid-test-token-123"
	storedToken := pkgstrings.SHA256(validToken)
	require.NoError(t, nodeRepo.Save(context.Background(), &domain.Node{
		ID:              1,
		Enabled:         true,
		Name:            "Test Node",
		OS:              "linux",
		GdaemonHost:     "localhost",
		GdaemonPort:     8080,
		GdaemonAPIToken: &storedToken,
		WorkPath:        "/var/gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}))

	recorder := &auditCapture{}
	mw := NewDaemonAuthMiddleware(nodeRepo, api.NewResponder(), recorder)
	handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-Auth-Token", validToken)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code,
		"a valid daemon token must authenticate; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventAuthDaemonRejected),
		"a successfully authenticated node must not produce a false-positive rejection event")
}
