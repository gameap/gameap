package putserver

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
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name        string
		serverID    string
		requestBody string
		setupRepo   func(repo *inmemory.ServerRepository)
		setupAuth   func(ctx context.Context) context.Context
		wantStatus  int
		wantError   string
	}{
		{
			name:     "successful server update",
			serverID: "1",
			requestBody: `{
				"enabled": 1,
				"installed": 1,
				"blocked": 0,
				"name": "Updated Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"query_port": 27016,
				"rcon_port": 27017
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
				if err != nil {
					t.Fatalf("failed to setup test server: %v", err)
				}
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "server not found",
			serverID: "999",
			requestBody: `{
				"name": "Non-Existent Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(_ *inmemory.ServerRepository) {},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusNotFound,
			wantError:  "server not found",
		},
		{
			name:     "missing required name field",
			serverID: "1",
			requestBody: `{
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "name is required",
		},
		{
			name:     "missing required game_id field",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game_id is required",
		},
		{
			name:     "invalid server IP",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "invalid!!!",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "server_ip is not a valid IP address or hostname",
		},
		{
			name:     "invalid server port",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 99999
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "server_port must be between 1 and 65535",
		},
		{
			name:     "invalid query port - minimum",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015,
				"query_port": 0
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "query_port must be between 1 and 65535",
		},
		{
			name:     "invalid rcon port - minimum",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015,
				"rcon_port": 0
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "rcon_port must be between 1 and 65535",
		},
		{
			name:     "invalid rcon port - maximum",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015,
				"rcon_port": 99999
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "rcon_port must be between 1 and 65535",
		},
		{
			name:     "empty server IP",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "server_ip is required",
		},
		{
			name:     "game mod ID zero",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 0,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game_mod_id is required",
		},
		{
			name:     "game mod ID negative",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": -1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game_mod_id is required",
		},
		{
			name:     "ds ID zero",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 0,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "ds_id is required",
		},
		{
			name:     "ds ID negative",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": -1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "ds_id is required",
		},
		{
			name:     "name too long",
			serverID: "1",
			requestBody: `{
				"name": "` + strings.Repeat("a", 129) + `",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "name must not exceed 128 characters",
		},
		{
			name:        "invalid JSON body",
			serverID:    "1",
			requestBody: `{"invalid": json}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:     "update with all optional fields",
			serverID: "1",
			requestBody: `{
				"enabled": 1,
				"installed": 1,
				"blocked": 0,
				"name": "Complete Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"query_port": 27016,
				"rcon_port": 27017,
				"start_command": "./hlds_run -game cstrike",
				"dir": "/servers/cstrike"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
				if err != nil {
					t.Fatalf("failed to setup test server: %v", err)
				}
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "update server with valid hostname",
			serverID: "1",
			requestBody: `{
				"name": "Server with hostname",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "hldm.org",
				"server_port": 27018
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
				if err != nil {
					t.Fatalf("failed to setup test server: %v", err)
				}
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "update server with subdomain hostname",
			serverID: "1",
			requestBody: `{
				"name": "Server with subdomain",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "game.example.com",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
				if err != nil {
					t.Fatalf("failed to setup test server: %v", err)
				}
			},
			setupAuth: func(ctx context.Context) context.Context {
				session := &auth.Session{
					User: &domain.User{ID: 1},
				}

				return auth.ContextWithSession(ctx, session)
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, nil, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo)
			}

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/servers/"+tt.serverID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": tt.serverID})

			if tt.setupAuth != nil {
				req = req.WithContext(tt.setupAuth(req.Context()))
			}

			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("want error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, "ok", response["status"])
			}
		})
	}
}

func TestHandler_ServerUpdatePersistence(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, nil, responder)

	originalServer := &domain.Server{
		ID:         1,
		UUID:       uuid.New(),
		UUIDShort:  "12345678",
		Enabled:    true,
		Installed:  0,
		Blocked:    false,
		Name:       "Original Server",
		GameID:     "cstrike",
		DSID:       1,
		GameModID:  1,
		ServerIP:   "192.168.1.1",
		ServerPort: 27015,
	}

	err := serverRepo.Save(context.Background(), originalServer)
	require.NoError(t, err)

	updateData := map[string]any{
		"enabled":       lo.ToPtr(int8(1)),
		"installed":     lo.ToPtr(int8(1)),
		"blocked":       lo.ToPtr(int8(0)),
		"name":          "Updated Server Name",
		"game_id":       "valve",
		"ds_id":         2,
		"game_mod_id":   2,
		"server_ip":     "10.0.0.1",
		"server_port":   27016,
		"query_port":    lo.ToPtr(27017),
		"rcon_port":     lo.ToPtr(27018),
		"start_command": lo.ToPtr("./hlds_run -game valve"),
		"dir":           lo.ToPtr("/servers/valve"),
	}

	body, err := json.Marshal(updateData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/servers/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "1"})

	session := &auth.Session{
		User: &domain.User{ID: 1},
	}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))

	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	servers, err := serverRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	server := servers[0]
	assert.Equal(t, uint(1), server.ID)
	assert.True(t, server.Enabled)
	assert.Equal(t, domain.ServerInstalledStatusInstalled, server.Installed)
	assert.False(t, server.Blocked)
	assert.Equal(t, "Updated Server Name", server.Name)
	assert.Equal(t, "valve", server.GameID)
	assert.Equal(t, uint(2), server.DSID)
	assert.Equal(t, uint(2), server.GameModID)
	assert.Equal(t, "10.0.0.1", server.ServerIP)
	assert.Equal(t, 27016, server.ServerPort)
	require.NotNil(t, server.QueryPort)
	assert.Equal(t, lo.ToPtr(27017), server.QueryPort)
	require.NotNil(t, server.RconPort)
	assert.Equal(t, lo.ToPtr(27018), server.RconPort)
	require.NotNil(t, server.StartCommand)
	assert.Equal(t, lo.ToPtr("./hlds_run -game valve"), server.StartCommand)
	assert.Equal(t, "/servers/valve", server.Dir)
}

func TestHandler_InvalidServerID(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, nil, responder)

	requestBody := `{
		"name": "Test Server",
		"game_id": "cstrike",
		"ds_id": 1,
		"game_mod_id": 1,
		"server_ip": "192.168.1.1",
		"server_port": 27015
	}`

	req := httptest.NewRequest(http.MethodPut, "/servers/invalid", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "invalid"})

	session := &auth.Session{
		User: &domain.User{ID: 1},
	}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))

	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"].(string), "invalid server id")
}
