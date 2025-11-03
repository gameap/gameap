package getdaemontask

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		name               string
		taskID             string
		setupRepo          func(*inmemory.DaemonTaskRepository)
		authenticated      bool
		expectedStatusCode int
		expectedResponse   *daemonTaskOutputResponse
		expectedError      string
	}{
		{
			name:          "success",
			taskID:        "1",
			authenticated: true,
			setupRepo: func(repo *inmemory.DaemonTaskRepository) {
				createdAt := time.Date(2025, 9, 25, 18, 30, 0, 0, time.UTC)
				updatedAt := time.Date(2025, 9, 25, 18, 30, 19, 0, time.UTC)
				serverID := uint(2)
				output := "Installation completed successfully\nServer started on port 27015"

				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ID:                1,
					DedicatedServerID: 2,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerInstall,
					CreatedAt:         &createdAt,
					UpdatedAt:         &updatedAt,
					Output:            &output,
					Status:            domain.DaemonTaskStatusSuccess,
				})
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: &daemonTaskOutputResponse{
				ID:                1,
				DedicatedServerID: 2,
				ServerID:          uintPtr(2),
				Task:              domain.DaemonTaskTypeServerInstall,
				CreatedAt:         timePtr(time.Date(2025, 9, 25, 18, 30, 0, 0, time.UTC)),
				UpdatedAt:         timePtr(time.Date(2025, 9, 25, 18, 30, 19, 0, time.UTC)),
				Output:            stringPtr("Installation completed successfully\nServer started on port 27015"),
				Status:            domain.DaemonTaskStatusSuccess,
			},
		},
		{
			name:               "task not found",
			taskID:             "999",
			authenticated:      true,
			setupRepo:          func(_ *inmemory.DaemonTaskRepository) {},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      "daemon task not found",
		},
		{
			name:               "invalid task id",
			taskID:             "invalid",
			authenticated:      true,
			setupRepo:          func(_ *inmemory.DaemonTaskRepository) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "invalid task id",
		},
		{
			name:               "not authenticated",
			taskID:             "1",
			authenticated:      false,
			setupRepo:          func(_ *inmemory.DaemonTaskRepository) {},
			expectedStatusCode: http.StatusUnauthorized,
			expectedError:      "user not authenticated",
		},
		{
			name:          "task_with_minimal_data",
			taskID:        "3",
			authenticated: true,
			setupRepo: func(repo *inmemory.DaemonTaskRepository) {
				updatedAt := time.Date(2025, 9, 25, 18, 30, 0, 0, time.UTC)
				_ = repo.Save(context.Background(), &domain.DaemonTask{
					ID:                3,
					DedicatedServerID: 1,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWaiting,
					UpdatedAt:         &updatedAt,
				})
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: &daemonTaskOutputResponse{
				ID:                3,
				DedicatedServerID: 1,
				ServerID:          nil,
				Task:              domain.DaemonTaskTypeServerStop,
				CreatedAt:         nil,
				UpdatedAt:         timePtr(time.Date(2025, 9, 25, 18, 30, 0, 0, time.UTC)),
				Output:            nil,
				Status:            domain.DaemonTaskStatusWaiting,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			repo := inmemory.NewDaemonTaskRepository()
			tc.setupRepo(repo)

			handler := NewHandler(repo, api.NewResponder(), true)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/api/gdaemon_tasks/"+tc.taskID+"/output", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tc.taskID})

			// Add authentication if needed
			if tc.authenticated {
				ctx := auth.ContextWithSession(req.Context(), &auth.Session{
					User: &domain.User{ID: 1},
				})
				req = req.WithContext(ctx)
			}

			// Execute
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// Assert
			assert.Equal(t, tc.expectedStatusCode, rec.Code)

			if tc.expectedResponse != nil {
				var actualResponse daemonTaskOutputResponse
				err := json.Unmarshal(rec.Body.Bytes(), &actualResponse)
				require.NoError(t, err)
				assert.Equal(t, *tc.expectedResponse, actualResponse)
			}

			if tc.expectedError != "" {
				var errorResponse map[string]any
				err := json.Unmarshal(rec.Body.Bytes(), &errorResponse)
				require.NoError(t, err)
				assert.Contains(t, errorResponse["error"].(string), tc.expectedError)
			}
		})
	}
}

func TestHandler_ServeHTTP_LargeOutput(t *testing.T) {
	// Setup
	repo := inmemory.NewDaemonTaskRepository()

	// Create a task with large output
	largeOutput := ""
	var largeOutputSb165 strings.Builder
	for i := range 1000 {
		largeOutputSb165.WriteString("Line " + string(rune('0'+i%10)) + ": This is a log line with some content\n")
	}
	largeOutput += largeOutputSb165.String()

	_ = repo.Save(context.Background(), &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerUpdate,
		Output:            &largeOutput,
		Status:            domain.DaemonTaskStatusSuccess,
	})

	handler := NewHandler(repo, api.NewResponder(), true)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/gdaemon_tasks/1/output", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})

	// Add authentication
	ctx := auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	})
	req = req.WithContext(ctx)

	// Execute
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Assert
	assert.Equal(t, http.StatusOK, rec.Code)

	var response daemonTaskOutputResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, uint(1), response.ID)
	assert.NotNil(t, response.Output)
	assert.Equal(t, largeOutput, *response.Output)
}

// Helper functions.
func uintPtr(i uint) *uint {
	return &i
}

func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
