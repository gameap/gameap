package getsummary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errConnectionRefused = errors.New("connection refused")
var errNotImplemented = errors.New("not implemented")
var errShouldNotBeCalled = errors.New("should not be called")

var testUser = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

type mockStatusService struct {
	versionFunc func(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
}

func (m *mockStatusService) Version(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error) {
	if m.versionFunc != nil {
		return m.versionFunc(ctx, node)
	}

	return nil, errNotImplemented
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.NodeRepository)
		setupVersionFunc func(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
		expectedStatus   int
		wantError        string
		validateResponse func(t *testing.T, resp summaryResponse)
	}{
		{
			name: "successful summary with all nodes online",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()
				nodes := []*domain.Node{
					{
						ID:            1,
						Enabled:       true,
						Name:          "Node 1",
						OS:            "linux",
						Location:      "US",
						GdaemonHost:   "127.0.0.1",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-1",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
					{
						ID:            2,
						Enabled:       true,
						Name:          "Node 2",
						OS:            "linux",
						Location:      "EU",
						GdaemonHost:   "127.0.0.2",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-2",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
				}

				for _, node := range nodes {
					require.NoError(t, nodeRepo.Save(context.Background(), node))
				}
			},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return &daemon.NodeVersion{
					Version:   "3.0.0",
					BuildDate: "2024-01-15",
				}, nil
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, resp summaryResponse) {
				t.Helper()

				assert.Equal(t, 2, resp.Total)
				assert.Equal(t, 2, resp.Enabled)
				assert.Equal(t, 0, resp.Disabled)
				assert.Equal(t, 2, resp.Online)
				assert.Equal(t, 0, resp.Offline)
				assert.Len(t, resp.OnlineNodes, 2)
				assert.Len(t, resp.OfflineNodes, 0)

				for _, node := range resp.OnlineNodes {
					assert.True(t, node.Online)
					assert.Equal(t, "3.0.0", node.Version)
					assert.Equal(t, "2024-01-15", node.BuildDate)
				}
			},
		},
		{
			name: "successful summary with mixed online and offline nodes",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()
				nodes := []*domain.Node{
					{
						ID:            1,
						Enabled:       true,
						Name:          "Node 1",
						OS:            "linux",
						Location:      "US",
						GdaemonHost:   "127.0.0.1",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-1",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
					{
						ID:            2,
						Enabled:       false,
						Name:          "Node 2",
						OS:            "linux",
						Location:      "EU",
						GdaemonHost:   "127.0.0.2",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-2",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
					{
						ID:            3,
						Enabled:       true,
						Name:          "Node 3",
						OS:            "windows",
						Location:      "ASIA",
						GdaemonHost:   "127.0.0.3",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-3",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
				}

				for _, node := range nodes {
					require.NoError(t, nodeRepo.Save(context.Background(), node))
				}
			},
			setupVersionFunc: func(_ context.Context, node *domain.Node) (*daemon.NodeVersion, error) {
				if node.ID == 2 {
					return nil, errConnectionRefused
				}

				return &daemon.NodeVersion{
					Version:   "3.1.0",
					BuildDate: "2024-02-01",
				}, nil
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, resp summaryResponse) {
				t.Helper()

				assert.Equal(t, 3, resp.Total)
				assert.Equal(t, 2, resp.Enabled)
				assert.Equal(t, 1, resp.Disabled)
				assert.Equal(t, 2, resp.Online)
				assert.Equal(t, 1, resp.Offline)
				assert.Len(t, resp.OnlineNodes, 2)
				assert.Len(t, resp.OfflineNodes, 1)

				offlineNode := resp.OfflineNodes[0]
				assert.False(t, offlineNode.Online)
				assert.Equal(t, uint(2), offlineNode.ID)
				assert.Equal(t, "Node 2", offlineNode.Name)
				assert.Empty(t, offlineNode.Version)
				assert.Empty(t, offlineNode.BuildDate)
			},
		},
		{
			name: "successful summary with empty node list",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return nil, errShouldNotBeCalled
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, resp summaryResponse) {
				t.Helper()

				assert.Equal(t, 0, resp.Total)
				assert.Equal(t, 0, resp.Enabled)
				assert.Equal(t, 0, resp.Disabled)
				assert.Equal(t, 0, resp.Online)
				assert.Equal(t, 0, resp.Offline)
				assert.Len(t, resp.OnlineNodes, 0)
				assert.Len(t, resp.OfflineNodes, 0)
			},
		},
		{
			name: "successful summary with all nodes offline",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()
				nodes := []*domain.Node{
					{
						ID:            1,
						Enabled:       true,
						Name:          "Node 1",
						OS:            "linux",
						Location:      "US",
						GdaemonHost:   "127.0.0.1",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-1",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
					{
						ID:            2,
						Enabled:       false,
						Name:          "Node 2",
						OS:            "linux",
						Location:      "EU",
						GdaemonHost:   "127.0.0.2",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-2",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
				}

				for _, node := range nodes {
					require.NoError(t, nodeRepo.Save(context.Background(), node))
				}
			},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return nil, errConnectionRefused
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, resp summaryResponse) {
				t.Helper()

				assert.Equal(t, 2, resp.Total)
				assert.Equal(t, 1, resp.Enabled)
				assert.Equal(t, 1, resp.Disabled)
				assert.Equal(t, 0, resp.Online)
				assert.Equal(t, 2, resp.Offline)
				assert.Len(t, resp.OnlineNodes, 0)
				assert.Len(t, resp.OfflineNodes, 2)

				for _, node := range resp.OfflineNodes {
					assert.False(t, node.Online)
					assert.Empty(t, node.Version)
					assert.Empty(t, node.BuildDate)
				}
			},
		},
		{
			name:      "user not authenticated",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return nil, errShouldNotBeCalled
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "successful summary with different versions",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodeRepo *inmemory.NodeRepository) {
				now := time.Now()
				nodes := []*domain.Node{
					{
						ID:            1,
						Enabled:       true,
						Name:          "Old Node",
						OS:            "linux",
						Location:      "US",
						GdaemonHost:   "127.0.0.1",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-1",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
					{
						ID:            2,
						Enabled:       true,
						Name:          "New Node",
						OS:            "linux",
						Location:      "EU",
						GdaemonHost:   "127.0.0.2",
						GdaemonPort:   31717,
						GdaemonAPIKey: "test-api-key-2",
						CreatedAt:     &now,
						UpdatedAt:     &now,
					},
				}

				for _, node := range nodes {
					require.NoError(t, nodeRepo.Save(context.Background(), node))
				}
			},
			setupVersionFunc: func(_ context.Context, node *domain.Node) (*daemon.NodeVersion, error) {
				if node.ID == 1 {
					return &daemon.NodeVersion{
						Version:   "2.0.0",
						BuildDate: "2023-06-01",
					}, nil
				}

				return &daemon.NodeVersion{
					Version:   "3.0.0",
					BuildDate: "2024-01-15",
				}, nil
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(t *testing.T, resp summaryResponse) {
				t.Helper()

				assert.Equal(t, 2, resp.Total)
				assert.Equal(t, 2, resp.Online)
				assert.Equal(t, 0, resp.Offline)
				assert.Len(t, resp.OnlineNodes, 2)

				versionMap := make(map[uint]string)
				for _, node := range resp.OnlineNodes {
					versionMap[node.ID] = node.Version
				}

				assert.Equal(t, "2.0.0", versionMap[1])
				assert.Equal(t, "3.0.0", versionMap[2])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeRepo := inmemory.NewNodeRepository()
			mockStatus := &mockStatusService{
				versionFunc: tt.setupVersionFunc,
			}
			responder := api.NewResponder()
			handler := NewHandler(nodeRepo, mockStatus, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(nodeRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/summary", nil)
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

				return
			}

			if tt.validateResponse != nil {
				var resp summaryResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				tt.validateResponse(t, resp)
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	nodeRepo := inmemory.NewNodeRepository()
	mockStatus := &mockStatusService{}
	responder := api.NewResponder()

	handler := NewHandler(nodeRepo, mockStatus, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodeRepo, handler.nodeRepo)
	assert.Equal(t, mockStatus, handler.statusService)
	assert.Equal(t, responder, handler.responder)
}

func TestHandler_CalculateSummary(t *testing.T) {
	tests := []struct {
		name             string
		nodes            []domain.Node
		setupVersionFunc func(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
		want             summaryResponse
	}{
		{
			name:  "empty nodes",
			nodes: []domain.Node{},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return nil, errShouldNotBeCalled
			},
			want: summaryResponse{
				Total:        0,
				Enabled:      0,
				Disabled:     0,
				Online:       0,
				Offline:      0,
				OnlineNodes:  []nodeSummary{},
				OfflineNodes: []nodeSummary{},
			},
		},
		{
			name: "all nodes online",
			nodes: []domain.Node{
				{ID: 1, Name: "Node 1", Location: "US", Enabled: true},
				{ID: 2, Name: "Node 2", Location: "EU", Enabled: true},
			},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return &daemon.NodeVersion{
					Version:   "3.0.0",
					BuildDate: "2024-01-15",
				}, nil
			},
			want: summaryResponse{
				Total:    2,
				Enabled:  2,
				Disabled: 0,
				Online:   2,
				Offline:  0,
			},
		},
		{
			name: "mixed enabled and disabled",
			nodes: []domain.Node{
				{ID: 1, Name: "Node 1", Location: "US", Enabled: true},
				{ID: 2, Name: "Node 2", Location: "EU", Enabled: false},
				{ID: 3, Name: "Node 3", Location: "ASIA", Enabled: true},
			},
			setupVersionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
				return &daemon.NodeVersion{
					Version:   "3.0.0",
					BuildDate: "2024-01-15",
				}, nil
			},
			want: summaryResponse{
				Total:    3,
				Enabled:  2,
				Disabled: 1,
				Online:   3,
				Offline:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeRepo := inmemory.NewNodeRepository()
			mockStatus := &mockStatusService{
				versionFunc: tt.setupVersionFunc,
			}
			responder := api.NewResponder()
			handler := NewHandler(nodeRepo, mockStatus, responder)

			got := handler.calculateSummary(context.Background(), tt.nodes)

			assert.Equal(t, tt.want.Total, got.Total)
			assert.Equal(t, tt.want.Enabled, got.Enabled)
			assert.Equal(t, tt.want.Disabled, got.Disabled)
			assert.Equal(t, tt.want.Online, got.Online)
			assert.Equal(t, tt.want.Offline, got.Offline)

			if tt.want.OnlineNodes != nil {
				assert.Len(t, got.OnlineNodes, len(tt.want.OnlineNodes))
			}
			if tt.want.OfflineNodes != nil {
				assert.Len(t, got.OfflineNodes, len(tt.want.OfflineNodes))
			}
		})
	}
}
