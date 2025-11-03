package getnode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		setupRepo      func(*inmemory.NodeRepository)
		expectedStatus int
		wantError      string
		expectNode     bool
	}{
		{
			name:   "successful node retrieval",
			nodeID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(nodesRepo *inmemory.NodeRepository) {
				now := time.Now()

				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				require.NoError(t, nodesRepo.Save(context.Background(), node))
			},
			expectedStatus: http.StatusOK,
			expectNode:     true,
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
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
			expectNode:     false,
		},
		{
			name:           "user not authenticated",
			nodeID:         "1",
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectNode:     false,
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
			setupRepo:      func(_ *inmemory.NodeRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid node id",
			expectNode:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodesRepo := inmemory.NewNodeRepository()
			responder := api.NewResponder()
			handler := NewHandler(nodesRepo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(nodesRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_server/"+tt.nodeID, nil)
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

			if tt.expectNode {
				var node nodeResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &node))
				assert.NotZero(t, node.ID)
				assert.NotEmpty(t, node.Name)
				assert.NotEmpty(t, node.OS)
				assert.NotEmpty(t, node.Location)
				assert.NotEmpty(t, node.IPs)
			}
		})
	}
}

func TestHandler_NodeResponseFields(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()
	handler := NewHandler(nodesRepo, responder)

	now := time.Now()
	provider := "AWS"
	ram := "1024"
	cpu := "1"
	steamcmdPath := "/srv/gameap/steamcmd"
	gdaemonAPIToken := "INi5pAMySv93RkgANYszIzsQV0qa6sSoXbR3kbhDhIfwAqP33jp0KvRTJ394TaiT"
	gdaemonLogin := "some_login"
	gdaemonPassword := "some_password"
	scriptInstall := "install.sh"
	scriptReinstall := "reinstall.sh"
	scriptUpdate := "update.sh"
	scriptStart := "start.sh"
	scriptPause := "pause.sh"
	scriptUnpause := "unpause.sh"
	scriptStop := "stop.sh"
	scriptKill := "kill.sh"
	scriptRestart := "restart.sh"
	scriptStatus := "status.sh"
	scriptStats := "stats.sh"
	scriptGetConsole := "server-output {id}"
	scriptSendCommand := "server-command {id} {command}"
	scriptDelete := "delete.sh"

	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "22aeaf65bbcd",
		OS:                  "linux",
		Location:            "Montenegro",
		Provider:            &provider,
		IPs:                 []string{"172.18.0.5"},
		RAM:                 &ram,
		CPU:                 &cpu,
		WorkPath:            "/srv/gameap",
		SteamcmdPath:        &steamcmdPath,
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "gNvgQbFtgkIgYEVd88ns0vO4li70clyaHm4e1bAeF2wJkF9of6UbTxX0i9SAQ2mP",
		GdaemonAPIToken:     &gdaemonAPIToken,
		GdaemonLogin:        &gdaemonLogin,
		GdaemonPassword:     &gdaemonPassword,
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		ScriptInstall:       &scriptInstall,
		ScriptReinstall:     &scriptReinstall,
		ScriptUpdate:        &scriptUpdate,
		ScriptStart:         &scriptStart,
		ScriptPause:         &scriptPause,
		ScriptUnpause:       &scriptUnpause,
		ScriptStop:          &scriptStop,
		ScriptKill:          &scriptKill,
		ScriptRestart:       &scriptRestart,
		ScriptStatus:        &scriptStatus,
		ScriptStats:         &scriptStats,
		ScriptGetConsole:    &scriptGetConsole,
		ScriptSendCommand:   &scriptSendCommand,
		ScriptDelete:        &scriptDelete,
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}
	require.NoError(t, nodesRepo.Save(context.Background(), node))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/dedicated_server/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var nodeResp nodeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nodeResp))

	assert.Equal(t, uint(1), nodeResp.ID)
	assert.True(t, nodeResp.Enabled)
	assert.Equal(t, "22aeaf65bbcd", nodeResp.Name)
	assert.Equal(t, "linux", nodeResp.OS)
	assert.Equal(t, "Montenegro", nodeResp.Location)
	require.NotNil(t, nodeResp.Provider)
	assert.Equal(t, "AWS", *nodeResp.Provider)
	require.Len(t, nodeResp.IPs, 1)
	assert.Equal(t, "172.18.0.5", nodeResp.IPs[0])
	require.NotNil(t, nodeResp.RAM)
	assert.Equal(t, "1024", *nodeResp.RAM)
	require.NotNil(t, nodeResp.CPU)
	assert.Equal(t, "1", *nodeResp.CPU)
	assert.Equal(t, "/srv/gameap", nodeResp.WorkPath)
	require.NotNil(t, nodeResp.SteamcmdPath)
	assert.Equal(t, "/srv/gameap/steamcmd", *nodeResp.SteamcmdPath)
	assert.Equal(t, "172.18.0.5", nodeResp.GdaemonHost)
	assert.Equal(t, 31717, nodeResp.GdaemonPort)
	assert.Equal(t, "gNvgQbFtgkIgYEVd88ns0vO4li70clyaHm4e1bAeF2wJkF9of6UbTxX0i9SAQ2mP", nodeResp.GdaemonAPIKey)
	require.NotNil(t, nodeResp.GdaemonLogin)
	assert.Equal(t, "some_login", *nodeResp.GdaemonLogin)
	require.NotNil(t, nodeResp.GdaemonPassword)
	assert.Equal(t, "some_password", *nodeResp.GdaemonPassword)
	assert.Equal(t, "certs/root.crt", nodeResp.GdaemonServerCert)
	assert.Equal(t, uint(1), nodeResp.ClientCertificateID)
	assert.Equal(t, "auto", nodeResp.PreferInstallMethod)
	require.NotNil(t, nodeResp.ScriptInstall)
	assert.Equal(t, "install.sh", *nodeResp.ScriptInstall)
	require.NotNil(t, nodeResp.ScriptReinstall)
	assert.Equal(t, "reinstall.sh", *nodeResp.ScriptReinstall)
	require.NotNil(t, nodeResp.ScriptUpdate)
	assert.Equal(t, "update.sh", *nodeResp.ScriptUpdate)
	require.NotNil(t, nodeResp.ScriptStart)
	assert.Equal(t, "start.sh", *nodeResp.ScriptStart)
	require.NotNil(t, nodeResp.ScriptPause)
	assert.Equal(t, "pause.sh", *nodeResp.ScriptPause)
	require.NotNil(t, nodeResp.ScriptUnpause)
	assert.Equal(t, "unpause.sh", *nodeResp.ScriptUnpause)
	require.NotNil(t, nodeResp.ScriptStop)
	assert.Equal(t, "stop.sh", *nodeResp.ScriptStop)
	require.NotNil(t, nodeResp.ScriptKill)
	assert.Equal(t, "kill.sh", *nodeResp.ScriptKill)
	require.NotNil(t, nodeResp.ScriptRestart)
	assert.Equal(t, "restart.sh", *nodeResp.ScriptRestart)
	require.NotNil(t, nodeResp.ScriptStatus)
	assert.Equal(t, "status.sh", *nodeResp.ScriptStatus)
	require.NotNil(t, nodeResp.ScriptStats)
	assert.Equal(t, "stats.sh", *nodeResp.ScriptStats)
	require.NotNil(t, nodeResp.ScriptGetConsole)
	assert.Equal(t, "server-output {id}", *nodeResp.ScriptGetConsole)
	require.NotNil(t, nodeResp.ScriptSendCommand)
	assert.Equal(t, "server-command {id} {command}", *nodeResp.ScriptSendCommand)
	require.NotNil(t, nodeResp.ScriptDelete)
	assert.Equal(t, "delete.sh", *nodeResp.ScriptDelete)
	assert.NotNil(t, nodeResp.CreatedAt)
	assert.NotNil(t, nodeResp.UpdatedAt)
}

func TestHandler_NewHandler(t *testing.T) {
	nodesRepo := inmemory.NewNodeRepository()
	responder := api.NewResponder()

	handler := NewHandler(nodesRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, nodesRepo, handler.nodesRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestNewNodeResponseFromNode(t *testing.T) {
	now := time.Now()
	provider := "DigitalOcean"
	ram := "2048"
	cpu := "2"
	steamcmdPath := "/opt/steamcmd"
	scriptInstall := "install.sh"

	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "NYC",
		Provider:            &provider,
		IPs:                 []string{"10.0.0.1"},
		RAM:                 &ram,
		CPU:                 &cpu,
		WorkPath:            "/var/gameap",
		SteamcmdPath:        &steamcmdPath,
		GdaemonHost:         "10.0.0.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/server.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		ScriptInstall:       &scriptInstall,
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	response := newNodeResponseFromNode(node)

	assert.Equal(t, uint(1), response.ID)
	assert.True(t, response.Enabled)
	assert.Equal(t, "test-node", response.Name)
	assert.Equal(t, "linux", response.OS)
	assert.Equal(t, "NYC", response.Location)
	require.NotNil(t, response.Provider)
	assert.Equal(t, "DigitalOcean", *response.Provider)
	require.Len(t, response.IPs, 1)
	assert.Equal(t, "10.0.0.1", response.IPs[0])
	require.NotNil(t, response.RAM)
	assert.Equal(t, "2048", *response.RAM)
	require.NotNil(t, response.CPU)
	assert.Equal(t, "2", *response.CPU)
	assert.Equal(t, "/var/gameap", response.WorkPath)
	require.NotNil(t, response.SteamcmdPath)
	assert.Equal(t, "/opt/steamcmd", *response.SteamcmdPath)
	assert.Equal(t, "10.0.0.1", response.GdaemonHost)
	assert.Equal(t, 31717, response.GdaemonPort)
	assert.Equal(t, "test-api-key", response.GdaemonAPIKey)
	assert.Equal(t, "certs/server.crt", response.GdaemonServerCert)
	assert.Equal(t, uint(1), response.ClientCertificateID)
	assert.Equal(t, "auto", response.PreferInstallMethod)
	require.NotNil(t, response.ScriptInstall)
	assert.Equal(t, "install.sh", *response.ScriptInstall)
	assert.Equal(t, &now, response.CreatedAt)
	assert.Equal(t, &now, response.UpdatedAt)
}

func TestNewNodeResponseFromNode_EmptyIP(t *testing.T) {
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "NYC",
		IPs:                 []string{},
		WorkPath:            "/var/gameap",
		GdaemonHost:         "10.0.0.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certs/server.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
	}

	response := newNodeResponseFromNode(node)

	assert.Equal(t, uint(1), response.ID)
	assert.Empty(t, response.IPs)
}

func TestNewNodeResponseFromNode_NullableFields(t *testing.T) {
	now := time.Now()

	node := &domain.Node{
		ID:                  1,
		Enabled:             false,
		Name:                "minimal-node",
		OS:                  "windows",
		Location:            "EU",
		IPs:                 []string{"192.168.1.1"},
		WorkPath:            "C:\\gameap",
		GdaemonHost:         "192.168.1.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "minimal-key",
		GdaemonServerCert:   "certs/minimal.crt",
		ClientCertificateID: 2,
		PreferInstallMethod: "manual",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	response := newNodeResponseFromNode(node)

	assert.Equal(t, uint(1), response.ID)
	assert.False(t, response.Enabled)
	assert.Equal(t, "minimal-node", response.Name)
	assert.Equal(t, "windows", response.OS)
	assert.Nil(t, response.Provider)
	assert.Nil(t, response.RAM)
	assert.Nil(t, response.CPU)
	assert.Nil(t, response.SteamcmdPath)
	assert.Nil(t, response.GdaemonLogin)
	assert.Nil(t, response.GdaemonPassword)
	assert.Nil(t, response.ScriptInstall)
	assert.Nil(t, response.ScriptReinstall)
	assert.Nil(t, response.ScriptUpdate)
	assert.Nil(t, response.ScriptStart)
	assert.Nil(t, response.ScriptPause)
	assert.Nil(t, response.ScriptUnpause)
	assert.Nil(t, response.ScriptStop)
	assert.Nil(t, response.ScriptKill)
	assert.Nil(t, response.ScriptRestart)
	assert.Nil(t, response.ScriptStatus)
	assert.Nil(t, response.ScriptStats)
	assert.Nil(t, response.ScriptGetConsole)
	assert.Nil(t, response.ScriptSendCommand)
	assert.Nil(t, response.ScriptDelete)
	assert.Nil(t, response.DeletedAt)
}
