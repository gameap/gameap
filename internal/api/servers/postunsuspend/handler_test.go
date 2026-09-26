package postunsuspend

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
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/internal/services/serversuspension"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serverID   string
		suspended  bool
		wantStatus int
		wantError  string
	}{
		{
			name:       "suspension_is_lifted",
			serverID:   "1",
			suspended:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "server_that_is_not_suspended",
			serverID:   "1",
			suspended:  false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_server",
			serverID:   "404",
			suspended:  true,
			wantStatus: http.StatusNotFound,
			wantError:  "server not found",
		},
		{
			name:       "invalid_server_id",
			serverID:   "abc",
			suspended:  true,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid server id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := context.Background()
			serverRepo := inmemory.NewServerRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()
			handler := NewHandler(
				serverRepo,
				serversuspension.NewService(
					serverRepo,
					taskRepo,
					servercontrol.NewService(taskRepo, inmemory.NewServerSettingRepository(), services.NewNilTransactionManager()),
					nil,
					nil,
					nil,
				),
				api.NewResponder(),
			)

			server := &domain.Server{
				ID:           1,
				Name:         "Game Server",
				Installed:    domain.ServerInstalledStatusInstalled,
				StartCommand: new("./start.sh"),
				Metadata:     domain.Metadata{"public_ip": "203.0.113.10"},
			}
			if tt.suspended {
				server.Suspend(time.Now(), new("unpaid"))
			}
			require.NoError(t, serverRepo.Save(ctx, server))

			req := httptest.NewRequest(http.MethodPost, "/api/servers/"+tt.serverID+"/unsuspend", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.serverID})
			req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: &domain.User{ID: 1}}))
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			stored, err := serverRepo.Find(ctx, filters.FindServerByIDs(1), nil, nil)
			require.NoError(t, err)
			require.Len(t, stored, 1)

			if tt.wantError != "" {
				assert.Contains(t, response["error"], tt.wantError)
				assert.Equal(t, tt.suspended, stored[0].Blocked)

				return
			}

			assert.Equal(t, "ok", response["status"])
			assert.False(t, stored[0].Blocked)
			assert.Nil(t, stored[0].Suspension())
			assert.Equal(t, domain.Metadata{"public_ip": "203.0.113.10"}, stored[0].Metadata)

			tasks, err := taskRepo.FindAll(ctx, nil, nil)
			require.NoError(t, err)
			assert.Empty(t, tasks, "unsuspending starts nothing")
		})
	}
}
