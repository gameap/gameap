package config

import (
	"log/slog"
	"os"
	"strings"
)

// renamedVar is an environment variable that was renamed. The old name keeps
// working for one release: when it is set and the new one is not, its value is
// carried over and a warning names the replacement.
//
// Convert adapts a value whose unit used to live in the variable name (an
// integer count of megabytes or seconds) to the new self-describing form
// ("512" -> "512M", "30" -> "30s"). It is nil when only the name changed.
type renamedVar struct {
	Old     string
	New     string
	Convert func(string) string
}

// appendUnit turns a bare number into a value carrying its unit. A value that
// already has a non-numeric tail is passed through: an operator who set the
// old variable to "512M" gets what they meant, not "512MM".
func appendUnit(unit string) func(string) string {
	return func(value string) string {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return value
		}

		for _, r := range trimmed {
			if r < '0' || r > '9' {
				return trimmed
			}
		}

		return trimmed + unit
	}
}

// renamedVars is the compatibility table. Entries stay for one release after
// the rename, then go.
//
// The plugin block was renamed so that every name follows its position in the
// config struct (Plugin.Net.MaxTimeout -> PLUGIN_NET_MAX_TIMEOUT) and so that
// no name carries a unit the value can carry itself
// (PLUGIN_NET_MAX_TIMEOUT_SECONDS=10 -> PLUGIN_NET_MAX_TIMEOUT=10s).
//
// Only names that reached a release belong here. The rest of the plugin
// limits were renamed before they ever shipped, so no installation can have
// them set and carrying them would only keep dead names alive.
var renamedVars = []renamedVar{
	{Old: "PLUGIN_HTTP_MAX_TIMEOUT_SECONDS", New: "PLUGIN_HTTP_MAX_TIMEOUT", Convert: appendUnit("s")},
	{Old: "PLUGIN_NET_MAX_TIMEOUT_SECONDS", New: "PLUGIN_NET_MAX_TIMEOUT", Convert: appendUnit("s")},
	{Old: "PLUGIN_NET_READ_BUFFER_BYTES", New: "PLUGIN_NET_READ_BUFFER"},
}

// applyRenamedVars copies deprecated variables onto their replacements before
// the config is parsed. The new name always wins, so an operator migrating one
// variable at a time is never surprised by a stale value; each carried-over
// variable is reported once at startup.
//
// It is called before env parsing, hence os.Setenv rather than a config field:
// the parser only sees the current names.
func applyRenamedVars() {
	for _, v := range renamedVars {
		oldValue, oldSet := os.LookupEnv(v.Old)
		if !oldSet {
			continue
		}

		if _, newSet := os.LookupEnv(v.New); newSet {
			slog.Warn("both the deprecated and the current environment variable are set, the deprecated one is ignored",
				slog.String("deprecated", v.Old),
				slog.String("use", v.New))

			continue
		}

		value := oldValue
		if v.Convert != nil {
			value = v.Convert(oldValue)
		}

		if err := os.Setenv(v.New, value); err != nil {
			slog.Error("failed to apply deprecated environment variable",
				slog.String("deprecated", v.Old),
				slog.String("use", v.New),
				slog.String("error", err.Error()))

			continue
		}

		attrs := []any{
			slog.String("deprecated", v.Old),
			slog.String("use", v.New),
		}
		if value != oldValue {
			attrs = append(attrs, slog.String("value", value))
		}

		slog.Warn("environment variable is deprecated and will be removed in a future release", attrs...)
	}
}
