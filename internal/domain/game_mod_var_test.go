package domain

import (
	"encoding/json"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameModVarOption_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		option GameModVarOption
		want   string
	}{
		{
			name:   "value_only_uses_shorthand",
			option: GameModVarOption{Value: "1.20.4"},
			want:   `"1.20.4"`,
		},
		{
			name:   "label_forces_object",
			option: GameModVarOption{Value: "paper", Label: "Paper"},
			want:   `{"value":"paper","label":"Paper"}`,
		},
		{
			name: "translations_force_object",
			option: GameModVarOption{
				Value: "vanilla",
				I18n:  GameModVarOptionI18n{"ru": {Label: "Ванильный"}},
			},
			want: `{"value":"vanilla","i18n":{"ru":{"label":"Ванильный"}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ACT
			encoded, err := json.Marshal(test.option)

			// ASSERT
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(encoded))
		})
	}
}

func TestGameModVarOption_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		want      GameModVarOption
		wantError string
	}{
		{
			name:  "plain_string",
			input: `"1.20.4"`,
			want:  GameModVarOption{Value: "1.20.4"},
		},
		{
			name:  "object_with_label",
			input: `{"value":"paper","label":"Paper"}`,
			want:  GameModVarOption{Value: "paper", Label: "Paper"},
		},
		{
			name:  "object_with_translations",
			input: `{"value":"vanilla","i18n":{"ru":{"label":"Ванильный"}}}`,
			want: GameModVarOption{
				Value: "vanilla",
				I18n:  GameModVarOptionI18n{"ru": {Label: "Ванильный"}},
			},
		},
		{
			name:      "number_is_rejected",
			input:     `42`,
			wantError: "must be a string or an object",
		},
		{
			name:      "array_is_rejected",
			input:     `["a"]`,
			wantError: "must be a string or an object",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var option GameModVarOption

			// ACT
			err := json.Unmarshal([]byte(test.input), &option)

			// ASSERT
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, option)
		})
	}
}

func TestGameModVarOption_YAMLRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// wantShorthand tells, per option, whether the encoder must emit a bare
		// scalar rather than a mapping. The encoded text itself is not asserted:
		// whether "1.20.4" comes back quoted is the YAML library's business.
		input         string
		want          GameModVarOptions
		wantShorthand []bool
	}{
		{
			name:          "shorthand_list_stays_shorthand",
			input:         "- '1.21'\n- '1.20.4'\n",
			want:          GameModVarOptions{{Value: "1.21"}, {Value: "1.20.4"}},
			wantShorthand: []bool{true, true},
		},
		{
			name:          "object_list_keeps_labels",
			input:         "- {value: paper, label: Paper}\n",
			want:          GameModVarOptions{{Value: "paper", Label: "Paper"}},
			wantShorthand: []bool{false},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var options GameModVarOptions

			// ACT
			err := yaml.Unmarshal([]byte(test.input), &options)
			require.NoError(t, err)

			encoded, encodeErr := yaml.Marshal(options)

			// ASSERT
			require.NoError(t, encodeErr)
			assert.Equal(t, test.want, options)

			var shapes []any
			require.NoError(t, yaml.Unmarshal(encoded, &shapes))
			require.Len(t, shapes, len(test.wantShorthand))

			for i, wantShorthand := range test.wantShorthand {
				_, isScalar := shapes[i].(string)
				assert.Equalf(t, wantShorthand, isScalar, "options[%d] shorthand mismatch: %v", i, shapes[i])
			}

			var reparsed GameModVarOptions
			require.NoError(t, yaml.Unmarshal(encoded, &reparsed))
			assert.Equal(t, test.want, reparsed)
		})
	}
}

func TestGameModVar_LegacyVarSerializesUnchanged(t *testing.T) {
	t.Parallel()

	// ARRANGE
	legacy := GameModVar{Var: "maxplayers", Default: "32", Info: "Max players", AdminVar: true}

	// ACT
	encoded, err := json.Marshal(legacy)

	// ASSERT
	require.NoError(t, err)
	assert.JSONEq(
		t,
		`{"var":"maxplayers","default":"32","info":"Max players","admin_var":true}`,
		string(encoded),
	)
}

func TestGameModVar_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	original := GameModVar{
		Var:         "version",
		Default:     "1.20.4",
		Info:        "Minecraft version",
		AdminVar:    true,
		Type:        GameModVarTypeSelect,
		Description: "The version the server runs",
		Options: GameModVarOptions{
			{Value: "1.21"},
			{Value: "1.20.4", Label: "1.20.4 (LTS)", I18n: GameModVarOptionI18n{"ru": {Label: "1.20.4 (LTS)"}}},
		},
		AllowCustom: true,
		Rules: &GameModVarRules{
			Required:  new(true),
			MinLength: new(1),
			MaxLength: new(16),
			Pattern:   `[0-9.]+`,
		},
		I18n: GameModVarI18n{"ru": {Info: "Версия Minecraft", Description: "Версия, на которой работает сервер"}},
	}

	// ACT
	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded GameModVar
	decodeErr := json.Unmarshal(encoded, &decoded)

	// ASSERT
	require.NoError(t, decodeErr)
	assert.Equal(t, original, decoded)
}

func TestGameModVar_YAMLRoundTrip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	original := GameModVar{
		Var:        "pvp",
		Default:    "on",
		Info:       "PvP",
		Type:       GameModVarTypeBool,
		TrueValue:  new("on"),
		FalseValue: new("off"),
		I18n:       GameModVarI18n{"ru": {Info: "PvP"}},
	}

	// ACT
	encoded, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded GameModVar
	decodeErr := yaml.Unmarshal(encoded, &decoded)

	// ASSERT
	require.NoError(t, decodeErr)
	assert.Equal(t, original, decoded)
}

func TestGameModVarDefault_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  GameModVarDefault
	}{
		{
			name:  "quoted_string",
			input: "default: '32'\n",
			want:  "32",
		},
		{
			name:  "bare_number",
			input: "default: 32\n",
			want:  "32",
		},
		{
			name:  "null",
			input: "default: null\n",
			want:  "",
		},
		{
			name:  "missing_key",
			input: "info: x\n",
			want:  "",
		},
		{
			name:  "bare_bool",
			input: "default: true\n",
			want:  "true",
		},
		{
			name:  "fractional_number",
			input: "default: 1.5\n",
			want:  "1.5",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var holder struct {
				Default GameModVarDefault `yaml:"default"`
			}

			// ACT
			err := yaml.Unmarshal([]byte(test.input), &holder)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, test.want, holder.Default)
		})
	}
}

func TestGameModVar_Normalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input GameModVar
		want  GameModVar
	}{
		{
			name: "select_only_fields_dropped_for_string",
			input: GameModVar{
				Var:         "hostname",
				Info:        "Hostname",
				Options:     GameModVarOptions{{Value: "a"}},
				AllowCustom: true,
			},
			want: GameModVar{Var: "hostname", Info: "Hostname"},
		},
		{
			name: "bool_only_fields_dropped_for_int",
			input: GameModVar{
				Var:        "maxplayers",
				Info:       "Max players",
				Type:       GameModVarTypeInt,
				TrueValue:  new("1"),
				FalseValue: new("0"),
			},
			want: GameModVar{Var: "maxplayers", Info: "Max players", Type: GameModVarTypeInt},
		},
		{
			name: "numeric_rules_dropped_for_string",
			input: GameModVar{
				Var:   "hostname",
				Info:  "Hostname",
				Rules: &GameModVarRules{Min: new(1.0), Max: new(5.0), MaxLength: new(64)},
			},
			want: GameModVar{
				Var:   "hostname",
				Info:  "Hostname",
				Rules: &GameModVarRules{MaxLength: new(64)},
			},
		},
		{
			name: "empty_rules_object_dropped",
			input: GameModVar{
				Var:   "hostname",
				Info:  "Hostname",
				Rules: &GameModVarRules{Required: new(false)},
			},
			want: GameModVar{Var: "hostname", Info: "Hostname"},
		},
		{
			name: "en_locale_and_empty_translations_dropped",
			input: GameModVar{
				Var:  "hostname",
				Info: "Hostname",
				I18n: GameModVarI18n{"en": {Info: "Hostname"}, "RU": {Info: "Имя"}, "de": {}},
			},
			want: GameModVar{
				Var:  "hostname",
				Info: "Hostname",
				I18n: GameModVarI18n{"ru": {Info: "Имя"}},
			},
		},
		{
			name: "redundant_option_label_dropped",
			input: GameModVar{
				Var:     "mod",
				Info:    "Mod",
				Type:    GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "paper", Label: "paper"}},
			},
			want: GameModVar{
				Var:     "mod",
				Info:    "Mod",
				Type:    GameModVarTypeSelect,
				Options: GameModVarOptions{{Value: "paper"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			variable := test.input

			// ACT
			variable.Normalize()

			// ASSERT
			assert.Equal(t, test.want, variable)
		})
	}
}
