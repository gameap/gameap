package domain

import (
	"slices"
	"time"
)

// PATAbility represents a personal access token ability.
type PATAbility string

const (
	PATAbilityServerCreate    PATAbility = "admin:server:create"
	PATAbilityGDaemonTaskRead PATAbility = "admin:gdaemon-task:read"
)

const (
	PATAbilityServerList           PATAbility = "server:list"
	PATAbilityServerStart          PATAbility = "server:start"
	PATAbilityServerStop           PATAbility = "server:stop"
	PATAbilityServerRestart        PATAbility = "server:restart"
	PATAbilityServerUpdate         PATAbility = "server:update"
	PATAbilityServerConsole        PATAbility = "server:console"
	PATAbilityServerRconConsole    PATAbility = "server:rcon-console"
	PATAbilityServerRconPlayers    PATAbility = "server:rcon-players"
	PATAbilityServerTasksManage    PATAbility = "server:tasks-manage"
	PATAbilityServerSettingsManage PATAbility = "server:settings-manage"
)

type PATAbilityGroup string

const (
	PATAbilityGroupServer      PATAbilityGroup = "server"
	PATAbilityGroupGDaemonTask PATAbilityGroup = "gdaemon-task"
)

type PersonalAccessToken struct {
	ID            uint          `db:"id"`
	TokenableType EntityType    `db:"tokenable_type"`
	TokenableID   uint          `db:"tokenable_id"`
	Name          string        `db:"name"`
	Token         string        `db:"token"`
	Abilities     *[]PATAbility `db:"abilities"`
	LastUsedAt    *time.Time    `db:"last_used_at"`
	CreatedAt     *time.Time    `db:"created_at"`
	UpdatedAt     *time.Time    `db:"updated_at"`
}

type PasswordReset struct {
	Email     string     `db:"email"`
	Token     string     `db:"token"`
	CreatedAt *time.Time `db:"created_at"`
}

func (token *PersonalAccessToken) HasAbility(ability PATAbility) bool {
	return slices.Contains(*token.Abilities, ability)
}

func (token *PersonalAccessToken) HasAnyAbility(abilities ...PATAbility) bool {
	return slices.ContainsFunc(abilities, token.HasAbility)
}

func (token *PersonalAccessToken) HasAllAbilities(abilities ...PATAbility) bool {
	for _, ability := range abilities {
		if !token.HasAbility(ability) {
			return false
		}
	}

	return true
}

type AbilityDescription struct {
	Ability     PATAbility
	Description string
}

type GroupedAbilities map[PATAbilityGroup][]AbilityDescription

func GetUserAbilities() []PATAbility {
	return []PATAbility{
		PATAbilityServerList,
		PATAbilityServerStart,
		PATAbilityServerStop,
		PATAbilityServerRestart,
		PATAbilityServerUpdate,
		PATAbilityServerConsole,
		PATAbilityServerRconConsole,
		PATAbilityServerRconPlayers,
		PATAbilityServerTasksManage,
		PATAbilityServerSettingsManage,
	}
}

func GetAdminAbilities() []PATAbility {
	return []PATAbility{
		PATAbilityServerCreate,
		PATAbilityGDaemonTaskRead,
	}
}

func GetAllAbilities() []PATAbility {
	abilities := GetUserAbilities()
	abilities = append(abilities, GetAdminAbilities()...)

	return abilities
}

func GetAbilityDescriptions() map[PATAbility]string {
	return map[PATAbility]string{
		PATAbilityServerCreate:         "Create game server",
		PATAbilityGDaemonTaskRead:      "Read GameAP Daemon task",
		PATAbilityServerList:           "List game servers",
		PATAbilityServerStart:          "Start game server",
		PATAbilityServerStop:           "Stop game server",
		PATAbilityServerRestart:        "Restart game server",
		PATAbilityServerUpdate:         "Update game server",
		PATAbilityServerConsole:        "Access to read and write into game server console",
		PATAbilityServerRconConsole:    "Access to game server RCON console",
		PATAbilityServerRconPlayers:    "Access to players management on game server",
		PATAbilityServerTasksManage:    "Manage game server tasks",
		PATAbilityServerSettingsManage: "Manage game server settings",
	}
}

func GetGroupedAbilities(includeAdmin bool) GroupedAbilities {
	grouped := make(GroupedAbilities)
	descriptions := GetAbilityDescriptions()

	serverAbilities := []AbilityDescription{
		{PATAbilityServerList, descriptions[PATAbilityServerList]},
		{PATAbilityServerStart, descriptions[PATAbilityServerStart]},
		{PATAbilityServerStop, descriptions[PATAbilityServerStop]},
		{PATAbilityServerRestart, descriptions[PATAbilityServerRestart]},
		{PATAbilityServerUpdate, descriptions[PATAbilityServerUpdate]},
		{PATAbilityServerConsole, descriptions[PATAbilityServerConsole]},
		{PATAbilityServerRconConsole, descriptions[PATAbilityServerRconConsole]},
		{PATAbilityServerRconPlayers, descriptions[PATAbilityServerRconPlayers]},
		{PATAbilityServerTasksManage, descriptions[PATAbilityServerTasksManage]},
		{PATAbilityServerSettingsManage, descriptions[PATAbilityServerSettingsManage]},
	}

	if includeAdmin {
		serverAbilities = append(serverAbilities, AbilityDescription{
			PATAbilityServerCreate, descriptions[PATAbilityServerCreate],
		})

		grouped[PATAbilityGroupGDaemonTask] = []AbilityDescription{
			{PATAbilityGDaemonTaskRead, descriptions[PATAbilityGDaemonTaskRead]},
		}
	}

	grouped[PATAbilityGroupServer] = serverAbilities

	return grouped
}

func ValidateAbility(ability string) bool {
	allAbilities := GetAllAbilities()
	for _, a := range allAbilities {
		if string(a) == ability {
			return true
		}
	}

	return false
}

func ParseAbilities(abilities []string) []PATAbility {
	var result []PATAbility
	for _, ability := range abilities {
		if ValidateAbility(ability) {
			result = append(result, PATAbility(ability))
		}
	}

	return result
}
