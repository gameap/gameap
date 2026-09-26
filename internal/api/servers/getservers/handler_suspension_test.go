package getservers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_OwnerSeesTheSuspension(t *testing.T) {
	t.Parallel()

	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	handler := NewHandler(
		serverRepo,
		inmemory.NewGameRepository(),
		rbac.NewRBAC(services.NewNilTransactionManager(), inmemory.NewRBACRepository(), 0),
		api.NewResponder(),
	)

	suspended := &domain.Server{ID: 1, Name: "Suspended", GameID: "cs", DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}
	suspended.Suspend(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), new("Invoice #1042 is overdue"))
	require.NoError(t, serverRepo.Save(context.Background(), suspended))
	require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
		ID: 2, Name: "Running", GameID: "cs", DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27016,
	}))
	serverRepo.AddUserServer(testUser1.ID, 1)
	serverRepo.AddUserServer(testUser1.ID, 2)

	req := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{User: &testUser1}))
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)

	byName := map[string]map[string]any{}
	for _, server := range response.Data {
		byName[server["name"].(string)] = server
	}

	assert.Equal(t, true, byName["Suspended"]["blocked"])
	assert.Equal(t, map[string]any{
		"since":  "2026-09-01T08:00:00Z",
		"reason": "Invoice #1042 is overdue",
	}, byName["Suspended"]["suspension"])

	require.Contains(t, byName["Running"], "suspension")
	assert.Nil(t, byName["Running"]["suspension"])
}
