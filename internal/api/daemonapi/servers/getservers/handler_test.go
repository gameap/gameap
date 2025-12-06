package getservers

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
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.GameModRepository, *inmemory.ServerSettingRepository) context.Context
		setupQuery     string
		expectedStatus int
		wantError      string
		expectServers  int
	}{
		{
			name: "successful servers retrieval",
			setupContext: func(
				serverRepo *inmemory.ServerRepository,
				gameRepo *inmemory.GameRepository,
				gameModRepo *inmemory.GameModRepository,
				serverSettingRepo *inmemory.ServerSettingRepository,
			) context.Context {
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

				game := &domain.Game{
					Code:                  "test",
					Name:                  "Test Game",
					Engine:                "source",
					EngineVersion:         "1.0",
					RemoteRepositoryLinux: lo.ToPtr("http://example.com/repo"),
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				gameMod := &domain.GameMod{
					ID:                    1,
					GameCode:              "test",
					Name:                  "Test Mod",
					StartCmdLinux:         lo.ToPtr("./start.sh"),
					RemoteRepositoryLinux: lo.ToPtr("http://example.com/mod"),
				}
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

				serverUUID := uuid.New()
				server := &domain.Server{
					ID:            1,
					UUID:          serverUUID,
					UUIDShort:     serverUUID.String()[:8],
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				setting := &domain.ServerSetting{
					ID:       1,
					ServerID: 1,
					Name:     "autostart",
					Value:    domain.NewServerSettingValue(true),
				}
				require.NoError(t, serverSettingRepo.Save(context.Background(), setting))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "",
			expectedStatus: http.StatusOK,
			expectServers:  1,
		},
		{
			name: "filter by server ID",
			setupContext: func(
				serverRepo *inmemory.ServerRepository,
				gameRepo *inmemory.GameRepository,
				gameModRepo *inmemory.GameModRepository,
				_ *inmemory.ServerSettingRepository,
			) context.Context {
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

				game := &domain.Game{
					Code:   "test",
					Name:   "Test Game",
					Engine: "source",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "test",
					Name:     "Test Mod",
				}
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

				for i := 1; i <= 3; i++ {
					serverUUID := uuid.New()
					server := &domain.Server{
						ID:            uint(i),
						UUID:          serverUUID,
						UUIDShort:     serverUUID.String()[:8],
						Enabled:       true,
						Installed:     1,
						Blocked:       false,
						Name:          "Test Server",
						GameID:        "test",
						DSID:          1,
						GameModID:     1,
						ServerIP:      "127.0.0.1",
						ServerPort:    27015 + i,
						Dir:           "/servers/test",
						ProcessActive: false,
						CreatedAt:     &now,
						UpdatedAt:     &now,
					}
					require.NoError(t, serverRepo.Save(context.Background(), server))
				}

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "?filter[id]=2",
			expectedStatus: http.StatusOK,
			expectServers:  1,
		},
		{
			name: "daemon session not found",
			setupContext: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameRepository,
				_ *inmemory.GameModRepository,
				_ *inmemory.ServerSettingRepository,
			) context.Context {
				return context.Background()
			},
			setupQuery:     "",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectServers:  0,
		},
		{
			name: "daemon session with nil node",
			setupContext: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameRepository,
				_ *inmemory.GameModRepository,
				_ *inmemory.ServerSettingRepository,
			) context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectServers:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			serverSettingRepo := inmemory.NewServerSettingRepository()
			responder := api.NewResponder()

			handler := NewHandler(
				serverRepo,
				gameRepo,
				gameModRepo,
				serverSettingRepo,
				responder,
			)

			ctx := tt.setupContext(serverRepo, gameRepo, gameModRepo, serverSettingRepo)

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers"+tt.setupQuery, nil)
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

			if tt.expectServers > 0 {
				var response []ServerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response, tt.expectServers)
			}
		})
	}
}

func TestHandler_ResponseStructure(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		serverRepo,
		gameRepo,
		gameModRepo,
		serverSettingRepo,
		responder,
	)

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

	steamAppIDLinux := uint(90)
	game := &domain.Game{
		Code:                  "cs16",
		Name:                  "Counter-Strike 1.6",
		Engine:                "goldsrc",
		EngineVersion:         "48",
		SteamAppIDLinux:       &steamAppIDLinux,
		RemoteRepositoryLinux: lo.ToPtr("http://example.com/repo"),
		LocalRepositoryLinux:  lo.ToPtr("/var/local/repo"),
	}
	require.NoError(t, gameRepo.Save(context.Background(), game))

	gameMod := &domain.GameMod{
		ID:                    1,
		GameCode:              "cs16",
		Name:                  "Classic",
		StartCmdLinux:         lo.ToPtr("./hlds_run -game cstrike"),
		StartCmdWindows:       lo.ToPtr("hlds.exe -game cstrike"),
		RemoteRepositoryLinux: lo.ToPtr("http://example.com/mod"),
		LocalRepositoryLinux:  lo.ToPtr("/var/local/mod"),
	}
	require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

	serverUUID := uuid.New()
	queryPort := 27016
	rcon := "test_rcon"
	server := &domain.Server{
		ID:            1,
		UUID:          serverUUID,
		UUIDShort:     serverUUID.String()[:8],
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test CS 1.6 Server",
		GameID:        "cs16",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		QueryPort:     &queryPort,
		Rcon:          &rcon,
		Dir:           "/servers/cs16",
		ProcessActive: true,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))

	setting := &domain.ServerSetting{
		ID:       1,
		ServerID: 1,
		Name:     "autostart",
		Value:    domain.NewServerSettingValue(true),
	}
	require.NoError(t, serverSettingRepo.Save(context.Background(), setting))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []ServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)

	serverResp := response[0]
	assert.Equal(t, uint(1), serverResp.ID)
	assert.Equal(t, serverUUID, serverResp.UUID)
	assert.True(t, serverResp.Enabled)
	assert.Equal(t, domain.ServerInstalledStatusInstalled, serverResp.Installed)
	assert.False(t, serverResp.Blocked)
	assert.Equal(t, "Test CS 1.6 Server", serverResp.Name)
	assert.Equal(t, "cs16", serverResp.GameID)
	assert.Equal(t, uint(1), serverResp.DSID)
	assert.Equal(t, uint(1), serverResp.GameModID)
	assert.Equal(t, "127.0.0.1", serverResp.ServerIP)
	assert.Equal(t, 27015, serverResp.ServerPort)
	require.NotNil(t, serverResp.QueryPort)
	assert.Equal(t, 27016, *serverResp.QueryPort)
	require.NotNil(t, serverResp.Rcon)
	assert.Equal(t, "test_rcon", *serverResp.Rcon)
	assert.Equal(t, "/servers/cs16", serverResp.Dir)
	assert.True(t, serverResp.ProcessActive)

	assert.Equal(t, "cs16", serverResp.Game.Code)
	assert.Equal(t, "Counter-Strike 1.6", serverResp.Game.Name)
	assert.Equal(t, "goldsrc", serverResp.Game.Engine)
	require.NotNil(t, serverResp.Game.SteamAppID)
	assert.Equal(t, uint(90), *serverResp.Game.SteamAppID)
	require.NotNil(t, serverResp.Game.RemoteRepository)
	assert.Equal(t, "http://example.com/repo", *serverResp.Game.RemoteRepository)

	assert.Equal(t, uint(1), serverResp.GameMod.ID)
	assert.Equal(t, "cs16", serverResp.GameMod.GameCode)
	assert.Equal(t, "Classic", serverResp.GameMod.Name)
	require.NotNil(t, serverResp.GameMod.DefaultStartCmd)
	assert.Equal(t, "./hlds_run -game cstrike", *serverResp.GameMod.DefaultStartCmd)
	require.NotNil(t, serverResp.GameMod.DefaultStartCmdLinux)
	assert.Equal(t, "./hlds_run -game cstrike", *serverResp.GameMod.DefaultStartCmdLinux)
	require.NotNil(t, serverResp.GameMod.DefaultStartCmdWindows)
	assert.Equal(t, "hlds.exe -game cstrike", *serverResp.GameMod.DefaultStartCmdWindows)
	require.NotNil(t, serverResp.GameMod.RemoteRepository)
	assert.Equal(t, "http://example.com/mod", *serverResp.GameMod.RemoteRepository)

	require.Len(t, serverResp.Settings, 1)
	assert.Equal(t, uint(1), serverResp.Settings[0].ServerID)
	assert.Equal(t, "autostart", serverResp.Settings[0].Name)
	assert.Equal(t, "true", serverResp.Settings[0].Value)
}

func TestHandler_WindowsOS(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		serverRepo,
		gameRepo,
		gameModRepo,
		serverSettingRepo,
		responder,
	)

	now := time.Now()
	node := &domain.Node{
		ID:                  1,
		OS:                  "windows",
		Enabled:             true,
		Name:                "test-node-windows",
		Location:            "US",
		IPs:                 []string{"192.168.1.1"},
		WorkPath:            "C:\\gameap",
		GdaemonHost:         "192.168.1.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	steamAppIDWindows := uint(90)
	game := &domain.Game{
		Code:                    "cs16",
		Name:                    "Counter-Strike 1.6",
		Engine:                  "goldsrc",
		SteamAppIDWindows:       &steamAppIDWindows,
		RemoteRepositoryWindows: lo.ToPtr("http://example.com/repo-win"),
		LocalRepositoryWindows:  lo.ToPtr("C:\\local\\repo"),
	}
	require.NoError(t, gameRepo.Save(context.Background(), game))

	gameMod := &domain.GameMod{
		ID:                      1,
		GameCode:                "cs16",
		Name:                    "Classic",
		StartCmdLinux:           lo.ToPtr("./hlds_run -game cstrike"),
		StartCmdWindows:         lo.ToPtr("hlds.exe -game cstrike"),
		RemoteRepositoryWindows: lo.ToPtr("http://example.com/mod-win"),
		LocalRepositoryWindows:  lo.ToPtr("C:\\local\\mod"),
	}
	require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

	serverUUID := uuid.New()
	server := &domain.Server{
		ID:            1,
		UUID:          serverUUID,
		UUIDShort:     serverUUID.String()[:8],
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test CS 1.6 Server",
		GameID:        "cs16",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "192.168.1.1",
		ServerPort:    27015,
		Dir:           "C:\\servers\\cs16",
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []ServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)

	serverResp := response[0]

	require.NotNil(t, serverResp.Game.RemoteRepository)
	assert.Equal(t, "http://example.com/repo-win", *serverResp.Game.RemoteRepository)
	require.NotNil(t, serverResp.Game.SteamAppID)
	assert.Equal(t, uint(90), *serverResp.Game.SteamAppID)

	require.NotNil(t, serverResp.GameMod.DefaultStartCmd)
	assert.Equal(t, "hlds.exe -game cstrike", *serverResp.GameMod.DefaultStartCmd)
	require.NotNil(t, serverResp.GameMod.DefaultStartCmdLinux)
	assert.Equal(t, "./hlds_run -game cstrike", *serverResp.GameMod.DefaultStartCmdLinux)
	require.NotNil(t, serverResp.GameMod.DefaultStartCmdWindows)
	assert.Equal(t, "hlds.exe -game cstrike", *serverResp.GameMod.DefaultStartCmdWindows)
	require.NotNil(t, serverResp.GameMod.RemoteRepository)
	assert.Equal(t, "http://example.com/mod-win", *serverResp.GameMod.RemoteRepository)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		serverRepo,
		gameRepo,
		gameModRepo,
		serverSettingRepo,
		responder,
	)

	require.NotNil(t, handler)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, gameRepo, handler.gameRepo)
	assert.Equal(t, gameModRepo, handler.gameModRepo)
	assert.Equal(t, serverSettingRepo, handler.serverSettingRepo)
	assert.Equal(t, responder, handler.responder)
}
