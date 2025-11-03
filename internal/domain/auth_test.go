package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalAccessToken_HasAbility(t *testing.T) {
	tests := []struct {
		name    string
		token   *PersonalAccessToken
		ability PATAbility
		want    bool
	}{
		{
			name: "has_single_ability",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStart},
			},
			ability: PATAbilityServerStart,
			want:    true,
		},
		{
			name: "has_ability_among_multiple",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{
					PATAbilityServerStart,
					PATAbilityServerStop,
					PATAbilityServerRestart,
				},
			},
			ability: PATAbilityServerStop,
			want:    true,
		},
		{
			name: "does_not_have_ability",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStart},
			},
			ability: PATAbilityServerStop,
			want:    false,
		},
		{
			name: "empty_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{},
			},
			ability: PATAbilityServerStart,
			want:    false,
		},
		{
			name: "admin_ability_present",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{
					PATAbilityServerCreate,
					PATAbilityGDaemonTaskRead,
				},
			},
			ability: PATAbilityServerCreate,
			want:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.token.HasAbility(test.ability)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestPersonalAccessToken_HasAnyAbility(t *testing.T) {
	tests := []struct {
		name      string
		token     *PersonalAccessToken
		abilities []PATAbility
		want      bool
	}{
		{
			name: "has_first_ability",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStart},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      true,
		},
		{
			name: "has_second_ability",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStop},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      true,
		},
		{
			name: "has_none_of_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerRestart},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      false,
		},
		{
			name: "has_all_requested_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{
					PATAbilityServerStart,
					PATAbilityServerStop,
					PATAbilityServerRestart,
				},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      true,
		},
		{
			name: "empty_abilities_check",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStart},
			},
			abilities: []PATAbility{},
			want:      false,
		},
		{
			name: "token_has_no_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{},
			},
			abilities: []PATAbility{PATAbilityServerStart},
			want:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.token.HasAnyAbility(test.abilities...)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestPersonalAccessToken_HasAllAbilities(t *testing.T) {
	tests := []struct {
		name      string
		token     *PersonalAccessToken
		abilities []PATAbility
		want      bool
	}{
		{
			name: "has_all_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{
					PATAbilityServerStart,
					PATAbilityServerStop,
					PATAbilityServerRestart,
				},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      true,
		},
		{
			name: "missing_one_ability",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStart},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      false,
		},
		{
			name: "has_exact_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{
					PATAbilityServerStart,
					PATAbilityServerStop,
				},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      true,
		},
		{
			name: "has_none_of_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerRestart},
			},
			abilities: []PATAbility{PATAbilityServerStart, PATAbilityServerStop},
			want:      false,
		},
		{
			name: "empty_abilities_check",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{PATAbilityServerStart},
			},
			abilities: []PATAbility{},
			want:      true,
		},
		{
			name: "token_has_no_abilities",
			token: &PersonalAccessToken{
				Abilities: &[]PATAbility{},
			},
			abilities: []PATAbility{PATAbilityServerStart},
			want:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.token.HasAllAbilities(test.abilities...)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestGetUserAbilities(t *testing.T) {
	abilities := GetUserAbilities()

	assert.Len(t, abilities, 10, "should return 10 user abilities")
	assert.Contains(t, abilities, PATAbilityServerStart)
	assert.Contains(t, abilities, PATAbilityServerStop)
	assert.Contains(t, abilities, PATAbilityServerRestart)
	assert.Contains(t, abilities, PATAbilityServerUpdate)
	assert.Contains(t, abilities, PATAbilityServerConsole)
	assert.Contains(t, abilities, PATAbilityServerRconConsole)
	assert.Contains(t, abilities, PATAbilityServerRconPlayers)
	assert.Contains(t, abilities, PATAbilityServerTasksManage)
	assert.Contains(t, abilities, PATAbilityServerSettingsManage)

	assert.NotContains(t, abilities, PATAbilityServerCreate)
	assert.NotContains(t, abilities, PATAbilityGDaemonTaskRead)
}

func TestGetAdminAbilities(t *testing.T) {
	abilities := GetAdminAbilities()

	assert.Len(t, abilities, 2, "should return 2 admin abilities")
	assert.Contains(t, abilities, PATAbilityServerCreate)
	assert.Contains(t, abilities, PATAbilityGDaemonTaskRead)

	assert.NotContains(t, abilities, PATAbilityServerStart)
	assert.NotContains(t, abilities, PATAbilityServerStop)
}

func TestGetAllAbilities(t *testing.T) {
	abilities := GetAllAbilities()

	userAbilities := GetUserAbilities()
	adminAbilities := GetAdminAbilities()
	expectedCount := len(userAbilities) + len(adminAbilities)

	assert.Len(t, abilities, expectedCount, "should return all user and admin abilities")

	for _, ability := range userAbilities {
		assert.Contains(t, abilities, ability)
	}
	for _, ability := range adminAbilities {
		assert.Contains(t, abilities, ability)
	}
}

func TestGetAbilityDescriptions(t *testing.T) {
	descriptions := GetAbilityDescriptions()

	allAbilities := GetAllAbilities()
	assert.Len(t, descriptions, len(allAbilities), "should have description for each ability")

	for _, ability := range allAbilities {
		description, exists := descriptions[ability]
		assert.True(t, exists, "should have description for ability %s", ability)
		assert.NotEmpty(t, description, "description should not be empty for ability %s", ability)
	}

	assert.Equal(t, "Start game server", descriptions[PATAbilityServerStart])
	assert.Equal(t, "Stop game server", descriptions[PATAbilityServerStop])
	assert.Equal(t, "Create game server", descriptions[PATAbilityServerCreate])
	assert.Equal(t, "Read GameAP Daemon task", descriptions[PATAbilityGDaemonTaskRead])
}

func TestGetGroupedAbilities(t *testing.T) {
	t.Run("without_admin_abilities", func(t *testing.T) {
		grouped := GetGroupedAbilities(false)

		require.Contains(t, grouped, PATAbilityGroupServer)
		assert.NotContains(t, grouped, PATAbilityGroupGDaemonTask)

		serverAbilities := grouped[PATAbilityGroupServer]
		assert.Len(t, serverAbilities, 10, "should have 10 server abilities without admin")

		var hasServerCreate bool
		for _, ab := range serverAbilities {
			if ab.Ability == PATAbilityServerCreate {
				hasServerCreate = true

				break
			}
		}
		assert.False(t, hasServerCreate, "should not include admin server create ability")
	})

	t.Run("with_admin_abilities", func(t *testing.T) {
		grouped := GetGroupedAbilities(true)

		require.Contains(t, grouped, PATAbilityGroupServer)
		require.Contains(t, grouped, PATAbilityGroupGDaemonTask)

		serverAbilities := grouped[PATAbilityGroupServer]
		assert.Len(t, serverAbilities, 11, "should have 11 server abilities with admin")

		var hasServerCreate bool
		for _, ab := range serverAbilities {
			if ab.Ability == PATAbilityServerCreate {
				hasServerCreate = true
				assert.NotEmpty(t, ab.Description)

				break
			}
		}
		assert.True(t, hasServerCreate, "should include admin server create ability")

		gdaemonAbilities := grouped[PATAbilityGroupGDaemonTask]
		require.Len(t, gdaemonAbilities, 1)
		assert.Equal(t, PATAbilityGDaemonTaskRead, gdaemonAbilities[0].Ability)
		assert.NotEmpty(t, gdaemonAbilities[0].Description)
	})

	t.Run("all_abilities_have_descriptions", func(t *testing.T) {
		grouped := GetGroupedAbilities(true)

		for group, abilities := range grouped {
			for _, ab := range abilities {
				assert.NotEmpty(t, ab.Description, "ability %s in group %s should have description", ab.Ability, group)
			}
		}
	})
}

func TestValidateAbility(t *testing.T) {
	tests := []struct {
		name     string
		ability  string
		expected bool
	}{
		{
			name:     "valid_user_ability",
			ability:  string(PATAbilityServerStart),
			expected: true,
		},
		{
			name:     "valid_admin_ability",
			ability:  string(PATAbilityServerCreate),
			expected: true,
		},
		{
			name:     "another_valid_ability",
			ability:  string(PATAbilityServerConsole),
			expected: true,
		},
		{
			name:     "invalid_ability",
			ability:  "invalid:ability",
			expected: false,
		},
		{
			name:     "empty_string",
			ability:  "",
			expected: false,
		},
		{
			name:     "random_string",
			ability:  "random",
			expected: false,
		},
		{
			name:     "partial_match",
			ability:  "server:start:extra",
			expected: false,
		},
		{
			name:     "case_sensitive",
			ability:  "SERVER:START",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateAbility(test.ability)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestParseAbilities(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    []PATAbility
		expectEmpty bool
	}{
		{
			name: "all_valid_abilities",
			input: []string{
				string(PATAbilityServerStart),
				string(PATAbilityServerStop),
			},
			expected: []PATAbility{
				PATAbilityServerStart,
				PATAbilityServerStop,
			},
		},
		{
			name: "mixed_valid_and_invalid",
			input: []string{
				string(PATAbilityServerStart),
				"invalid:ability",
				string(PATAbilityServerStop),
			},
			expected: []PATAbility{
				PATAbilityServerStart,
				PATAbilityServerStop,
			},
		},
		{
			name:        "all_invalid_abilities",
			input:       []string{"invalid:one", "invalid:two"},
			expectEmpty: true,
		},
		{
			name:        "empty_input",
			input:       []string{},
			expectEmpty: true,
		},
		{
			name:        "empty_strings",
			input:       []string{"", "", ""},
			expectEmpty: true,
		},
		{
			name: "all_user_abilities",
			input: []string{
				string(PATAbilityServerStart),
				string(PATAbilityServerStop),
				string(PATAbilityServerRestart),
				string(PATAbilityServerUpdate),
				string(PATAbilityServerConsole),
				string(PATAbilityServerRconConsole),
				string(PATAbilityServerRconPlayers),
				string(PATAbilityServerTasksManage),
				string(PATAbilityServerSettingsManage),
			},
			expected: []PATAbility{
				PATAbilityServerStart,
				PATAbilityServerStop,
				PATAbilityServerRestart,
				PATAbilityServerUpdate,
				PATAbilityServerConsole,
				PATAbilityServerRconConsole,
				PATAbilityServerRconPlayers,
				PATAbilityServerTasksManage,
				PATAbilityServerSettingsManage,
			},
		},
		{
			name: "admin_abilities",
			input: []string{
				string(PATAbilityServerCreate),
				string(PATAbilityGDaemonTaskRead),
			},
			expected: []PATAbility{
				PATAbilityServerCreate,
				PATAbilityGDaemonTaskRead,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ParseAbilities(test.input)
			if test.expectEmpty {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestPersonalAccessToken_Fields(t *testing.T) {
	now := time.Now()
	tokenableType := EntityTypeUser
	abilities := []PATAbility{PATAbilityServerStart, PATAbilityServerStop}

	token := PersonalAccessToken{
		ID:            1,
		TokenableType: tokenableType,
		TokenableID:   100,
		Name:          "Test Token",
		Token:         "secret_token_value",
		Abilities:     &abilities,
		LastUsedAt:    &now,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	assert.Equal(t, uint(1), token.ID)
	assert.Equal(t, tokenableType, token.TokenableType)
	assert.Equal(t, uint(100), token.TokenableID)
	assert.Equal(t, "Test Token", token.Name)
	assert.Equal(t, "secret_token_value", token.Token)
	assert.Equal(t, &abilities, token.Abilities)
	assert.Equal(t, &now, token.LastUsedAt)
	assert.Equal(t, &now, token.CreatedAt)
	assert.Equal(t, &now, token.UpdatedAt)
}

func TestPasswordReset_Fields(t *testing.T) {
	now := time.Now()

	reset := PasswordReset{
		Email:     "user@example.com",
		Token:     "reset_token_value",
		CreatedAt: &now,
	}

	assert.Equal(t, "user@example.com", reset.Email)
	assert.Equal(t, "reset_token_value", reset.Token)
	assert.Equal(t, &now, reset.CreatedAt)
}

func TestPATAbilityConstants(t *testing.T) {
	assert.Equal(t, PATAbility("admin:server:create"), PATAbilityServerCreate)
	assert.Equal(t, PATAbility("admin:gdaemon-task:read"), PATAbilityGDaemonTaskRead)
	assert.Equal(t, PATAbility("server:start"), PATAbilityServerStart)
	assert.Equal(t, PATAbility("server:stop"), PATAbilityServerStop)
	assert.Equal(t, PATAbility("server:restart"), PATAbilityServerRestart)
	assert.Equal(t, PATAbility("server:update"), PATAbilityServerUpdate)
	assert.Equal(t, PATAbility("server:console"), PATAbilityServerConsole)
	assert.Equal(t, PATAbility("server:rcon-console"), PATAbilityServerRconConsole)
	assert.Equal(t, PATAbility("server:rcon-players"), PATAbilityServerRconPlayers)
	assert.Equal(t, PATAbility("server:tasks-manage"), PATAbilityServerTasksManage)
	assert.Equal(t, PATAbility("server:settings-manage"), PATAbilityServerSettingsManage)
}

func TestPATAbilityGroupConstants(t *testing.T) {
	assert.Equal(t, PATAbilityGroup("server"), PATAbilityGroupServer)
	assert.Equal(t, PATAbilityGroup("gdaemon-task"), PATAbilityGroupGDaemonTask)
}
