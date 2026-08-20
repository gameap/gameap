package base

import (
	"encoding/json"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testGameMod() *domain.GameMod {
	return &domain.GameMod{
		GameCode: "minecraft",
		Name:     "Default",
		Vars: domain.GameModVarList{
			{Var: "hostname", Default: "Server", Info: "Hostname"},
			{
				Var: "maxplayers", Default: "20", Info: "Max players", Type: domain.GameModVarTypeInt,
				Rules: &domain.GameModVarRules{Min: new(1.0), Max: new(64.0)},
			},
			{
				Var: "pvp", Default: "on", Info: "PvP", Type: domain.GameModVarTypeBool,
				TrueValue: new("on"), FalseValue: new("off"),
			},
			{
				Var: "mod", Default: "vanilla", Info: "Mod", Type: domain.GameModVarTypeSelect,
				AdminVar: true,
				Options:  domain.GameModVarOptions{{Value: "vanilla"}, {Value: "paper"}},
			},
		},
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     []InputSetting
		isAdmin   bool
		want      map[string]string
		wantError string
	}{
		{
			name: "typed_values_are_canonicalized",
			input: []InputSetting{
				{Name: "hostname", Value: "My Server"},
				{Name: "maxplayers", Value: json.Number("32")},
				{Name: "pvp", Value: false},
			},
			want: map[string]string{"hostname": "My Server", "maxplayers": "32", "pvp": "off"},
		},
		{
			name:  "built_in_bool_accepts_a_legacy_string",
			input: []InputSetting{{Name: "autostart", Value: "true"}},
			want:  map[string]string{"autostart": "true"},
		},
		{
			name:  "built_in_bool_accepts_a_real_boolean",
			input: []InputSetting{{Name: "update_before_start", Value: true}},
			want:  map[string]string{"update_before_start": "true"},
		},
		{
			name:  "autostart_current_is_not_writable",
			input: []InputSetting{{Name: "autostart_current", Value: true}},
			want:  map[string]string{},
		},
		{
			name:  "unknown_names_are_ignored",
			input: []InputSetting{{Name: "not_a_var", Value: "x"}},
			want:  map[string]string{},
		},
		{
			name:    "admin_var_is_ignored_for_regular_users",
			input:   []InputSetting{{Name: "mod", Value: "paper"}, {Name: "hostname", Value: "x"}},
			isAdmin: false,
			want:    map[string]string{"hostname": "x"},
		},
		{
			name:    "admin_var_is_accepted_for_admins",
			input:   []InputSetting{{Name: "mod", Value: "paper"}},
			isAdmin: true,
			want:    map[string]string{"mod": "paper"},
		},
		{
			name:      "rule_violation_is_reported",
			input:     []InputSetting{{Name: "maxplayers", Value: json.Number("100")}},
			wantError: "maxplayers: value must be at most 64",
		},
		{
			name: "every_violation_is_reported_at_once",
			input: []InputSetting{
				{Name: "maxplayers", Value: json.Number("100")},
				{Name: "pvp", Value: "maybe"},
			},
			wantError: "maxplayers: value must be at most 64; pvp: value must be a boolean",
		},
		{
			name:      "select_rejects_an_unknown_option",
			input:     []InputSetting{{Name: "mod", Value: "forge"}},
			isAdmin:   true,
			wantError: "mod: value must be one of the allowed options",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			result, err := Normalize(testGameMod(), test.input, test.isAdmin)

			// ASSERT
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)
				assert.Nil(t, result, "nothing may be written when a value is rejected")

				return
			}

			require.NoError(t, err)

			stored := make(map[string]string, len(result))
			for _, setting := range result {
				raw, present := setting.Value.Raw()
				require.True(t, present, "setting %s has no value", setting.Name)
				stored[setting.Name] = raw
			}

			assert.Equal(t, test.want, stored)
		})
	}
}

func TestNormalize_PreservesInputOrder(t *testing.T) {
	t.Parallel()

	// ARRANGE
	input := []InputSetting{
		{Name: "pvp", Value: true},
		{Name: "hostname", Value: "x"},
		{Name: "maxplayers", Value: json.Number("10")},
	}

	// ACT
	result, err := Normalize(testGameMod(), input, false)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "pvp", result[0].Name)
	assert.Equal(t, "hostname", result[1].Name)
	assert.Equal(t, "maxplayers", result[2].Name)
}

func TestNormalizeVars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     map[string]string
		want      map[string]string
		wantError string
	}{
		{
			name:  "nil_stays_nil",
			input: nil,
			want:  nil,
		},
		{
			name:  "unknown_keys_pass_through",
			input: map[string]string{"custom_flag": "-v"},
			want:  map[string]string{"custom_flag": "-v"},
		},
		{
			name:  "known_keys_are_canonicalized",
			input: map[string]string{"pvp": "true", "maxplayers": "32"},
			want:  map[string]string{"pvp": "on", "maxplayers": "32"},
		},
		{
			name:      "known_keys_are_validated",
			input:     map[string]string{"maxplayers": "100"},
			wantError: "maxplayers: value must be at most 64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			result, err := NormalizeVars(testGameMod(), test.input)

			// ASSERT
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, result)
		})
	}
}
