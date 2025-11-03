package getiplist

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
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		nodeID         string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.NodeRepository)
		expectedStatus int
		wantError      string
		expectedIPs    []string
	}{
		{
			name:   "successful ip list retrieval",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                1,
					Enabled:           true,
					Name:              "Node 1",
					OS:                "linux",
					Location:          "us-east-1",
					IPs:               domain.IPList{"172.17.0.2", "172.17.0.3"},
					WorkPath:          "/var/gameap",
					GdaemonHost:       "localhost",
					GdaemonPort:       8080,
					GdaemonAPIKey:     "test-key",
					GdaemonServerCert: "cert",
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			expectedStatus: http.StatusOK,
			expectedIPs:    []string{"172.17.0.2", "172.17.0.3"},
		},
		{
			name:   "empty ip list",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                1,
					Enabled:           true,
					Name:              "Node 1",
					OS:                "linux",
					Location:          "us-east-1",
					IPs:               domain.IPList{},
					WorkPath:          "/var/gameap",
					GdaemonHost:       "localhost",
					GdaemonPort:       8080,
					GdaemonAPIKey:     "test-key",
					GdaemonServerCert: "cert",
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			expectedStatus: http.StatusOK,
			expectedIPs:    []string{},
		},
		{
			name:   "node not found",
			nodeID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                1,
					Enabled:           true,
					Name:              "Node 1",
					OS:                "linux",
					Location:          "us-east-1",
					IPs:               domain.IPList{"172.17.0.2"},
					WorkPath:          "/var/gameap",
					GdaemonHost:       "localhost",
					GdaemonPort:       8080,
					GdaemonAPIKey:     "test-key",
					GdaemonServerCert: "cert",
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:           "user not authenticated",
			nodeID:         "1",
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:   "invalid node id",
			nodeID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid node id",
		},
		{
			name:   "nil ip list",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                1,
					Enabled:           true,
					Name:              "Node 1",
					OS:                "linux",
					Location:          "us-east-1",
					IPs:               nil,
					WorkPath:          "/var/gameap",
					GdaemonHost:       "localhost",
					GdaemonPort:       8080,
					GdaemonAPIKey:     "test-key",
					GdaemonServerCert: "cert",
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			expectedStatus: http.StatusOK,
			expectedIPs:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeRepo := inmemory.NewNodeRepository()
			responder := api.NewResponder()
			handler := NewHandler(nodeRepo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(nodeRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/"+tt.nodeID+"/ip_list", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"node": tt.nodeID})
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

			if tt.expectedIPs != nil {
				var ips ipListResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ips))
				assert.Equal(t, tt.expectedIPs, []string(ips))
			}
		})
	}
}

func TestNewHandler(t *testing.T) {
	nodeRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()

	handler := NewHandler(nodeRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodeRepo, handler.nodesRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestNewIPListResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    domain.IPList
		expected ipListResponse
	}{
		{
			name:     "with data",
			input:    domain.IPList{"172.17.0.2", "172.17.0.3"},
			expected: ipListResponse{"172.17.0.2", "172.17.0.3"},
		},
		{
			name:     "empty list",
			input:    domain.IPList{},
			expected: ipListResponse{},
		},
		{
			name:     "nil list",
			input:    nil,
			expected: ipListResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newIPListResponse(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
