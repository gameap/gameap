package domain

import (
	"time"

	"github.com/samber/lo"
)

type EntityType string

const (
	EntityTypeEmpty             EntityType = ""
	EntityTypeUser              EntityType = "Gameap\\Models\\User"
	EntityTypeNode              EntityType = "Gameap\\Models\\DedicatedServer"
	EntityTypeClientCertificate EntityType = "Gameap\\Models\\ClientCertificate"
	EntityTypeGame              EntityType = "Gameap\\Models\\Game"
	EntityTypeGameMod           EntityType = "Gameap\\Models\\GameMod"
	EntityTypeServer            EntityType = "Gameap\\Models\\Server"
	EntityTypeRole              EntityType = "roles"
)

type AbilityName string

const (
	// Game Server Abilities.
	AbilityNameGameServerCommon      AbilityName = "game-server-common"
	AbilityNameGameServerStart       AbilityName = "game-server-start"
	AbilityNameGameServerStop        AbilityName = "game-server-stop"
	AbilityNameGameServerRestart     AbilityName = "game-server-restart"
	AbilityNameGameServerPause       AbilityName = "game-server-pause"
	AbilityNameGameServerUpdate      AbilityName = "game-server-update"
	AbilityNameGameServerFiles       AbilityName = "game-server-files"
	AbilityNameGameServerTasks       AbilityName = "game-server-tasks"
	AbilityNameGameServerSettings    AbilityName = "game-server-settings"
	AbilityNameGameServerConsoleView AbilityName = "game-server-console-view"
	AbilityNameGameServerConsoleSend AbilityName = "game-server-console-send"
	AbilityNameGameServerRconConsole AbilityName = "game-server-rcon-console"
	AbilityNameGameServerRconPlayers AbilityName = "game-server-rcon-players"

	// General.
	AbilityNameCreate AbilityName = "create"
	AbilityNameView   AbilityName = "view"
	AbilityNameEdit   AbilityName = "edit"
	AbilityNameDelete AbilityName = "delete"

	// Admin ability.
	AbilityNameAdminRolesPermissions AbilityName = "admin roles & permissions"
)

var ServersAbilities = []AbilityName{
	AbilityNameGameServerCommon,
	AbilityNameGameServerStart,
	AbilityNameGameServerStop,
	AbilityNameGameServerRestart,
	AbilityNameGameServerPause,
	AbilityNameGameServerUpdate,
	AbilityNameGameServerFiles,
	AbilityNameGameServerTasks,
	AbilityNameGameServerSettings,

	// Console
	AbilityNameGameServerConsoleView,
	AbilityNameGameServerConsoleSend,

	// Rcon
	AbilityNameGameServerRconConsole,
	AbilityNameGameServerRconPlayers,
}

type Ability struct {
	ID         uint        `db:"id"`
	Name       AbilityName `db:"name"`
	Title      *string     `db:"title"`
	EntityID   *uint       `db:"entity_id"`
	EntityType *EntityType `db:"entity_type"`
	OnlyOwned  bool        `db:"only_owned"`
	Options    *string     `db:"options"`
	Scope      *int        `db:"scope"`
	CreatedAt  *time.Time  `db:"created_at"`
	UpdatedAt  *time.Time  `db:"updated_at"`
}

func CreateAbilityForEntity(name AbilityName, entityID uint, entityType EntityType) Ability {
	return Ability{
		Name:       name,
		EntityID:   &entityID,
		EntityType: &entityType,
		OnlyOwned:  false,
		Options:    nil,
		Scope:      nil,
		CreatedAt:  lo.ToPtr(time.Now()),
		UpdatedAt:  lo.ToPtr(time.Now()),
	}
}

type Role struct {
	ID        uint       `db:"id"`
	Name      string     `db:"name"`
	Title     *string    `db:"title"`
	Level     *uint      `db:"level"`
	Scope     *int       `db:"scope"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// RestrictedRole is used with role assignments to represent roles with restrictions.
// If you want to assign a role to an entity, you can.
type RestrictedRole struct {
	Role

	RestrictedToID   *uint       `db:"-"`
	RestrictedToType *EntityType `db:"-"`
}

func NewRestrictedRoleFromRole(role Role) RestrictedRole {
	return RestrictedRole{
		Role:             role,
		RestrictedToID:   nil,
		RestrictedToType: nil,
	}
}

type Permission struct {
	ID         uint        `db:"id"`
	AbilityID  uint        `db:"ability_id"`
	EntityID   *uint       `db:"entity_id"`
	EntityType *EntityType `db:"entity_type"`
	Forbidden  bool        `db:"forbidden"`
	Scope      *int        `db:"scope"`
	Ability    *Ability    `db:"-"`
}

type AssignedRole struct {
	ID               uint        `db:"id"`
	RoleID           uint        `db:"role_id"`
	EntityID         uint        `db:"entity_id"`
	EntityType       EntityType  `db:"entity_type"`
	RestrictedToID   *uint       `db:"restricted_to_id"`
	RestrictedToType *EntityType `db:"restricted_to_type"`
	Scope            *int        `db:"scope"`
}
