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
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name        string
		serverID    string
		requestBody string
		setupRepo   func(repo *inmemory.ServerRepository, nodeRepo *inmemory.NodeRepository, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository)
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(_ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
			},
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
				"dir": "servers/cstrike"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			name:     "successful_update_with_vars",
			serverID: "1",
			requestBody: `{
				"name": "Server with vars",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"vars": {"maxplayers": "32", "hostname": "My Server"}
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			name:     "successful_update_with_cpu_and_ram_limits",
			serverID: "1",
			requestBody: `{
				"name": "Server with limits",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"cpu_limit": 2000,
				"ram_limit": 4294967296
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				err := repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			name:     "negative_cpu_limit",
			serverID: "1",
			requestBody: `{
				"name": "Server with negative cpu limit",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"cpu_limit": -1
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "cpu_limit must be >= 0",
		},
		{
			name:     "negative_ram_limit",
			serverID: "1",
			requestBody: `{
				"name": "Server with negative ram limit",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"ram_limit": -1
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "ram_limit must be >= 0",
		},
		{
			name:     "dir_with_windows_drive_letter_is_rejected",
			serverID: "1",
			requestBody: `{
				"name": "Server with windows path",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "C:\\gameap\\servers\\cs"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name:     "dir_with_dot_dot_segment_is_rejected",
			serverID: "1",
			requestBody: `{
				"name": "Server with traversal dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "../etc/passwd"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name:     "dir_with_leading_slash_is_rejected",
			serverID: "1",
			requestBody: `{
				"name": "Server with absolute dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "/srv/gameap/servers/cs"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name:     "dir_with_leading_backslash_is_rejected",
			serverID: "1",
			requestBody: `{
				"name": "Server with backslash dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "\\gameap\\servers\\cs"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name:     "dir_with_unc_share_path_is_rejected",
			serverID: "1",
			requestBody: `{
				"name": "Server with UNC dir",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.100",
				"server_port": 27015,
				"dir": "\\\\server\\share\\dir"
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
			wantError:  "dir must be a relative path without drive letter or '..' segments",
		},
		{
			name:     "node_not_found_when_ds_id_changed",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 99,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
				return auth.ContextWithSession(ctx, &auth.Session{User: &domain.User{ID: 1}})
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "Internal Server Error",
		},
		{
			name:     "game_not_found_when_game_id_changed",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "missing",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
				return auth.ContextWithSession(ctx, &auth.Session{User: &domain.User{ID: 1}})
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "Internal Server Error",
		},
		{
			name:     "game_mod_not_found_when_game_mod_id_changed",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 99,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
				return auth.ContextWithSession(ctx, &auth.Session{User: &domain.User{ID: 1}})
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "Internal Server Error",
		},
		{
			name:     "game_mod_not_belongs_to_game_when_game_id_changed",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "valve",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
					UUIDShort:  "12345678",
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
				})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "cstrike"})
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "valve"})
				_ = gameModRepo.Save(context.Background(), &domain.GameMod{ID: 1, GameCode: "cstrike"})
			},
			setupAuth: func(ctx context.Context) context.Context {
				return auth.ContextWithSession(ctx, &auth.Session{User: &domain.User{ID: 1}})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game mod does not belong to the specified game",
		},
		{
			name:     "no_validation_when_relations_unchanged",
			serverID: "1",
			requestBody: `{
				"name": "Renamed Server",
				"game_id": "cstrike",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
				return auth.ContextWithSession(ctx, &auth.Session{User: &domain.User{ID: 1}})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "node_repo_error_when_ds_id_changed",
			serverID: "1",
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 2,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
			setupRepo: func(repo *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.Server{
					ID:         1,
					UID:        uuid.New(),
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
				return auth.ContextWithSession(ctx, &auth.Session{User: &domain.User{ID: 1}})
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, nil, nil, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, nodeRepo, gameRepo, gameModRepo)
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
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, nil, nil, responder)

	require.NoError(t, nodeRepo.Save(context.Background(), &domain.Node{ID: 2, Name: "node2"}))
	require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{Code: "valve"}))
	require.NoError(t, gameModRepo.Save(context.Background(), &domain.GameMod{ID: 2, GameCode: "valve"}))

	originalServer := &domain.Server{
		ID:         1,
		UID:        uuid.New(),
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
		"enabled":       new(int8(1)),
		"installed":     new(int8(1)),
		"blocked":       new(int8(0)),
		"name":          "Updated Server Name",
		"game_id":       "valve",
		"ds_id":         2,
		"game_mod_id":   2,
		"server_ip":     "10.0.0.1",
		"server_port":   27016,
		"query_port":    new(27017),
		"rcon_port":     new(27018),
		"start_command": new("./hlds_run -game valve"),
		"dir":           new("servers/valve"),
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
	assert.Equal(t, new(27017), server.QueryPort)
	require.NotNil(t, server.RconPort)
	assert.Equal(t, new(27018), server.RconPort)
	require.NotNil(t, server.StartCommand)
	assert.Equal(t, new("./hlds_run -game valve"), server.StartCommand)
	assert.Equal(t, "servers/valve", server.Dir)
}

func TestHandler_ServerUpdatePersistence_WithVarsAndLimits(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, nil, nil, responder)

	require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{Code: "valve"}))
	require.NoError(t, gameModRepo.Save(context.Background(), &domain.GameMod{ID: 2, GameCode: "valve"}))

	originalServer := &domain.Server{
		ID:         1,
		UID:        uuid.New(),
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
		"name":        "Updated Server Name",
		"game_id":     "valve",
		"ds_id":       1,
		"game_mod_id": 2,
		"server_ip":   "10.0.0.1",
		"server_port": 27016,
		"vars":        map[string]string{"maxplayers": "32", "hostname": "My Server"},
		"cpu_limit":   2000,
		"ram_limit":   4294967296,
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
	assert.Equal(t, "Updated Server Name", server.Name)
	assert.Equal(t, domain.ServerVars{"maxplayers": "32", "hostname": "My Server"}, server.Vars)
	require.NotNil(t, server.CPULimit)
	assert.Equal(t, 2000, *server.CPULimit)
	require.NotNil(t, server.RAMLimit)
	assert.Equal(t, 4294967296, *server.RAMLimit)
}

func TestHandler_InvalidServerID(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, nil, nil, responder)

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

func TestHandler_PrepareUpdateRepoErrors(t *testing.T) {
	tests := []struct {
		name        string
		buildRepos  func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository)
		requestBody string
	}{
		{
			name: "node_repo_returns_error",
			buildRepos: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				return &errNodeRepo{
					NodeRepository: inmemory.NewNodeRepository(),
					findErr:        pkgerrors.New("db down"),
				}, inmemory.NewGameRepository(), inmemory.NewGameModRepository()
			},
			requestBody: `{
				"name": "Test Server",
				"game_id": "cstrike",
				"ds_id": 2,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
		},
		{
			name: "game_repo_returns_error",
			buildRepos: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				return inmemory.NewNodeRepository(), &errGameRepo{
					GameRepository: inmemory.NewGameRepository(),
					findErr:        pkgerrors.New("db down"),
				}, inmemory.NewGameModRepository()
			},
			requestBody: `{
				"name": "Test Server",
				"game_id": "valve",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
		},
		{
			name: "game_mod_repo_returns_error",
			buildRepos: func() (repositories.NodeRepository, repositories.GameRepository, repositories.GameModRepository) {
				gameRepo := inmemory.NewGameRepository()
				_ = gameRepo.Save(context.Background(), &domain.Game{Code: "valve"})

				return inmemory.NewNodeRepository(), gameRepo, &errGameModRepo{
					GameModRepository: inmemory.NewGameModRepository(),
					findErr:           pkgerrors.New("db down"),
				}
			},
			requestBody: `{
				"name": "Test Server",
				"game_id": "valve",
				"ds_id": 1,
				"game_mod_id": 1,
				"server_ip": "192.168.1.1",
				"server_port": 27015
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo, gameRepo, gameModRepo := tt.buildRepos()
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, nodeRepo, gameRepo, gameModRepo, nil, nil, responder)

			require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
				ID:         1,
				UID:        uuid.New(),
				UUIDShort:  "12345678",
				Name:       "Test Server",
				GameID:     "cstrike",
				DSID:       1,
				GameModID:  1,
				ServerIP:   "192.168.1.1",
				ServerPort: 27015,
			}))

			req := httptest.NewRequest(http.MethodPut, "/servers/1", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": "1"})
			req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: &domain.User{ID: 1}}))

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "error", response["status"])
			assert.Contains(t, response["error"].(string), "Internal Server Error")

			servers, err := serverRepo.FindAll(context.Background(), nil, nil)
			require.NoError(t, err)
			require.Len(t, servers, 1)
			assert.Equal(t, "Test Server", servers[0].Name, "server must remain unchanged when prepareUpdate fails")
		})
	}
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
