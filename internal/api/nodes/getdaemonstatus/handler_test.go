package getdaemonstatus

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
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNotImplemented = errors.New("not implemented")
var errConnectionRefused = errors.New("connection refused")
var errShouldNotBeCalled = errors.New("should not be called")

var testUser = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

type mockDaemonStatusService struct {
	statusFunc  func(ctx context.Context, node *domain.Node) (*daemon.NodeStatus, error)
	versionFunc func(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
}

func (m *mockDaemonStatusService) Status(ctx context.Context, node *domain.Node) (*daemon.NodeStatus, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx, node)
	}

	return nil, errNotImplemented
}

func (m *mockDaemonStatusService) Version(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error) {
	if m.versionFunc != nil {
		return m.versionFunc(ctx, node)
	}

	return nil, errNotImplemented
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name             string
		nodeID           string
		setupAuth        func() context.Context
		setupRepo        func(*inmemory.NodeRepository)
		setupStatusFunc  func(ctx context.Context, node *domain.Node) (*daemon.NodeStatus, error)
		expectedStatus   int
		wantError        string
		expectResponse   bool
		validateResponse func(t *testing.T, resp daemonStatusResponse)
	}{
		{
			name:   "successful daemon status retrieval",
			nodeID: "1",
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
				node := &domain.Node{
					ID:            1,
					Enabled:       true,
					Name:          "Test Node",
					OS:            "linux",
					Location:      "US",
					GdaemonHost:   "127.0.0.1",
					GdaemonPort:   31717,
					GdaemonAPIKey: "test-api-key",
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return &daemon.NodeStatus{
					Uptime:        3600 * time.Second,
					Version:       "3.0.0",
					BuildDate:     "2024-01-15",
					WorkingTasks:  2,
					WaitingTasks:  5,
					OnlineServers: 10,
				}, nil
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			validateResponse: func(t *testing.T, resp daemonStatusResponse) {
				t.Helper()

				assert.Equal(t, uint(1), resp.ID)
				assert.Equal(t, "Test Node", resp.Name)
				assert.Equal(t, "test-api-key", resp.APIKey)
				assert.Equal(t, "3.0.0", resp.Version.Version)
				assert.Equal(t, "2024-01-15", resp.Version.CompileDate)
				assert.Equal(t, "1h0m0s", resp.BaseInfo.Uptime)
				assert.Equal(t, "2", resp.BaseInfo.WorkingTasksCount)
				assert.Equal(t, "5", resp.BaseInfo.WaitingTasksCount)
				assert.Equal(t, "10", resp.BaseInfo.OnlineServersCount)
			},
		},
		{
			name:   "node not found",
			nodeID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return nil, errShouldNotBeCalled
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
			expectResponse: false,
		},
		{
			name:      "user not authenticated",
			nodeID:    "1",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return nil, errShouldNotBeCalled
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectResponse: false,
		},
		{
			name:   "invalid node id",
			nodeID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return nil, errShouldNotBeCalled
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid node id",
			expectResponse: false,
		},
		{
			name:   "daemon connection error",
			nodeID: "1",
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
				node := &domain.Node{
					ID:            1,
					Enabled:       true,
					Name:          "Test Node",
					OS:            "linux",
					Location:      "US",
					GdaemonHost:   "127.0.0.1",
					GdaemonPort:   31717,
					GdaemonAPIKey: "test-api-key",
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return nil, errConnectionRefused
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectResponse: false,
		},
		{
			name:   "daemon status with zero values",
			nodeID: "2",
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
				node := &domain.Node{
					ID:            2,
					Enabled:       true,
					Name:          "Test Node 2",
					OS:            "windows",
					Location:      "EU",
					GdaemonHost:   "192.168.1.1",
					GdaemonPort:   31717,
					GdaemonAPIKey: "test-api-key-2",
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return &daemon.NodeStatus{
					Uptime:        0,
					Version:       "2.5.0",
					BuildDate:     "2023-12-01",
					WorkingTasks:  0,
					WaitingTasks:  0,
					OnlineServers: 0,
				}, nil
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			validateResponse: func(t *testing.T, resp daemonStatusResponse) {
				t.Helper()

				assert.Equal(t, uint(2), resp.ID)
				assert.Equal(t, "Test Node 2", resp.Name)
				assert.Equal(t, "test-api-key-2", resp.APIKey)
				assert.Equal(t, "2.5.0", resp.Version.Version)
				assert.Equal(t, "2023-12-01", resp.Version.CompileDate)
				assert.Equal(t, "0s", resp.BaseInfo.Uptime)
				assert.Equal(t, "0", resp.BaseInfo.WorkingTasksCount)
				assert.Equal(t, "0", resp.BaseInfo.WaitingTasksCount)
				assert.Equal(t, "0", resp.BaseInfo.OnlineServersCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeRepo := inmemory.NewNodeRepository()
			mockStatus := &mockDaemonStatusService{
				statusFunc: tt.setupStatusFunc,
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

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/"+tt.nodeID+"/daemon", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.nodeID})
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

			if tt.expectResponse {
				var resp daemonStatusResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

				if tt.validateResponse != nil {
					tt.validateResponse(t, resp)
				}
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	nodeRepo := inmemory.NewNodeRepository()
	mockStatus := &mockDaemonStatusService{}
	responder := api.NewResponder()

	handler := NewHandler(nodeRepo, mockStatus, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodeRepo, handler.nodeRepo)
	assert.Equal(t, mockStatus, handler.daemonStatus)
	assert.Equal(t, responder, handler.responder)
}

func TestNewDaemonStatusResponse(t *testing.T) {
	tests := []struct {
		name   string
		node   *domain.Node
		status *daemon.NodeStatus
		want   daemonStatusResponse
	}{
		{
			name: "complete_status_response",
			node: &domain.Node{
				ID:            1,
				Name:          "Test Node",
				GdaemonAPIKey: "api-key-123",
			},
			status: &daemon.NodeStatus{
				Uptime:        7200 * time.Second,
				Version:       "3.1.0",
				BuildDate:     "2024-02-01",
				WorkingTasks:  3,
				WaitingTasks:  7,
				OnlineServers: 15,
			},
			want: daemonStatusResponse{
				ID:     1,
				Name:   "Test Node",
				APIKey: "api-key-123",
				Version: versionInfo{
					Version:     "3.1.0",
					CompileDate: "2024-02-01",
				},
				BaseInfo: baseInfo{
					Uptime:             "2h0m0s",
					WorkingTasksCount:  "3",
					WaitingTasksCount:  "7",
					OnlineServersCount: "15",
				},
			},
		},
		{
			name: "zero_values",
			node: &domain.Node{
				ID:            2,
				Name:          "Node 2",
				GdaemonAPIKey: "key-2",
			},
			status: &daemon.NodeStatus{
				Uptime:        0,
				Version:       "",
				BuildDate:     "",
				WorkingTasks:  0,
				WaitingTasks:  0,
				OnlineServers: 0,
			},
			want: daemonStatusResponse{
				ID:     2,
				Name:   "Node 2",
				APIKey: "key-2",
				Version: versionInfo{
					Version:     "",
					CompileDate: "",
				},
				BaseInfo: baseInfo{
					Uptime:             "0s",
					WorkingTasksCount:  "0",
					WaitingTasksCount:  "0",
					OnlineServersCount: "0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDaemonStatusResponse(tt.node, tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}
