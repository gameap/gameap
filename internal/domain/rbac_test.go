package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJSONPayload = `{"key":"value"}`

func TestEntityTypeConstants(t *testing.T) {
	assert.Equal(t, EntityType(""), EntityTypeEmpty)
	assert.Equal(t, EntityType("Gameap\\Models\\User"), EntityTypeUser)
	assert.Equal(t, EntityType("Gameap\\Models\\DedicatedServer"), EntityTypeNode)
	assert.Equal(t, EntityType("Gameap\\Models\\ClientCertificate"), EntityTypeClientCertificate)
	assert.Equal(t, EntityType("Gameap\\Models\\Game"), EntityTypeGame)
	assert.Equal(t, EntityType("Gameap\\Models\\GameMod"), EntityTypeGameMod)
	assert.Equal(t, EntityType("Gameap\\Models\\Server"), EntityTypeServer)
	assert.Equal(t, EntityType("roles"), EntityTypeRole)
}

func TestAbilityNameConstants_GameServer(t *testing.T) {
	assert.Equal(t, AbilityName("game-server-common"), AbilityNameGameServerCommon)
	assert.Equal(t, AbilityName("game-server-start"), AbilityNameGameServerStart)
	assert.Equal(t, AbilityName("game-server-stop"), AbilityNameGameServerStop)
	assert.Equal(t, AbilityName("game-server-restart"), AbilityNameGameServerRestart)
	assert.Equal(t, AbilityName("game-server-pause"), AbilityNameGameServerPause)
	assert.Equal(t, AbilityName("game-server-update"), AbilityNameGameServerUpdate)
	assert.Equal(t, AbilityName("game-server-files"), AbilityNameGameServerFiles)
	assert.Equal(t, AbilityName("game-server-tasks"), AbilityNameGameServerTasks)
	assert.Equal(t, AbilityName("game-server-settings"), AbilityNameGameServerSettings)
	assert.Equal(t, AbilityName("game-server-console-view"), AbilityNameGameServerConsoleView)
	assert.Equal(t, AbilityName("game-server-console-send"), AbilityNameGameServerConsoleSend)
	assert.Equal(t, AbilityName("game-server-rcon-console"), AbilityNameGameServerRconConsole)
	assert.Equal(t, AbilityName("game-server-rcon-players"), AbilityNameGameServerRconPlayers)
}

func TestAbilityNameConstants_General(t *testing.T) {
	assert.Equal(t, AbilityName("create"), AbilityNameCreate)
	assert.Equal(t, AbilityName("view"), AbilityNameView)
	assert.Equal(t, AbilityName("edit"), AbilityNameEdit)
	assert.Equal(t, AbilityName("delete"), AbilityNameDelete)
}

func TestAbilityNameConstants_Admin(t *testing.T) {
	assert.Equal(t, AbilityName("admin roles & permissions"), AbilityNameAdminRolesPermissions)
}

func TestServersAbilities(t *testing.T) {
	expectedAbilities := []AbilityName{
		AbilityNameGameServerCommon,
		AbilityNameGameServerStart,
		AbilityNameGameServerStop,
		AbilityNameGameServerRestart,
		AbilityNameGameServerPause,
		AbilityNameGameServerUpdate,
		AbilityNameGameServerFiles,
		AbilityNameGameServerTasks,
		AbilityNameGameServerSettings,
		AbilityNameGameServerConsoleView,
		AbilityNameGameServerConsoleSend,
		AbilityNameGameServerRconConsole,
		AbilityNameGameServerRconPlayers,
	}

	assert.Equal(t, len(expectedAbilities), len(ServersAbilities), "should have 13 server abilities")
	assert.Equal(t, expectedAbilities, ServersAbilities)

	for _, ability := range expectedAbilities {
		assert.Contains(t, ServersAbilities, ability)
	}
}

func TestCreateAbilityForEntity(t *testing.T) {
	tests := []struct {
		name        string
		abilityName AbilityName
		entityID    uint
		entityType  EntityType
	}{
		{
			name:        "create_server_ability",
			abilityName: AbilityNameGameServerStart,
			entityID:    1,
			entityType:  EntityTypeServer,
		},
		{
			name:        "create_user_ability",
			abilityName: AbilityNameView,
			entityID:    42,
			entityType:  EntityTypeUser,
		},
		{
			name:        "create_node_ability",
			abilityName: AbilityNameEdit,
			entityID:    100,
			entityType:  EntityTypeNode,
		},
		{
			name:        "create_game_ability",
			abilityName: AbilityNameCreate,
			entityID:    5,
			entityType:  EntityTypeGame,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := time.Now()
			ability := CreateAbilityForEntity(test.abilityName, test.entityID, test.entityType)
			after := time.Now()

			assert.Equal(t, test.abilityName, ability.Name)
			require.NotNil(t, ability.EntityID)
			assert.Equal(t, test.entityID, *ability.EntityID)
			require.NotNil(t, ability.EntityType)
			assert.Equal(t, test.entityType, *ability.EntityType)
			assert.False(t, ability.OnlyOwned)
			assert.Nil(t, ability.Options)
			assert.Nil(t, ability.Scope)
			assert.Nil(t, ability.Title)

			require.NotNil(t, ability.CreatedAt)
			assert.True(t, ability.CreatedAt.After(before) || ability.CreatedAt.Equal(before))
			assert.True(t, ability.CreatedAt.Before(after) || ability.CreatedAt.Equal(after))

			require.NotNil(t, ability.UpdatedAt)
			assert.True(t, ability.UpdatedAt.After(before) || ability.UpdatedAt.Equal(before))
			assert.True(t, ability.UpdatedAt.Before(after) || ability.UpdatedAt.Equal(after))
		})
	}
}

func TestCreateAbilityForEntity_TimestampsSet(t *testing.T) {
	ability1 := CreateAbilityForEntity(AbilityNameGameServerStart, 1, EntityTypeServer)
	time.Sleep(10 * time.Millisecond)
	ability2 := CreateAbilityForEntity(AbilityNameGameServerStop, 2, EntityTypeServer)

	require.NotNil(t, ability1.CreatedAt)
	require.NotNil(t, ability2.CreatedAt)
	assert.True(t, ability2.CreatedAt.After(*ability1.CreatedAt))
}

func TestNewRestrictedRoleFromRole(t *testing.T) {
	tests := []struct {
		name string
		role Role
	}{
		{
			name: "basic_role",
			role: Role{
				ID:    1,
				Name:  "admin",
				Title: stringPtr("Administrator"),
				Level: uintPtr(10),
			},
		},
		{
			name: "role_without_optional_fields",
			role: Role{
				ID:   2,
				Name: "user",
			},
		},
		{
			name: "role_with_all_fields",
			role: Role{
				ID:        3,
				Name:      "moderator",
				Title:     stringPtr("Moderator"),
				Level:     uintPtr(5),
				Scope:     intPtr(1),
				CreatedAt: timePtr(time.Now()),
				UpdatedAt: timePtr(time.Now()),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restrictedRole := NewRestrictedRoleFromRole(test.role)

			assert.Equal(t, test.role.ID, restrictedRole.ID)
			assert.Equal(t, test.role.Name, restrictedRole.Name)
			assert.Equal(t, test.role.Title, restrictedRole.Title)
			assert.Equal(t, test.role.Level, restrictedRole.Level)
			assert.Equal(t, test.role.Scope, restrictedRole.Scope)
			assert.Equal(t, test.role.CreatedAt, restrictedRole.CreatedAt)
			assert.Equal(t, test.role.UpdatedAt, restrictedRole.UpdatedAt)

			assert.Nil(t, restrictedRole.RestrictedToID)
			assert.Nil(t, restrictedRole.RestrictedToType)
		})
	}
}

func TestAbility_Fields(t *testing.T) {
	now := time.Now()
	entityID := uint(42)
	entityType := EntityTypeServer
	title := "Test Ability"
	options := testJSONPayload
	scope := 1

	ability := Ability{
		ID:         1,
		Name:       AbilityNameGameServerStart,
		Title:      &title,
		EntityID:   &entityID,
		EntityType: &entityType,
		OnlyOwned:  true,
		Options:    &options,
		Scope:      &scope,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}

	assert.Equal(t, uint(1), ability.ID)
	assert.Equal(t, AbilityNameGameServerStart, ability.Name)
	assert.Equal(t, &title, ability.Title)
	assert.Equal(t, &entityID, ability.EntityID)
	assert.Equal(t, &entityType, ability.EntityType)
	assert.True(t, ability.OnlyOwned)
	assert.Equal(t, &options, ability.Options)
	assert.Equal(t, &scope, ability.Scope)
	assert.Equal(t, &now, ability.CreatedAt)
	assert.Equal(t, &now, ability.UpdatedAt)
}

func TestRole_Fields(t *testing.T) {
	now := time.Now()
	title := "Administrator"
	level := uint(10)
	scope := 1

	role := Role{
		ID:        1,
		Name:      "admin",
		Title:     &title,
		Level:     &level,
		Scope:     &scope,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	assert.Equal(t, uint(1), role.ID)
	assert.Equal(t, "admin", role.Name)
	assert.Equal(t, &title, role.Title)
	assert.Equal(t, &level, role.Level)
	assert.Equal(t, &scope, role.Scope)
	assert.Equal(t, &now, role.CreatedAt)
	assert.Equal(t, &now, role.UpdatedAt)
}

func TestRestrictedRole_Fields(t *testing.T) {
	now := time.Now()
	title := "Moderator"
	level := uint(5)
	restrictedToID := uint(100)
	restrictedToType := EntityTypeServer

	restrictedRole := RestrictedRole{
		Role: Role{
			ID:        1,
			Name:      "moderator",
			Title:     &title,
			Level:     &level,
			CreatedAt: &now,
			UpdatedAt: &now,
		},
		RestrictedToID:   &restrictedToID,
		RestrictedToType: &restrictedToType,
	}

	assert.Equal(t, uint(1), restrictedRole.ID)
	assert.Equal(t, "moderator", restrictedRole.Name)
	assert.Equal(t, &title, restrictedRole.Title)
	assert.Equal(t, &level, restrictedRole.Level)
	assert.Equal(t, &now, restrictedRole.CreatedAt)
	assert.Equal(t, &now, restrictedRole.UpdatedAt)
	assert.Equal(t, &restrictedToID, restrictedRole.RestrictedToID)
	assert.Equal(t, &restrictedToType, restrictedRole.RestrictedToType)
}

func TestPermission_Fields(t *testing.T) {
	entityID := uint(42)
	entityType := EntityTypeServer
	scope := 1
	ability := &Ability{
		ID:   10,
		Name: AbilityNameGameServerStart,
	}

	permission := Permission{
		ID:         1,
		AbilityID:  10,
		EntityID:   &entityID,
		EntityType: &entityType,
		Forbidden:  true,
		Scope:      &scope,
		Ability:    ability,
	}

	assert.Equal(t, uint(1), permission.ID)
	assert.Equal(t, uint(10), permission.AbilityID)
	assert.Equal(t, &entityID, permission.EntityID)
	assert.Equal(t, &entityType, permission.EntityType)
	assert.True(t, permission.Forbidden)
	assert.Equal(t, &scope, permission.Scope)
	assert.Equal(t, ability, permission.Ability)
}

func TestPermission_NotForbidden(t *testing.T) {
	permission := Permission{
		ID:        1,
		AbilityID: 10,
		Forbidden: false,
	}

	assert.False(t, permission.Forbidden)
}

func TestAssignedRole_Fields(t *testing.T) {
	restrictedToID := uint(100)
	restrictedToType := EntityTypeServer
	scope := 1

	assignedRole := AssignedRole{
		ID:               1,
		RoleID:           5,
		EntityID:         42,
		EntityType:       EntityTypeUser,
		RestrictedToID:   &restrictedToID,
		RestrictedToType: &restrictedToType,
		Scope:            &scope,
	}

	assert.Equal(t, uint(1), assignedRole.ID)
	assert.Equal(t, uint(5), assignedRole.RoleID)
	assert.Equal(t, uint(42), assignedRole.EntityID)
	assert.Equal(t, EntityTypeUser, assignedRole.EntityType)
	assert.Equal(t, &restrictedToID, assignedRole.RestrictedToID)
	assert.Equal(t, &restrictedToType, assignedRole.RestrictedToType)
	assert.Equal(t, &scope, assignedRole.Scope)
}

func TestAssignedRole_WithoutRestrictions(t *testing.T) {
	assignedRole := AssignedRole{
		ID:               1,
		RoleID:           5,
		EntityID:         42,
		EntityType:       EntityTypeUser,
		RestrictedToID:   nil,
		RestrictedToType: nil,
		Scope:            nil,
	}

	assert.Nil(t, assignedRole.RestrictedToID)
	assert.Nil(t, assignedRole.RestrictedToType)
	assert.Nil(t, assignedRole.Scope)
}

func TestServersAbilities_NoGeneralAbilities(t *testing.T) {
	generalAbilities := []AbilityName{
		AbilityNameCreate,
		AbilityNameView,
		AbilityNameEdit,
		AbilityNameDelete,
		AbilityNameAdminRolesPermissions,
	}

	for _, generalAbility := range generalAbilities {
		assert.NotContains(t, ServersAbilities, generalAbility,
			"ServersAbilities should not contain general ability %s", generalAbility)
	}
}

func TestCreateAbilityForEntity_DifferentEntityTypes(t *testing.T) {
	entityTypes := []EntityType{
		EntityTypeUser,
		EntityTypeNode,
		EntityTypeClientCertificate,
		EntityTypeGame,
		EntityTypeGameMod,
		EntityTypeServer,
		EntityTypeRole,
	}

	for _, entityType := range entityTypes {
		t.Run(string(entityType), func(t *testing.T) {
			ability := CreateAbilityForEntity(AbilityNameView, 1, entityType)

			require.NotNil(t, ability.EntityType)
			assert.Equal(t, entityType, *ability.EntityType)
		})
	}
}

func TestRestrictedRole_CanBeRestricted(t *testing.T) {
	role := Role{
		ID:   1,
		Name: "admin",
	}

	restrictedRole := NewRestrictedRoleFromRole(role)

	restrictedToID := uint(42)
	restrictedToType := EntityTypeServer

	restrictedRole.RestrictedToID = &restrictedToID
	restrictedRole.RestrictedToType = &restrictedToType

	assert.Equal(t, &restrictedToID, restrictedRole.RestrictedToID)
	assert.Equal(t, &restrictedToType, restrictedRole.RestrictedToType)
}

func stringPtr(s string) *string {
	return &s
}

func uintPtr(u uint) *uint {
	return &u
}

func intPtr(i int) *int {
	return &i
}

func timePtr(t time.Time) *time.Time {
	return &t
}
