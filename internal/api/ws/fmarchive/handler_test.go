package fmarchive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

func newTestServer() *domain.Server {
	now := time.Now()

	return &domain.Server{
		ID:            1,
		UID:           uuid.New(),
		UUIDShort:     "short",
		Enabled:       true,
		Installed:     1,
		Name:          "Test Server",
		GameID:        "cs",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           "servers/test1",
		CreatedAt:     &now,
		UpdatedAt:     &now,
		ProcessActive: false,
	}
}

func allowUserFilesAbility(t *testing.T, rbacRepo *inmemory.RBACRepository, userID, serverID uint) {
	t.Helper()

	ability := &domain.Ability{
		Name:       domain.AbilityNameGameServerFiles,
		EntityType: lo.ToPtr(domain.EntityTypeServer),
		EntityID:   new(serverID),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))

	permission := &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(userID),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}
	require.NoError(t, rbacRepo.SavePermission(context.Background(), permission))
}

type handlerSetup struct {
	handler    *Handler
	serverRepo *inmemory.ServerRepository
	rbacRepo   *inmemory.RBACRepository
	hub        *ws.Hub
}

func newHandlerSetup(t *testing.T) *handlerSetup {
	t.Helper()

	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	hub := ws.NewHub(nil)
	t.Cleanup(hub.Close)

	return &handlerSetup{
		handler:    NewHandler(serverRepo, rbacService, hub, nil, api.NewResponder()),
		serverRepo: serverRepo,
		rbacRepo:   rbacRepo,
		hub:        hub,
	}
}

func authorizedRequest(serverID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/"+serverID+"/file-manager/archive-operations", nil)
	req = mux.SetURLVars(req, map[string]string{"server": serverID})

	return req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: &testUser1}))
}

func TestHandler_RejectsRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, s *handlerSetup)
		request    func() *http.Request
		wantStatus int
	}{
		{
			name: "unauthenticated_returns_401",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/1/file-manager/archive-operations", nil)

				return mux.SetURLVars(req, map[string]string{"server": "1"})
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid_server_id_returns_400",
			request:    func() *http.Request { return authorizedRequest("abc") },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_server_returns_404",
			request:    func() *http.Request { return authorizedRequest("999") },
			wantStatus: http.StatusNotFound,
		},
		{
			name: "user_without_files_ability_returns_403",
			setup: func(t *testing.T, s *handlerSetup) {
				t.Helper()
				require.NoError(t, s.serverRepo.Save(context.Background(), newTestServer()))
				s.serverRepo.AddUserServer(1, 1)
			},
			request:    func() *http.Request { return authorizedRequest("1") },
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			s := newHandlerSetup(t)
			if tt.setup != nil {
				tt.setup(t, s)
			}
			rec := httptest.NewRecorder()

			// ACT
			s.handler.ServeHTTP(rec, tt.request())

			// ASSERT
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_SubscribedClientReceivesServerArchiveFrames performs a real
// WebSocket handshake and proves the client lands on the server-scoped
// archive topic: a broadcast for server 1 is delivered, a broadcast for
// another server is not.
func TestHandler_SubscribedClientReceivesServerArchiveFrames(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := newHandlerSetup(t)
	require.NoError(t, s.serverRepo.Save(context.Background(), newTestServer()))
	s.serverRepo.AddUserServer(1, 1)
	allowUserFilesAbility(t, s.rbacRepo, 1, 1)

	router := mux.NewRouter()
	router.HandleFunc("/api/ws/servers/{server}/file-manager/archive-operations", s.handler.ServeHTTP)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.ContextWithSession(r.Context(), &auth.Session{User: &testUser1})
		router.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws/servers/1/file-manager/archive-operations"

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	ownTopic := ws.ChannelToTopic(channels.BuildRealtimeFMArchiveChannel(1))
	foreignTopic := ws.ChannelToTopic(channels.BuildRealtimeFMArchiveChannel(2))

	require.Eventually(t, func() bool {
		return s.hub.TopicSubscriberCount(ownTopic) == 1
	}, 2*time.Second, 10*time.Millisecond, "client must subscribe to its server's archive topic")

	foreign, err := json.Marshal(map[string]any{"type": "archive.progress", "payload": map[string]any{
		"operation_id": "op-foreign",
	}})
	require.NoError(t, err)
	own, err := json.Marshal(map[string]any{"type": "archive.progress", "payload": map[string]any{
		"operation_id": "op-own",
	}})
	require.NoError(t, err)

	// ACT
	s.hub.Broadcast(foreignTopic, foreign)
	s.hub.Broadcast(ownTopic, own)

	// ASSERT: the first (and only) delivered frame is the own-server one.
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()

	_, data, err := conn.Read(readCtx)
	require.NoError(t, err)

	var msg map[string]any
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "archive.progress", msg["type"])
	payload, ok := msg["payload"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "op-own", payload["operation_id"],
		"frames of other servers must never reach this socket")

	// The foreign broadcast preceded the own one; if it had been delivered
	// it would already be queued — a second read must find nothing.
	silenceCtx, silenceCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer silenceCancel()

	_, _, err = conn.Read(silenceCtx)
	require.Error(t, err, "no further frame may arrive")
	assert.ErrorIs(t, err, context.DeadlineExceeded,
		"the read must fail by timing out, not by receiving a foreign frame")
}
