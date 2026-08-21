package pelicaneggimporter

import (
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/domain/gamesimport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformVariables_RulesMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		variable gamesimport.PelicanEggVariable
		want     domain.GameModVar
	}{
		{
			name: "legacy_piped_string_rules",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Server Name",
				EnvVariable: "SERVER_NAME",
				Rules:       gamesimport.FlexibleRules{"required|string|max:64"},
			},
			want: domain.GameModVar{
				Var:      "SERVER_NAME",
				Info:     "Server Name",
				AdminVar: true,
				Rules:    &domain.GameModVarRules{Required: new(true), MaxLength: new(64)},
			},
		},
		{
			name: "array_rules",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Max players",
				EnvVariable: "MAX_PLAYERS",
				Rules:       gamesimport.FlexibleRules{"required", "integer", "between:1,64"},
			},
			want: domain.GameModVar{
				Var:      "MAX_PLAYERS",
				Info:     "Max players",
				AdminVar: true,
				Type:     domain.GameModVarTypeInt,
				Rules:    &domain.GameModVarRules{Required: new(true), Min: new(1.0), Max: new(64.0)},
			},
		},
		{
			name: "numeric_becomes_float",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Rate",
				EnvVariable: "RATE",
				Rules:       gamesimport.FlexibleRules{"numeric|min:0.5|max:2"},
			},
			want: domain.GameModVar{
				Var:      "RATE",
				Info:     "Rate",
				AdminVar: true,
				Type:     domain.GameModVarTypeFloat,
				Rules:    &domain.GameModVarRules{Min: new(0.5), Max: new(2.0)},
			},
		},
		{
			name: "bounds_before_type_still_read_as_numeric",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Max players",
				EnvVariable: "MAX_PLAYERS",
				Rules:       gamesimport.FlexibleRules{"max:64|integer"},
			},
			want: domain.GameModVar{
				Var:      "MAX_PLAYERS",
				Info:     "Max players",
				AdminVar: true,
				Type:     domain.GameModVarTypeInt,
				Rules:    &domain.GameModVarRules{Max: new(64.0)},
			},
		},
		{
			name: "in_rule_becomes_select",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Mod",
				EnvVariable: "MOD",
				Rules:       gamesimport.FlexibleRules{"required|in:vanilla,paper,forge"},
			},
			want: domain.GameModVar{
				Var:      "MOD",
				Info:     "Mod",
				AdminVar: true,
				Type:     domain.GameModVarTypeSelect,
				Options: domain.GameModVarOptions{
					{Value: "vanilla"}, {Value: "paper"}, {Value: "forge"},
				},
				Rules: &domain.GameModVarRules{Required: new(true)},
			},
		},
		{
			name: "in_zero_one_becomes_bool",
			variable: gamesimport.PelicanEggVariable{
				Name:         "PvP",
				EnvVariable:  "PVP",
				DefaultValue: "1",
				Rules:        gamesimport.FlexibleRules{"in:0,1"},
			},
			want: domain.GameModVar{
				Var:        "PVP",
				Default:    "1",
				Info:       "PvP",
				AdminVar:   true,
				Type:       domain.GameModVarTypeBool,
				TrueValue:  new("1"),
				FalseValue: new("0"),
			},
		},
		{
			name: "boolean_rule",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Verbose",
				EnvVariable: "VERBOSE",
				Rules:       gamesimport.FlexibleRules{"boolean"},
			},
			want: domain.GameModVar{
				Var:      "VERBOSE",
				Info:     "Verbose",
				AdminVar: true,
				Type:     domain.GameModVarTypeBool,
			},
		},
		{
			name: "password_field_type_wins_over_string_rule",
			variable: gamesimport.PelicanEggVariable{
				Name:        "RCON password",
				EnvVariable: "RCON_PASSWORD",
				Rules:       gamesimport.FlexibleRules{"required|string|max:32"},
				FieldType:   "password",
			},
			want: domain.GameModVar{
				Var:      "RCON_PASSWORD",
				Info:     "RCON password",
				AdminVar: true,
				Type:     domain.GameModVarTypePassword,
				Rules:    &domain.GameModVarRules{Required: new(true), MaxLength: new(32)},
			},
		},
		{
			name: "re2_compatible_regex_is_kept",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Map",
				EnvVariable: "MAP",
				Rules:       gamesimport.FlexibleRules{`regex:/^de_[a-z0-9]+$/`},
			},
			want: domain.GameModVar{
				Var:      "MAP",
				Info:     "Map",
				AdminVar: true,
				Rules:    &domain.GameModVarRules{Pattern: `de_[a-z0-9]+`},
			},
		},
		{
			name: "regex_alternative_keeps_its_pipe",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Mod",
				EnvVariable: "MOD",
				Rules:       gamesimport.FlexibleRules{`regex:/^(vanilla|paper)$/`},
			},
			want: domain.GameModVar{
				Var:      "MOD",
				Info:     "Mod",
				AdminVar: true,
				Rules:    &domain.GameModVarRules{Pattern: `(vanilla|paper)`},
			},
		},
		{
			name: "regex_alternative_inside_a_piped_rule_string",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Mod",
				EnvVariable: "MOD",
				Rules:       gamesimport.FlexibleRules{`required|regex:/^(vanilla|paper)$/|max:32`},
			},
			want: domain.GameModVar{
				Var:      "MOD",
				Info:     "Mod",
				AdminVar: true,
				Rules: &domain.GameModVarRules{
					Required:  new(true),
					MaxLength: new(32),
					Pattern:   `(vanilla|paper)`,
				},
			},
		},
		{
			name: "description_survives_an_invalid_rule_mapping",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Mod",
				EnvVariable: "MOD",
				Description: "Which server jar to run",
				Rules:       gamesimport.FlexibleRules{"in:vanilla,vanilla"},
			},
			want: domain.GameModVar{
				Var:         "MOD",
				Info:        "Mod",
				AdminVar:    true,
				Description: "Which server jar to run",
			},
		},
		{
			name: "lookahead_regex_is_dropped",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Map",
				EnvVariable: "MAP",
				Rules:       gamesimport.FlexibleRules{`regex:/^(?=.*a).*$/`},
			},
			want: domain.GameModVar{
				Var:      "MAP",
				Info:     "Map",
				AdminVar: true,
			},
		},
		{
			name: "nullable_is_ignored",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Extra",
				EnvVariable: "EXTRA",
				Rules:       gamesimport.FlexibleRules{"nullable|string"},
			},
			want: domain.GameModVar{
				Var:      "EXTRA",
				Info:     "Extra",
				AdminVar: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			result := transformVariables([]gamesimport.PelicanEggVariable{test.variable})

			// ASSERT
			require.Len(t, result, 1)
			assert.Equal(t, test.want, result[0])
			require.NoError(t, result[0].Validate())
		})
	}
}

func TestTransformVariables_SkipsVariablesThatCannotBeStored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		variable gamesimport.PelicanEggVariable
	}{
		{
			name: "no_env_variable_name",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Nameless",
				EnvVariable: "",
			},
		},
		{
			name: "env_variable_name_longer_than_the_column",
			variable: gamesimport.PelicanEggVariable{
				Name:        "Too long",
				EnvVariable: strings.Repeat("A", maxVarNameLength+1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			result := transformVariables([]gamesimport.PelicanEggVariable{test.variable})

			// ASSERT
			assert.Empty(t, result)
		})
	}
}

func TestSplitRuleEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry string
		want  []string
	}{
		{
			name:  "plain_rules",
			entry: "required|string|max:64",
			want:  []string{"required", "string", "max:64"},
		},
		{
			name:  "regex_alternative_is_not_a_delimiter",
			entry: `required|regex:/^(vanilla|paper)$/|max:32`,
			want:  []string{"required", `regex:/^(vanilla|paper)$/`, "max:32"},
		},
		{
			name:  "not_regex_is_handled_too",
			entry: `not_regex:/(a|b)/|nullable`,
			want:  []string{`not_regex:/(a|b)/`, "nullable"},
		},
		{
			name:  "escaped_delimiter_stays_inside_the_pattern",
			entry: `regex:/^a\/b|c$/i|string`,
			want:  []string{`regex:/^a\/b|c$/i`, "string"},
		},
		{
			name:  "unterminated_pattern_swallows_the_rest",
			entry: `regex:/^(a|b)$`,
			want:  []string{`regex:/^(a|b)$`},
		},
		{
			name:  "empty_entry",
			entry: "",
			want:  []string{""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			result := splitRuleEntry(test.entry)

			// ASSERT
			assert.Equal(t, test.want, result)
		})
	}
}
