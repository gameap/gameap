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
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
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
				"server_ip": "not-an-ip",
				"server_port": 27015
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "server_ip is not a valid IPs address",
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
			wantError:      "server_ip is not a valid IPs address",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			gameModRepo := inmemory.NewGameModRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			responder := api.NewResponder()

			_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
			_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

			handler := NewHandler(serverRepo, nodeRepo, gameModRepo, daemonTaskRepo, responder)

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
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})

	handler := NewHandler(serverRepo, nodeRepo, gameModRepo, daemonTaskRepo, responder)

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
	gameModRepo := inmemory.NewGameModRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 1, OS: "linux"})
	_ = nodeRepo.Save(context.Background(), &domain.Node{ID: 2, OS: "windows"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})
	_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 2, GameCode: "valve"})

	handler := NewHandler(serverRepo, nodeRepo, gameModRepo, daemonTaskRepo, responder)

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
