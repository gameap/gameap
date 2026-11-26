// API Security Tests for OWASP API Security Top 10:2023.
// Category: API5:2023 — Broken Function Level Authorization.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/
//
// A server task has the scheduler run a command on a game server. Creating one
// used to take game-server-tasks alone, so a user who may neither update nor
// restart a server could have it reinstalled, at a time of their choosing.
// These tests hold a new task to what its command takes when it is sent
// directly.

package postservertask

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rightsUserID uint = 3

// grantOnServer gives the user or the role the abilities on the server with
// the ID 1.
func grantOnServer(
	rbacRepo *inmemory.RBACRepository,
	entityType domain.EntityType,
	entityID uint,
	abilities ...domain.AbilityName,
) error {
	for _, name := range abilities {
		ability := domain.CreateAbilityForEntity(name, 1, domain.EntityTypeServer)

		err := rbacRepo.SaveAbility(context.Background(), &ability)
		if err != nil {
			return err
		}

		err = rbacRepo.Allow(context.Background(), entityID, entityType, []domain.Ability{ability})
		if err != nil {
			return err
		}
	}

	return nil
}

func rightsSession(tokenAbilities *[]domain.PATAbility) context.Context {
	session := &auth.Session{
		Login: "user",
		Email: "user@example.com",
		User:  &domain.User{ID: rightsUserID, Login: "user", Email: "user@example.com"},
	}

	if tokenAbilities != nil {
		session.Token = &domain.PersonalAccessToken{ID: 9, Abilities: tokenAbilities}
	}

	return auth.ContextWithSession(context.Background(), session)
}

// OWASP API5:2023 — creating a task takes game-server-tasks and the abilities
// of the command the task runs; a personal access token takes the token
// ability of the command besides.
func TestHandler_ServeHTTP_CommandRights(t *testing.T) {
	t.Parallel()

	everyTokenAbility := domain.GetAllAbilities()

	tests := []struct {
		name string
		// granted are the abilities of the user on the server.
		granted []domain.AbilityName
		// tokenAbilities are the abilities of the personal access token the
		// request is made with; nil stands for the session of a user.
		tokenAbilities *[]domain.PATAbility
		command        string
		wantStatus     int
		wantError      string
	}{
		{
			name:       "start_with_the_schedule_alone",
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:    "start",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:       "stop_with_the_schedule_alone",
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:    "stop",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:       "restart_with_the_schedule_alone",
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:       "update_with_the_schedule_alone",
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:    "update",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:       "reinstall_with_the_schedule_alone",
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:    "reinstall",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name: "reinstall_with_every_ability_but_update",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
				domain.AbilityNameGameServerStop,
				domain.AbilityNameGameServerRestart,
				domain.AbilityNameGameServerFiles,
				domain.AbilityNameGameServerSettings,
			},
			command:    "reinstall",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name: "stop_with_the_ability_of_another_command",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
			},
			command:    "stop",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name: "restart_without_the_common_ability",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name: "restart_with_its_abilities_but_without_the_schedule",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name: "start_with_its_abilities",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
			},
			command:    "start",
			wantStatus: http.StatusCreated,
		},
		{
			name: "stop_with_its_abilities",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStop,
			},
			command:    "stop",
			wantStatus: http.StatusCreated,
		},
		{
			name: "restart_with_its_abilities",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusCreated,
		},
		{
			name: "update_with_its_abilities",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			command:    "update",
			wantStatus: http.StatusCreated,
		},
		{
			name: "reinstall_with_the_update_ability",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			command:    "reinstall",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "unknown_command_is_a_validation_error_whatever_the_abilities",
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:    "install",
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "invalid command",
		},
		{
			name:           "token_that_manages_the_schedule_only",
			granted:        domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{domain.PATAbilityServerTasksManage},
			command:        "reinstall",
			wantStatus:     http.StatusForbidden,
			wantError:      "missing required ability: server:update",
		},
		{
			name:    "token_with_the_ability_of_another_command",
			granted: domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{
				domain.PATAbilityServerTasksManage,
				domain.PATAbilityServerRestart,
			},
			command:    "stop",
			wantStatus: http.StatusForbidden,
			wantError:  "missing required ability: server:stop",
		},
		{
			name:    "token_with_the_ability_of_the_command",
			granted: domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{
				domain.PATAbilityServerTasksManage,
				domain.PATAbilityServerUpdate,
			},
			command:    "reinstall",
			wantStatus: http.StatusCreated,
		},
		{
			name:           "token_with_every_ability_of_a_user_without_the_abilities",
			granted:        []domain.AbilityName{domain.AbilityNameGameServerTasks},
			tokenAbilities: &everyTokenAbility,
			command:        "update",
			wantStatus:     http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rr, tasks := scheduleCommand(t, tt.granted, tt.tokenAbilities, tt.command)

			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())

			if tt.wantError != "" {
				var errorResponse map[string]any
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errorResponse))
				assert.Contains(t, errorResponse["message"], tt.wantError)
				assert.Empty(t, tasks, "a request that is refused leaves no task behind")

				return
			}

			require.Len(t, tasks, 1)
			assert.Equal(t, domain.ServerTaskCommand(tt.command), tasks[0].Command)
		})
	}
}

// OWASP API5:2023 — no command the schedule accepts is left without abilities
// of its own: each is scheduled with every ability of a server and none with
// the schedule alone. A command added to the list has to be given its
// abilities before this passes.
func TestHandler_ServeHTTP_EveryValidCommandTakesAbilities(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, validCommands)

	for _, command := range validCommands {
		t.Run(command+"/scheduled_with_every_ability_of_a_server", func(t *testing.T) {
			t.Parallel()

			rr, tasks := scheduleCommand(t, domain.ServersAbilities, nil, command)

			require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
			require.Len(t, tasks, 1)
			assert.Equal(t, domain.ServerTaskCommand(command), tasks[0].Command)
		})

		t.Run(command+"/refused_with_the_schedule_alone", func(t *testing.T) {
			t.Parallel()

			rr, tasks := scheduleCommand(t, []domain.AbilityName{domain.AbilityNameGameServerTasks}, nil, command)

			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			assert.Empty(t, tasks)
		})
	}
}

// scheduleCommand has the user, who holds the abilities granted on server 1,
// ask for a task with the command, and returns the answer with the tasks the
// server has afterwards.
func scheduleCommand(
	t *testing.T,
	granted []domain.AbilityName,
	tokenAbilities *[]domain.PATAbility,
	command string,
) (*httptest.ResponseRecorder, []domain.ServerTask) {
	t.Helper()

	serversRepo := inmemory.NewServerRepository()
	serverTasksRepo := inmemory.NewServerTaskRepository(serversRepo)
	rbacRepo := inmemory.NewRBACRepository()

	now := time.Now()
	require.NoError(t, serversRepo.Save(context.Background(), &domain.Server{
		ID:         1,
		UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:  "short1",
		Name:       "Test Server",
		GameID:     "cs",
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		Dir:        "servers/test1",
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}))
	serversRepo.AddUserServer(rightsUserID, 1)
	require.NoError(t, grantOnServer(rbacRepo, domain.EntityTypeUser, rightsUserID, granted...))

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	handler := NewHandler(serverTasksRepo, serversRepo, rbacService, nil, api.NewResponder())

	body, err := json.Marshal(map[string]any{
		"command":      command,
		"repeat":       1,
		"execute_date": time.Now().Add(time.Hour).Format(time.RFC3339),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/servers/1/tasks", bytes.NewReader(body))
	req = req.WithContext(rightsSession(tokenAbilities))
	req = mux.SetURLVars(req, map[string]string{"server": "1"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	tasks, err := serverTasksRepo.Find(
		context.Background(),
		&filters.FindServerTask{ServersIDs: []uint{1}},
		nil,
		nil,
	)
	require.NoError(t, err)

	return rr, tasks
}
