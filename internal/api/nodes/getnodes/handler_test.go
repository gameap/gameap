package getnodes

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.NodeRepository)
		expectedStatus int
		wantError      string
		expectNodes    bool
		expectedCount  int
	}{
		{
			name: "successful nodes retrieval",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodesRepo *inmemory.NodeRepository) {
				now := time.Now()

				node1 := &domain.Node{
					ID:          1,
					Enabled:     true,
					Name:        "node1",
					OS:          "linux",
					Location:    "US",
					IPs:         []string{"192.168.1.1"},
					WorkPath:    "/var/gameap",
					GdaemonHost: "192.168.1.1",
					GdaemonPort: 31717,
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}
				node2 := &domain.Node{
					ID:          2,
					Enabled:     true,
					Name:        "node2",
					OS:          "windows",
					Location:    "EU",
					IPs:         []string{"192.168.1.2"},
					WorkPath:    "C:\\gameap",
					GdaemonHost: "192.168.1.2",
					GdaemonPort: 31717,
					CreatedAt:   &now,
					UpdatedAt:   &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node1))
				require.NoError(t, nodesRepo.Save(context.Background(), node2))
			},
			expectedStatus: http.StatusOK,
			expectNodes:    true,
			expectedCount:  2,
		},
		{
			name:           "user not authenticated",
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectNodes:    false,
		},
		{
			name: "no nodes available",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusOK,
			expectNodes:    true,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodesRepo := inmemory.NewNodeRepository()
			responder := api.NewResponder()
			handler := NewHandler(nodesRepo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(nodesRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers", nil)
			req = req.WithContext(ctx)
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

			if tt.expectNodes {
				var nodes []nodeResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nodes))
				assert.Len(t, nodes, tt.expectedCount)

				if tt.expectedCount > 0 {
					node := nodes[0]
					assert.NotZero(t, node.ID)
					assert.NotEmpty(t, node.Name)
					assert.NotEmpty(t, node.OS)
					assert.NotEmpty(t, node.Location)
					assert.NotEmpty(t, node.IP)
				}
			}
		})
	}
}

func TestHandler_NodesResponseFields(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, responder)

	now := time.Now()
	provider := "AWS"
	node := &domain.Node{
		ID:          1,
		Enabled:     true,
		Name:        "test-node",
		OS:          "linux",
		Location:    "Montenegro",
		Provider:    &provider,
		IPs:         []string{"172.18.0.5"},
		WorkPath:    "/var/gameap",
		GdaemonPort: 31717,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	require.NoError(t, nodesRepo.Save(context.Background(), node))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var nodes []nodeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nodes))

	require.Len(t, nodes, 1)
	nodeResp := nodes[0]

	assert.Equal(t, uint(1), nodeResp.ID)
	assert.True(t, nodeResp.Enabled)
	assert.Equal(t, "test-node", nodeResp.Name)
	assert.Equal(t, "linux", nodeResp.OS)
	assert.Equal(t, "Montenegro", nodeResp.Location)
	require.NotNil(t, nodeResp.Provider)
	assert.Equal(t, "AWS", *nodeResp.Provider)
	require.Len(t, nodeResp.IP, 1)
	assert.Equal(t, "172.18.0.5", nodeResp.IP[0])
}

func TestNewNodesResponseFromNodes(t *testing.T) {
	now := time.Now()
	nodes := []domain.Node{
		{
			ID:          1,
			Enabled:     true,
			Name:        "node1",
			OS:          "linux",
			Location:    "US",
			IPs:         []string{"192.168.1.1"},
			WorkPath:    "/var/gameap",
			GdaemonHost: "192.168.1.1",
			GdaemonPort: 31717,
			CreatedAt:   &now,
		},
		{
			ID:          2,
			Enabled:     false,
			Name:        "node2",
			OS:          "windows",
			Location:    "EU",
			IPs:         []string{"192.168.1.2"},
			WorkPath:    "C:\\gameap",
			GdaemonHost: "192.168.1.2",
			GdaemonPort: 31717,
			CreatedAt:   &now,
		},
	}

	response := newNodesResponseFromNodes(nodes)

	require.Len(t, response, 2)

	assert.Equal(t, uint(1), response[0].ID)
	assert.Equal(t, "node1", response[0].Name)
	assert.Equal(t, "linux", response[0].OS)
	assert.True(t, response[0].Enabled)
	assert.Len(t, response[0].IP, 1)
	assert.Equal(t, "192.168.1.1", response[0].IP[0])

	assert.Equal(t, uint(2), response[1].ID)
	assert.Equal(t, "node2", response[1].Name)
	assert.Equal(t, "windows", response[1].OS)
	assert.False(t, response[1].Enabled)
	assert.Len(t, response[1].IP, 1)
	assert.Equal(t, "192.168.1.2", response[1].IP[0])
}

func TestNewNodeResponseFromNode(t *testing.T) {
	now := time.Now()
	provider := "DigitalOcean"
	node := &domain.Node{
		ID:          1,
		Enabled:     true,
		Name:        "test-node",
		OS:          "linux",
		Location:    "NYC",
		Provider:    &provider,
		IPs:         []string{"10.0.0.1"},
		WorkPath:    "/var/gameap",
		GdaemonHost: "10.0.0.1",
		GdaemonPort: 31717,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	response := newNodeResponseFromNode(node)

	assert.Equal(t, uint(1), response.ID)
	assert.True(t, response.Enabled)
	assert.Equal(t, "test-node", response.Name)
	assert.Equal(t, "linux", response.OS)
	assert.Equal(t, "NYC", response.Location)
	require.NotNil(t, response.Provider)
	assert.Equal(t, "DigitalOcean", *response.Provider)
	require.Len(t, response.IP, 1)
	assert.Equal(t, "10.0.0.1", response.IP[0])
}

func TestNewNodeResponseFromNode_EmptyIP(t *testing.T) {
	node := &domain.Node{
		ID:          1,
		Enabled:     true,
		Name:        "test-node",
		OS:          "linux",
		Location:    "NYC",
		IPs:         []string{},
		WorkPath:    "/var/gameap",
		GdaemonHost: "10.0.0.1",
		GdaemonPort: 31717,
	}

	response := newNodeResponseFromNode(node)

	assert.Equal(t, uint(1), response.ID)
	assert.Empty(t, response.IP)
}
