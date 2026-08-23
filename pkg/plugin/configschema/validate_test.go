package configschema_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/configschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeValues(t *testing.T, text string) map[string]any {
	t.Helper()

	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()

	var values map[string]any
	require.NoError(t, dec.Decode(&values))

	return values
}

func TestValidate(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(sampleSchema)
	require.NoError(t, err)

	tests := []struct {
		name   string
		values string
		want   map[string]string
	}{
		{name: "valid", values: `{"api_key": "k", "port": 8080, "region": "us", "ratio": 1, "enabled": false, "name": "abc", "extra": "free"}`},
		{name: "missing_required", values: `{"port": 1}`, want: map[string]string{"api_key": "is required"}},
		{name: "required_with_default_may_be_omitted", values: `{"api_key": "k"}`},
		{name: "null_counts_as_absent", values: `{"api_key": null, "port": 1}`, want: map[string]string{"api_key": "is required"}},
		{name: "wrong_types", values: `{"api_key": 1, "port": "80", "ratio": "x", "enabled": "yes", "name": 3}`,
			want: map[string]string{
				"api_key": "must be a string", "port": "must be an integer", "ratio": "must be a number",
				"enabled": "must be a boolean", "name": "must be a string",
			}},
		{name: "fraction_is_not_integer", values: `{"api_key": "k", "port": 1.5}`, want: map[string]string{"port": "must be an integer"}},
		{name: "bounds", values: `{"api_key": "k", "port": 70000}`, want: map[string]string{"port": "must be between 1 and 65535"}},
		{name: "enum", values: `{"api_key": "k", "port": 1, "region": "asia"}`, want: map[string]string{"region": "must be one of: eu, us"}},
		{name: "length", values: `{"api_key": "k", "port": 1, "name": "a"}`, want: map[string]string{"name": "must be between 2 and 5 characters"}},
		{name: "pattern", values: `{"api_key": "k", "port": 1, "name": "AB"}`, want: map[string]string{"name": "must match ^[a-z]+$"}},
		{name: "unknown_must_be_string", values: `{"api_key": "k", "port": 1, "extra": 5}`, want: map[string]string{"extra": "must be a string"}},
		{name: "unknown_bad_key", values: `{"api_key": "k", "port": 1, "bad key": "x"}`,
			want: map[string]string{"bad key": "must match " + configschema.KeyPattern.String()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errs := schema.Validate(decodeValues(t, tt.values))
			if tt.want == nil {
				assert.Empty(t, errs)

				return
			}

			assert.Equal(t, tt.want, errs.Map())
			assert.NotEmpty(t, errs.Error())
		})
	}
}

func TestValidate_additional_properties_false(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(`{"additionalProperties": false, "properties": {"a": {"type": "string"}}}`)
	require.NoError(t, err)

	errs := schema.Validate(decodeValues(t, `{"a": "x", "zzz": "y", "b": "z"}`))
	assert.Equal(t, []configschema.FieldError{
		{Field: "b", Message: "unknown key"},
		{Field: "zzz", Message: "unknown key"},
	}, []configschema.FieldError(errs))
}

func TestValidate_accepts_go_typed_values(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(sampleSchema)
	require.NoError(t, err)

	errs := schema.Validate(map[string]any{
		"api_key": "k", "port": 8080, "ratio": float64(2), "enabled": true,
	})
	assert.Empty(t, errs)

	errs = schema.Validate(map[string]any{"api_key": "k", "port": int64(0)})
	assert.Equal(t, map[string]string{"port": "must be between 1 and 65535"}, errs.Map())
}

func TestValidate_string_size_cap(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(`{"properties": {"a": {"type": "string"}}}`)
	require.NoError(t, err)

	huge := strings.Repeat("x", configschema.MaxValueBytes+1)
	errs := schema.Validate(map[string]any{"a": huge, "free": huge})
	assert.Equal(t, map[string]string{
		"a":    "must be at most 8192 bytes",
		"free": "must be at most 8192 bytes",
	}, errs.Map())
}

func TestValidateFreeForm(t *testing.T) {
	t.Parallel()

	var nilSchema *configschema.Schema
	errs := nilSchema.Validate(decodeValues(t, `{"ok": "x", "num": 1, "skip": null, "bad key": "y"}`))
	assert.Equal(t, map[string]string{
		"num":     "must be a string",
		"bad key": "must match " + configschema.KeyPattern.String(),
	}, errs.Map())

	assert.Empty(t, configschema.ValidateFreeForm(map[string]any{"a": "b"}))
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	schema, err := configschema.Parse(sampleSchema)
	require.NoError(t, err)

	values := decodeValues(t, `{"api_key": "k", "port": 8080, "ratio": 1, "enabled": true, "extra": "free", "gone": null}`)

	normalized, errs := schema.Normalize(values)
	require.Empty(t, errs)
	assert.Equal(t, map[string]any{
		"api_key": "k",
		"port":    int64(8080),
		"ratio":   float64(1),
		"enabled": true,
		"extra":   "free",
	}, normalized)

	_, errs = schema.Normalize(map[string]any{"port": "x", "extra": 1})
	assert.Equal(t, map[string]string{"port": "must be an integer", "extra": "must be a string"}, errs.Map())
}
