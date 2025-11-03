package postgamemod

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
	"github.com/samber/lo"
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
			name: "valid game mod creation",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"start_cmd_linux": "./hlds_run -game valve +map crossfire",
				"start_cmd_windows": "hlds.exe -game valve +map crossfire"
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing required name field",
			requestBody: `{
				"game_code": "valve",
				"start_cmd_linux": "./hlds_run -game valve"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game mod name is required",
		},
		{
			name: "missing required game_code field",
			requestBody: `{
				"name": "Default Mod",
				"start_cmd_linux": "./hlds_run -game valve"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game code is required",
		},
		{
			name: "name too long",
			requestBody: `{
				"game_code": "valve",
				"name": "` + strings.Repeat("a", 256) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game mod name must not exceed 255 characters",
		},
		{
			name: "game code too long",
			requestBody: `{
				"game_code": "` + strings.Repeat("a", 256) + `",
				"name": "Default"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game code must not exceed 255 characters",
		},
		{
			name: "start cmd linux too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"start_cmd_linux": "` + strings.Repeat("a", 1001) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "start command linux must not exceed 1000 characters",
		},
		{
			name: "start cmd windows too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"start_cmd_windows": "` + strings.Repeat("a", 1001) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "start command windows must not exceed 1000 characters",
		},
		{
			name:           "invalid JSON body",
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
		{
			name: "kick cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"kick_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "kick command must not exceed 200 characters",
		},
		{
			name: "ban cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"ban_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "ban command must not exceed 200 characters",
		},
		{
			name: "chname cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"chname_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "chname command must not exceed 200 characters",
		},
		{
			name: "srestart cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"srestart_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "srestart command must not exceed 200 characters",
		},
		{
			name: "chmap cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"chmap_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "chmap command must not exceed 200 characters",
		},
		{
			name: "sendmsg cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"sendmsg_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "sendmsg command must not exceed 200 characters",
		},
		{
			name: "passwd cmd too long",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"passwd_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "passwd command must not exceed 200 characters",
		},
		{
			name: "complete game mod with all fields",
			requestBody: `{
				"game_code": "valve",
				"name": "Default Valve Mod",
				"fast_rcon": [{"info":"Status","command":"status"},{"info":"Stats","command":"stats"}],
				"vars": [{"var":"default_map","default":"crossfire","info":"Default Map","admin_var":false}],
				"remote_repository_linux": "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz",
				"remote_repository_windows": "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip",
				"local_repository_linux": "/opt/steamcmd/valve",
				"local_repository_windows": "C:\\steamcmd\\valve",
				"start_cmd_linux": "./hlds_run -console -game valve +ip {ip} +port {port} +map {default_map} +maxplayers {maxplayers} +rcon_password {rcon_password}",
				"start_cmd_windows": "hlds.exe -console -game valve +ip {ip} +port {port} +map {default_map} +maxplayers {maxplayers} +rcon_password {rcon_password}",
				"kick_cmd": "kick #{id}",
				"ban_cmd": "ban #{id}",
				"chname_cmd": "hostname {name}",
				"srestart_cmd": "restart",
				"chmap_cmd": "changelevel {map}",
				"sendmsg_cmd": "say \"{msg}\"",
				"passwd_cmd": "password {password}"
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "minimal required fields only",
			requestBody: `{
				"game_code": "cstrike",
				"name": "Counter-Strike Default"
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "game mod with empty optional strings",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"fast_rcon": [],
				"vars": [],
				"kick_cmd": ""
			}`,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewGameModRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			body := []byte(tt.requestBody)

			req := httptest.NewRequest(http.MethodPost, "/api/game_mods", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else {
				require.Equal(t, http.StatusOK, w.Code)

				gameMods, err := repo.FindAll(context.Background(), nil, nil)
				if err != nil {
					t.Errorf("Failed to retrieve game mods from repository: %v", err)
				}
				if len(gameMods) == 0 {
					t.Error("Expected game mod to be saved to repository, but none found")
				}
			}
		})
	}
}

func TestHandler_GameModPersistence(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	gameModData := map[string]any{
		"game_code": "valve",
		"name":      "Half-Life Default",
		"fast_rcon": []map[string]any{
			{"info": "Status", "command": "status"},
		},
		"vars": []map[string]any{
			{"var": "default_map", "default": "crossfire", "info": "Default Map"},
		},
		"remote_repository_linux":   "https://example.com/hl/linux",
		"remote_repository_windows": "https://example.com/hl/windows",
		"local_repository_linux":    "/local/hl/linux",
		"local_repository_windows":  "C:\\local\\hl\\windows",
		"start_cmd_linux":           "./hlds_run -console -game valve +ip {ip} +port {port} +map {default_map}",
		"start_cmd_windows":         "hlds.exe -console -game valve +ip {ip} +port {port} +map {default_map}",
		"kick_cmd":                  "kick #{id}",
		"ban_cmd":                   "ban #{id}",
		"chname_cmd":                "hostname {name}",
		"srestart_cmd":              "restart",
		"chmap_cmd":                 "changelevel {map}",
		"sendmsg_cmd":               "say \"{msg}\"",
		"passwd_cmd":                "password {password}",
	}

	body, err := json.Marshal(gameModData)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/game_mods", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	gameMods, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, gameMods, 1)

	gameMod := gameMods[0]
	assert.Equal(t, "valve", gameMod.GameCode)
	assert.Equal(t, "Half-Life Default", gameMod.Name)
	assert.Equal(t, domain.GameModFastRconList{
		{
			Info:    "Status",
			Command: "status",
		},
	}, gameMod.FastRcon)
	assert.Equal(t, domain.GameModVarList{
		{
			Var:      "default_map",
			Default:  "crossfire",
			Info:     "Default Map",
			AdminVar: false,
		},
	}, gameMod.Vars)
	assert.Equal(t, "https://example.com/hl/linux", lo.FromPtr(gameMod.RemoteRepositoryLinux))
	assert.Equal(t, "https://example.com/hl/windows", lo.FromPtr(gameMod.RemoteRepositoryWindows))
	assert.Equal(t, "/local/hl/linux", lo.FromPtr(gameMod.LocalRepositoryLinux))
	assert.Equal(t, "C:\\local\\hl\\windows", lo.FromPtr(gameMod.LocalRepositoryWindows))
	assert.Equal(t, "./hlds_run -console -game valve +ip {ip} +port {port} +map {default_map}", lo.FromPtr(gameMod.StartCmdLinux))
	assert.Equal(t, "hlds.exe -console -game valve +ip {ip} +port {port} +map {default_map}", lo.FromPtr(gameMod.StartCmdWindows))
	assert.Equal(t, "kick #{id}", lo.FromPtr(gameMod.KickCmd))
	assert.Equal(t, "ban #{id}", lo.FromPtr(gameMod.BanCmd))
	assert.Equal(t, "hostname {name}", lo.FromPtr(gameMod.ChnameCmd))
	assert.Equal(t, "restart", lo.FromPtr(gameMod.SrestartCmd))
	assert.Equal(t, "changelevel {map}", lo.FromPtr(gameMod.ChmapCmd))
	assert.Equal(t, "say \"{msg}\"", lo.FromPtr(gameMod.SendmsgCmd))
	assert.Equal(t, "password {password}", lo.FromPtr(gameMod.PasswdCmd))
}

func TestHandler_EmptyRequiredFields(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantError   string
	}{
		{
			name: "empty name field",
			requestBody: `{
				"game_code": "valve",
				"name": ""
			}`,
			wantError: "game mod name is required",
		},
		{
			name: "empty game_code field",
			requestBody: `{
				"game_code": "",
				"name": "Default"
			}`,
			wantError: "game code is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewGameModRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/game_mods", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			assert.Equal(t, "error", response["status"])
			if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
				t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
			}
		})
	}
}
