// API Security Tests for OWASP API Security Top 10:2023.
// Category: API5:2023 — Broken Function Level Authorization.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/
//
// A server task has the scheduler run a command on a game server. Managing
// the schedule (game-server-tasks, server:tasks-manage) used to be all it took,
// so a user without the right to update a server could have it reinstalled at
// a time of their choosing. These tests hold a scheduled command to what the
// same command takes when it is sent directly, of the user and of a personal
// access token.

package base_test

import (
	"context"
	"net/http"
	"testing"

	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	taskUserID   uint = 7
	taskServerID uint = 70
)

// directCommandAbilities repeats what the direct command endpoints take
// (postcommand), written out once more on purpose: the test is to notice a
// scheduled command that takes anything else.
var directCommandAbilities = map[domain.ServerTaskCommand][]domain.AbilityName{
	domain.ServerTaskCommandStart:     {domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerStart},
	domain.ServerTaskCommandStop:      {domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerStop},
	domain.ServerTaskCommandRestart:   {domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerRestart},
	domain.ServerTaskCommandUpdate:    {domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate},
	domain.ServerTaskCommandReinstall: {domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate},
}

// directCommandTokenAbility repeats the token abilities the routes of the
// direct commands are registered with.
var directCommandTokenAbility = map[domain.ServerTaskCommand]domain.PATAbility{
	domain.ServerTaskCommandStart:     domain.PATAbilityServerStart,
	domain.ServerTaskCommandStop:      domain.PATAbilityServerStop,
	domain.ServerTaskCommandRestart:   domain.PATAbilityServerRestart,
	domain.ServerTaskCommandUpdate:    domain.PATAbilityServerUpdate,
	domain.ServerTaskCommandReinstall: domain.PATAbilityServerUpdate,
}

func allowTaskUser(t *testing.T, repo *inmemory.RBACRepository, abilities ...domain.AbilityName) {
	t.Helper()

	for _, ability := range abilities {
		allowUserAbilityForServer(t, repo, taskUserID, taskServerID, ability)
	}
}

func requireForbidden(t *testing.T, err error, wantError string) {
	t.Helper()

	require.Error(t, err)
	assert.Contains(t, err.Error(), wantError)

	httpErr, ok := err.(interface{ HTTPStatus() int })
	require.True(t, ok, "the error has to carry an HTTP status")
	assert.Equal(t, http.StatusForbidden, httpErr.HTTPStatus())
}

// OWASP API5:2023 — a scheduled command takes the abilities the command takes
// when it is sent directly; no other ability stands in for them.
func TestAbilityChecker_CheckTaskCommandsOrError_EveryCommandTakesItsAbilities(t *testing.T) {
	t.Parallel()

	for command, abilities := range directCommandAbilities {
		t.Run(string(command)+"/allowed_with_its_abilities", func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, abilities...)

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, command)

			require.NoError(t, err)
		})

		for _, missing := range abilities {
			t.Run(string(command)+"/refused_without_"+string(missing), func(t *testing.T) {
				t.Parallel()

				rbacService, repo := setupRBAC(t)
				defer rbacService.Close()

				// Everything a user can be given on a server but the one
				// ability, the schedule itself included.
				for _, ability := range domain.ServersAbilities {
					if ability != missing {
						allowTaskUser(t, repo, ability)
					}
				}

				err := serversbase.NewAbilityChecker(rbacService).
					CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, command)

				requireForbidden(t, err, "user does not have required permissions")
			})
		}

		t.Run(string(command)+"/refused_with_the_schedule_alone", func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, domain.AbilityNameGameServerTasks)

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, command)

			requireForbidden(t, err, "user does not have required permissions")
		})

		t.Run(string(command)+"/refused_with_the_abilities_on_another_server", func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			for _, ability := range abilities {
				allowUserAbilityForServer(t, repo, taskUserID, taskServerID+1, ability)
			}

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, command)

			requireForbidden(t, err, "user does not have required permissions")
		})

		t.Run(string(command)+"/allowed_for_an_administrator", func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			assignRoleToUser(t, repo, taskUserID, createAdminRole(t, repo))

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, command)

			require.NoError(t, err)
		})
	}
}

// OWASP API5:2023 — a task that is changed takes the abilities of the command
// it has and of the command it is given, together.
func TestAbilityChecker_CheckTaskCommandsOrError_SeveralCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		granted   []domain.AbilityName
		commands  []domain.ServerTaskCommand
		wantError string
	}{
		{
			name: "abilities_of_both_commands",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
				domain.AbilityNameGameServerUpdate,
			},
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandUpdate, domain.ServerTaskCommandRestart},
		},
		{
			name: "abilities_of_the_first_command_only",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandRestart, domain.ServerTaskCommandReinstall},
			wantError: "user does not have required permissions",
		},
		{
			name: "abilities_of_the_second_command_only",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandUpdate, domain.ServerTaskCommandRestart},
			wantError: "user does not have required permissions",
		},
		{
			name: "same_command_twice",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStop,
			},
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandStop, domain.ServerTaskCommandStop},
		},
		{
			name: "update_and_reinstall_share_an_ability",
			granted: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandUpdate, domain.ServerTaskCommandReinstall},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, tt.granted...)

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, tt.commands...)

			if tt.wantError != "" {
				requireForbidden(t, err, tt.wantError)

				return
			}

			require.NoError(t, err)
		})
	}
}

// OWASP API5:2023 — a command the schedule does not know is refused instead of
// taking nothing, for an administrator as for anybody else.
func TestAbilityChecker_CheckTaskCommandsOrError_UnknownCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		commands []domain.ServerTaskCommand
		admin    bool
	}{
		{name: "command_of_the_direct_endpoints_only", commands: []domain.ServerTaskCommand{"install"}},
		{name: "empty_command", commands: []domain.ServerTaskCommand{""}},
		{name: "command_in_another_case", commands: []domain.ServerTaskCommand{"Restart"}},
		{
			name:     "next_to_a_known_command",
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandRestart, "delete"},
		},
		{name: "asked_by_an_administrator", commands: []domain.ServerTaskCommand{"install"}, admin: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, domain.ServersAbilities...)
			if tt.admin {
				assignRoleToUser(t, repo, taskUserID, createAdminRole(t, repo))
			}

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(context.Background(), taskUserID, taskServerID, tt.commands...)

			requireForbidden(t, err, "cannot be scheduled")
		})
	}
}

func tokenContext(abilities *[]domain.PATAbility) context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		User:  &domain.User{ID: taskUserID},
		Token: &domain.PersonalAccessToken{ID: 5, Abilities: abilities},
	})
}

// OWASP API5:2023 — a personal access token is held to the abilities it was
// granted: managing the schedule does not let it schedule a command it cannot
// send.
func TestAbilityChecker_CheckTaskCommandsOrError_Token(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ctx       func() context.Context
		commands  []domain.ServerTaskCommand
		admin     bool
		wantError string
	}{
		{
			name: "session_of_a_user_takes_no_token_ability",
			ctx: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{User: &domain.User{ID: taskUserID}})
			},
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandReinstall},
		},
		{
			name:     "request_without_a_session_takes_no_token_ability",
			ctx:      context.Background,
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandReinstall},
		},
		{
			name: "token_with_the_ability_of_the_command",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerRestart,
				})
			},
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandRestart},
		},
		{
			name: "token_that_manages_the_schedule_only",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{domain.PATAbilityServerTasksManage})
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandRestart},
			wantError: "missing required ability: server:restart",
		},
		{
			name: "token_with_the_ability_of_another_command",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerStart,
					domain.PATAbilityServerStop,
					domain.PATAbilityServerRestart,
				})
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandReinstall},
			wantError: "missing required ability: server:update",
		},
		{
			name: "token_with_the_ability_of_one_command_out_of_two",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerRestart,
				})
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandUpdate, domain.ServerTaskCommandRestart},
			wantError: "missing required ability: server:update",
		},
		{
			name: "token_with_the_ability_of_the_first_command_out_of_two",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{
					domain.PATAbilityServerTasksManage,
					domain.PATAbilityServerRestart,
				})
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandRestart, domain.ServerTaskCommandUpdate},
			wantError: "missing required ability: server:update",
		},
		{
			name: "token_with_the_abilities_of_both_commands",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{
					domain.PATAbilityServerUpdate,
					domain.PATAbilityServerRestart,
				})
			},
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandUpdate, domain.ServerTaskCommandRestart},
		},
		{
			name: "token_without_abilities",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{})
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandStart},
			wantError: "missing required ability: server:start",
		},
		{
			name: "token_whose_abilities_are_not_set",
			ctx: func() context.Context {
				return tokenContext(nil)
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandStop},
			wantError: "missing required ability: server:stop",
		},
		{
			name: "token_of_an_administrator_is_held_to_its_abilities_too",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{domain.PATAbilityServerTasksManage})
			},
			commands:  []domain.ServerTaskCommand{domain.ServerTaskCommandUpdate},
			admin:     true,
			wantError: "missing required ability: server:update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, domain.ServersAbilities...)
			if tt.admin {
				assignRoleToUser(t, repo, taskUserID, createAdminRole(t, repo))
			}

			err := serversbase.NewAbilityChecker(rbacService).
				CheckTaskCommandsOrError(tt.ctx(), taskUserID, taskServerID, tt.commands...)

			if tt.wantError != "" {
				requireForbidden(t, err, tt.wantError)

				return
			}

			require.NoError(t, err)
		})
	}
}

// OWASP API5:2023 — every command takes the token ability its direct route is
// registered with, and no other token ability stands in for it.
func TestAbilityChecker_CheckTaskCommandsOrError_EveryCommandTakesItsTokenAbility(t *testing.T) {
	t.Parallel()

	for command, tokenAbility := range directCommandTokenAbility {
		t.Run(string(command)+"/allowed_with_its_token_ability", func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, domain.ServersAbilities...)

			err := serversbase.NewAbilityChecker(rbacService).CheckTaskCommandsOrError(
				tokenContext(&[]domain.PATAbility{tokenAbility}), taskUserID, taskServerID, command,
			)

			require.NoError(t, err)
		})

		t.Run(string(command)+"/refused_with_every_other_token_ability", func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, domain.ServersAbilities...)

			others := make([]domain.PATAbility, 0, len(domain.GetAllAbilities()))
			for _, ability := range domain.GetAllAbilities() {
				if ability != tokenAbility {
					others = append(others, ability)
				}
			}

			err := serversbase.NewAbilityChecker(rbacService).CheckTaskCommandsOrError(
				tokenContext(&others), taskUserID, taskServerID, command,
			)

			requireForbidden(t, err, "missing required ability: "+string(tokenAbility))
		})
	}
}

// OWASP API5:2023 — a refusal leaves a record that names what was missing.
func TestAbilityChecker_CheckTaskCommandsOrError_Audit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ctx        func() context.Context
		granted    []domain.AbilityName
		commands   []domain.ServerTaskCommand
		wantReason string
		wantExtra  map[string]string
	}{
		{
			name:       "user_without_the_abilities_of_the_commands",
			ctx:        context.Background,
			granted:    []domain.AbilityName{domain.AbilityNameGameServerTasks},
			commands:   []domain.ServerTaskCommand{domain.ServerTaskCommandRestart, domain.ServerTaskCommandReinstall},
			wantReason: "missing_ability",
			wantExtra: map[string]string{
				"required_abilities": "game-server-common,game-server-restart,game-server-update",
			},
		},
		{
			name: "token_without_the_ability_of_the_command",
			ctx: func() context.Context {
				return tokenContext(&[]domain.PATAbility{domain.PATAbilityServerTasksManage})
			},
			granted:    domain.ServersAbilities,
			commands:   []domain.ServerTaskCommand{domain.ServerTaskCommandReinstall},
			wantReason: "missing_token_ability",
			wantExtra:  map[string]string{"required_abilities": "server:update"},
		},
		{
			name:       "unknown_command",
			ctx:        context.Background,
			granted:    domain.ServersAbilities,
			commands:   []domain.ServerTaskCommand{"install"},
			wantReason: "unknown_task_command",
			wantExtra:  map[string]string{"command": "install"},
		},
		{
			name:     "allowed_request_leaves_no_record",
			ctx:      context.Background,
			granted:  domain.ServersAbilities,
			commands: []domain.ServerTaskCommand{domain.ServerTaskCommandReinstall},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			allowTaskUser(t, repo, tt.granted...)

			capture := &auditCapture{}
			checker := serversbase.NewAbilityChecker(rbacService, serversbase.WithAuditLogger(capture))

			err := checker.CheckTaskCommandsOrError(tt.ctx(), taskUserID, taskServerID, tt.commands...)

			events := capture.snapshot()

			if tt.wantReason == "" {
				require.NoError(t, err)
				assert.Empty(t, events)

				return
			}

			require.Error(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, audit.EventAccessDenied, events[0].Type)
			assert.Equal(t, "server", events[0].ResourceType)
			assert.Equal(t, "70", events[0].ResourceID)
			assert.Equal(t, tt.wantReason, events[0].Reason)

			for key, want := range tt.wantExtra {
				got, ok := extraString(events[0], key)
				require.True(t, ok, "the record has to carry %q", key)
				assert.Equal(t, want, got)
			}
		})
	}
}
