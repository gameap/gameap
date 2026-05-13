package postserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		wantError      string
	}{
		{
			name: "valid server creation",
			requestBody: `{
				"name": "My CS Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "valid server creation with all optional fields",
			requestBody: `{
				"install": true,
				"name": "My CS Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"query_port": 27016,
				"rcon_port": 27017,
				"dir": "servers/cs-server"
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "missing name",
			requestBody: `{
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "name is required",
		},
		{
			name: "empty name",
			requestBody: `{
				"name": "",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "name is required",
		},
		{
			name: "name too long",
			requestBody: `{
				"name": "` + strings.Repeat("a", 129) + `",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "name must not exceed 128 characters",
		},
		{
			name: "missing game_id",
			requestBody: `{
				"name": "My Server",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game_id is required",
		},
		{
			name: "empty game_id",
			requestBody: `{
				"name": "My Server",
				"game_id": "",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game_id is required",
		},
		{
			name: "missing ds_id",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "ds_id is required",
		},
		{
			name: "invalid ds_id (zero)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 0,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "ds_id is required",
		},
		{
			name: "invalid ds_id (negative)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": -1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "ds_id is required",
		},
		{
			name: "missing game_mod_id",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game_mod_id is required",
		},
		{
			name: "invalid game_mod_id (zero)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 0,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game_mod_id is required",
		},
		{
			name: "invalid game_mod_id (negative)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": -1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game_mod_id is required",
		},
		{
			name: "missing server_ip",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_ip is required",
		},
		{
			name: "empty server_ip",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_ip is required",
		},
		{
			name: "invalid server_ip",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "not_valid!!!",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_ip is not a valid IP address or hostname",
		},
		{
			name: "invalid server_ip format",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.999",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_ip is not a valid IP address or hostname",
		},
		{
			name: "missing server_port",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_port must be between 1 and 65535",
		},
		{
			name: "invalid server_port (zero)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 0
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_port must be between 1 and 65535",
		},
		{
			name: "invalid server_port (negative)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": -1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_port must be between 1 and 65535",
		},
		{
			name: "invalid server_port (too high)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 65536
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_port must be between 1 and 65535",
		},
		{
			name: "invalid query_port (zero)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"query_port": 0
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "query_port must be between 1 and 65535",
		},
		{
			name: "invalid query_port (too high)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"query_port": 65536
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "query_port must be between 1 and 65535",
		},
		{
			name: "invalid rcon_port (zero)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"rcon_port": 0
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "rcon_port must be between 1 and 65535",
		},
		{
			name: "invalid rcon_port (too high)",
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"rcon_port": 65536
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "rcon_port must be between 1 and 65535",
		},
		{
			name:           "invalid JSON",
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
		{
			name: "IPv6 address",
			requestBody: `{
				"name": "IPv6 Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "IPv6 address short form",
			requestBody: `{
				"name": "IPv6 Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "::1",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "install flag false",
			requestBody: `{
				"install": false,
				"name": "Not Installed Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "empty dir",
			requestBody: `{
				"name": "Server with empty dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": ""
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "dir_with_leading_slash_is_rejected",
			requestBody: `{
				"name": "Server with absolute dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "/srv/gameap/servers/cs"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name: "dir_with_windows_drive_letter_is_rejected",
			requestBody: `{
				"name": "Server with windows path",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "C:\\gameap\\servers\\cs"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name: "dir_with_dot_dot_segment_is_rejected",
			requestBody: `{
				"name": "Server with traversal dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "../etc/passwd"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name: "valid hostname",
			requestBody: `{
				"name": "Server with hostname",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "hldm.org",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "valid subdomain hostname",
			requestBody: `{
				"name": "Server with subdomain",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "game.example.com",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "valid hostname with hyphen",
			requestBody: `{
				"name": "Server with hyphenated hostname",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "game-server.example.com",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			serverSettingsRepo := inmemory.NewServerSettingRepository()
			responder := api.NewResponder()

			_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
			_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
			_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

			handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else {
				require.Equal(t, http.StatusCreated, w.Code)

				var response createServerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

				// Verify the server was saved to repository
				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)

				// Verify response contains expected fields
				assert.Equal(t, "success", response.Message)
				assert.NotEmpty(t, response.Result.ServerID)
			}
		})
	}
}

func TestHandler_ServerPersistence(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

	serverData := map[string]any{
		"install":     true,
		"name":        "Test CS Server",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "192.168.1.100",
		"server_port": 27015,
		"query_port":  27016,
		"rcon_port":   27017,
		"dir":         "servers/test-cs",
	}

	body, err := json.Marshal(serverData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusCreated, w.Code)

	var response createServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	servers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	server := servers[0]
	assert.Equal(t, "Test CS Server", server.Name)
	assert.Equal(t, "cstrike", server.GameID)
	assert.Equal(t, uint(1), server.DSID)
	assert.Equal(t, uint(1), server.GameModID)
	assert.Equal(t, "192.168.1.100", server.ServerIP)
	assert.Equal(t, 27015, server.ServerPort)
	assert.Equal(t, 27016, *server.QueryPort)
	assert.Equal(t, 27017, *server.RconPort)
	assert.Equal(t, "servers/test-cs", server.Dir)
	assert.Equal(t, domain.ServerInstalledStatusNotInstalled, server.Installed)
	assert.True(t, server.Enabled)
	assert.False(t, server.Blocked)
	assert.NotEmpty(t, server.UUID)
	assert.NotEmpty(t, server.UUIDShort)
	assert.NotNil(t, server.Rcon)

	// Verify response matches saved server
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, server.ID, response.Result.ServerID)
	assert.NotEmpty(t, response.Result.TaskID)
}

func TestHandler_MultipleServers(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 2, OS: "windows"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "valve"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 2, GameCode: "valve"})

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

	servers := []map[string]any{
		{
			"name":        "Server 1",
			"game_id":     "cstrike",
			"ds_id":       1,
			"game_mod_id": 1,
			"server_ip":   "192.168.1.100",
			"server_port": 27015,
		},
		{
			"name":        "Server 2",
			"game_id":     "valve",
			"ds_id":       2,
			"game_mod_id": 2,
			"server_ip":   "192.168.1.101",
			"server_port": 27016,
		},
	}

	// ACT & ASSERT
	for i, serverData := range servers {
		body, err := json.Marshal(serverData)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		allServers, err := serverRepo.FindAll(context.Background(), nil, nil)
		require.NoError(t, err)
		require.Len(t, allServers, i+1)
	}

	// Verify all servers were saved correctly
	allServers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allServers, 2)

	assert.Equal(t, "Server 1", allServers[0].Name)
	assert.Equal(t, "Server 2", allServers[1].Name)
}

func TestHandler_ServerWithSettings(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{
		ID:       1,
		GameCode: "cstrike",
		Vars: []domain.GameModVar{
			{Var: "maxplayers"},
			{Var: "hostname"},
		},
	})

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

	serverData := map[string]any{
		"name":        "Server with settings",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "192.168.1.100",
		"server_port": 27015,
		"settings": []map[string]any{
			{"name": "autostart", "value": true},
			{"name": "maxplayers", "value": "32"},
			{"name": "hostname", "value": "My Server"},
		},
	}

	body, err := json.Marshal(serverData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusCreated, w.Code)

	var response createServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	servers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	settings, err := serverSettingsRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, settings, 3)

	settingsMap := make(map[string]domain.ServerSettingValue)
	for _, s := range settings {
		settingsMap[s.Name] = s.Value
	}

	autostartVal, ok := settingsMap["autostart"].Bool()
	require.True(t, ok)
	assert.Equal(t, true, autostartVal)

	maxplayersVal, ok := settingsMap["maxplayers"].String()
	require.True(t, ok)
	assert.Equal(t, "32", maxplayersVal)

	hostnameVal, ok := settingsMap["hostname"].String()
	require.True(t, ok)
	assert.Equal(t, "My Server", hostnameVal)
}

func TestHandler_ServerWithoutSettings_BackwardCompatibility(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

	serverData := map[string]any{
		"name":        "Server without settings",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "192.168.1.100",
		"server_port": 27015,
	}

	body, err := json.Marshal(serverData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusCreated, w.Code)

	servers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	settings, err := serverSettingsRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestHandler_SettingEmptyName_ValidationError(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

	serverData := map[string]any{
		"name":        "Server with invalid setting",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "192.168.1.100",
		"server_port": 27015,
		"settings": []map[string]any{
			{"name": "", "value": "some value"},
		},
	}

	body, err := json.Marshal(serverData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"], "setting name is required")
}

func TestHandler_DisallowedSettings_Ignored(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{
		ID:       1,
		GameCode: "cstrike",
		Vars: []domain.GameModVar{
			{Var: "maxplayers"},
		},
	})

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

	serverData := map[string]any{
		"name":        "Server with disallowed settings",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "192.168.1.100",
		"server_port": 27015,
		"settings": []map[string]any{
			{"name": "autostart", "value": true},
			{"name": "maxplayers", "value": "32"},
			{"name": "unknown_setting", "value": "ignored"},
			{"name": "another_unknown", "value": "also_ignored"},
		},
	}

	body, err := json.Marshal(serverData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusCreated, w.Code)

	servers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	settings, err := serverSettingsRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, settings, 2)

	settingsMap := make(map[string]domain.ServerSettingValue)
	for _, s := range settings {
		settingsMap[s.Name] = s.Value
	}

	autostartVal, ok := settingsMap["autostart"].Bool()
	require.True(t, ok)
	assert.Equal(t, true, autostartVal)

	maxplayersVal, ok := settingsMap["maxplayers"].String()
	require.True(t, ok)
	assert.Equal(t, "32", maxplayersVal)

	_, hasUnknown := settingsMap["unknown_setting"]
	assert.False(t, hasUnknown, "unknown_setting should not be saved")
}

func TestHandler_GameModBelongsToGame_Validation(t *testing.T) {
	tests := []struct {
		name              string
		setupRepo         func(nodeRepo *inmemory.NodeRepository, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository)
		requestBody       string
		expectedStatus    int
		wantError         string
		expectedGameID    string
		expectedGameModID uint
	}{
		{
			name: "game_mod_belongs_to_different_game",
			setupRepo: func(nodeRepo *inmemory.NodeRepository, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "valve"})
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "valve",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game mod does not belong to the specified game",
		},
		{
			name: "wrong_mod_selected_among_many",
			setupRepo: func(nodeRepo *inmemory.NodeRepository, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "valve"})
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 2, GameCode: "valve"})
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "valve",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game mod does not belong to the specified game",
		},
		{
			name: "correct_mod_selected_among_many",
			setupRepo: func(nodeRepo *inmemory.NodeRepository, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "valve"})
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 2, GameCode: "valve"})
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "valve",
				"ds_id": 1,
				"game_mod_id": 2,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			expectedStatus:    http.StatusCreated,
			expectedGameID:    "valve",
			expectedGameModID: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			serverSettingsRepo := inmemory.NewServerSettingRepository()
			responder := api.NewResponder()

			tt.setupRepo(nodeRepo, gameRepo, gameModRepo)

			handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
				}

				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, servers, "no server must be saved when validation fails")
			} else {
				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)
				assert.Equal(t, tt.expectedGameID, servers[0].GameID)
				assert.Equal(t, tt.expectedGameModID, servers[0].GameModID)
			}
		})
	}
}

func TestHandler_PrepareServerErrors(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository)
		requestBody string
		wantError   string
	}{
		{
			name: "node_not_found",
			setupRepo: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				nodeRepo := inmemory.NewNodeRepository()
				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				return nodeRepo, gameRepo, gameModRepo
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 999,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError: "Internal Server Error",
		},
		{
			name: "node_repo_returns_error",
			setupRepo: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				return &errNodeRepo{
					NodeRepository: inmemory.NewNodeRepository(),
					findErr:        pkgerrors.New("db down"),
				}, gameRepo, gameModRepo
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError: "Internal Server Error",
		},
		{
			name: "game_not_found",
			setupRepo: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				return nodeRepo, inmemory.NewGameRepository(), gameModRepo
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError: "Internal Server Error",
		},
		{
			name: "game_repo_returns_error",
			setupRepo: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				return nodeRepo, &errGameRepo{
					GameRepository: inmemory.NewGameRepository(),
					findErr:        pkgerrors.New("db down"),
				}, gameModRepo
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError: "Internal Server Error",
		},
		{
			name: "game_mod_not_found",
			setupRepo: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})

				return nodeRepo, gameRepo, inmemory.NewGameModRepository()
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 999,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError: "Internal Server Error",
		},
		{
			name: "game_mod_repo_returns_error",
			setupRepo: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})

				return nodeRepo, gameRepo, &errGameModRepo{
					GameModRepository: inmemory.NewGameModRepository(),
					findErr:           pkgerrors.New("db down"),
				}
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError: "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			nodeRepo, gameRepo, gameModRepo := tt.setupRepo()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			serverSettingsRepo := inmemory.NewServerSettingRepository()
			responder := api.NewResponder()

			handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

			req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, http.StatusInternalServerError, w.Code, "prepareServer errors must surface as 500")

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "error", response["status"])

			errorMsg, ok := response["error"].(string)
			require.True(t, ok, "error field must be a string")
			assert.Contains(t, errorMsg, tt.wantError, "error message mismatch")

			servers, err := serverRepo.FindAll(context.Background(), nil, nil)
			require.NoError(t, err)
			assert.Empty(t, servers, "no server must be saved when prepareServer fails")
		})
	}
}

func TestHandler_PersistenceErrors(t *testing.T) {
	tests := []struct {
		name           string
		buildHandler   func(t *testing.T) (*Handler, *inmemory.ServerRepository, *inmemory.DaemonTaskRepository, *stubTaskDispatcher)
		requestBody    string
		wantError      string
		assertSideFx   func(t *testing.T, serverRepo *inmemory.ServerRepository, taskRepo *inmemory.DaemonTaskRepository, dispatcher *stubTaskDispatcher)
		expectedStatus int
	}{
		{
			name: "server_repo_save_error",
			buildHandler: func(_ *testing.T) (*Handler, *inmemory.ServerRepository, *inmemory.DaemonTaskRepository, *stubTaskDispatcher) {
				serverRepo := inmemory.NewServerRepository()
				wrappedRepo := &errServerRepo{
					ServerRepository: serverRepo,
					saveErr:          pkgerrors.New("db down"),
				}

				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})

				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})

				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				daemonTaskRepo := inmemory.NewDaemonTaskRepository()
				serverSettingsRepo := inmemory.NewServerSettingRepository()
				responder := api.NewResponder()

				h := NewHandler(wrappedRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

				return h, serverRepo, daemonTaskRepo, nil
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError:      "Internal Server Error",
			expectedStatus: http.StatusInternalServerError,
			assertSideFx: func(t *testing.T, serverRepo *inmemory.ServerRepository, _ *inmemory.DaemonTaskRepository, _ *stubTaskDispatcher) {
				t.Helper()
				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, servers, "underlying repo must not have stored a server")
			},
		},
		{
			name: "settings_repo_save_error",
			buildHandler: func(_ *testing.T) (*Handler, *inmemory.ServerRepository, *inmemory.DaemonTaskRepository, *stubTaskDispatcher) {
				serverRepo := inmemory.NewServerRepository()

				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})

				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})

				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "cstrike",
					Vars: []domain.GameModVar{
						{Var: "maxplayers"},
					},
				})

				daemonTaskRepo := inmemory.NewDaemonTaskRepository()
				serverSettingsRepo := &errServerSettingsRepo{
					ServerSettingRepository: inmemory.NewServerSettingRepository(),
					saveErr:                 pkgerrors.New("db down"),
				}
				responder := api.NewResponder()

				h := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, nil, responder)

				return h, serverRepo, daemonTaskRepo, nil
			},
			requestBody: `{
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"settings": [
					{"name": "maxplayers", "value": "32"}
				]
			}`,
			wantError:      "Internal Server Error",
			expectedStatus: http.StatusInternalServerError,
			assertSideFx: func(t *testing.T, serverRepo *inmemory.ServerRepository, _ *inmemory.DaemonTaskRepository, _ *stubTaskDispatcher) {
				t.Helper()
				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1, "server must be persisted before settings save fails")
			},
		},
		{
			name: "daemon_task_repo_save_error",
			buildHandler: func(_ *testing.T) (*Handler, *inmemory.ServerRepository, *inmemory.DaemonTaskRepository, *stubTaskDispatcher) {
				serverRepo := inmemory.NewServerRepository()

				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})

				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})

				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				underlyingTaskRepo := inmemory.NewDaemonTaskRepository()
				wrappedTaskRepo := &errDaemonTaskRepo{
					DaemonTaskRepository: underlyingTaskRepo,
					saveErr:              pkgerrors.New("db down"),
				}

				serverSettingsRepo := inmemory.NewServerSettingRepository()
				responder := api.NewResponder()

				h := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, wrappedTaskRepo, serverSettingsRepo, nil, responder)

				return h, serverRepo, underlyingTaskRepo, nil
			},
			requestBody: `{
				"install": true,
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError:      "Internal Server Error",
			expectedStatus: http.StatusInternalServerError,
			assertSideFx: func(t *testing.T, serverRepo *inmemory.ServerRepository, taskRepo *inmemory.DaemonTaskRepository, _ *stubTaskDispatcher) {
				t.Helper()
				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1, "server must be persisted before daemon task creation is attempted")

				tasks, err := taskRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, tasks, "no daemon task must remain after failing repo save")
			},
		},
		{
			name: "task_dispatcher_dispatch_error",
			buildHandler: func(_ *testing.T) (*Handler, *inmemory.ServerRepository, *inmemory.DaemonTaskRepository, *stubTaskDispatcher) {
				serverRepo := inmemory.NewServerRepository()

				nodeRepo := inmemory.NewNodeRepository()
				_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})

				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})

				gameModRepo := inmemory.NewGameModRepository()
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

				daemonTaskRepo := inmemory.NewDaemonTaskRepository()
				serverSettingsRepo := inmemory.NewServerSettingRepository()
				responder := api.NewResponder()

				dispatcher := &stubTaskDispatcher{err: pkgerrors.New("grpc down")}

				h := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, dispatcher, responder)

				return h, serverRepo, daemonTaskRepo, dispatcher
			},
			requestBody: `{
				"install": true,
				"name": "My Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015
			}`,
			wantError:      "Internal Server Error",
			expectedStatus: http.StatusInternalServerError,
			assertSideFx: func(t *testing.T, serverRepo *inmemory.ServerRepository, taskRepo *inmemory.DaemonTaskRepository, dispatcher *stubTaskDispatcher) {
				t.Helper()
				servers, err := serverRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1, "server must be persisted before dispatch is attempted")

				tasks, err := taskRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, tasks, "dispatcher path must not write to repo")

				require.NotNil(t, dispatcher.dispatched, "dispatcher must have been invoked")
				assert.Equal(t, domain.DaemonTaskTypeServerInstall, dispatcher.dispatched.Task)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			handler, serverRepo, taskRepo, dispatcher := tt.buildHandler(t)

			req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "error", response["status"])

			errorMsg, ok := response["error"].(string)
			require.True(t, ok, "error field must be a string")
			assert.Contains(t, errorMsg, tt.wantError, "error body mismatch")

			if tt.assertSideFx != nil {
				tt.assertSideFx(t, serverRepo, taskRepo, dispatcher)
			}
		})
	}
}

func TestHandler_TaskDispatcher_Success(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingsRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

	dispatcher := &stubTaskDispatcher{}

	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, daemonTaskRepo, serverSettingsRepo, dispatcher, responder)

	serverData := map[string]any{
		"install":     true,
		"name":        "Dispatched CS Server",
		"game_id":     "cstrike",
		"ds_id":       1,
		"game_mod_id": 1,
		"server_ip":   "192.168.1.100",
		"server_port": 27015,
	}

	body, err := json.Marshal(serverData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusCreated, w.Code)

	var response createServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "success", response.Message)
	assert.Equal(t, uint(stubDispatchedTaskID), response.Result.TaskID, "dispatcher-assigned task ID must be in response")

	servers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)
	assert.Equal(t, servers[0].ID, response.Result.ServerID)

	require.NotNil(t, dispatcher.dispatched, "dispatcher must have been invoked")
	assert.Equal(t, domain.DaemonTaskTypeServerInstall, dispatcher.dispatched.Task)
	assert.Equal(t, domain.DaemonTaskStatusWaiting, dispatcher.dispatched.Status)
	require.NotNil(t, dispatcher.dispatched.ServerID, "ServerID must be populated on the dispatched task")
	assert.Equal(t, servers[0].ID, *dispatcher.dispatched.ServerID, "dispatched task must reference the saved server")

	tasks, err := daemonTaskRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, tasks, "dispatcher path must bypass the repository")
}

const stubDispatchedTaskID = 42

type stubTaskDispatcher struct {
	err        error
	dispatched *domain.DaemonTask
}

func (s *stubTaskDispatcher) Dispatch(_ context.Context, task *domain.DaemonTask) error {
	s.dispatched = task

	if s.err != nil {
		return s.err
	}

	task.ID = stubDispatchedTaskID

	return nil
}

type errServerRepo struct {
	repositories.ServerRepository

	saveErr error
}

func (r *errServerRepo) Save(ctx context.Context, s *domain.Server) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.ServerRepository.Save(ctx, s)
}

type errNodeRepo struct {
	repositories.NodeRepository

	findErr error
}

func (r *errNodeRepo) Find(
	ctx context.Context,
	filter *filters.FindNode,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}

	return r.NodeRepository.Find(ctx, filter, order, pagination)
}

type errGameRepo struct {
	repositories.GameRepository

	findErr error
}

func (r *errGameRepo) Find(
	ctx context.Context,
	filter *filters.FindGame,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}

	return r.GameRepository.Find(ctx, filter, order, pagination)
}

type errGameModRepo struct {
	repositories.GameModRepository

	findErr error
}

func (r *errGameModRepo) Find(
	ctx context.Context,
	filter *filters.FindGameMod,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}

	return r.GameModRepository.Find(ctx, filter, order, pagination)
}

type errDaemonTaskRepo struct {
	repositories.DaemonTaskRepository

	saveErr error
}

func (r *errDaemonTaskRepo) Save(ctx context.Context, task *domain.DaemonTask) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.DaemonTaskRepository.Save(ctx, task)
}

type errServerSettingsRepo struct {
	repositories.ServerSettingRepository

	saveErr error
}

func (r *errServerSettingsRepo) Save(ctx context.Context, setting *domain.ServerSetting) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.ServerSettingRepository.Save(ctx, setting)
}
