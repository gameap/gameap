package putserver

import (
	"context"
	"encoding/json"
	"maps"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var suspendedAt = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func suspensionRequestBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"name":        "Game Server",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "10.0.0.5",
		"server_port": 27015,
	}

	maps.Copy(body, extra)

	return body
}

func TestHandler_Suspension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		suspended      bool
		stopInFlight   bool
		metadata       domain.Metadata
		body           map[string]any
		wantStatus     int
		wantError      string
		wantBlocked    bool
		wantFreshSince bool
		wantReason     string
		wantSuspension *domain.ServerSuspension
		wantMetadata   domain.Metadata
		wantStopTasks  int
	}{
		{
			name:           "blocking_suspends_and_stops_the_server",
			body:           map[string]any{"blocked": true, "suspend_reason": "  unpaid  "},
			wantStatus:     http.StatusOK,
			wantBlocked:    true,
			wantFreshSince: true,
			wantReason:     "unpaid",
			wantStopTasks:  1,
		},
		{
			name:           "blocking_with_a_stop_already_queued_reuses_it",
			stopInFlight:   true,
			body:           map[string]any{"blocked": 1},
			wantStatus:     http.StatusOK,
			wantBlocked:    true,
			wantFreshSince: true,
			// Integrations send a stop and then the block, which must not
			// fail on the stop already queued.
			wantStopTasks: 1,
		},
		{
			name:          "lifting_the_block_drops_the_record",
			suspended:     true,
			metadata:      domain.Metadata{"public_ip": "203.0.113.10"},
			body:          map[string]any{"blocked": false},
			wantStatus:    http.StatusOK,
			wantMetadata:  domain.Metadata{"public_ip": "203.0.113.10"},
			wantStopTasks: 0,
		},
		{
			name:           "update_without_blocked_keeps_the_suspension",
			suspended:      true,
			wantBlocked:    true,
			body:           map[string]any{"metadata": map[string]any{"public_ip": "203.0.113.20"}},
			wantStatus:     http.StatusOK,
			wantSuspension: &domain.ServerSuspension{Since: new(suspendedAt), Reason: "unpaid"},
			wantMetadata: domain.Metadata{
				"public_ip":  "203.0.113.20",
				"suspension": map[string]any{"since": "2026-09-01T08:00:00Z", "reason": "unpaid"},
			},
			wantStopTasks: 0,
		},
		{
			name:        "record_sent_in_metadata_is_ignored",
			suspended:   true,
			wantBlocked: true,
			body: map[string]any{"metadata": map[string]any{
				"suspension": map[string]any{"since": "2000-01-01T00:00:00Z", "reason": "forged"},
			}},
			wantStatus:     http.StatusOK,
			wantSuspension: &domain.ServerSuspension{Since: new(suspendedAt), Reason: "unpaid"},
			wantStopTasks:  0,
		},
		{
			name: "record_sent_in_metadata_does_not_suspend",
			body: map[string]any{"metadata": map[string]any{
				"suspension": map[string]any{"reason": "forged"},
			}},
			wantStatus:    http.StatusOK,
			wantMetadata:  domain.Metadata{},
			wantStopTasks: 0,
		},
		{
			name:           "reason_alone_updates_a_suspended_server",
			suspended:      true,
			wantBlocked:    true,
			body:           map[string]any{"suspend_reason": "abuse report"},
			wantStatus:     http.StatusOK,
			wantSuspension: &domain.ServerSuspension{Since: new(suspendedAt), Reason: "abuse report"},
			wantStopTasks:  0,
		},
		{
			name:          "reason_alone_is_ignored_for_a_server_that_is_not_suspended",
			body:          map[string]any{"suspend_reason": "unpaid"},
			wantStatus:    http.StatusOK,
			wantStopTasks: 0,
		},
		{
			name:           "blocking_a_suspended_server_again_keeps_the_record",
			suspended:      true,
			wantBlocked:    true,
			body:           map[string]any{"blocked": "1"},
			wantStatus:     http.StatusOK,
			wantSuspension: &domain.ServerSuspension{Since: new(suspendedAt), Reason: "unpaid"},
			wantStopTasks:  0,
		},
		{
			name:           "reason_of_the_longest_length_is_accepted",
			body:           map[string]any{"blocked": true, "suspend_reason": strings.Repeat("я", domain.MaxSuspendReasonLength)},
			wantStatus:     http.StatusOK,
			wantBlocked:    true,
			wantFreshSince: true,
			wantReason:     strings.Repeat("я", domain.MaxSuspendReasonLength),
			wantStopTasks:  1,
		},
		{
			name:       "too_long_reason_is_rejected",
			body:       map[string]any{"blocked": true, "suspend_reason": strings.Repeat("я", domain.MaxSuspendReasonLength+1)},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "validation failed: suspend_reason must not exceed 255 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := context.Background()
			serverRepo := inmemory.NewServerRepository()
			taskRepo := inmemory.NewDaemonTaskRepository()
			suspension := serversuspension.NewService(
				serverRepo,
				taskRepo,
				servercontrol.NewService(taskRepo, inmemory.NewServerSettingRepository(), services.NewNilTransactionManager()),
				nil,
				nil,
				nil,
			)
			handler := NewHandler(
				serverRepo, inmemory.NewNodeRepository(), inmemory.NewGameRepository(), inmemory.NewGameModRepository(),
				newServerPorts(serverRepo), nil, suspension, nil, nil, api.NewResponder(),
			)

			server := &domain.Server{
				Name:             "Game Server",
				GameID:           "cstrike",
				DSID:             1,
				GameModID:        1,
				ServerIP:         "10.0.0.5",
				ServerPort:       27015,
				Installed:        domain.ServerInstalledStatusInstalled,
				StartCommand:     new("./start.sh"),
				ProcessActive:    true,
				LastProcessCheck: new(time.Now()),
				Metadata:         tt.metadata,
			}
			if tt.suspended {
				server.Suspend(suspendedAt, new("unpaid"))
			}
			require.NoError(t, serverRepo.Save(ctx, server))

			if tt.stopInFlight {
				require.NoError(t, taskRepo.Save(ctx, &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          new(server.ID),
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWaiting,
				}))
			}

			w := httptest.NewRecorder()
			before := time.Now().UTC().Truncate(time.Second)

			// ACT
			handler.ServeHTTP(w, putServerRequest(t, server.ID, suspensionRequestBody(tt.body)))

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.wantError, response["error"])
			}

			stored, err := serverRepo.Find(ctx, filters.FindServerByIDs(server.ID), nil, nil)
			require.NoError(t, err)
			require.Len(t, stored, 1)

			tasks, err := taskRepo.Find(ctx, &filters.FindDaemonTask{
				Tasks: []domain.DaemonTaskType{domain.DaemonTaskTypeServerStop},
			}, nil, nil)
			require.NoError(t, err)
			assert.Len(t, tasks, tt.wantStopTasks)

			if tt.wantSuspension != nil {
				assert.Equal(t, tt.wantSuspension, stored[0].Suspension())
			}

			if tt.wantMetadata != nil {
				assert.Equal(t, tt.wantMetadata, stored[0].Metadata)
			}

			assert.Equal(t, tt.wantBlocked, stored[0].Blocked)

			if !tt.wantBlocked {
				assert.Nil(t, stored[0].Suspension())
			}

			if tt.wantFreshSince {
				suspension := stored[0].Suspension()
				require.NotNil(t, suspension)
				require.NotNil(t, suspension.Since)
				assert.False(t, suspension.Since.Before(before), "the suspension is dated by the update")
				assert.Equal(t, tt.wantReason, suspension.Reason)
			}
		})
	}
}
