package getsummary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errConnectionRefused = errors.New("connection refused")
var errNotImplemented = errors.New("not implemented")
var errShouldNotBeCalled = errors.New("should not be called")
var errSimulatedRepoFailure = errors.New("simulated repo failure")

var testUser = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

type mockStatusService struct {
	versionFunc func(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
	callCount   atomic.Int64
}

func (m *mockStatusService) Version(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error) {
	m.callCount.Add(1)

	if m.versionFunc != nil {
		return m.versionFunc(ctx, node)
	}

	return nil, errNotImplemented
}

type failingNodeRepo struct {
	*inmemory.NodeRepository

	fail atomic.Bool
}

func (r *failingNodeRepo) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	if r.fail.Load() {
		return nil, errSimulatedRepoFailure
	}

	return r.NodeRepository.FindAll(ctx, order, pagination)
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			nodeRepo := inmemory.NewNodeRepository()
			mockStatus := &mockStatusService{
				versionFunc: tt.setupVersionFunc,
			}
			responder := api.NewResponder()
			handler := NewHandler(nodeRepo, mockStatus, responder, cache.NewInMemory())

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
	t.Parallel()
	nodeRepo := inmemory.NewNodeRepository()
	mockStatus := &mockStatusService{}
	responder := api.NewResponder()
	c := cache.NewInMemory()

	handler := NewHandler(nodeRepo, mockStatus, responder, c)

	require.NotNil(t, handler)
	assert.Equal(t, nodeRepo, handler.nodeRepo)
	assert.Equal(t, mockStatus, handler.statusService)
	assert.Equal(t, responder, handler.responder)
	assert.Equal(t, c, handler.cache)
	assert.Equal(t, defaultCacheTTL, handler.cacheTTL)
	assert.Equal(t, backgroundRefreshTimeout, handler.backgroundRefreshTimeout)
}

func TestHandler_CalculateSummary(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			nodeRepo := inmemory.NewNodeRepository()
			mockStatus := &mockStatusService{
				versionFunc: tt.setupVersionFunc,
			}
			responder := api.NewResponder()
			handler := NewHandler(nodeRepo, mockStatus, responder, cache.NewInMemory())

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

func newAuthCtx() context.Context {
	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser,
	}

	return auth.ContextWithSession(context.Background(), session)
}

func saveTwoEnabledNodes(t *testing.T, repo *inmemory.NodeRepository) {
	t.Helper()

	now := time.Now()
	for _, n := range []*domain.Node{
		{
			ID: 1, Name: "Node 1", Enabled: true, OS: "linux", Location: "US",
			GdaemonHost: "127.0.0.1", GdaemonPort: 31717, GdaemonAPIKey: "test-api-key-1",
			CreatedAt: &now, UpdatedAt: &now,
		},
		{
			ID: 2, Name: "Node 2", Enabled: true, OS: "linux", Location: "EU",
			GdaemonHost: "127.0.0.2", GdaemonPort: 31717, GdaemonAPIKey: "test-api-key-2",
			CreatedAt: &now, UpdatedAt: &now,
		},
	} {
		require.NoError(t, repo.Save(context.Background(), n))
	}
}

func versionOK(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
	return &daemon.NodeVersion{Version: "3.0.0", BuildDate: "2024-01-15"}, nil
}

func TestHandler_CachesFreshResponse(t *testing.T) {
	t.Parallel()
	nodeRepo := inmemory.NewNodeRepository()
	saveTwoEnabledNodes(t, nodeRepo)

	mockStatus := &mockStatusService{versionFunc: versionOK}
	handler := NewHandler(nodeRepo, mockStatus, api.NewResponder(), cache.NewInMemory())
	ctx := newAuthCtx()

	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/nodes/summary", nil).WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	}

	assert.Equal(t, int64(2), mockStatus.callCount.Load(),
		"expected single compute round across sequential requests within refresh window")
}

func TestHandler_ProactivelyRefreshesBeforeExpiry(t *testing.T) {
	t.Parallel()
	nodeRepo := inmemory.NewNodeRepository()
	saveTwoEnabledNodes(t, nodeRepo)

	mockStatus := &mockStatusService{versionFunc: versionOK}
	c := cache.NewInMemory()
	handler := NewHandler(nodeRepo, mockStatus, api.NewResponder(), c)
	handler.cacheTTL = 200 * time.Millisecond
	handler.backgroundRefreshTimeout = 20 * time.Millisecond
	ctx := newAuthCtx()

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/summary", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(2), mockStatus.callCount.Load())

	assert.Eventually(t, func() bool {
		return mockStatus.callCount.Load() >= 4
	}, time.Second, 5*time.Millisecond,
		"scheduled refresh must fire before the cache TTL expires")

	cached, err := cache.GetTyped[summaryResponse](context.Background(), c, cacheKey)
	require.NoError(t, err, "cache should still hold data after the proactive refresh")
	assert.Equal(t, 2, cached.Total)
	assert.Equal(t, 2, cached.Online)
}

func TestHandler_ConcurrentColdStartCollapses(t *testing.T) {
	t.Parallel()
	nodeRepo := inmemory.NewNodeRepository()
	saveTwoEnabledNodes(t, nodeRepo)

	mockStatus := &mockStatusService{
		versionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
			time.Sleep(20 * time.Millisecond)

			return &daemon.NodeVersion{Version: "3.0.0", BuildDate: "2024-01-15"}, nil
		},
	}
	handler := NewHandler(nodeRepo, mockStatus, api.NewResponder(), cache.NewInMemory())
	ctx := newAuthCtx()

	const concurrency = 10
	var wg sync.WaitGroup
	startGate := make(chan struct{})

	for range concurrency {
		wg.Go(func() {
			<-startGate

			req := httptest.NewRequest(http.MethodGet, "/api/nodes/summary", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}

	close(startGate)
	wg.Wait()

	assert.Equal(t, int64(2), mockStatus.callCount.Load(),
		"singleflight should collapse concurrent cold-start computes into one")
}

func TestHandler_ScheduledRefreshErrorPreservesCache(t *testing.T) {
	t.Parallel()
	baseRepo := inmemory.NewNodeRepository()
	saveTwoEnabledNodes(t, baseRepo)
	failRepo := &failingNodeRepo{NodeRepository: baseRepo}

	mockStatus := &mockStatusService{versionFunc: versionOK}
	c := cache.NewInMemory()
	handler := NewHandler(failRepo, mockStatus, api.NewResponder(), c)
	ctx := newAuthCtx()

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/summary", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var firstResp summaryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &firstResp))

	failRepo.fail.Store(true)
	handler.runScheduledRefresh()

	cached, err := cache.GetTyped[summaryResponse](context.Background(), c, cacheKey)
	require.NoError(t, err, "cache must remain populated after a failed refresh")
	assert.Equal(t, firstResp, cached,
		"failed scheduled refresh must not overwrite the cached response")
}

func TestHandler_NotAuthenticatedDoesNotConsultCache(t *testing.T) {
	t.Parallel()
	nodeRepo := inmemory.NewNodeRepository()
	saveTwoEnabledNodes(t, nodeRepo)

	mockStatus := &mockStatusService{
		versionFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeVersion, error) {
			return nil, errShouldNotBeCalled
		},
	}
	c := cache.NewInMemory()
	handler := NewHandler(nodeRepo, mockStatus, api.NewResponder(), c)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/summary", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Zero(t, mockStatus.callCount.Load(),
		"Version must not be called when caller is unauthenticated")

	_, err := c.Get(context.Background(), cacheKey)
	assert.ErrorIs(t, err, cache.ErrNotFound,
		"cache must not be populated by an unauthenticated request")
}
