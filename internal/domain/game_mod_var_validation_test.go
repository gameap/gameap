package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameModVar_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     GameModVar
		wantError string
	}{
		{
			name:  "minimal_untyped_var",
			input: GameModVar{Var: "maxplayers", Info: "Max players"},
		},
		{
			name:  "uppercase_name_from_pelican_egg",
			input: GameModVar{Var: "SERVER_JARFILE", Info: "Server jar"},
		},
		{
			name:      "empty_name",
			input:     GameModVar{Info: "Max players"},
			wantError: "variable name is required",
		},
		{
			name:      "name_too_long",
			input:     GameModVar{Var: strings.Repeat("a", 33), Info: "Max players"},
			wantError: "variable name must be at most 32 characters",
		},
		{
			name:      "name_with_dash",
			input:     GameModVar{Var: "max-players", Info: "Max players"},
			wantError: "variable name must match pattern",
		},
		{
			name:      "name_starting_with_digit",
			input:     GameModVar{Var: "1player", Info: "Max players"},
			wantError: "variable name must match pattern",
		},
		{
			name:      "empty_info",
			input:     GameModVar{Var: "maxplayers"},
			wantError: "variable info is required",
		},
		{
			name:      "info_too_long",
			input:     GameModVar{Var: "maxplayers", Info: strings.Repeat("a", 129)},
			wantError: "variable info must be at most 128 characters",
		},
		{
			name:      "description_too_long",
			input:     GameModVar{Var: "maxplayers", Info: "Max", Description: strings.Repeat("a", 1001)},
			wantError: "variable description must be at most 1000 characters",
		},
		{
			name:      "default_too_long",
			input:     GameModVar{Var: "maxplayers", Info: "Max", Default: GameModVarDefault(strings.Repeat("a", 65))},
			wantError: "variable default must be at most 64 characters",
		},
		{
			name:      "unknown_type",
			input:     GameModVar{Var: "maxplayers", Info: "Max", Type: "date"},
			wantError: "unknown variable type: date",
		},
		{
			name: "select_with_options",
			input: GameModVar{
				Var:     "mod",
				Info:    "Mod",
				Type:    GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "vanilla"}, {Value: "paper", Label: "Paper"}},
			},
		},
		{
			name:      "select_without_options",
			input:     GameModVar{Var: "mod", Info: "Mod", Type: GameModVarTypeSelect},
			wantError: "type select requires a non-empty options list",
		},
		{
			name: "options_on_non_select",
			input: GameModVar{
				Var: "mod", Info: "Mod", Options: GameModVarOptions{{Value: "vanilla"}},
			},
			wantError: "options require type select",
		},
		{
			name:      "allow_custom_on_non_select",
			input:     GameModVar{Var: "mod", Info: "Mod", AllowCustom: true},
			wantError: "allow_custom requires type select",
		},
		{
			name: "duplicate_option_values",
			input: GameModVar{
				Var: "mod", Info: "Mod", Type: GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "paper"}, {Value: "paper", Label: "Paper"}},
			},
			wantError: "options[1]: duplicate value: paper",
		},
		{
			name: "empty_option_value",
			input: GameModVar{
				Var: "mod", Info: "Mod", Type: GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: ""}},
			},
			wantError: "options[0]: value is required",
		},
		{
			name: "bool_with_custom_values",
			input: GameModVar{
				Var: "pvp", Info: "PvP", Type: GameModVarTypeBool, Default: "on",
				TrueValue: new("on"), FalseValue: new("off"),
			},
		},
		{
			name: "bool_with_empty_false_value",
			input: GameModVar{
				Var: "verbose", Info: "Verbose", Type: GameModVarTypeBool, Default: "",
				TrueValue: new("-v"), FalseValue: new(""),
			},
		},
		{
			name:      "true_value_on_non_bool",
			input:     GameModVar{Var: "pvp", Info: "PvP", TrueValue: new("1")},
			wantError: "true_value requires type bool",
		},
		{
			name:      "false_value_on_non_bool",
			input:     GameModVar{Var: "pvp", Info: "PvP", FalseValue: new("0")},
			wantError: "false_value requires type bool",
		},
		{
			name: "identical_bool_values",
			input: GameModVar{
				Var: "pvp", Info: "PvP", Type: GameModVarTypeBool,
				TrueValue: new("x"), FalseValue: new("x"),
			},
			wantError: "true_value and false_value must differ",
		},
		{
			name: "bool_default_outside_values",
			input: GameModVar{
				Var: "pvp", Info: "PvP", Type: GameModVarTypeBool, Default: "maybe",
			},
			wantError: "default must equal true_value or false_value",
		},
		{
			name: "numeric_rules_on_int",
			input: GameModVar{
				Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt,
				Rules: &GameModVarRules{Min: new(1.0), Max: new(64.0)},
			},
		},
		{
			name: "numeric_rules_on_string",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				Rules: &GameModVarRules{Min: new(1.0)},
			},
			wantError: "rules min and max require type int or float",
		},
		{
			name: "min_greater_than_max",
			input: GameModVar{
				Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt,
				Rules: &GameModVarRules{Min: new(10.0), Max: new(5.0)},
			},
			wantError: "rules min must not be greater than max",
		},
		{
			name: "length_rules_on_int",
			input: GameModVar{
				Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt,
				Rules: &GameModVarRules{MaxLength: new(4)},
			},
			wantError: "rules min_length, max_length and pattern require a textual type",
		},
		{
			name: "length_rules_on_select_without_allow_custom",
			input: GameModVar{
				Var: "mod", Info: "Mod", Type: GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "paper"}},
				Rules:   &GameModVarRules{MaxLength: new(4)},
			},
			wantError: "rules min_length, max_length and pattern require a textual type",
		},
		{
			name: "length_rules_on_select_with_allow_custom",
			input: GameModVar{
				Var: "version", Info: "Version", Type: GameModVarTypeSelect, AllowCustom: true,
				Options: GameModVarOptions{{Value: "1.21"}},
				Rules:   &GameModVarRules{MaxLength: new(16)},
			},
		},
		{
			name: "min_length_greater_than_max_length",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				Rules: &GameModVarRules{MinLength: new(10), MaxLength: new(5)},
			},
			wantError: "rules min_length must not be greater than max_length",
		},
		{
			name: "empty_rules_object",
			input: GameModVar{
				Var: "hostname", Info: "Hostname", Rules: &GameModVarRules{},
			},
			wantError: "rules must contain at least one rule",
		},
		{
			name: "pattern_with_lookahead_is_rejected",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				Rules: &GameModVarRules{Pattern: `(?=.*a).*`},
			},
			wantError: "invalid regular expression",
		},
		{
			name: "valid_pattern",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				Rules: &GameModVarRules{Pattern: `[a-z0-9 ]+`},
			},
		},
		{
			name: "translations",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				I18n: GameModVarI18n{"ru": {Info: "Имя сервера"}, "pt-br": {Description: "Nome"}},
			},
		},
		{
			name: "en_translation_is_rejected",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				I18n: GameModVarI18n{"en": {Info: "Hostname"}},
			},
			wantError: "i18n must not contain the en locale",
		},
		{
			name: "invalid_locale",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				I18n: GameModVarI18n{"RU": {Info: "Имя"}},
			},
			wantError: "invalid locale",
		},
		{
			name: "empty_translation_entry",
			input: GameModVar{
				Var: "hostname", Info: "Hostname",
				I18n: GameModVarI18n{"ru": {}},
			},
			wantError: "at least one of info or description is required",
		},
		{
			name: "option_translation_without_label",
			input: GameModVar{
				Var: "mod", Info: "Mod", Type: GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "paper", I18n: GameModVarOptionI18n{"ru": {}}}},
			},
			wantError: "options[0]: i18n[ru]: label is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			variable := test.input

			// ACT
			err := variable.Validate()

			// ASSERT
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGameModVarList_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     GameModVarList
		wantError string
	}{
		{
			name: "distinct_names",
			input: GameModVarList{
				{Var: "maxplayers", Info: "Max"},
				{Var: "hostname", Info: "Hostname"},
			},
		},
		{
			name: "duplicate_names",
			input: GameModVarList{
				{Var: "maxplayers", Info: "Max"},
				{Var: "maxplayers", Info: "Max again"},
			},
			wantError: "vars[1]: duplicate variable name: maxplayers",
		},
		{
			name: "invalid_var_reports_index",
			input: GameModVarList{
				{Var: "maxplayers", Info: "Max"},
				{Var: "bad name", Info: "Bad"},
			},
			wantError: "vars[1]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			err := test.input.Validate()

			// ASSERT
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGameModFastRcon_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     GameModFastRcon
		wantError string
	}{
		{
			name:  "valid",
			input: GameModFastRcon{Info: "Status", Command: "status"},
		},
		{
			name: "valid_with_translations",
			input: GameModFastRcon{
				Info: "Status", Command: "status",
				I18n: GameModFastRconI18n{"ru": {Info: "Статус"}},
			},
		},
		{
			name:      "empty_info",
			input:     GameModFastRcon{Command: "status"},
			wantError: "info is required",
		},
		{
			name:      "empty_command",
			input:     GameModFastRcon{Info: "Status"},
			wantError: "command is required",
		},
		{
			name: "en_translation_is_rejected",
			input: GameModFastRcon{
				Info: "Status", Command: "status",
				I18n: GameModFastRconI18n{"en": {Info: "Status"}},
			},
			wantError: "i18n must not contain the en locale",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			command := test.input

			// ACT
			err := command.Validate()

			// ASSERT
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGameModVar_NormalizeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		variable  GameModVar
		value     any
		want      string
		wantError string
	}{
		{
			name:     "string_passthrough",
			variable: GameModVar{Var: "hostname", Info: "Hostname"},
			value:    "My Server",
			want:     "My Server",
		},
		{
			name:     "string_keeps_leading_zeros",
			variable: GameModVar{Var: "hostname", Info: "Hostname"},
			value:    "007",
			want:     "007",
		},
		{
			name:     "number_becomes_string",
			variable: GameModVar{Var: "hostname", Info: "Hostname"},
			value:    json.Number("32"),
			want:     "32",
		},
		{
			name:     "empty_value_allowed_without_required",
			variable: GameModVar{Var: "hostname", Info: "Hostname"},
			value:    "",
			want:     "",
		},
		{
			name: "empty_value_rejected_when_required",
			variable: GameModVar{
				Var: "hostname", Info: "Hostname", Rules: &GameModVarRules{Required: new(true)},
			},
			value:     "",
			wantError: "hostname: value is required",
		},
		{
			name: "nil_value_rejected_when_required",
			variable: GameModVar{
				Var: "hostname", Info: "Hostname", Rules: &GameModVarRules{Required: new(true)},
			},
			value:     nil,
			wantError: "hostname: value is required",
		},
		{
			name:      "unsupported_value_type",
			variable:  GameModVar{Var: "hostname", Info: "Hostname"},
			value:     []string{"a"},
			wantError: "value must be a string, a number or a boolean",
		},
		{
			name:     "int_from_number",
			variable: GameModVar{Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt},
			value:    json.Number("32"),
			want:     "32",
		},
		{
			name:     "int_from_string",
			variable: GameModVar{Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt},
			value:    "32",
			want:     "32",
		},
		{
			name:      "int_rejects_fraction",
			variable:  GameModVar{Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt},
			value:     json.Number("32.5"),
			wantError: "maxplayers: value must be an integer",
		},
		{
			name: "int_below_min",
			variable: GameModVar{
				Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt,
				Rules: &GameModVarRules{Min: new(1.0), Max: new(64.0)},
			},
			value:     json.Number("0"),
			wantError: "maxplayers: value must be at least 1",
		},
		{
			name: "int_above_max",
			variable: GameModVar{
				Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt,
				Rules: &GameModVarRules{Min: new(1.0), Max: new(64.0)},
			},
			value:     json.Number("65"),
			wantError: "maxplayers: value must be at most 64",
		},
		{
			name:     "float_keeps_decimal_notation",
			variable: GameModVar{Var: "rate", Info: "Rate", Type: GameModVarTypeFloat},
			value:    0.5,
			want:     "0.5",
		},
		{
			name:     "float_accepts_integer",
			variable: GameModVar{Var: "rate", Info: "Rate", Type: GameModVarTypeFloat},
			value:    json.Number("30"),
			want:     "30",
		},
		{
			name:      "float_rejects_text",
			variable:  GameModVar{Var: "rate", Info: "Rate", Type: GameModVarTypeFloat},
			value:     "fast",
			wantError: "rate: value must be a number",
		},
		{
			name:     "bool_true",
			variable: GameModVar{Var: "pvp", Info: "PvP", Type: GameModVarTypeBool},
			value:    true,
			want:     "1",
		},
		{
			name:     "bool_false",
			variable: GameModVar{Var: "pvp", Info: "PvP", Type: GameModVarTypeBool},
			value:    false,
			want:     "0",
		},
		{
			name: "bool_uses_declared_values",
			variable: GameModVar{
				Var: "pvp", Info: "PvP", Type: GameModVarTypeBool,
				TrueValue: new("on"), FalseValue: new("off"),
			},
			value: true,
			want:  "on",
		},
		{
			name: "bool_accepts_declared_value_as_string",
			variable: GameModVar{
				Var: "pvp", Info: "PvP", Type: GameModVarTypeBool,
				TrueValue: new("on"), FalseValue: new("off"),
			},
			value: "on",
			want:  "on",
		},
		{
			name:     "bool_accepts_legacy_string",
			variable: GameModVar{Var: "pvp", Info: "PvP", Type: GameModVarTypeBool},
			value:    "true",
			want:     "1",
		},
		{
			name: "bool_with_empty_false_value",
			variable: GameModVar{
				Var: "verbose", Info: "Verbose", Type: GameModVarTypeBool,
				TrueValue: new("-v"), FalseValue: new(""),
			},
			value: false,
			want:  "",
		},
		{
			name:      "bool_rejects_arbitrary_text",
			variable:  GameModVar{Var: "pvp", Info: "PvP", Type: GameModVarTypeBool},
			value:     "maybe",
			wantError: "pvp: value must be a boolean",
		},
		{
			name: "select_accepts_known_option",
			variable: GameModVar{
				Var: "mod", Info: "Mod", Type: GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "vanilla"}, {Value: "paper"}},
			},
			value: "paper",
			want:  "paper",
		},
		{
			name: "select_rejects_unknown_option",
			variable: GameModVar{
				Var: "mod", Info: "Mod", Type: GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "vanilla"}, {Value: "paper"}},
			},
			value:     "forge",
			wantError: "mod: value must be one of the allowed options",
		},
		{
			name: "select_with_allow_custom_accepts_unknown_option",
			variable: GameModVar{
				Var: "version", Info: "Version", Type: GameModVarTypeSelect, AllowCustom: true,
				Options: GameModVarOptions{{Value: "1.21"}},
			},
			value: "1.19.4",
			want:  "1.19.4",
		},
		{
			name: "select_with_allow_custom_applies_length_rules",
			variable: GameModVar{
				Var: "version", Info: "Version", Type: GameModVarTypeSelect, AllowCustom: true,
				Options: GameModVarOptions{{Value: "1.21"}},
				Rules:   &GameModVarRules{MaxLength: new(4)},
			},
			value:     "1.19.4",
			wantError: "version: value must be at most 4 characters",
		},
		{
			name: "min_length_violation",
			variable: GameModVar{
				Var: "hostname", Info: "Hostname", Rules: &GameModVarRules{MinLength: new(4)},
			},
			value:     "ab",
			wantError: "hostname: value must be at least 4 characters",
		},
		{
			name: "length_counts_runes",
			variable: GameModVar{
				Var: "hostname", Info: "Hostname", Rules: &GameModVarRules{MaxLength: new(3)},
			},
			value: "Ноя",
			want:  "Ноя",
		},
		{
			name: "pattern_matches_whole_value",
			variable: GameModVar{
				Var: "map", Info: "Map", Rules: &GameModVarRules{Pattern: `de_[a-z0-9]+`},
			},
			value: "de_dust2",
			want:  "de_dust2",
		},
		{
			name: "pattern_rejects_partial_match",
			variable: GameModVar{
				Var: "map", Info: "Map", Rules: &GameModVarRules{Pattern: `de_[a-z0-9]+`},
			},
			value:     "cs_de_dust2",
			wantError: "map: value has an invalid format",
		},
		{
			name:      "value_longer_than_the_global_guard",
			variable:  GameModVar{Var: "hostname", Info: "Hostname"},
			value:     strings.Repeat("a", 4097),
			wantError: "hostname: value must be at most 4096 characters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			variable := test.variable

			// ACT
			result, err := variable.NormalizeValue(test.value)

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

func TestGameModVar_NormalizeValue_ReportsRule(t *testing.T) {
	t.Parallel()

	// ARRANGE
	variable := GameModVar{
		Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt,
		Rules: &GameModVarRules{Max: new(64.0)},
	}

	// ACT
	_, err := variable.NormalizeValue(json.Number("65"))

	// ASSERT
	require.Error(t, err)

	var valueErr *GameModVarValueError
	require.ErrorAs(t, err, &valueErr)
	assert.Equal(t, "maxplayers", valueErr.Var)
	assert.Equal(t, "max", valueErr.Rule)
	assert.Equal(t, "value must be at most 64", valueErr.Detail)
}

func TestGameModVar_FormatValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		variable GameModVar
		stored   string
		want     any
	}{
		{
			name:     "string_stays_string",
			variable: GameModVar{Var: "hostname", Info: "Hostname"},
			stored:   "007",
			want:     "007",
		},
		{
			name:     "int_becomes_number",
			variable: GameModVar{Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt},
			stored:   "32",
			want:     int64(32),
		},
		{
			name:     "unparseable_int_stays_string",
			variable: GameModVar{Var: "maxplayers", Info: "Max", Type: GameModVarTypeInt},
			stored:   "many",
			want:     "many",
		},
		{
			name:     "float_becomes_number",
			variable: GameModVar{Var: "rate", Info: "Rate", Type: GameModVarTypeFloat},
			stored:   "0.5",
			want:     0.5,
		},
		{
			name:     "bool_true",
			variable: GameModVar{Var: "pvp", Info: "PvP", Type: GameModVarTypeBool},
			stored:   "1",
			want:     true,
		},
		{
			name: "bool_from_declared_value",
			variable: GameModVar{
				Var: "pvp", Info: "PvP", Type: GameModVarTypeBool,
				TrueValue: new("on"), FalseValue: new("off"),
			},
			stored: "on",
			want:   true,
		},
		{
			name: "bool_empty_false_value",
			variable: GameModVar{
				Var: "verbose", Info: "Verbose", Type: GameModVarTypeBool,
				TrueValue: new("-v"), FalseValue: new(""),
			},
			stored: "",
			want:   false,
		},
		{
			name:     "password_stays_string",
			variable: GameModVar{Var: "rcon", Info: "RCON", Type: GameModVarTypePassword},
			stored:   "secret",
			want:     "secret",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			variable := test.variable

			// ACT
			result := variable.FormatValue(test.stored)

			// ASSERT
			assert.Equal(t, test.want, result)
		})
	}
}
