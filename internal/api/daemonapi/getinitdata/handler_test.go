package getinitdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() context.Context
		expectedStatus int
		wantError      string
		expectData     bool
	}{
		{
			name: "successful init data retrieval",
			setupContext: func() context.Context {
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
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			expectedStatus: http.StatusOK,
			expectData:     true,
		},
		{
			name:           "daemon session not found",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectData:     false,
		},
		{
			name: "daemon session with nil node",
			setupContext: func() context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectData:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responder := api.NewResponder()
			handler := NewHandler(responder)

			ctx := context.Background()

			if tt.setupContext != nil {
				ctx = tt.setupContext()
			}

			req := httptest.NewRequest(http.MethodPost, "/gdaemon_api/dedicated_servers/get_init_data/1", nil)
			req = req.WithContext(ctx)
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

			if tt.expectData {
				var response initDataResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.WorkPath)
				assert.NotEmpty(t, response.PreferInstallMethod)
			}
		})
	}
}

func TestHandler_InitDataResponseFields(t *testing.T) {
	responder := api.NewResponder()
	handler := NewHandler(responder)

	now := time.Now()
	steamcmdPath := "/srv/gameap/steamcmd"
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
	scriptGetConsole := "server-output {id}"
	scriptSendCommand := "server-command {id} {command}"
	scriptDelete := "delete.sh"

	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		SteamcmdPath:        &steamcmdPath,
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
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
		ScriptGetConsole:    &scriptGetConsole,
		ScriptSendCommand:   &scriptSendCommand,
		ScriptDelete:        &scriptDelete,
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon_api/dedicated_servers/get_init_data/1", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response initDataResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "/srv/gameap", response.WorkPath)
	require.NotNil(t, response.SteamcmdPath)
	assert.Equal(t, "/srv/gameap/steamcmd", *response.SteamcmdPath)
	assert.Equal(t, "auto", response.PreferInstallMethod)
	require.NotNil(t, response.ScriptInstall)
	assert.Equal(t, "install.sh", *response.ScriptInstall)
	require.NotNil(t, response.ScriptReinstall)
	assert.Equal(t, "reinstall.sh", *response.ScriptReinstall)
	require.NotNil(t, response.ScriptUpdate)
	assert.Equal(t, "update.sh", *response.ScriptUpdate)
	require.NotNil(t, response.ScriptStart)
	assert.Equal(t, "start.sh", *response.ScriptStart)
	require.NotNil(t, response.ScriptPause)
	assert.Equal(t, "pause.sh", *response.ScriptPause)
	require.NotNil(t, response.ScriptUnpause)
	assert.Equal(t, "unpause.sh", *response.ScriptUnpause)
	require.NotNil(t, response.ScriptStop)
	assert.Equal(t, "stop.sh", *response.ScriptStop)
	require.NotNil(t, response.ScriptKill)
	assert.Equal(t, "kill.sh", *response.ScriptKill)
	require.NotNil(t, response.ScriptRestart)
	assert.Equal(t, "restart.sh", *response.ScriptRestart)
	require.NotNil(t, response.ScriptStatus)
	assert.Equal(t, "status.sh", *response.ScriptStatus)
	require.NotNil(t, response.ScriptGetConsole)
	assert.Equal(t, "server-output {id}", *response.ScriptGetConsole)
	require.NotNil(t, response.ScriptSendCommand)
	assert.Equal(t, "server-command {id} {command}", *response.ScriptSendCommand)
	require.NotNil(t, response.ScriptDelete)
	assert.Equal(t, "delete.sh", *response.ScriptDelete)
}

func TestHandler_NewHandler(t *testing.T) {
	responder := api.NewResponder()

	handler := NewHandler(responder)

	require.NotNil(t, handler)
	assert.Equal(t, responder, handler.responder)
}

func TestNewInitDataResponseFromNode(t *testing.T) {
	steamcmdPath := "/opt/steamcmd"
	scriptInstall := "install.sh"
	scriptStart := "start.sh"

	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "NYC",
		IPs:                 []string{"10.0.0.1"},
		WorkPath:            "/var/gameap",
		SteamcmdPath:        &steamcmdPath,
		GdaemonHost:         "10.0.0.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/server.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		ScriptInstall:       &scriptInstall,
		ScriptStart:         &scriptStart,
	}

	response := newInitDataResponseFromNode(node)

	assert.Equal(t, "/var/gameap", response.WorkPath)
	require.NotNil(t, response.SteamcmdPath)
	assert.Equal(t, "/opt/steamcmd", *response.SteamcmdPath)
	assert.Equal(t, "auto", response.PreferInstallMethod)
	require.NotNil(t, response.ScriptInstall)
	assert.Equal(t, "install.sh", *response.ScriptInstall)
	require.NotNil(t, response.ScriptStart)
	assert.Equal(t, "start.sh", *response.ScriptStart)
}

func TestNewInitDataResponseFromNode_NullableFields(t *testing.T) {
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
	}

	response := newInitDataResponseFromNode(node)

	assert.Equal(t, "C:\\gameap", response.WorkPath)
	assert.Equal(t, "manual", response.PreferInstallMethod)
	assert.Nil(t, response.SteamcmdPath)
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
	assert.Nil(t, response.ScriptGetConsole)
	assert.Nil(t, response.ScriptSendCommand)
	assert.Nil(t, response.ScriptDelete)
}

func TestHandler_ResponseJSON(t *testing.T) {
	responder := api.NewResponder()
	handler := NewHandler(responder)

	now := time.Now()
	steamcmdPath := "/srv/gameap/steamcmd"

	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		SteamcmdPath:        &steamcmdPath,
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	req := httptest.NewRequest(http.MethodPost, "/gdaemon_api/dedicated_servers/get_init_data/1", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var rawResponse map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rawResponse))

	workPath, workPathExists := rawResponse["work_path"]
	assert.True(t, workPathExists)
	assert.Equal(t, "/srv/gameap", workPath)

	steamcmd, steamcmdExists := rawResponse["steamcmd_path"]
	assert.True(t, steamcmdExists)
	assert.Equal(t, "/srv/gameap/steamcmd", steamcmd)

	preferInstallMethod, preferInstallMethodExists := rawResponse["prefer_install_method"]
	assert.True(t, preferInstallMethodExists)
	assert.Equal(t, "auto", preferInstallMethod)
}
