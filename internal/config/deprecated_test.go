package config //nolint:testpackage // applyRenamedVars and renamedVars are unexported

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// varsBornWithCurrentName are the PLUGINS_ variables that never had another
// spelling, so no compatibility entry can point at them.
var varsBornWithCurrentName = []string{
	"PLUGINS_DISABLED",
	"PLUGINS_AUTOLOAD",
	"PLUGINS_STRICT_LOAD",
}

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

// unsetForTest removes a variable for the duration of the test and restores
// the process environment afterwards, the way t.Setenv does for a value. The
// shim writes the current names, so a test that leaves them behind would
// silently change what a later test parses.
func unsetForTest(t *testing.T, name string) {
	t.Helper()

	// t.Setenv records what the process had and restores it when the test
	// ends; unsetting right after leaves the test itself without a value.
	t.Setenv(name, os.Getenv(name))
	require.NoError(t, os.Unsetenv(name))
}

func TestApplyRenamedVars_carries_the_deprecated_value_over(t *testing.T) {
	tests := []struct {
		name  string
		old   string
		value string
		newer string
	}{
		{
			name:  "capability_limit_keeps_working_under_the_new_prefix",
			old:   "PLUGIN_SSH_ENABLED",
			value: "true",
			newer: "PLUGINS_SSH_ENABLED",
		},
		{
			name:  "store_setting_keeps_working_under_the_new_prefix",
			old:   "PLUGIN_STORE_URL",
			value: "https://plugins.example.test/api",
			newer: "PLUGINS_STORE_URL",
		},
		{
			name:  "module_cache_moved_under_the_runtime_block",
			old:   "PLUGINS_CACHE_DIR",
			value: "/var/lib/gameap/wasm",
			newer: "PLUGINS_RUNTIME_CACHE_DIR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.old, tt.value)
			unsetForTest(t, tt.newer)

			applyRenamedVars()

			assert.Equal(t, tt.value, os.Getenv(tt.newer))
		})
	}
}

func TestApplyRenamedVars_keeps_the_current_variable(t *testing.T) {
	t.Setenv("PLUGIN_NET_MAX_TIMEOUT", "10s")
	t.Setenv("PLUGINS_NET_MAX_TIMEOUT", "90s")

	applyRenamedVars()

	assert.Equal(t, "90s", os.Getenv("PLUGINS_NET_MAX_TIMEOUT"),
		"the current name wins over the deprecated one")
}

//nolint:paralleltest // mutates process-global environment via os.Unsetenv
func TestApplyRenamedVars_leaves_an_unset_variable_alone(t *testing.T) {
	unsetForTest(t, "PLUGIN_NET_MAX_TIMEOUT")
	unsetForTest(t, "PLUGINS_NET_MAX_TIMEOUT")

	applyRenamedVars()

	_, set := os.LookupEnv("PLUGINS_NET_MAX_TIMEOUT")
	assert.False(t, set, "an unset deprecated variable must not materialise the new one")
}

func TestApplyRenamedVars_deprecated_values_reach_the_parsed_config(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://localhost/test")
	t.Setenv("AUTH_SECRET", "test-secret")
	t.Setenv("PLUGIN_SSH_ENABLED", "true")
	t.Setenv("PLUGIN_NET_READ_BUFFER", "128K")
	t.Setenv("PLUGIN_STORE_URL", "https://plugins.example.test/api")
	t.Setenv("PLUGINS_CACHE_DIR", "/var/lib/gameap/wasm")
	unsetForTest(t, "PLUGINS_SSH_ENABLED")
	unsetForTest(t, "PLUGINS_NET_READ_BUFFER")
	unsetForTest(t, "PLUGINS_STORE_URL")
	unsetForTest(t, "PLUGINS_RUNTIME_CACHE_DIR")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.True(t, cfg.Plugins.SSH.Enabled)
	assert.Equal(t, uint64(128*1024), cfg.Plugins.Net.ReadBuffer.Uint64())
	assert.Equal(t, "https://plugins.example.test/api", cfg.Plugins.Store.URL)
	assert.Equal(t, "/var/lib/gameap/wasm", cfg.Plugins.Runtime.Cache.Dir)
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

func TestRenamedVars_have_no_duplicates(t *testing.T) {
	t.Parallel()

	seenOld := make(map[string]struct{}, len(renamedVars))
	seenNew := make(map[string]struct{}, len(renamedVars))

	for _, v := range renamedVars {
		_, repeatedOld := seenOld[v.Old]
		assert.Falsef(t, repeatedOld, "%s is listed twice", v.Old)
		seenOld[v.Old] = struct{}{}

		_, repeatedNew := seenNew[v.New]
		assert.Falsef(t, repeatedNew, "two deprecated names point at %s", v.New)
		seenNew[v.New] = struct{}{}
	}
}

// A deprecated name that is itself the target of another entry would make the
// carried-over value depend on where the two entries sit in the table.
func TestRenamedVars_are_order_independent(t *testing.T) {
	t.Parallel()

	deprecated := make(map[string]struct{}, len(renamedVars))
	for _, v := range renamedVars {
		deprecated[v.Old] = struct{}{}
	}

	for _, v := range renamedVars {
		_, chained := deprecated[v.New]
		assert.Falsef(t, chained, "%s points at %s, which is deprecated as well", v.Old, v.New)
	}
}

// The plugin block was renamed wholesale, so a missing entry is a setting an
// upgrading installation loses without being told.
func TestRenamedVars_cover_every_renamed_plugin_variable(t *testing.T) {
	t.Parallel()

	deprecatedNameOf := make(map[string]string, len(renamedVars))
	for _, v := range renamedVars {
		deprecatedNameOf[v.New] = v.Old
	}

	for _, name := range declaredEnvNames(t) {
		if !strings.HasPrefix(name, "PLUGINS_") || slices.Contains(varsBornWithCurrentName, name) {
			continue
		}

		old, covered := deprecatedNameOf[name]
		require.Truef(t, covered, "%s has no compatibility entry", name)

		if strings.HasPrefix(old, "PLUGIN_") {
			assert.Equalf(t, "PLUGIN_"+strings.TrimPrefix(name, "PLUGINS_"), old,
				"the deprecated spelling of %s must differ from it by the prefix only", name)
		}
	}
}
