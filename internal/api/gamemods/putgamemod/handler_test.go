package putgamemod

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmBase "github.com/gameap/gameap/internal/api/gamemods/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name        string
		gameModID   string
		requestBody string
		setupRepo   func(repo *inmemory.GameModRepository)
		wantStatus  int
		wantError   string
	}{
		{
			name:      "successful game mod update",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default Updated",
				"start_cmd_linux": "./hlds_run -game valve +map crossfire",
				"start_cmd_windows": "hlds.exe -game valve +map crossfire"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
				if err != nil {
					t.Fatalf("failed to setup test game mod: %v", err)
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:      "game mod not found",
			gameModID: "999",
			requestBody: `{
				"game_code": "valve",
				"name": "Non-Existent Mod",
				"start_cmd_linux": "./hlds_run -game valve"
			}`,
			setupRepo:  func(_ *inmemory.GameModRepository) {},
			wantStatus: http.StatusNotFound,
			wantError:  "game mod not found",
		},
		{
			name:      "missing required name field",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game mod name is required",
		},
		{
			name:      "missing required game_code field",
			gameModID: "1",
			requestBody: `{
				"name": "Default Mod"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game code is required",
		},
		{
			name:        "invalid JSON body",
			gameModID:   "1",
			requestBody: `{"invalid": json}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request",
		},
		{
			name:      "name too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "` + strings.Repeat("a", 256) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game mod name must not exceed 255 characters",
		},
		{
			name:      "game code too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "` + strings.Repeat("a", 256) + `",
				"name": "Default"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game code must not exceed 255 characters",
		},
		{
			name:      "start cmd linux too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"start_cmd_linux": "` + strings.Repeat("a", 1001) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "start command linux must not exceed 1000 characters",
		},
		{
			name:      "start cmd windows too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"start_cmd_windows": "` + strings.Repeat("a", 1001) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "start command windows must not exceed 1000 characters",
		},
		{
			name:      "kick cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"kick_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "kick command must not exceed 200 characters",
		},
		{
			name:      "ban cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"ban_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "ban command must not exceed 200 characters",
		},
		{
			name:      "chname cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"chname_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "chname command must not exceed 200 characters",
		},
		{
			name:      "srestart cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"srestart_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "srestart command must not exceed 200 characters",
		},
		{
			name:      "chmap cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"chmap_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "chmap command must not exceed 200 characters",
		},
		{
			name:      "sendmsg cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"sendmsg_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "sendmsg command must not exceed 200 characters",
		},
		{
			name:      "passwd cmd too long",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default",
				"passwd_cmd": "` + strings.Repeat("a", 201) + `"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "passwd command must not exceed 200 characters",
		},
		{
			name:      "update game mod with all fields",
			gameModID: "1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default Valve Mod Updated",
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
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:      "minimal required fields only",
			gameModID: "1",
			requestBody: `{
				"game_code": "cstrike",
				"name": "Counter-Strike Updated"
			}`,
			setupRepo: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					ID:       1,
					GameCode: "valve",
					Name:     "Default",
				})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:      "invalid game mod id - not a number",
			gameModID: "abc",
			requestBody: `{
				"game_code": "valve",
				"name": "Default"
			}`,
			setupRepo:  func(_ *inmemory.GameModRepository) {},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid game mod id",
		},
		{
			name:      "invalid game mod id - zero",
			gameModID: "0",
			requestBody: `{
				"game_code": "valve",
				"name": "Default"
			}`,
			setupRepo:  func(_ *inmemory.GameModRepository) {},
			wantStatus: http.StatusNotFound,
			wantError:  "game mod not found",
		},
		{
			name:      "invalid game mod id - negative",
			gameModID: "-1",
			requestBody: `{
				"game_code": "valve",
				"name": "Default"
			}`,
			setupRepo:  func(_ *inmemory.GameModRepository) {},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid game mod id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewGameModRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/api/game_mods/"+tt.gameModID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": tt.gameModID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantError != "" {
				var errorResponse map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errorResponse))
				assert.Equal(t, "error", errorResponse["status"])
				if errorMsg, ok := errorResponse["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("want error containing '%s', got: %v", tt.wantError, errorResponse["error"])
				}
			} else {
				require.Equal(t, http.StatusOK, w.Code)

				var response gmBase.GameModResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

				// Verify response has expected fields
				assert.NotZero(t, response.ID, "response should have non-zero ID")
				assert.NotEmpty(t, response.GameCode, "response should have game_code")
				assert.NotEmpty(t, response.Name, "response should have name")
			}
		})
	}
}

func TestHandler_GameModUpdatePersistence(t *testing.T) {
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	originalGameMod := &domain.GameMod{
		ID:       1,
		GameCode: "valve",
		Name:     "Half-Life Default",
	}

	err := repo.Save(context.Background(), originalGameMod)
	require.NoError(t, err)

	updateData := map[string]any{
		"game_code": "valve",
		"name":      "Half-Life Default Updated",
		"fast_rcon": []map[string]any{
			{"info": "Status", "command": "status"},
			{"info": "Stats", "command": "stats"},
		},
		"vars": []map[string]any{
			{"var": "default_map", "default": "crossfire", "info": "Default Map", "admin_var": false},
			{"var": "maxplayers", "default": "32", "info": "Max Players", "admin_var": true},
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

	body, err := json.Marshal(updateData)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/game_mods/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify response structure
	var response gmBase.GameModResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "valve", response.GameCode)
	assert.Equal(t, "Half-Life Default Updated", response.Name)

	// Verify data was persisted correctly
	gameMods, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, gameMods, 1)

	gameMod := gameMods[0]
	assert.Equal(t, uint(1), gameMod.ID)
	assert.Equal(t, "valve", gameMod.GameCode)
	assert.Equal(t, "Half-Life Default Updated", gameMod.Name)
	assert.Equal(t, domain.GameModFastRconList{
		{
			Info:    "Status",
			Command: "status",
		},
		{
			Info:    "Stats",
			Command: "stats",
		},
	}, gameMod.FastRcon)
	assert.Equal(t, domain.GameModVarList{
		{
			Var:      "default_map",
			Default:  "crossfire",
			Info:     "Default Map",
			AdminVar: false,
		},
		{
			Var:      "maxplayers",
			Default:  "32",
			Info:     "Max Players",
			AdminVar: true,
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

func TestHandler_EmptyGameModID(t *testing.T) {
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	requestBody := `{
		"game_code": "valve",
		"name": "Default"
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/game_mods/", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": ""})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"].(string), "invalid game mod id")
}

func TestHandler_GameModChangeGameCode(t *testing.T) {
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	originalGameMod := &domain.GameMod{
		ID:       1,
		GameCode: "valve",
		Name:     "Valve Default",
	}

	err := repo.Save(context.Background(), originalGameMod)
	require.NoError(t, err)

	updateData := map[string]any{
		"game_code": "cstrike",
		"name":      "Counter-Strike Default",
	}

	body, err := json.Marshal(updateData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/game_mods/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify response structure
	var response gmBase.GameModResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "cstrike", response.GameCode)
	assert.Equal(t, "Counter-Strike Default", response.Name)

	// Verify data was persisted correctly
	gameMods, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, gameMods, 1)

	gameMod := gameMods[0]
	assert.Equal(t, uint(1), gameMod.ID)
	assert.Equal(t, "cstrike", gameMod.GameCode)
	assert.Equal(t, "Counter-Strike Default", gameMod.Name)
}
