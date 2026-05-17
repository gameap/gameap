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
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
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

// auditCapture is a concurrency-safe audit.Logger that records every event
// the handler emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
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
			handler := NewHandler(nodesRepo, connChecker, responder, nil)

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
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, responder, nil)

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

	handler := NewHandler(nodesRepo, connChecker, responder, nil)

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
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, responder, nil)

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
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, responder, nil)

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

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — issuing a daemon (node) API
//     token is a credential-issuance event that must be recorded (OWASP ASVS
//     §7.2.1) so node-credential rotation is auditable. The plaintext token
//     must never appear in the audit record (OWASP ASVS §7.1.1).
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

func newGetTokenAuditNode() *domain.Node {
	now := time.Now()

	return &domain.Node{
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
}

// TestHandler_Audit_SuccessfulTokenIssueIsRecorded covers OWASP API8:2023. A
// successful daemon-token issuance must emit exactly one token.daemon.issue
// event with outcome success, category token_op, the node id as the
// resource, and must NOT leak the plaintext token.
func TestHandler_Audit_SuccessfulTokenIssueIsRecorded(t *testing.T) {
	// ARRANGE
	nodesRepo := inmemory.NewNodeRepository()
	require.NoError(t, nodesRepo.Save(context.Background(), newGetTokenAuditNode()))

	recorder := &auditCapture{}
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/init", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "token issuance must succeed; body=%s", w.Body.String())

	var response tokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotEmpty(t, response.Token)

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventDaemonTokenIssue),
		"exactly one token.daemon.issue event must be emitted per successful issuance")

	ev, ok := findEvent(events, audit.EventDaemonTokenIssue)
	require.True(t, ok, "a successful daemon token issuance must leave a token.daemon.issue event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryTokenOp, ev.Category)
	assert.Equal(t, "node", ev.ResourceType, "a daemon token is issued for a node")
	assert.Equal(t, "1", ev.ResourceID, "the node id must be recorded as resource_id")
	assert.Equal(t, "issue", ev.Action)

	assert.NotEqual(t, response.Token, ev.ResourceID,
		"resource_id must be the node id, never the secret token value")
	for _, a := range ev.Extra {
		assert.NotContains(t, a.Value.String(), response.Token,
			"no Extra attr may contain the plaintext daemon token")
	}
}

// TestHandler_Audit_RejectedTokenIssueIsNotRecorded covers OWASP API8:2023.
// A request with an unknown API key is rejected before any token is minted
// and must NOT emit a token.daemon.issue event.
func TestHandler_Audit_RejectedTokenIssueIsNotRecorded(t *testing.T) {
	// ARRANGE
	nodesRepo := inmemory.NewNodeRepository()
	require.NoError(t, nodesRepo.Save(context.Background(), newGetTokenAuditNode()))

	recorder := &auditCapture{}
	handler := NewHandler(nodesRepo, &fakeConnectionChecker{}, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/init", nil)
	req.Header.Set("Authorization", "Bearer wrong-api-key")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code,
		"an unknown api key must be rejected; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventDaemonTokenIssue),
		"a rejected issuance must not be recorded as a successful token.daemon.issue")
}
