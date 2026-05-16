package deletenode

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
					UID:        uuid.New(),
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
			handler := NewHandler(nodesRepo, serversRepo, responder, nil)

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
	handler := NewHandler(nodesRepo, serversRepo, responder, nil)

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

	handler := NewHandler(nodesRepo, serversRepo, responder, nil)

	require.NotNil(t, handler)
	assert.Equal(t, nodesRepo, handler.nodesRepo)
	assert.Equal(t, serversRepo, handler.serversRepo)
	assert.Equal(t, responder, handler.responder)
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — deleting a node is a destructive
//     infrastructure operation that must be recorded (OWASP ASVS §7.2.1) so
//     node decommissioning is auditable.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

func deleteNodeAuthCtx() context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	})
}

// TestHandler_Audit_SuccessfulNodeDeleteIsRecorded covers OWASP API8:2023. A
// successful node deletion must emit exactly one node.delete event with
// outcome success, category node_op, and the deleted node id as the resource.
func TestHandler_Audit_SuccessfulNodeDeleteIsRecorded(t *testing.T) {
	// ARRANGE
	nodesRepo := inmemory.NewNodeRepository()
	serversRepo := inmemory.NewServerRepository()
	now := time.Now()
	require.NoError(t, nodesRepo.Save(context.Background(), &domain.Node{
		ID:        1,
		Enabled:   true,
		Name:      "test-node",
		OS:        "linux",
		Location:  "datacenter-1",
		CreatedAt: &now,
		UpdatedAt: &now,
	}))

	recorder := &auditCapture{}
	handler := NewHandler(nodesRepo, serversRepo, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodDelete, "/api/dedicated_servers/1", nil)
	req = req.WithContext(deleteNodeAuthCtx())
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusNoContent, w.Code,
		"node deletion must succeed; body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventNodeDelete),
		"exactly one node.delete event must be emitted per successful deletion")

	ev, ok := findEvent(events, audit.EventNodeDelete)
	require.True(t, ok, "a successful node deletion must leave a node.delete audit event")
	assert.Equal(t, audit.OutcomeSuccess, ev.Outcome, "a completed sensitive op records success")
	assert.Equal(t, audit.CategoryNodeOp, ev.Category)
	assert.Equal(t, "node", ev.ResourceType)
	assert.Equal(t, "1", ev.ResourceID, "the deleted node id must be recorded")
	assert.Equal(t, "delete", ev.Action)
	assert.Equal(t, testUser1.ID, ev.ActorID, "the acting admin must be attributed as the actor")
	assert.Equal(t, audit.AuthMethodSession, ev.AuthMethod)
}

// TestHandler_Audit_BlockedNodeDeleteDoesNotEmitNodeDelete covers OWASP
// API8:2023. A deletion refused because the node still has game servers must
// NOT emit a node.delete event (the audit trail must not claim a deletion
// that did not happen).
func TestHandler_Audit_BlockedNodeDeleteDoesNotEmitNodeDelete(t *testing.T) {
	// ARRANGE
	nodesRepo := inmemory.NewNodeRepository()
	serversRepo := inmemory.NewServerRepository()
	now := time.Now()
	require.NoError(t, nodesRepo.Save(context.Background(), &domain.Node{
		ID:        1,
		Enabled:   true,
		Name:      "test-node",
		OS:        "linux",
		Location:  "datacenter-1",
		CreatedAt: &now,
		UpdatedAt: &now,
	}))
	require.NoError(t, serversRepo.Save(context.Background(), &domain.Server{
		ID:         1,
		UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:  "short1",
		Enabled:    true,
		Name:       "Server On Node",
		GameID:     "cs",
		DSID:       1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}))

	recorder := &auditCapture{}
	handler := NewHandler(nodesRepo, serversRepo, api.NewResponder(), recorder)

	req := httptest.NewRequest(http.MethodDelete, "/api/dedicated_servers/1", nil)
	req = req.WithContext(deleteNodeAuthCtx())
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusConflict, w.Code,
		"a node with game servers must not be deletable; body=%s", w.Body.String())
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventNodeDelete),
		"a blocked deletion must not be recorded as a successful node.delete")
}
