package configschema_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/configschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleSchema = `{
  "type": "object",
  "properties": {
    "api_key": {"type": "string", "format": "secret", "title": "API key"},
    "region": {"type": "string", "enum": ["eu", "us"], "default": "eu"},
    "port": {"type": "integer", "minimum": 1, "maximum": 65535, "default": 8080},
    "ratio": {"type": "number", "default": 0.5},
    "enabled": {"type": "boolean", "default": true, "description": "Turn it on"},
    "name": {"type": "string", "minLength": 2, "maxLength": 5, "pattern": "^[a-z]+$"}
  },
  "required": ["api_key", "port"]
}`

func TestParse_keeps_declaration_order_and_types(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(sampleSchema)
	require.NoError(t, err)
	require.NotNil(t, schema)

	names := make([]string, 0, len(schema.Properties))
	for _, property := range schema.Properties {
		names = append(names, property.Name)
	}

	assert.Equal(t, []string{"api_key", "region", "port", "ratio", "enabled", "name"}, names)
	assert.True(t, schema.AdditionalProperties)

	apiKey, ok := schema.Property("api_key")
	require.True(t, ok)
	assert.True(t, apiKey.Secret)
	assert.True(t, apiKey.Required)
	assert.Equal(t, "API key", apiKey.Title)

	port, ok := schema.Property("port")
	require.True(t, ok)
	assert.Equal(t, configschema.TypeInteger, port.Type)
	assert.Equal(t, int64(8080), port.Default)
	assert.Equal(t, 1.0, *port.Minimum)
	assert.Equal(t, 65535.0, *port.Maximum)
	assert.True(t, port.Required)

	ratio, ok := schema.Property("ratio")
	require.True(t, ok)
	assert.Equal(t, 0.5, ratio.Default)

	enabled, ok := schema.Property("enabled")
	require.True(t, ok)
	assert.Equal(t, true, enabled.Default)
	assert.Equal(t, "Turn it on", enabled.Description)

	region, ok := schema.Property("region")
	require.True(t, ok)
	assert.Equal(t, []any{"eu", "us"}, region.Enum)

	name, ok := schema.Property("name")
	require.True(t, ok)
	assert.Equal(t, 2, *name.MinLength)
	assert.Equal(t, 5, *name.MaxLength)
	assert.Equal(t, "^[a-z]+$", name.Pattern.String())

	_, ok = schema.Property("missing")
	assert.False(t, ok)
}

func TestParse_empty_text_is_no_schema(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse("  ")
	require.NoError(t, err)
	assert.Nil(t, schema)
}

func TestParse_rejects_unsupported_schemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "not_json", text: "{", want: "not a JSON object"},
		{name: "array_root", text: "[]", want: "not a JSON object"},
		{name: "root_type", text: `{"type": "array"}`, want: `root "type" must be "object"`},
		{name: "nested_object", text: `{"properties": {"a": {"type": "object"}}}`, want: `unsupported type "object"`},
		{name: "array_property", text: `{"properties": {"a": {"type": "array"}}}`, want: `unsupported type "array"`},
		{name: "missing_type", text: `{"properties": {"a": {"title": "x"}}}`, want: `missing "type"`},
		{name: "ref", text: `{"$ref": "#/x"}`, want: `unsupported keyword "$ref"`},
		{name: "one_of", text: `{"oneOf": []}`, want: `unsupported keyword "oneOf"`},
		{name: "bad_key", text: `{"properties": {"-bad": {"type": "string"}}}`, want: `property name "-bad"`},
		{name: "bad_pattern", text: `{"properties": {"a": {"type": "string", "pattern": "("}}}`, want: `invalid "pattern"`},
		{name: "secret_default", text: `{"properties": {"a": {"type": "string", "format": "secret", "default": "x"}}}`,
			want: "a secret must not declare a default"},
		{name: "secret_not_string", text: `{"properties": {"a": {"type": "integer", "format": "secret"}}}`,
			want: `"format": "secret" requires type string`},
		{name: "default_type", text: `{"properties": {"a": {"type": "integer", "default": "x"}}}`, want: "must be an integer"},
		{name: "default_fraction", text: `{"properties": {"a": {"type": "integer", "default": 1.5}}}`, want: "must be an integer"},
		{name: "enum_type", text: `{"properties": {"a": {"type": "boolean", "enum": ["x"]}}}`, want: "must be a boolean"},
		{name: "required_unknown", text: `{"properties": {}, "required": ["nope"]}`, want: `unknown property "nope"`},
		{name: "additional_object", text: `{"additionalProperties": {}}`, want: `"additionalProperties" must be a boolean`},
		{name: "min_over_max", text: `{"properties": {"a": {"type": "integer", "minimum": 5, "maximum": 1}}}`,
			want: `"minimum" exceeds "maximum"`},
		{name: "negative_length", text: `{"properties": {"a": {"type": "string", "minLength": -1}}}`,
			want: `"minLength" must be a non-negative integer`},
		{name: "duplicate_property", text: `{"properties": {"a": {"type": "string"}, "a": {"type": "string"}}}`,
			want: `declared twice`},
		{name: "huge_integer", text: `{"properties": {"a": {"type": "integer", "default": 9007199254740993}}}`,
			want: "within ±2^53"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := configschema.Parse(tt.text)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParse_limits(t *testing.T) {
	t.Parallel()

	_, err := configschema.Parse(`{"properties": {}, "x": "` + strings.Repeat("a", configschema.MaxSchemaBytes) + `"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")

	var sb strings.Builder
	sb.WriteString(`{"properties": {`)
	for i := 0; i <= configschema.MaxProperties; i++ {
		if i > 0 {
			sb.WriteString(",")
		}

		sb.WriteString(`"p` + strconv.Itoa(i) + `": {"type": "string"}`)
	}
	sb.WriteString(`}}`)

	_, err = configschema.Parse(sb.String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than")
}

func TestParse_ignores_unknown_annotations(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(`{"$schema": "x", "title": "Plugin", "properties": {"a": {"type": "string", "examples": ["e"], "x-ui": 1}}}`)
	require.NoError(t, err)
	require.Len(t, schema.Properties, 1)
}

func TestDefaults_and_ApplyDefaults(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(sampleSchema)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"region":  "eu",
		"port":    "8080",
		"ratio":   "0.5",
		"enabled": "true",
	}, schema.Defaults())

	merged := configschema.ApplyDefaults(sampleSchema, map[string]string{"port": "9000", "api_key": "k"})
	assert.Equal(t, map[string]string{
		"api_key": "k",
		"region":  "eu",
		"port":    "9000",
		"ratio":   "0.5",
		"enabled": "true",
	}, merged)

	assert.Nil(t, configschema.ApplyDefaults("", nil))
	assert.Equal(t, map[string]string{"a": "b"}, configschema.ApplyDefaults("{", map[string]string{"a": "b"}))
	assert.Equal(t, map[string]string{"a": "b"},
		configschema.ApplyDefaults(`{"properties": {"x": {"type": "string"}}}`, map[string]string{"a": "b"}))
}

func TestValueToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value any
		want  string
		ok    bool
	}{
		{value: nil, want: "", ok: false},
		{value: "text", want: "text", ok: true},
		{value: true, want: "true", ok: true},
		{value: float64(8080), want: "8080", ok: true},
		{value: 0.25, want: "0.25", ok: true},
		{value: int64(-3), want: "-3", ok: true},
		{value: 7, want: "7", ok: true},
		{value: json.Number("12"), want: "12", ok: true},
		{value: map[string]any{"b": 1, "a": []any{"x"}}, want: `{"a":["x"],"b":1}`, ok: true},
	}

	for _, tt := range tests {
		got, ok := configschema.ValueToString(tt.value)
		assert.Equal(t, tt.ok, ok)
		assert.Equal(t, tt.want, got)
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, configschema.SummaryInfo{Valid: true}, configschema.Summary(""))
	assert.Equal(t, configschema.SummaryInfo{Valid: true, Properties: 6, Required: 2, Secrets: 1},
		configschema.Summary(sampleSchema))

	invalid := configschema.Summary("{")
	assert.False(t, invalid.Valid)
	assert.NotEmpty(t, invalid.Error)
}

func TestJSON_renders_ordered_properties(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(sampleSchema)
	require.NoError(t, err)

	encoded, err := json.Marshal(schema.JSON())
	require.NoError(t, err)

	var decoded struct {
		Properties []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Secret   bool   `json:"secret"`
			Required bool   `json:"required"`
			Default  any    `json:"default"`
			Pattern  string `json:"pattern"`
		} `json:"properties"`
		AdditionalProperties bool `json:"additional_properties"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded.Properties, 6)
	assert.Equal(t, "api_key", decoded.Properties[0].Name)
	assert.True(t, decoded.Properties[0].Secret)
	assert.True(t, decoded.Properties[0].Required)
	assert.Equal(t, "port", decoded.Properties[2].Name)
	assert.Equal(t, float64(8080), decoded.Properties[2].Default)
	assert.Equal(t, "^[a-z]+$", decoded.Properties[5].Pattern)
	assert.True(t, decoded.AdditionalProperties)

	var nilSchema *configschema.Schema
	assert.Nil(t, nilSchema.JSON())
}
