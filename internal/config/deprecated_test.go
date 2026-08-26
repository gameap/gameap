package config //nolint:testpackage // applyRenamedVars and renamedVars are unexported

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// declaredEnvNames collects every env tag the Config struct declares, so the
// compatibility table can be checked against the real names.
func declaredEnvNames(t *testing.T) []string {
	t.Helper()

	names := make([]string, 0)

	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for field := range typ.Fields() {
			if name, ok := field.Tag.Lookup("env"); ok {
				names = append(names, name)
			}

			if field.Type.Kind() == reflect.Struct {
				walk(field.Type)
			}
		}
	}

	walk(reflect.TypeFor[Config]())

	return names
}

func TestApplyRenamedVars_carries_the_deprecated_value_over(t *testing.T) {
	tests := []struct {
		name  string
		old   string
		value string
		want  string
		newer string
	}{
		{
			name:  "http_second_count_gains_its_unit",
			old:   "PLUGIN_HTTP_MAX_TIMEOUT_SECONDS",
			value: "45",
			want:  "45s",
			newer: "PLUGIN_HTTP_MAX_TIMEOUT",
		},
		{
			name:  "net_second_count_gains_its_unit",
			old:   "PLUGIN_NET_MAX_TIMEOUT_SECONDS",
			value: "10",
			want:  "10s",
			newer: "PLUGIN_NET_MAX_TIMEOUT",
		},
		{
			name:  "byte_count_is_carried_over_unchanged",
			old:   "PLUGIN_NET_READ_BUFFER_BYTES",
			value: "65536",
			want:  "65536",
			newer: "PLUGIN_NET_READ_BUFFER",
		},
		{
			name:  "value_that_already_carries_a_unit_is_not_suffixed_twice",
			old:   "PLUGIN_HTTP_MAX_TIMEOUT_SECONDS",
			value: "45s",
			want:  "45s",
			newer: "PLUGIN_HTTP_MAX_TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.old, tt.value)
			require.NoError(t, os.Unsetenv(tt.newer))

			applyRenamedVars()

			assert.Equal(t, tt.want, os.Getenv(tt.newer))
		})
	}
}

func TestApplyRenamedVars_keeps_the_current_variable(t *testing.T) {
	t.Setenv("PLUGIN_NET_MAX_TIMEOUT_SECONDS", "10")
	t.Setenv("PLUGIN_NET_MAX_TIMEOUT", "90s")

	applyRenamedVars()

	assert.Equal(t, "90s", os.Getenv("PLUGIN_NET_MAX_TIMEOUT"),
		"the current name wins over the deprecated one")
}

//nolint:paralleltest // mutates process-global environment via os.Unsetenv
func TestApplyRenamedVars_leaves_an_unset_variable_alone(t *testing.T) {
	require.NoError(t, os.Unsetenv("PLUGIN_NET_MAX_TIMEOUT_SECONDS"))
	require.NoError(t, os.Unsetenv("PLUGIN_NET_MAX_TIMEOUT"))

	applyRenamedVars()

	_, set := os.LookupEnv("PLUGIN_NET_MAX_TIMEOUT")
	assert.False(t, set, "an unset deprecated variable must not materialise the new one")
}

func TestApplyRenamedVars_deprecated_values_reach_the_parsed_config(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://localhost/test")
	t.Setenv("AUTH_SECRET", "test-secret")
	t.Setenv("PLUGIN_HTTP_MAX_TIMEOUT_SECONDS", "45")
	t.Setenv("PLUGIN_NET_MAX_TIMEOUT_SECONDS", "20")
	t.Setenv("PLUGIN_NET_READ_BUFFER_BYTES", "65536")
	require.NoError(t, os.Unsetenv("PLUGIN_HTTP_MAX_TIMEOUT"))
	require.NoError(t, os.Unsetenv("PLUGIN_NET_MAX_TIMEOUT"))
	require.NoError(t, os.Unsetenv("PLUGIN_NET_READ_BUFFER"))

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, 45.0, cfg.Plugin.HTTP.MaxTimeout.Seconds())
	assert.Equal(t, 20.0, cfg.Plugin.Net.MaxTimeout.Seconds())
	assert.Equal(t, uint64(65536), cfg.Plugin.Net.ReadBuffer.Uint64())
}

// Every renamed variable must point at a name the config actually declares,
// otherwise the compatibility shim writes an environment variable nothing reads.
func TestRenamedVars_target_a_declared_variable(t *testing.T) {
	t.Parallel()

	declared := declaredEnvNames(t)

	for _, v := range renamedVars {
		assert.Containsf(t, declared, v.New, "%s points at %s, which no config field declares", v.Old, v.New)
		assert.NotContainsf(t, declared, v.Old, "%s is still declared by a config field", v.Old)
	}
}
