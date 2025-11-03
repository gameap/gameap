package deletenode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
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
		nodeID         string
		setupAuth      func() context.Context
		setupRepos     func(*inmemory.NodeRepository, *inmemory.ServerRepository)
		expectedStatus int
		wantError      string
	}{
		{
			name:   "successful node deletion",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(nodesRepo *inmemory.NodeRepository, _ *inmemory.ServerRepository) {
				now := time.Now()
				node := &domain.Node{
					ID:        1,
					Enabled:   true,
					Name:      "test-node",
					OS:        "linux",
					Location:  "datacenter-1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:   "node not found",
			nodeID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos:     func(_ *inmemory.NodeRepository, _ *inmemory.ServerRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:           "user not authenticated",
			nodeID:         "1",
			setupRepos:     func(_ *inmemory.NodeRepository, _ *inmemory.ServerRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:   "invalid node id",
			nodeID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos:     func(_ *inmemory.NodeRepository, _ *inmemory.ServerRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid node id",
		},
		{
			name:   "node has associated servers - conflict",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(nodesRepo *inmemory.NodeRepository, serversRepo *inmemory.ServerRepository) {
				now := time.Now()
				node := &domain.Node{
					ID:        1,
					Enabled:   true,
					Name:      "test-node",
					OS:        "linux",
					Location:  "datacenter-1",
					CreatedAt: &now,
					UpdatedAt: &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "test",
					Enabled:    true,
					Installed:  1,
					Name:       "test-server",
					GameID:     "cs16",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serversRepo.Save(context.Background(), server))
			},
			expectedStatus: http.StatusConflict,
			wantError:      "cannot delete node with existing game servers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodesRepo := inmemory.NewNodeRepository()
			serversRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()
			handler := NewHandler(nodesRepo, serversRepo, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(nodesRepo, serversRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/dedicated_servers/"+tt.nodeID, nil)
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

			if tt.expectedStatus == http.StatusNoContent {
				assert.Empty(t, w.Body.String())
			}
		})
	}
}

func TestHandler_NodeActuallySoftDeleted(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	serversRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, serversRepo, responder)

	now := time.Now()
	node := &domain.Node{
		ID:        1,
		Enabled:   true,
		Name:      "test-node",
		OS:        "linux",
		Location:  "datacenter-1",
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	require.NoError(t, nodesRepo.Save(context.Background(), node))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodDelete, "/api/dedicated_servers/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	nodes, err := nodesRepo.Find(ctx, nil, nil, nil)
	require.NoError(t, err)
	assert.Len(t, nodes, 0)

	nodesWithDeleted, err := nodesRepo.Find(ctx, &filters.FindNode{WithDeleted: true}, nil, nil)
	require.NoError(t, err)
	assert.Len(t, nodesWithDeleted, 1)
	assert.NotNil(t, nodesWithDeleted[0].DeletedAt)
}

func TestHandler_NewHandler(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	serversRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()

	handler := NewHandler(nodesRepo, serversRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodesRepo, handler.nodesRepo)
	assert.Equal(t, serversRepo, handler.serversRepo)
	assert.Equal(t, responder, handler.responder)
}
