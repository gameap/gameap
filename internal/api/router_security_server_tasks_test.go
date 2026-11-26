// API Security Tests for OWASP API Security Top 10:2023.
// Category: API5:2023 — Broken Function Level Authorization.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/
//
// A server task has the scheduler send a command to a game server. The routes
// of the commands are scoped one by one, of a user (game-server-start, ...) and
// of a personal access token (server:start, ...). The routes of the schedule
// took game-server-tasks and server:tasks-manage alone, which made the schedule
// a way round every one of those scopes: a user who may not update a server
// had it reinstalled, at a time of their choosing, by scheduling the reinstall.
//
// These tests run the real router and put one caller to both routes: the
// schedule refuses every command the route of the command refuses, and a task
// that is refused is neither kept nor announced to the node.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/grpc/gateway"
	"github.com/gameap/gameap/internal/services/servertaskdispatcher"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingNode stands in for the session of a daemon: it keeps the commands
// of the tasks the panel announces to the node, which is where they would be
// run from.
type recordingNode struct {
	mu        sync.Mutex
	announced []domain.ServerTaskCommand
}

func (n *recordingNode) SendTask(_ context.Context, _ uint64, msg *proto.GatewayMessage) error {
	task := msg.GetServerTaskDelta().GetUpserted()
	if task == nil {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.announced = append(n.announced, gateway.ProtoServerTaskCommandToDomain(task.GetCommand()))

	return nil
}

func (n *recordingNode) commands() []domain.ServerTaskCommand {
	n.mu.Lock()
	defer n.mu.Unlock()

	return append([]domain.ServerTaskCommand(nil), n.announced...)
}

// scheduleContainer gives the router the dispatcher of server tasks the
// in-memory container leaves out, wired to the recording node.
type scheduleContainer struct {
	*testcontainer.InmemoryContainer

	dispatcher *servertaskdispatcher.Dispatcher
}

func (c *scheduleContainer) ServerTaskDispatcher() *servertaskdispatcher.Dispatcher {
	return c.dispatcher
}

type scheduleTestEnv struct {
	*securityTestEnv

	node *recordingNode
}

func setupScheduleSecurityTest(t *testing.T) *scheduleTestEnv {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)

	ctx := context.Background()

	fixtures, err := testcontainer.SetupFixtures(ctx, c)
	require.NoError(t, err)

	node := &recordingNode{}

	return &scheduleTestEnv{
		securityTestEnv: &securityTestEnv{
			container: c,
			fixtures:  fixtures,
			router: api.CreateRouter(&scheduleContainer{
				InmemoryContainer: c,
				dispatcher: servertaskdispatcher.NewDispatcher(
					node,
					c.ServerTaskRepository(),
					c.ServerTaskExecutionRepository(),
					nil,
					nil,
				),
			}),
			ctx: ctx,
		},
		node: node,
	}
}

// scheduledCommands are the commands a server task can run; each has a route
// of its own, POST /api/servers/{server}/{command}.
//
//nolint:gochecknoglobals
var scheduledCommands = []domain.ServerTaskCommand{
	domain.ServerTaskCommandStart,
	domain.ServerTaskCommandStop,
	domain.ServerTaskCommandRestart,
	domain.ServerTaskCommandUpdate,
	domain.ServerTaskCommandReinstall,
}

// commandCaller is a caller of the API: the regular user of the fixtures, who
// has every ability on server 1, less the abilities taken away, speaking for
// themselves or through a personal access token.
type commandCaller struct {
	name string
	// withoutAbilities are taken away from the user on server 1.
	withoutAbilities []domain.AbilityName
	// tokenAbilities are the abilities of the personal access token; nil
	// stands for the session of the user.
	tokenAbilities []domain.PATAbility
	// maySend are the commands the routes of the commands take from the
	// caller. The tests check it against those routes before they hold the
	// schedule to it.
	maySend []domain.ServerTaskCommand
	// maySchedule tells whether the caller may manage the schedule at all.
	maySchedule bool
}

//nolint:gochecknoglobals
var commandCallers = []commandCaller{
	{
		name:        "user_with_every_ability",
		maySchedule: true,
		maySend:     scheduledCommands,
	},
	{
		name:             "user_without_the_common_ability",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerCommon},
		maySchedule:      true,
	},
	{
		name:             "user_without_the_start_ability",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerStart},
		maySchedule:      true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStop,
			domain.ServerTaskCommandRestart,
			domain.ServerTaskCommandUpdate,
			domain.ServerTaskCommandReinstall,
		},
	},
	{
		name:             "user_without_the_stop_ability",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerStop},
		maySchedule:      true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStart,
			domain.ServerTaskCommandRestart,
			domain.ServerTaskCommandUpdate,
			domain.ServerTaskCommandReinstall,
		},
	},
	{
		name:             "user_without_the_restart_ability",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerRestart},
		maySchedule:      true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStart,
			domain.ServerTaskCommandStop,
			domain.ServerTaskCommandUpdate,
			domain.ServerTaskCommandReinstall,
		},
	},
	{
		name:             "user_without_the_update_ability",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerUpdate},
		maySchedule:      true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStart,
			domain.ServerTaskCommandStop,
			domain.ServerTaskCommandRestart,
		},
	},
	{
		name: "user_with_the_schedule_alone",
		withoutAbilities: []domain.AbilityName{
			domain.AbilityNameGameServerCommon,
			domain.AbilityNameGameServerStart,
			domain.AbilityNameGameServerStop,
			domain.AbilityNameGameServerRestart,
			domain.AbilityNameGameServerUpdate,
		},
		maySchedule: true,
	},
	{
		name:             "user_without_the_schedule",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerTasks},
		maySend:          scheduledCommands,
	},
	{
		name:           "token_that_manages_the_schedule_only",
		tokenAbilities: []domain.PATAbility{domain.PATAbilityServerTasksManage},
		maySchedule:    true,
	},
	{
		name: "token_with_the_start_ability",
		tokenAbilities: []domain.PATAbility{
			domain.PATAbilityServerTasksManage,
			domain.PATAbilityServerStart,
		},
		maySchedule: true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStart,
		},
	},
	{
		name: "token_with_the_stop_ability",
		tokenAbilities: []domain.PATAbility{
			domain.PATAbilityServerTasksManage,
			domain.PATAbilityServerStop,
		},
		maySchedule: true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStop,
		},
	},
	{
		name: "token_with_the_restart_ability",
		tokenAbilities: []domain.PATAbility{
			domain.PATAbilityServerTasksManage,
			domain.PATAbilityServerRestart,
		},
		maySchedule: true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandRestart,
		},
	},
	{
		name: "token_with_the_update_ability",
		tokenAbilities: []domain.PATAbility{
			domain.PATAbilityServerTasksManage,
			domain.PATAbilityServerUpdate,
		},
		maySchedule: true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandUpdate,
			domain.ServerTaskCommandReinstall,
		},
	},
	{
		name:           "token_with_every_ability_of_a_user",
		tokenAbilities: domain.GetUserAbilities(),
		maySchedule:    true,
		maySend:        scheduledCommands,
	},
	{
		name: "token_with_every_command_but_without_the_schedule",
		tokenAbilities: []domain.PATAbility{
			domain.PATAbilityServerStart,
			domain.PATAbilityServerStop,
			domain.PATAbilityServerRestart,
			domain.PATAbilityServerUpdate,
		},
		maySend: scheduledCommands,
	},
	{
		name:             "token_with_every_ability_of_a_user_without_the_update_ability",
		withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerUpdate},
		tokenAbilities:   domain.GetUserAbilities(),
		maySchedule:      true,
		maySend: []domain.ServerTaskCommand{
			domain.ServerTaskCommandStart,
			domain.ServerTaskCommandStop,
			domain.ServerTaskCommandRestart,
		},
	},
}

// givenCommandCaller prepares a router of its own for the caller and returns
// the credentials the caller presents.
func givenCommandCaller(t *testing.T, caller commandCaller) (*scheduleTestEnv, string) {
	t.Helper()

	env := setupScheduleSecurityTest(t)
	user := env.fixtures.RegularUser

	if len(caller.withoutAbilities) > 0 {
		require.NoError(t, env.container.RBAC().RevokeOrForbidUserAbilitiesForEntity(
			env.ctx,
			user.ID,
			env.fixtures.Server1.ID,
			domain.EntityTypeServer,
			caller.withoutAbilities,
		))
	}

	if caller.tokenAbilities != nil {
		return env, issuePAT(t, env.securityTestEnv, user, caller.tokenAbilities)
	}

	return env, issuePASETOToken(t, env.securityTestEnv, user)
}

// givenServerTask puts a task on the schedule of server 1 the way the panel
// keeps one, bound to the node of the server.
func givenServerTask(
	t *testing.T,
	env *scheduleTestEnv,
	command domain.ServerTaskCommand,
	executeDate time.Time,
) *domain.ServerTask {
	t.Helper()

	task := &domain.ServerTask{
		Command:     command,
		ServerID:    env.fixtures.Server1.ID,
		NodeID:      new(env.fixtures.Server1.DSID),
		Enabled:     true,
		Repeat:      1,
		ExecuteDate: executeDate,
	}
	require.NoError(t, env.container.ServerTaskRepository().Save(env.ctx, task))

	return task
}

// requireCommandRouteAnswer checks what the caller is said to be allowed
// against the route of the command itself, which is what the schedule is held
// to.
func requireCommandRouteAnswer(t *testing.T, direct *httptest.ResponseRecorder, maySend bool) {
	t.Helper()

	if maySend {
		require.Equalf(t, http.StatusOK, direct.Code,
			"the route of the command takes it from this caller; body=%s", direct.Body.String())

		return
	}

	require.Equalf(t, http.StatusForbidden, direct.Code,
		"the route of the command refuses this caller; body=%s", direct.Body.String())
}

func serverTaskBody(t *testing.T, command domain.ServerTaskCommand, executeDate time.Time) []byte {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"command":      command,
		"repeat":       1,
		"execute_date": executeDate.Format(time.RFC3339),
	})
	require.NoError(t, err)

	return body
}

func serverTasks(t *testing.T, env *scheduleTestEnv, serverID uint) []domain.ServerTask {
	t.Helper()

	tasks, err := env.container.ServerTaskRepository().Find(
		env.ctx,
		&filters.FindServerTask{ServersIDs: []uint{serverID}},
		nil,
		nil,
	)
	require.NoError(t, err)

	return tasks
}

// OWASP API5:2023 — a caller who is refused a command by the route of the
// command is refused the same command by the schedule, and a caller who may
// send it and may manage the schedule may schedule it.
func TestRouterSecurity_API5_ServerTasks_ScheduleRefusesWhatTheCommandRouteRefuses(t *testing.T) {
	t.Parallel()

	for _, caller := range commandCallers {
		for _, command := range scheduledCommands {
			t.Run(caller.name+"/"+string(command), func(t *testing.T) {
				t.Parallel()

				env, credentials := givenCommandCaller(t, caller)
				serverID := env.fixtures.Server1.ID

				direct := doRequest(
					t, env.securityTestEnv, http.MethodPost,
					fmt.Sprintf("/api/servers/%d/%s", serverID, command),
					credentials,
				)
				mayRun := slices.Contains(caller.maySend, command)
				requireCommandRouteAnswer(t, direct, mayRun)

				scheduled := doRequestWithBody(
					t, env.securityTestEnv, http.MethodPost,
					fmt.Sprintf("/api/servers/%d/tasks", serverID),
					credentials,
					serverTaskBody(t, command, time.Now().Add(time.Hour)),
				)

				tasks := serverTasks(t, env, serverID)

				if mayRun && caller.maySchedule {
					require.Equalf(t, http.StatusCreated, scheduled.Code,
						"a caller who may send %q may schedule it; body=%s", command, scheduled.Body.String())
					require.Len(t, tasks, 1)
					assert.Equal(t, command, tasks[0].Command)
					assert.Equal(t, []domain.ServerTaskCommand{command}, env.node.commands())

					return
				}

				require.Equalf(t, http.StatusForbidden, scheduled.Code,
					"the route of %q refuses this caller or the caller does not manage the schedule; body=%s",
					command, scheduled.Body.String())
				assert.Empty(t, tasks, "a request that is refused leaves no task behind")
				assert.Empty(t, env.node.commands(), "a request that is refused announces nothing to the node")
			})
		}
	}
}

// OWASP API5:2023 — the same holds for a task that is there already: a caller
// who is refused a command cannot move a task that runs it, nor turn a task
// into one that does.
func TestRouterSecurity_API5_ServerTasks_ChangeRefusesWhatTheCommandRouteRefuses(t *testing.T) {
	t.Parallel()

	for _, caller := range commandCallers {
		for _, command := range scheduledCommands {
			t.Run(caller.name+"/"+string(command), func(t *testing.T) {
				t.Parallel()

				env, credentials := givenCommandCaller(t, caller)
				serverID := env.fixtures.Server1.ID

				storedExecuteDate := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
				stored := givenServerTask(t, env, command, storedExecuteDate)

				direct := doRequest(
					t, env.securityTestEnv, http.MethodPost,
					fmt.Sprintf("/api/servers/%d/%s", serverID, command),
					credentials,
				)
				mayRun := slices.Contains(caller.maySend, command)
				requireCommandRouteAnswer(t, direct, mayRun)

				moved := doRequestWithBody(
					t, env.securityTestEnv, http.MethodPut,
					fmt.Sprintf("/api/servers/%d/tasks/%d", serverID, stored.ID),
					credentials,
					serverTaskBody(t, command, time.Now().Add(time.Minute)),
				)

				tasks := serverTasks(t, env, serverID)
				require.Len(t, tasks, 1)

				if mayRun && caller.maySchedule {
					require.Equalf(t, http.StatusOK, moved.Code,
						"a caller who may send %q may move a task that runs it; body=%s", command, moved.Body.String())
					assert.False(t, storedExecuteDate.Equal(tasks[0].ExecuteDate), "the task is moved")
					assert.Equal(t, []domain.ServerTaskCommand{command}, env.node.commands())

					return
				}

				require.Equalf(t, http.StatusForbidden, moved.Code,
					"the route of %q refuses this caller or the caller does not manage the schedule; body=%s",
					command, moved.Body.String())
				assert.Equal(t, command, tasks[0].Command)
				assert.True(t, storedExecuteDate.Equal(tasks[0].ExecuteDate),
					"a request that is refused leaves the task where it was: %s", tasks[0].ExecuteDate)
				assert.Empty(t, env.node.commands(), "a request that is refused announces nothing to the node")
			})
		}
	}
}

// OWASP API5:2023 — a caller without the update ability cannot reach a
// reinstall through a task of another kind, in either direction.
func TestRouterSecurity_API5_ServerTasks_TaskCannotBeTurnedIntoARefusedCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		caller        commandCaller
		storedCommand domain.ServerTaskCommand
		command       domain.ServerTaskCommand
		wantStatus    int
		wantError     string
	}{
		{
			name: "restart_into_a_reinstall_by_a_user_without_the_update_ability",
			caller: commandCaller{
				withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerUpdate},
			},
			storedCommand: domain.ServerTaskCommandRestart,
			command:       domain.ServerTaskCommandReinstall,
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name: "reinstall_into_a_restart_by_a_user_without_the_update_ability",
			caller: commandCaller{
				withoutAbilities: []domain.AbilityName{domain.AbilityNameGameServerUpdate},
			},
			storedCommand: domain.ServerTaskCommandReinstall,
			command:       domain.ServerTaskCommandRestart,
			wantStatus:    http.StatusForbidden,
			wantError:     "user does not have required permissions",
		},
		{
			name: "restart_into_an_update_by_a_token_without_the_update_ability",
			caller: commandCaller{
				tokenAbilities: []domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerRestart,
				},
			},
			storedCommand: domain.ServerTaskCommandRestart,
			command:       domain.ServerTaskCommandUpdate,
			wantStatus:    http.StatusForbidden,
			wantError:     "missing required ability: server:update",
		},
		{
			name: "update_into_a_restart_by_a_token_without_the_update_ability",
			caller: commandCaller{
				tokenAbilities: []domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerRestart,
				},
			},
			storedCommand: domain.ServerTaskCommandUpdate,
			command:       domain.ServerTaskCommandRestart,
			wantStatus:    http.StatusForbidden,
			wantError:     "missing required ability: server:update",
		},
		{
			name: "restart_into_a_reinstall_by_a_token_with_both_abilities",
			caller: commandCaller{
				tokenAbilities: []domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerRestart,
					domain.PATAbilityServerUpdate,
				},
			},
			storedCommand: domain.ServerTaskCommandRestart,
			command:       domain.ServerTaskCommandReinstall,
			wantStatus:    http.StatusOK,
		},
		{
			name:          "restart_into_a_reinstall_by_a_user_with_every_ability",
			caller:        commandCaller{},
			storedCommand: domain.ServerTaskCommandRestart,
			command:       domain.ServerTaskCommandReinstall,
			wantStatus:    http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env, credentials := givenCommandCaller(t, tt.caller)
			serverID := env.fixtures.Server1.ID

			stored := givenServerTask(t, env, tt.storedCommand, time.Now().Add(30*24*time.Hour))

			w := doRequestWithBody(
				t, env.securityTestEnv, http.MethodPut,
				fmt.Sprintf("/api/servers/%d/tasks/%d", serverID, stored.ID),
				credentials,
				serverTaskBody(t, tt.command, time.Now().Add(time.Minute)),
			)

			require.Equalf(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())

			tasks := serverTasks(t, env, serverID)
			require.Len(t, tasks, 1)

			if tt.wantError == "" {
				assert.Equal(t, tt.command, tasks[0].Command)
				assert.Equal(t, []domain.ServerTaskCommand{tt.command}, env.node.commands())

				return
			}

			assert.Contains(t, w.Body.String(), tt.wantError)
			assert.Equal(t, tt.storedCommand, tasks[0].Command, "a request that is refused leaves the task as it was")
			assert.Empty(t, env.node.commands(), "a request that is refused announces nothing to the node")
		})
	}
}

// OWASP API5:2023 — an administrator is held to the abilities of the token the
// request is made with, on the schedule as on the routes of the commands.
func TestRouterSecurity_API5_ServerTasks_AdministratorToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tokenAbilities []domain.PATAbility
		command        domain.ServerTaskCommand
		wantStatus     int
		wantError      string
	}{
		{
			name:           "token_that_manages_the_schedule_only",
			tokenAbilities: []domain.PATAbility{domain.PATAbilityServerTasksManage},
			command:        domain.ServerTaskCommandReinstall,
			wantStatus:     http.StatusForbidden,
			wantError:      "missing required ability: server:update",
		},
		{
			name: "token_with_the_ability_of_the_command",
			tokenAbilities: []domain.PATAbility{
				domain.PATAbilityServerTasksManage,
				domain.PATAbilityServerUpdate,
			},
			command:    domain.ServerTaskCommandReinstall,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "session_of_the_administrator",
			command:    domain.ServerTaskCommandReinstall,
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := setupScheduleSecurityTest(t)
			admin := env.fixtures.AdminUser
			serverID := env.fixtures.Server1.ID

			credentials := issuePASETOToken(t, env.securityTestEnv, admin)
			if tt.tokenAbilities != nil {
				credentials = issuePAT(t, env.securityTestEnv, admin, tt.tokenAbilities)
			}

			w := doRequestWithBody(
				t, env.securityTestEnv, http.MethodPost,
				fmt.Sprintf("/api/servers/%d/tasks", serverID),
				credentials,
				serverTaskBody(t, tt.command, time.Now().Add(time.Hour)),
			)

			require.Equalf(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())

			tasks := serverTasks(t, env, serverID)

			if tt.wantError == "" {
				require.Len(t, tasks, 1)
				assert.Equal(t, tt.command, tasks[0].Command)
				assert.Equal(t, []domain.ServerTaskCommand{tt.command}, env.node.commands())

				return
			}

			assert.Contains(t, w.Body.String(), tt.wantError)
			assert.Empty(t, tasks, "a request that is refused leaves no task behind")
			assert.Empty(t, env.node.commands(), "a request that is refused announces nothing to the node")
		})
	}
}

// OWASP API5:2023 — deleting a task runs nothing and takes the schedule alone;
// the tasks that are there stay readable to whoever manages the schedule.
func TestRouterSecurity_API5_ServerTasks_ScheduleAloneStillReadsAndDeletes(t *testing.T) {
	t.Parallel()

	callers := []commandCaller{
		{
			name: "user_with_the_schedule_alone",
			withoutAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerStart,
				domain.AbilityNameGameServerStop,
				domain.AbilityNameGameServerRestart,
				domain.AbilityNameGameServerUpdate,
			},
		},
		{
			name:           "token_that_manages_the_schedule_only",
			tokenAbilities: []domain.PATAbility{domain.PATAbilityServerTasksManage},
		},
	}

	for _, caller := range callers {
		t.Run(caller.name, func(t *testing.T) {
			t.Parallel()

			env, credentials := givenCommandCaller(t, caller)
			serverID := env.fixtures.Server1.ID

			stored := givenServerTask(t, env, domain.ServerTaskCommandReinstall, time.Now().Add(time.Hour))

			listed := doRequest(
				t, env.securityTestEnv, http.MethodGet,
				fmt.Sprintf("/api/servers/%d/tasks", serverID),
				credentials,
			)
			require.Equalf(t, http.StatusOK, listed.Code, "body=%s", listed.Body.String())
			assert.Contains(t, listed.Body.String(), `"reinstall"`)

			deleted := doRequest(
				t, env.securityTestEnv, http.MethodDelete,
				fmt.Sprintf("/api/servers/%d/tasks/%d", serverID, stored.ID),
				credentials,
			)
			require.Containsf(t, []int{http.StatusOK, http.StatusNoContent}, deleted.Code,
				"body=%s", deleted.Body.String())
			assert.Empty(t, serverTasks(t, env, serverID))
		})
	}
}
