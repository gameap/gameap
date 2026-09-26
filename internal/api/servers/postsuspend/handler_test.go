package postsuspend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
		name          string
		serverID      string
		body          string
		processActive bool
		wantStatus    int
		wantError     string
		wantTask      bool
		wantBlocked   bool
		wantReason    string
	}{
		{
			name:          "running_server_is_suspended_and_stopped",
			serverID:      "1",
			body:          `{"reason": " Invoice #1042 is overdue "}`,
			processActive: true,
			wantStatus:    http.StatusOK,
			wantTask:      true,
			wantBlocked:   true,
			wantReason:    "Invoice #1042 is overdue",
		},
		{
			name:          "body_is_optional",
			serverID:      "1",
			body:          "",
			processActive: true,
			wantStatus:    http.StatusOK,
			wantTask:      true,
			wantBlocked:   true,
		},
		{
			name:          "empty_object_is_accepted",
			serverID:      "1",
			body:          `{}`,
			processActive: true,
			wantStatus:    http.StatusOK,
			wantTask:      true,
			wantBlocked:   true,
		},
		{
			name:          "stopped_server_is_suspended_without_a_stop",
			serverID:      "1",
			body:          `{"reason": "unpaid"}`,
			processActive: false,
			wantStatus:    http.StatusOK,
			wantTask:      false,
			wantBlocked:   true,
			wantReason:    "unpaid",
		},
		{
			name:       "unknown_server",
			serverID:   "404",
			body:       `{}`,
			wantStatus: http.StatusNotFound,
			wantError:  "server not found",
		},
		{
			name:       "invalid_server_id",
			serverID:   "abc",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid server id",
		},
		{
			name:       "invalid_json",
			serverID:   "1",
			body:       `{"reason":`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "reason_of_wrong_type",
			serverID:   "1",
			body:       `{"reason": 42}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "too_long_reason",
			serverID:   "1",
			body:       `{"reason": "` + strings.Repeat("я", domain.MaxSuspendReasonLength+1) + `"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "reason must not exceed 255 characters",
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

			require.NoError(t, serverRepo.Save(ctx, &domain.Server{
				ID:               1,
				Name:             "Game Server",
				DSID:             3,
				Installed:        domain.ServerInstalledStatusInstalled,
				ProcessActive:    tt.processActive,
				LastProcessCheck: new(time.Now()),
			}))

			req := httptest.NewRequest(http.MethodPost, "/api/servers/"+tt.serverID+"/suspend", strings.NewReader(tt.body))
			req = mux.SetURLVars(req, map[string]string{"id": tt.serverID})
			req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: &domain.User{ID: 1}}))
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())

			tasks, err := taskRepo.FindAll(ctx, nil, nil)
			require.NoError(t, err)

			stored, err := serverRepo.Find(ctx, filters.FindServerByIDs(1), nil, nil)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			assert.Equal(t, tt.wantBlocked, stored[0].Blocked)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Contains(t, response["error"], tt.wantError)
				assert.Empty(t, tasks)

				return
			}

			require.NotNil(t, stored[0].Suspension())
			assert.Equal(t, tt.wantReason, stored[0].Suspension().Reason)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Contains(t, response, "gdaemonTaskId")

			if !tt.wantTask {
				assert.Nil(t, response["gdaemonTaskId"], "no stop was queued")
				assert.Empty(t, tasks)

				return
			}

			require.Len(t, tasks, 1)
			assert.Equal(t, domain.DaemonTaskTypeServerStop, tasks[0].Task)
			assert.Equal(t, uint(3), tasks[0].DedicatedServerID)
			assert.InDelta(t, float64(tasks[0].ID), response["gdaemonTaskId"], 0)
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestReadInput_BodyReadError(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/suspend", errReader{})

	in, err := readInput(req)

	require.Error(t, err)
	assert.Nil(t, in)
	assert.Contains(t, err.Error(), "failed to read request body")
}
