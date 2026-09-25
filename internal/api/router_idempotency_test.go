package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postIdempotent sends a JSON POST with an Idempotency-Key through the router.
func postIdempotent(
	tb testing.TB,
	env *securityTestEnv,
	path, bearerToken, key string,
	body []byte,
) *httptest.ResponseRecorder {
	tb.Helper()

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)

	return doRequestRaw(tb, env, req)
}

//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterIdempotency_CreateServerRetryReturnsSameServer(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)

	gameMod := &domain.GameMod{GameCode: "test", Name: "Default"}
	require.NoError(t, env.container.GameModRepository().Save(env.ctx, gameMod))

	body := fmt.Appendf(nil,
		`{"name":"Idempotent server","game_id":"test","game_mod_id":%d,"ds_id":1,`+
			`"server_ip":"127.0.0.1","server_port":27015}`,
		gameMod.ID,
	)

	first := postIdempotent(t, env, "/api/servers", token, "order-42-create", body)
	require.Equalf(t, http.StatusCreated, first.Code, "body=%s", first.Body.String())

	retry := postIdempotent(t, env, "/api/servers", token, "order-42-create", body)

	assert.Equal(t, http.StatusCreated, retry.Code)
	assert.Equal(t, "true", retry.Header().Get("Idempotent-Replayed"))
	assert.JSONEq(t, first.Body.String(), retry.Body.String())

	servers, err := env.container.ServerRepository().Find(env.ctx, &filters.FindServer{
		Names: []string{"Idempotent server"},
	}, nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1, "a retried create must not make a second server")
}

//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterIdempotency_CommandRetryReturnsSameTask(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)

	first := postIdempotent(t, env, "/api/servers/1/start", token, "order-42-start", nil)
	require.Equalf(t, http.StatusOK, first.Code, "body=%s", first.Body.String())

	retry := postIdempotent(t, env, "/api/servers/1/start", token, "order-42-start", nil)

	assert.Equal(t, http.StatusOK, retry.Code)
	assert.Equal(t, "true", retry.Header().Get("Idempotent-Replayed"))

	var firstBody, retryBody struct {
		DaemonTaskID uint `json:"gdaemonTaskId"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstBody))
	require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryBody))
	assert.NotZero(t, firstBody.DaemonTaskID)
	assert.Equal(t, firstBody.DaemonTaskID, retryBody.DaemonTaskID)

	serverID := uint(1)
	tasks, err := env.container.DaemonTaskRepository().Find(env.ctx, &filters.FindDaemonTask{
		ServerIDs: []*uint{&serverID},
		Tasks:     []domain.DaemonTaskType{domain.DaemonTaskTypeServerStart},
	}, nil, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1, "a retried start must not queue a second task")
}

//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterIdempotency_KeyReusedForDifferentServerIsRejected(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)

	first := postIdempotent(t, env, "/api/servers/1/start", token, "order-42", nil)
	require.Equalf(t, http.StatusOK, first.Code, "body=%s", first.Body.String())

	other := postIdempotent(t, env, "/api/servers/2/start", token, "order-42", nil)

	assert.Equal(t, http.StatusUnprocessableEntity, other.Code)
	assert.Contains(t, other.Body.String(), "idempotency key has already been used for a different request")
}

//nolint:paralleltest // api.CreateRouter mutates the package-global ability-check audit sink.
func TestRouterIdempotency_NodeAliases(t *testing.T) {
	for _, prefix := range []string{"/api/nodes", "/api/dedicated_servers"} {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			t.Run(method+prefix, func(t *testing.T) {
				env := setupSecurityTest(t)
				token := issuePASETOToken(t, env, env.fixtures.AdminUser)
				path := prefix + "/2"
				body := []byte(`{"name":"Updated node"}`)
				if method == http.MethodDelete {
					body = nil
				}
				send := func() *httptest.ResponseRecorder {
					req := httptest.NewRequest(method, path, bytes.NewReader(body))
					req.Header.Set("Authorization", "Bearer "+token)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", "node-change")

					return doRequestRaw(t, env, req)
				}

				first := send()
				if method == http.MethodDelete {
					require.Equal(t, http.StatusNoContent, first.Code, first.Body.String())
				} else {
					require.Equal(t, http.StatusOK, first.Code, first.Body.String())
					node := *env.fixtures.Node2
					node.Name = "Subsequent change"
					require.NoError(t, env.container.NodeRepository().Save(env.ctx, &node))
				}

				retry := send()
				assert.Equal(t, first.Code, retry.Code)
				assert.Equal(t, first.Body.String(), retry.Body.String())
				assert.Equal(t, "true", retry.Header().Get("Idempotent-Replayed"))
				if method == http.MethodPut {
					nodes, err := env.container.NodeRepository().Find(env.ctx, &filters.FindNode{IDs: []uint{2}}, nil, nil)
					require.NoError(t, err)
					require.Len(t, nodes, 1)
					assert.Equal(t, "Subsequent change", nodes[0].Name)
				}
			})
		}
	}
}

//nolint:paralleltest // api.CreateRouter mutates the package-global ability-check audit sink.
func TestRouterIdempotency_DeletedServerTaskStillReplays(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.RegularUser)
	task := &domain.ServerTask{ServerID: 1, Command: domain.ServerTaskCommandStart}
	require.NoError(t, env.container.ServerTaskRepository().Save(env.ctx, task))

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/servers/1/tasks/%d", task.ID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "delete-task")

		return doRequestRaw(t, env, req)
	}

	first := send()
	require.Equal(t, http.StatusNoContent, first.Code, first.Body.String())
	retry := send()
	assert.Equal(t, first.Code, retry.Code, retry.Body.String())
	assert.Equal(t, "true", retry.Header().Get("Idempotent-Replayed"))
}
