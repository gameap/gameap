package base

import (
	"net/http"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireValidationError asserts that a rejected input is reported as an
// unprocessable-entity api error, which is what makes the handler answer 422
// instead of 500.
func requireValidationError(t *testing.T, err error, wantMessage string) {
	t.Helper()

	require.Error(t, err)
	assert.Contains(t, err.Error(), wantMessage, "error message mismatch")

	var apiErr *api.Error
	require.ErrorAs(t, err, &apiErr, "input problems must surface as an api validation error")
	assert.Equal(t, http.StatusUnprocessableEntity, apiErr.HTTPStatus(), "validation errors must answer 422")
}

func TestVarInput_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     VarInput
		wantError string
	}{
		{
			name:  "accepts_string_var",
			input: VarInput{Var: "maxplayers", Info: "Max players", Default: "32"},
		},
		{
			name: "accepts_untyped_var_with_uppercase_name",
			input: VarInput{
				Var:  "SERVER_PASSWORD",
				Info: "Server password",
			},
		},
		{
			name: "accepts_select_var_with_options",
			input: VarInput{
				Var:     "difficulty",
				Info:    "Difficulty",
				Type:    domain.GameModVarTypeSelect,
				Default: "easy",
				Options: domain.GameModVarOptions{
					{Value: "easy"},
					{Value: "hard", Label: "Hard mode"},
				},
			},
		},
		{
			name: "accepts_bool_var_with_default_naming_a_declared_state",
			input: VarInput{
				Var:        "pvp",
				Info:       "PvP enabled",
				Type:       domain.GameModVarTypeBool,
				Default:    "on",
				TrueValue:  new("on"),
				FalseValue: new("off"),
			},
		},
		{
			name: "accepts_translation_for_a_non_base_locale",
			input: VarInput{
				Var:  "maxplayers",
				Info: "Max players",
				I18n: domain.GameModVarI18n{"ru": {Info: "Максимум игроков"}},
			},
		},
		{
			name:      "rejects_empty_variable_name",
			input:     VarInput{Var: "", Info: "Max players"},
			wantError: "variable name is required",
		},
		{
			name:      "rejects_variable_name_with_a_leading_digit",
			input:     VarInput{Var: "1maxplayers", Info: "Max players"},
			wantError: "variable name must match pattern",
		},
		{
			name:      "rejects_var_without_info",
			input:     VarInput{Var: "maxplayers"},
			wantError: "variable info is required",
		},
		{
			name:      "rejects_unknown_type",
			input:     VarInput{Var: "maxplayers", Info: "Max players", Type: domain.GameModVarType("colour")},
			wantError: "unknown variable type: colour",
		},
		{
			// The submitted shape is judged, not the one Normalize would leave
			// behind: options on an int variable are a client mistake, not a
			// field to drop quietly.
			name: "rejects_options_on_a_non_select_type",
			input: VarInput{
				Var:     "maxplayers",
				Info:    "Max players",
				Type:    domain.GameModVarTypeInt,
				Options: domain.GameModVarOptions{{Value: "16"}},
			},
			wantError: "options require type select",
		},
		{
			name: "rejects_allow_custom_on_a_non_select_type",
			input: VarInput{
				Var:         "maxplayers",
				Info:        "Max players",
				Type:        domain.GameModVarTypeString,
				AllowCustom: flexible.Bool(true),
			},
			wantError: "allow_custom requires type select",
		},
		{
			name: "rejects_select_without_options",
			input: VarInput{
				Var:  "difficulty",
				Info: "Difficulty",
				Type: domain.GameModVarTypeSelect,
			},
			wantError: "type select requires a non-empty options list",
		},
		{
			name: "rejects_duplicate_option_values",
			input: VarInput{
				Var:  "difficulty",
				Info: "Difficulty",
				Type: domain.GameModVarTypeSelect,
				Options: domain.GameModVarOptions{
					{Value: "easy"},
					{Value: "easy", Label: "Easy again"},
				},
			},
			wantError: "options[1]: duplicate value: easy",
		},
		{
			name: "rejects_true_value_on_a_non_bool_type",
			input: VarInput{
				Var:       "maxplayers",
				Info:      "Max players",
				Type:      domain.GameModVarTypeString,
				TrueValue: new("yes"),
			},
			wantError: "true_value requires type bool",
		},
		{
			name: "rejects_equal_true_and_false_values",
			input: VarInput{
				Var:        "pvp",
				Info:       "PvP enabled",
				Type:       domain.GameModVarTypeBool,
				TrueValue:  new("on"),
				FalseValue: new("on"),
			},
			wantError: "true_value and false_value must differ",
		},
		{
			name: "rejects_bool_default_outside_the_declared_states",
			input: VarInput{
				Var:     "pvp",
				Info:    "PvP enabled",
				Type:    domain.GameModVarTypeBool,
				Default: "maybe",
			},
			wantError: "default must equal true_value or false_value",
		},
		{
			name: "rejects_rules_min_greater_than_max",
			input: VarInput{
				Var:   "maxplayers",
				Info:  "Max players",
				Type:  domain.GameModVarTypeInt,
				Rules: &domain.GameModVarRules{Min: new(64.0), Max: new(16.0)},
			},
			wantError: "rules min must not be greater than max",
		},
		{
			name: "rejects_the_base_locale_in_i18n",
			input: VarInput{
				Var:  "maxplayers",
				Info: "Max players",
				I18n: domain.GameModVarI18n{"en": {Info: "Max players"}},
			},
			wantError: "i18n must not contain the en locale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			input := tt.input

			// ACT
			err := input.Validate()

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)

				return
			}

			requireValidationError(t, err, tt.wantError)
		})
	}
}

func TestValidateVarInputs(t *testing.T) {
	t.Parallel()

	validVar := VarInput{Var: "maxplayers", Info: "Max players"}

	tests := []struct {
		name      string
		inputs    []VarInput
		wantError string
	}{
		{
			name:   "accepts_a_nil_list",
			inputs: nil,
		},
		{
			name:   "accepts_an_empty_list",
			inputs: []VarInput{},
		},
		{
			name: "accepts_distinct_valid_vars",
			inputs: []VarInput{
				validVar,
				{Var: "hostname", Info: "Hostname"},
			},
		},
		{
			name: "reports_the_index_of_the_invalid_element",
			inputs: []VarInput{
				validVar,
				{
					Var:     "difficulty",
					Info:    "Difficulty",
					Type:    domain.GameModVarTypeInt,
					Options: domain.GameModVarOptions{{Value: "easy"}},
				},
			},
			wantError: "game mod input Vars[1]: options require type select",
		},
		{
			name: "rejects_duplicate_variable_names",
			inputs: []VarInput{
				validVar,
				{Var: "maxplayers", Info: "Max players, again"},
			},
			wantError: "duplicate variable name: maxplayers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			inputs := tt.inputs

			// ACT
			err := ValidateVarInputs(inputs)

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)

				return
			}

			requireValidationError(t, err, tt.wantError)
		})
	}
}

func TestFastRconInput_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     FastRconInput
		wantError string
	}{
		{
			name:  "accepts_info_with_command",
			input: FastRconInput{Info: "Restart map", Command: "restart"},
		},
		{
			name: "accepts_translation_for_a_non_base_locale",
			input: FastRconInput{
				Info:    "Restart map",
				Command: "restart",
				I18n:    domain.GameModFastRconI18n{"ru": {Info: "Перезапустить карту"}},
			},
		},
		{
			name:      "rejects_empty_info",
			input:     FastRconInput{Command: "restart"},
			wantError: "info is required",
		},
		{
			name:      "rejects_empty_command",
			input:     FastRconInput{Info: "Restart map"},
			wantError: "command is required",
		},
		{
			name: "rejects_a_malformed_locale",
			input: FastRconInput{
				Info:    "Restart map",
				Command: "restart",
				I18n:    domain.GameModFastRconI18n{"r": {Info: "Перезапустить карту"}},
			},
			wantError: "i18n[r]: invalid locale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			input := tt.input

			// ACT
			err := input.Validate()

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)

				return
			}

			requireValidationError(t, err, tt.wantError)
		})
	}
}

func TestValidateFastRconInputs(t *testing.T) {
	t.Parallel()

	validFastRcon := FastRconInput{Info: "Restart map", Command: "restart"}

	tests := []struct {
		name      string
		inputs    []FastRconInput
		wantError string
	}{
		{
			name:   "accepts_a_nil_list",
			inputs: nil,
		},
		{
			name:   "accepts_an_empty_list",
			inputs: []FastRconInput{},
		},
		{
			name:   "accepts_a_valid_list",
			inputs: []FastRconInput{validFastRcon, {Info: "Kick", Command: "kick {player}"}},
		},
		{
			name:      "reports_the_first_index_when_it_is_invalid",
			inputs:    []FastRconInput{{Command: "restart"}, validFastRcon},
			wantError: "game mod input FastRcon[0]: info is required",
		},
		{
			name:      "reports_the_index_of_a_later_invalid_element",
			inputs:    []FastRconInput{validFastRcon, {Info: "Kick"}},
			wantError: "game mod input FastRcon[1]: command is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			inputs := tt.inputs

			// ACT
			err := ValidateFastRconInputs(inputs)

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)

				return
			}

			requireValidationError(t, err, tt.wantError)
		})
	}
}

func TestVarInput_ToDomain(t *testing.T) {
	t.Parallel()

	t.Run("maps_every_submitted_field", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:         "difficulty",
			Default:     "easy",
			Info:        "Difficulty",
			AdminVar:    flexible.Bool(true),
			Type:        domain.GameModVarTypeSelect,
			Description: "How hard the bots hit",
			Options: domain.GameModVarOptions{
				{Value: "easy", Label: "Easy", I18n: domain.GameModVarOptionI18n{"ru": {Label: "Легко"}}},
			},
			AllowCustom: flexible.Bool(true),
			Rules:       &domain.GameModVarRules{Required: new(true), MaxLength: new(16)},
			I18n:        domain.GameModVarI18n{"ru": {Info: "Сложность", Description: "Описание"}},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		assert.Equal(t, "difficulty", got.Var)
		assert.Equal(t, domain.GameModVarDefault("easy"), got.Default)
		assert.Equal(t, "Difficulty", got.Info)
		assert.True(t, got.AdminVar, "admin_var must be carried over from the flexible bool")
		assert.Equal(t, domain.GameModVarTypeSelect, got.Type)
		assert.Equal(t, "How hard the bots hit", got.Description)
		assert.True(t, got.AllowCustom, "allow_custom must survive on a select var")
		require.Len(t, got.Options, 1)
		assert.Equal(t, "easy", got.Options[0].Value)
		assert.Equal(t, "Easy", got.Options[0].Label)
		assert.Equal(t, domain.GameModVarOptionI18n{"ru": {Label: "Легко"}}, got.Options[0].I18n)
		require.NotNil(t, got.Rules)
		assert.Equal(t, new(true), got.Rules.Required)
		assert.Equal(t, new(16), got.Rules.MaxLength, "max_length applies to a select with allow_custom")
		assert.Equal(t, domain.GameModVarI18n{"ru": {Info: "Сложность", Description: "Описание"}}, got.I18n)
	})

	t.Run("drops_fields_that_do_not_apply_to_the_type", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:         "maxplayers",
			Info:        "Max players",
			Type:        domain.GameModVarTypeInt,
			Options:     domain.GameModVarOptions{{Value: "16"}},
			AllowCustom: flexible.Bool(true),
			TrueValue:   new("yes"),
			FalseValue:  new("no"),
			Rules:       &domain.GameModVarRules{MaxLength: new(16), Pattern: `\d+`},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		assert.Nil(t, got.Options, "options do not apply to an int var")
		assert.False(t, got.AllowCustom, "allow_custom does not apply to an int var")
		assert.Nil(t, got.TrueValue, "true_value does not apply to an int var")
		assert.Nil(t, got.FalseValue, "false_value does not apply to an int var")
		assert.Nil(t, got.Rules, "text rules do not apply to an int var, and an emptied rule set is dropped")
	})

	t.Run("drops_numeric_rules_but_keeps_text_rules_on_a_string_var", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:   "hostname",
			Info:  "Hostname",
			Type:  domain.GameModVarTypeString,
			Rules: &domain.GameModVarRules{Min: new(1.0), Max: new(10.0), MaxLength: new(64)},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		require.NotNil(t, got.Rules)
		assert.Nil(t, got.Rules.Min, "min does not apply to a string var")
		assert.Nil(t, got.Rules.Max, "max does not apply to a string var")
		assert.Equal(t, new(64), got.Rules.MaxLength, "max_length applies to a string var")
	})

	t.Run("drops_a_required_false_rule_set", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:   "hostname",
			Info:  "Hostname",
			Rules: &domain.GameModVarRules{Required: new(false)},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		assert.Nil(t, got.Rules, "required=false carries no information and leaves the rule set empty")
	})

	t.Run("shortens_an_option_label_equal_to_its_value", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:  "difficulty",
			Info: "Difficulty",
			Type: domain.GameModVarTypeSelect,
			Options: domain.GameModVarOptions{
				{Value: "easy", Label: "easy"},
				{Value: "hard", Label: "Hard mode"},
			},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		require.Len(t, got.Options, 2)
		assert.Empty(t, got.Options[0].Label, "a label repeating the value is stored in its shorthand form")
		assert.Equal(t, "Hard mode", got.Options[1].Label, "a distinct label is kept")
	})

	t.Run("does_not_mutate_the_submitted_options_and_rules", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:  "difficulty",
			Info: "Difficulty",
			Type: domain.GameModVarTypeSelect,
			Options: domain.GameModVarOptions{
				{Value: "easy", Label: "easy"},
			},
			Rules: &domain.GameModVarRules{Required: new(false), Min: new(1.0)},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		require.Len(t, got.Options, 1)
		assert.Empty(t, got.Options[0].Label, "the returned copy is normalized")
		require.Len(t, input.Options, 1)
		assert.Equal(t, "easy", input.Options[0].Label, "normalizing the copy must not rewrite the caller's options")
		require.NotNil(t, input.Rules)
		assert.Equal(t, new(false), input.Rules.Required, "normalizing the copy must not rewrite the caller's rules")
		assert.Equal(t, new(1.0), input.Rules.Min, "normalizing the copy must not rewrite the caller's rules")
	})

	t.Run("normalizes_i18n_locale_keys_and_drops_empty_translations", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := VarInput{
			Var:  "maxplayers",
			Info: "Max players",
			I18n: domain.GameModVarI18n{
				"RU":    {Info: "Максимум игроков"},
				"pt_BR": {Info: "Máximo de jogadores"},
				"de":    {},
			},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		assert.Equal(t, domain.GameModVarI18n{
			"ru":    {Info: "Максимум игроков"},
			"pt-br": {Info: "Máximo de jogadores"},
		}, got.I18n, "locale keys are lowercased, underscores become dashes and empty translations are dropped")
	})
}

func TestFastRconInput_ToDomain(t *testing.T) {
	t.Parallel()

	t.Run("maps_every_submitted_field", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := FastRconInput{
			Info:    "Restart map",
			Command: "restart",
			I18n:    domain.GameModFastRconI18n{"ru": {Info: "Перезапустить карту"}},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		assert.Equal(t, "Restart map", got.Info)
		assert.Equal(t, "restart", got.Command)
		assert.Equal(t, domain.GameModFastRconI18n{"ru": {Info: "Перезапустить карту"}}, got.I18n)
	})

	t.Run("normalizes_i18n_locale_keys", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := FastRconInput{
			Info:    "Restart map",
			Command: "restart",
			I18n:    domain.GameModFastRconI18n{"RU": {Info: "Перезапустить карту"}},
		}

		// ACT
		got := input.ToDomain()

		// ASSERT
		assert.Equal(t, domain.GameModFastRconI18n{"ru": {Info: "Перезапустить карту"}}, got.I18n)
	})
}

func TestVarInputsToDomain(t *testing.T) {
	t.Parallel()

	t.Run("returns_an_empty_non_nil_list_for_no_input", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		var inputs []VarInput

		// ACT
		got := VarInputsToDomain(inputs)

		// ASSERT
		assert.NotNil(t, got, "an empty list must marshal as [] rather than null")
		assert.Empty(t, got)
	})

	t.Run("maps_every_element_in_order_and_normalizes_each", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		inputs := []VarInput{
			{Var: "maxplayers", Info: "Max players", Default: "32", AdminVar: flexible.Bool(true)},
			{
				Var:       "hostname",
				Info:      "Hostname",
				Type:      domain.GameModVarTypeString,
				TrueValue: new("yes"),
			},
		}

		// ACT
		got := VarInputsToDomain(inputs)

		// ASSERT
		require.Len(t, got, 2)
		assert.Equal(t, "maxplayers", got[0].Var)
		assert.Equal(t, domain.GameModVarDefault("32"), got[0].Default)
		assert.True(t, got[0].AdminVar)
		assert.Equal(t, "hostname", got[1].Var, "the order of the submitted list is preserved")
		assert.Nil(t, got[1].TrueValue, "each element is normalized on the way in")
	})
}

func TestFastRconInputsToDomain(t *testing.T) {
	t.Parallel()

	t.Run("returns_an_empty_non_nil_list_for_no_input", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		var inputs []FastRconInput

		// ACT
		got := FastRconInputsToDomain(inputs)

		// ASSERT
		assert.NotNil(t, got, "an empty list must marshal as [] rather than null")
		assert.Empty(t, got)
	})

	t.Run("maps_every_element_in_order_and_normalizes_each", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		inputs := []FastRconInput{
			{Info: "Restart map", Command: "restart"},
			{Info: "Kick", Command: "kick {player}", I18n: domain.GameModFastRconI18n{"RU": {Info: "Кик"}}},
		}

		// ACT
		got := FastRconInputsToDomain(inputs)

		// ASSERT
		require.Len(t, got, 2)
		assert.Equal(t, "Restart map", got[0].Info)
		assert.Equal(t, "restart", got[0].Command)
		assert.Equal(t, "Kick", got[1].Info, "the order of the submitted list is preserved")
		assert.Equal(t, domain.GameModFastRconI18n{"ru": {Info: "Кик"}}, got[1].I18n,
			"each element is normalized on the way in")
	})
}
