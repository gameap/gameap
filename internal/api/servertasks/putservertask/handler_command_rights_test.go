// API Security Tests for OWASP API Security Top 10:2023.
// Category: API5:2023 — Broken Function Level Authorization.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/
//
// A server task has the scheduler run a command on a game server. Changing one
// used to take game-server-tasks alone, so a user who may not update a server
// could move the reinstall somebody had scheduled for it to the next minute,
// or turn a task of any kind into one. These tests hold a change to what the
// commands take when they are sent directly: the command the task has and the
// command it is given.

package putservertask

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

const (
	rightsUserID        uint = 3
	storedTaskPayload        = "payload of the task that is there"
	storedTaskVersion        = 4
	storedTaskRepeat         = 7
	changedTaskPayload       = "payload of the change"
	changedTaskRepeat        = 1
	storedTaskHoursAway      = 240
)

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

// OWASP API5:2023 — changing a task takes game-server-tasks and the abilities
// of the command the task has and of the command it is given; a personal
// access token takes the token abilities of both besides.
func TestHandler_ServeHTTP_CommandRights(t *testing.T) {
	t.Parallel()

	everyTokenAbility := domain.GetAllAbilities()

	tests := []struct {
		name string
		// storedCommand is the command of the task before the request.
		storedCommand domain.ServerTaskCommand
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
			name:          "start_task_moved_with_the_schedule_alone",
			storedCommand: domain.ServerTaskCommandStart,
			granted:       []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:       "start",
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name:          "stop_task_moved_with_the_schedule_alone",
			storedCommand: domain.ServerTaskCommandStop,
			granted:       []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:       "stop",
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name:          "restart_task_moved_with_the_schedule_alone",
			storedCommand: domain.ServerTaskCommandRestart,
			granted:       []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:       "restart",
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name:          "update_task_moved_with_the_schedule_alone",
			storedCommand: domain.ServerTaskCommandUpdate,
			granted:       []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:       "update",
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name:          "reinstall_task_moved_with_the_schedule_alone",
			storedCommand: domain.ServerTaskCommandReinstall,
			granted:       []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:       "reinstall",
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name:          "reinstall_task_moved_with_every_ability_but_update",
			storedCommand: domain.ServerTaskCommandReinstall,
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
			name:          "restart_task_turned_into_a_reinstall_without_the_update_ability",
			storedCommand: domain.ServerTaskCommandRestart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			command:    "reinstall",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:          "reinstall_task_turned_into_a_restart_without_the_update_ability",
			storedCommand: domain.ServerTaskCommandReinstall,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:          "start_task_turned_into_a_stop_without_the_stop_ability",
			storedCommand: domain.ServerTaskCommandStart,
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
			name:          "start_task_turned_into_a_stop_without_the_start_ability",
			storedCommand: domain.ServerTaskCommandStart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStop,
			},
			command:    "stop",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:          "restart_task_moved_without_the_common_ability",
			storedCommand: domain.ServerTaskCommandRestart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:          "restart_task_moved_with_its_abilities_but_without_the_schedule",
			storedCommand: domain.ServerTaskCommandRestart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "user does not have required permissions",
		},
		{
			name:          "task_with_a_command_that_is_not_known",
			storedCommand: "install",
			granted:       domain.ServersAbilities,
			command:       "restart",
			wantStatus:    http.StatusForbidden,
			wantError:     "cannot be scheduled",
		},
		{
			name:          "start_task_moved_with_its_abilities",
			storedCommand: domain.ServerTaskCommandStart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
			},
			command:    "start",
			wantStatus: http.StatusOK,
		},
		{
			name:          "stop_task_moved_with_its_abilities",
			storedCommand: domain.ServerTaskCommandStop,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStop,
			},
			command:    "stop",
			wantStatus: http.StatusOK,
		},
		{
			name:          "restart_task_moved_with_its_abilities",
			storedCommand: domain.ServerTaskCommandRestart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusOK,
		},
		{
			name:          "update_task_moved_with_its_abilities",
			storedCommand: domain.ServerTaskCommandUpdate,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			command:    "update",
			wantStatus: http.StatusOK,
		},
		{
			name:          "reinstall_task_moved_with_the_update_ability",
			storedCommand: domain.ServerTaskCommandReinstall,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			command:    "reinstall",
			wantStatus: http.StatusOK,
		},
		{
			name:          "start_task_turned_into_a_stop_with_the_abilities_of_both",
			storedCommand: domain.ServerTaskCommandStart,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
				domain.AbilityNameGameServerStop,
			},
			command:    "stop",
			wantStatus: http.StatusOK,
		},
		{
			name:          "update_task_turned_into_a_reinstall_with_the_update_ability",
			storedCommand: domain.ServerTaskCommandUpdate,
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerTasks,
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			command:    "reinstall",
			wantStatus: http.StatusOK,
		},
		{
			name:          "unknown_command_is_a_validation_error_whatever_the_abilities",
			storedCommand: domain.ServerTaskCommandRestart,
			granted:       []domain.AbilityName{domain.AbilityNameGameServerTasks},
			command:       "install",
			wantStatus:    http.StatusUnprocessableEntity,
			wantError:     "invalid command",
		},
		{
			name:           "token_that_manages_the_schedule_only",
			storedCommand:  domain.ServerTaskCommandRestart,
			granted:        domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{domain.PATAbilityServerTasksManage},
			command:        "restart",
			wantStatus:     http.StatusForbidden,
			wantError:      "missing required ability: server:restart",
		},
		{
			name:          "token_without_the_ability_of_the_command_the_task_has",
			storedCommand: domain.ServerTaskCommandUpdate,
			granted:       domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{
				domain.PATAbilityServerTasksManage,
				domain.PATAbilityServerRestart,
			},
			command:    "restart",
			wantStatus: http.StatusForbidden,
			wantError:  "missing required ability: server:update",
		},
		{
			name:          "token_without_the_ability_of_the_command_the_task_is_given",
			storedCommand: domain.ServerTaskCommandRestart,
			granted:       domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{
				domain.PATAbilityServerTasksManage,
				domain.PATAbilityServerRestart,
			},
			command:    "reinstall",
			wantStatus: http.StatusForbidden,
			wantError:  "missing required ability: server:update",
		},
		{
			name:          "token_with_the_abilities_of_both_commands",
			storedCommand: domain.ServerTaskCommandRestart,
			granted:       domain.ServersAbilities,
			tokenAbilities: &[]domain.PATAbility{
				domain.PATAbilityServerTasksManage,
				domain.PATAbilityServerRestart,
				domain.PATAbilityServerUpdate,
			},
			command:    "reinstall",
			wantStatus: http.StatusOK,
		},
		{
			name:           "token_with_every_ability_of_a_user_without_the_abilities",
			storedCommand:  domain.ServerTaskCommandUpdate,
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

			rr, task, storedExecuteDate := changeCommand(t, tt.storedCommand, tt.granted, tt.tokenAbilities, tt.command)

			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())

			if tt.wantError != "" {
				var errorResponse map[string]any
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errorResponse))
				assert.Contains(t, errorResponse["message"], tt.wantError)

				assertTaskLeftAsItWas(t, task, tt.storedCommand, storedExecuteDate)

				return
			}

			assert.Equal(t, domain.ServerTaskCommand(tt.command), task.Command)
			assert.False(t, storedExecuteDate.Equal(task.ExecuteDate), "the task is moved")
			assert.Equal(t, uint8(changedTaskRepeat), task.Repeat)
			assert.Greater(t, task.Version, uint64(storedTaskVersion))
			require.NotNil(t, task.Payload)
			assert.Equal(t, changedTaskPayload, *task.Payload)
		})
	}
}

// OWASP API5:2023 — no command the schedule accepts is left without abilities
// of its own: a task of each is moved with every ability of a server and none
// with the schedule alone. A command added to the list has to be given its
// abilities before this passes.
func TestHandler_ServeHTTP_EveryValidCommandTakesAbilities(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, validCommands)

	for _, command := range validCommands {
		stored := domain.ServerTaskCommand(command)

		t.Run(command+"/moved_with_every_ability_of_a_server", func(t *testing.T) {
			t.Parallel()

			rr, task, storedExecuteDate := changeCommand(t, stored, domain.ServersAbilities, nil, command)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Equal(t, stored, task.Command)
			assert.False(t, storedExecuteDate.Equal(task.ExecuteDate), "the task is moved")
		})

		t.Run(command+"/refused_with_the_schedule_alone", func(t *testing.T) {
			t.Parallel()

			rr, task, storedExecuteDate := changeCommand(
				t, stored, []domain.AbilityName{domain.AbilityNameGameServerTasks}, nil, command,
			)

			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			assertTaskLeftAsItWas(t, task, stored, storedExecuteDate)
		})
	}
}

func assertTaskLeftAsItWas(
	t *testing.T,
	task domain.ServerTask,
	storedCommand domain.ServerTaskCommand,
	storedExecuteDate time.Time,
) {
	t.Helper()

	assert.Equal(t, storedCommand, task.Command, "a request that is refused leaves the task as it was")
	assert.True(t, storedExecuteDate.Equal(task.ExecuteDate), "the task keeps its time: %s", task.ExecuteDate)
	assert.Equal(t, uint8(storedTaskRepeat), task.Repeat)
	assert.Equal(t, uint64(storedTaskVersion), task.Version)
	require.NotNil(t, task.Payload)
	assert.Equal(t, storedTaskPayload, *task.Payload)
}

// changeCommand has the user, who holds the abilities granted on server 1, ask
// for the one task of the server, which runs storedCommand far from now, to
// run the command a minute from now. It returns the answer, the task as it is
// afterwards and the time the task had.
func changeCommand(
	t *testing.T,
	storedCommand domain.ServerTaskCommand,
	granted []domain.AbilityName,
	tokenAbilities *[]domain.PATAbility,
	command string,
) (*httptest.ResponseRecorder, domain.ServerTask, time.Time) {
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

	storedExecuteDate := now.Add(storedTaskHoursAway * time.Hour).Truncate(time.Second)
	require.NoError(t, serverTasksRepo.Save(context.Background(), &domain.ServerTask{
		ID:          1,
		Command:     storedCommand,
		ServerID:    1,
		Enabled:     true,
		Version:     storedTaskVersion,
		Repeat:      storedTaskRepeat,
		ExecuteDate: storedExecuteDate,
		Payload:     new(storedTaskPayload),
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}))

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	handler := NewHandler(serverTasksRepo, serversRepo, rbacService, nil, api.NewResponder())

	body, err := json.Marshal(map[string]any{
		"command":      command,
		"repeat":       changedTaskRepeat,
		"execute_date": now.Add(time.Minute).Format(time.RFC3339),
		"payload":      changedTaskPayload,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/servers/1/tasks/1", bytes.NewReader(body))
	req = req.WithContext(rightsSession(tokenAbilities))
	req = mux.SetURLVars(req, map[string]string{"server": "1", "id": "1"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	tasks, err := serverTasksRepo.Find(
		context.Background(),
		&filters.FindServerTask{ServersIDs: []uint{1}},
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	return rr, tasks[0], storedExecuteDate
}
