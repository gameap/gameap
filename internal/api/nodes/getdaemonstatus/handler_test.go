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
	statusFunc         func(ctx context.Context, node *domain.Node) (*daemon.NodeStatus, error)
	versionFunc        func(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
	connectionTypeFunc func(nodeID uint64) string
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

func (m *mockDaemonStatusService) ConnectionType(nodeID uint64) string {
	if m.connectionTypeFunc != nil {
		return m.connectionTypeFunc(nodeID)
	}

	return daemon.ConnectionTypeNone
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		nodeID                  string
		setupAuth               func() context.Context
		setupRepo               func(*inmemory.NodeRepository)
		setupStatusFunc         func(ctx context.Context, node *domain.Node) (*daemon.NodeStatus, error)
		setupConnectionTypeFunc func(nodeID uint64) string
		expectedStatus          int
		wantError               string
		expectResponse          bool
		validateResponse        func(t *testing.T, resp daemonStatusResponse)
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
			setupConnectionTypeFunc: func(_ uint64) string {
				return daemon.ConnectionTypeGRPC
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			validateResponse: func(t *testing.T, resp daemonStatusResponse) {
				t.Helper()

				assert.Equal(t, uint(1), resp.ID)
				assert.Equal(t, "Test Node", resp.Name)
				assert.True(t, resp.HasAPIKey)
				assert.Equal(t, "grpc", resp.ConnectionType)
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
			setupConnectionTypeFunc: func(_ uint64) string {
				return daemon.ConnectionTypeGRPC
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			validateResponse: func(t *testing.T, resp daemonStatusResponse) {
				t.Helper()

				assert.Equal(t, uint(2), resp.ID)
				assert.Equal(t, "Test Node 2", resp.Name)
				assert.True(t, resp.HasAPIKey)
				assert.Equal(t, "grpc", resp.ConnectionType)
				assert.Equal(t, "2.5.0", resp.Version.Version)
				assert.Equal(t, "2023-12-01", resp.Version.CompileDate)
				assert.Equal(t, "0s", resp.BaseInfo.Uptime)
				assert.Equal(t, "0", resp.BaseInfo.WorkingTasksCount)
				assert.Equal(t, "0", resp.BaseInfo.WaitingTasksCount)
				assert.Equal(t, "0", resp.BaseInfo.OnlineServersCount)
			},
		},
		{
			name:   "connection_type_grpc_when_session_registry_has_node",
			nodeID: "3",
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
					ID:            3,
					Enabled:       true,
					Name:          "GRPC Node",
					OS:            "linux",
					Location:      "US",
					GdaemonHost:   "10.0.0.3",
					GdaemonPort:   31717,
					GdaemonAPIKey: "key-3",
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return &daemon.NodeStatus{Version: "3.1.0", BuildDate: "2024-03-01"}, nil
			},
			setupConnectionTypeFunc: func(nodeID uint64) string {
				assert.Equal(t, uint64(3), nodeID)

				return daemon.ConnectionTypeGRPC
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			validateResponse: func(t *testing.T, resp daemonStatusResponse) {
				t.Helper()

				assert.Equal(t, "grpc", resp.ConnectionType)
			},
		},
		{
			name:   "connection_type_none_when_neither_session_nor_legacy",
			nodeID: "5",
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
					ID:            5,
					Enabled:       true,
					Name:          "Detached Node",
					OS:            "linux",
					Location:      "AS",
					GdaemonHost:   "10.0.0.5",
					GdaemonPort:   31717,
					GdaemonAPIKey: "key-5",
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, nodeRepo.Save(context.Background(), node))
			},
			setupStatusFunc: func(_ context.Context, _ *domain.Node) (*daemon.NodeStatus, error) {
				return &daemon.NodeStatus{Version: "3.0.0", BuildDate: "2024-01-01"}, nil
			},
			setupConnectionTypeFunc: func(_ uint64) string {
				return daemon.ConnectionTypeNone
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			validateResponse: func(t *testing.T, resp daemonStatusResponse) {
				t.Helper()

				assert.Equal(t, "none", resp.ConnectionType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nodeRepo := inmemory.NewNodeRepository()
			mockStatus := &mockDaemonStatusService{
				statusFunc:         tt.setupStatusFunc,
				connectionTypeFunc: tt.setupConnectionTypeFunc,
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
	t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name           string
		node           *domain.Node
		status         *daemon.NodeStatus
		connectionType string
		want           daemonStatusResponse
	}{
		{
			name: "complete_status_response_grpc",
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
			connectionType: daemon.ConnectionTypeGRPC,
			want: daemonStatusResponse{
				ID:             1,
				Name:           "Test Node",
				HasAPIKey:      true,
				ConnectionType: "grpc",
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
			name: "none_connection_type",
			node: &domain.Node{
				ID:            3,
				Name:          "Detached",
				GdaemonAPIKey: "key-3",
			},
			status: &daemon.NodeStatus{
				Version:   "3.0.0",
				BuildDate: "2024-01-01",
			},
			connectionType: daemon.ConnectionTypeNone,
			want: daemonStatusResponse{
				ID:             3,
				Name:           "Detached",
				HasAPIKey:      true,
				ConnectionType: "none",
				Version: versionInfo{
					Version:     "3.0.0",
					CompileDate: "2024-01-01",
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
			t.Parallel()
			got := newDaemonStatusResponse(tt.node, tt.status, tt.connectionType)
			assert.Equal(t, tt.want, got)
		})
	}
}
