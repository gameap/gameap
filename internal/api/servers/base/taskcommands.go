package base

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// taskCommandRights is what it takes to have a command run on a game server.
type taskCommandRights struct {
	// abilities are the abilities of the user on the server.
	abilities []domain.AbilityName
	// tokenAbility is the ability a personal access token is granted for the
	// command.
	tokenAbility domain.PATAbility
}

// taskCommandsRights gives, for every command a server task can schedule, what
// the same command takes when it is sent directly: the abilities postcommand
// checks and the token ability its route is registered with. A task that took
// less would let its author have the scheduler run what they may not run
// themselves, on a schedule of their choosing.
var taskCommandsRights = map[domain.ServerTaskCommand]taskCommandRights{
	domain.ServerTaskCommandStart: {
		abilities:    []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerStart},
		tokenAbility: domain.PATAbilityServerStart,
	},
	domain.ServerTaskCommandStop: {
		abilities:    []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerStop},
		tokenAbility: domain.PATAbilityServerStop,
	},
	domain.ServerTaskCommandRestart: {
		abilities:    []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerRestart},
		tokenAbility: domain.PATAbilityServerRestart,
	},
	domain.ServerTaskCommandUpdate: {
		abilities:    []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate},
		tokenAbility: domain.PATAbilityServerUpdate,
	},
	domain.ServerTaskCommandReinstall: {
		abilities:    []domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerUpdate},
		tokenAbility: domain.PATAbilityServerUpdate,
	},
}

// CheckTaskCommandsOrError checks that the caller may have the commands run on
// the server by its schedule.
//
// It comes on top of the game-server-tasks check and does not replace it:
// managing the schedule of a server does not include the commands the schedule
// runs. Each command takes what it takes when it is sent directly, of the user
// and, where the request is made with a personal access token, of the token. A
// command that is not known is refused for everybody.
func (c *AbilityChecker) CheckTaskCommandsOrError(
	ctx context.Context,
	userID uint,
	serverID uint,
	commands ...domain.ServerTaskCommand,
) error {
	abilities := make([]domain.AbilityName, 0, 2*len(commands))
	tokenAbilities := make([]domain.PATAbility, 0, len(commands))

	for _, command := range commands {
		rights, known := taskCommandsRights[command]
		if !known {
			audit.AccessDenied(
				ctx,
				c.audit,
				"server",
				strconv.FormatUint(uint64(serverID), 10),
				"unknown_task_command",
				slog.String("command", string(command)),
			)

			return api.WrapHTTPError(
				errors.Errorf("command %q cannot be scheduled", command),
				http.StatusForbidden,
			)
		}

		for _, ability := range rights.abilities {
			if !slices.Contains(abilities, ability) {
				abilities = append(abilities, ability)
			}
		}

		if !slices.Contains(tokenAbilities, rights.tokenAbility) {
			tokenAbilities = append(tokenAbilities, rights.tokenAbility)
		}
	}

	if err := c.CheckOrError(ctx, userID, serverID, abilities); err != nil {
		return err
	}

	return c.checkTokenAbilities(ctx, serverID, tokenAbilities)
}

// checkTokenAbilities holds a personal access token to the abilities it was
// granted. The route of the schedule asks a token for server:tasks-manage
// alone, whatever the command; the commands themselves are scoped one by one,
// and a token that lives in a third-party system must not reach past its scope
// by scheduling what it cannot send.
func (c *AbilityChecker) checkTokenAbilities(
	ctx context.Context,
	serverID uint,
	abilities []domain.PATAbility,
) error {
	session := auth.SessionFromContext(ctx)
	if !session.IsTokenSession() {
		return nil
	}

	for _, ability := range abilities {
		if session.Token.Abilities != nil && session.Token.HasAbility(ability) {
			continue
		}

		audit.AccessDenied(
			ctx,
			c.audit,
			"server",
			strconv.FormatUint(uint64(serverID), 10),
			"missing_token_ability",
			slog.String("required_abilities", string(ability)),
		)

		return api.WrapHTTPError(
			errors.Errorf("missing required ability: %s", ability),
			http.StatusForbidden,
		)
	}

	return nil
}
