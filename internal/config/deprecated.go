package config

import (
	"log/slog"
	"os"
)

// renamedVar is an environment variable that was renamed. The old name keeps
// working for one release: when it is set and the new one is not, its value is
// carried over and a warning names the replacement.
type renamedVar struct {
	Old string
	New string
}

// renamedVars is the compatibility table. Entries stay for one release after
// the rename, then go.
//
// The plugin settings used to live under two prefixes: PLUGINS_ for the loader
// switches and PLUGIN_ for everything a plugin reaches through a host library.
// Nothing told an operator which name a given setting used, so the whole block
// now follows the single Plugins struct that holds it and is spelled PLUGINS_.
//
// PLUGINS_CACHE_ENABLED and PLUGINS_CACHE_DIR were renamed for the same
// reason: they configure the wazero compilation cache, not the gameap-cache
// host library that owns the PLUGINS_CACHE_ prefix now.
//
// No entry's New is another entry's Old, so the table does not depend on its
// own order.
var renamedVars = []renamedVar{
	{Old: "PLUGIN_STORE_URL", New: "PLUGINS_STORE_URL"},
	{Old: "PLUGIN_STORE_LICENSE_KEY", New: "PLUGINS_STORE_LICENSE_KEY"},
	{Old: "PLUGIN_HTTP_BLOCK_PRIVATE_IPS", New: "PLUGINS_HTTP_BLOCK_PRIVATE_IPS"},
	{Old: "PLUGIN_HTTP_ALLOWED_SCHEMES", New: "PLUGINS_HTTP_ALLOWED_SCHEMES"},
	{Old: "PLUGIN_HTTP_ALLOWED_HOSTS", New: "PLUGINS_HTTP_ALLOWED_HOSTS"},
	{Old: "PLUGIN_HTTP_MAX_TIMEOUT", New: "PLUGINS_HTTP_MAX_TIMEOUT"},
	{Old: "PLUGIN_HTTP_MAX_REDIRECTS", New: "PLUGINS_HTTP_MAX_REDIRECTS"},
	{Old: "PLUGIN_HTTP_RESPONSE_HEADER_ALLOWLIST", New: "PLUGINS_HTTP_RESPONSE_HEADER_ALLOWLIST"},
	{Old: "PLUGIN_SCHEDULER_MIN_INTERVAL", New: "PLUGINS_SCHEDULER_MIN_INTERVAL"},
	{Old: "PLUGIN_SCHEDULER_MAX_TASKS_PER_PLUGIN", New: "PLUGINS_SCHEDULER_MAX_TASKS_PER_PLUGIN"},
	{Old: "PLUGIN_SCHEDULER_CALL_TIMEOUT", New: "PLUGINS_SCHEDULER_CALL_TIMEOUT"},
	{Old: "PLUGIN_SCHEDULER_MAX_CALL_TIMEOUT", New: "PLUGINS_SCHEDULER_MAX_CALL_TIMEOUT"},
	{Old: "PLUGIN_SCHEDULER_MAX_RETRIES", New: "PLUGINS_SCHEDULER_MAX_RETRIES"},
	{Old: "PLUGIN_SCHEDULER_MAX_RETRY_DELAY", New: "PLUGINS_SCHEDULER_MAX_RETRY_DELAY"},
	{Old: "PLUGIN_SCHEDULER_MAX_JITTER", New: "PLUGINS_SCHEDULER_MAX_JITTER"},
	{Old: "PLUGIN_SCHEDULER_REFRESH_INTERVAL", New: "PLUGINS_SCHEDULER_REFRESH_INTERVAL"},
	{Old: "PLUGIN_SECRETS_MAX_KEYS_PER_PLUGIN", New: "PLUGINS_SECRETS_MAX_KEYS_PER_PLUGIN"},
	{Old: "PLUGIN_SECRETS_MAX_VALUE", New: "PLUGINS_SECRETS_MAX_VALUE"},
	{Old: "PLUGIN_SECRETS_REQUIRE_ENCRYPTION", New: "PLUGINS_SECRETS_REQUIRE_ENCRYPTION"},
	{Old: "PLUGIN_SSH_ENABLED", New: "PLUGINS_SSH_ENABLED"},
	{Old: "PLUGIN_SSH_BLOCK_PRIVATE_IPS", New: "PLUGINS_SSH_BLOCK_PRIVATE_IPS"},
	{Old: "PLUGIN_SSH_ALLOWED_HOSTS", New: "PLUGINS_SSH_ALLOWED_HOSTS"},
	{Old: "PLUGIN_SSH_ALLOW_ACCEPT_ANY_HOST_KEY", New: "PLUGINS_SSH_ALLOW_ACCEPT_ANY_HOST_KEY"},
	{Old: "PLUGIN_SSH_MAX_CONNECTIONS", New: "PLUGINS_SSH_MAX_CONNECTIONS"},
	{Old: "PLUGIN_SSH_MAX_OPERATIONS", New: "PLUGINS_SSH_MAX_OPERATIONS"},
	{Old: "PLUGIN_SSH_CONNECT_TIMEOUT", New: "PLUGINS_SSH_CONNECT_TIMEOUT"},
	{Old: "PLUGIN_SSH_MAX_EXEC_TIMEOUT", New: "PLUGINS_SSH_MAX_EXEC_TIMEOUT"},
	{Old: "PLUGIN_SSH_IDLE_TIMEOUT", New: "PLUGINS_SSH_IDLE_TIMEOUT"},
	{Old: "PLUGIN_SSH_MAX_OUTPUT_BYTES", New: "PLUGINS_SSH_MAX_OUTPUT_BYTES"},
	{Old: "PLUGIN_SSH_MAX_STDIN_BYTES", New: "PLUGINS_SSH_MAX_STDIN_BYTES"},
	{Old: "PLUGIN_SSH_OPERATION_RETENTION", New: "PLUGINS_SSH_OPERATION_RETENTION"},
	{Old: "PLUGIN_SSH_MAX_RETAINED_OPERATIONS", New: "PLUGINS_SSH_MAX_RETAINED_OPERATIONS"},
	{Old: "PLUGIN_SSH_KEEPALIVE_INTERVAL", New: "PLUGINS_SSH_KEEPALIVE_INTERVAL"},
	{Old: "PLUGIN_SSH_COMPLETION_CALL_TIMEOUT", New: "PLUGINS_SSH_COMPLETION_CALL_TIMEOUT"},
	{Old: "PLUGIN_SSH_BUSY_RETRY_DELAY", New: "PLUGINS_SSH_BUSY_RETRY_DELAY"},
	{Old: "PLUGIN_SSH_BUSY_RETRIES", New: "PLUGINS_SSH_BUSY_RETRIES"},
	{Old: "PLUGIN_NET_ENABLED", New: "PLUGINS_NET_ENABLED"},
	{Old: "PLUGIN_NET_BLOCK_PRIVATE_IPS", New: "PLUGINS_NET_BLOCK_PRIVATE_IPS"},
	{Old: "PLUGIN_NET_ALLOWED_HOSTS", New: "PLUGINS_NET_ALLOWED_HOSTS"},
	{Old: "PLUGIN_NET_MAX_TIMEOUT", New: "PLUGINS_NET_MAX_TIMEOUT"},
	{Old: "PLUGIN_NET_READ_BUFFER", New: "PLUGINS_NET_READ_BUFFER"},
	{Old: "PLUGIN_NET_MAX_CONNECTIONS", New: "PLUGINS_NET_MAX_CONNECTIONS"},
	{Old: "PLUGIN_RUNTIME_MAX_MEMORY", New: "PLUGINS_RUNTIME_MAX_MEMORY"},
	{Old: "PLUGIN_RUNTIME_MAX_MODULE_SIZE", New: "PLUGINS_RUNTIME_MAX_MODULE_SIZE"},
	{Old: "PLUGINS_CACHE_ENABLED", New: "PLUGINS_RUNTIME_CACHE_ENABLED"},
	{Old: "PLUGINS_CACHE_DIR", New: "PLUGINS_RUNTIME_CACHE_DIR"},
	{Old: "PLUGIN_PERMISSIONS_ENFORCE", New: "PLUGINS_PERMISSIONS_ENFORCE"},
	{Old: "PLUGIN_PERMISSIONS_CACHE_TTL", New: "PLUGINS_PERMISSIONS_CACHE_TTL"},
	{Old: "PLUGIN_RECOVERY_ENABLED", New: "PLUGINS_RECOVERY_ENABLED"},
	{Old: "PLUGIN_RECOVERY_INITIAL_DELAY", New: "PLUGINS_RECOVERY_INITIAL_DELAY"},
	{Old: "PLUGIN_RECOVERY_MAX_DELAY", New: "PLUGINS_RECOVERY_MAX_DELAY"},
	{Old: "PLUGIN_RECOVERY_MAX_ATTEMPTS", New: "PLUGINS_RECOVERY_MAX_ATTEMPTS"},
	{Old: "PLUGIN_SYNC_DISABLED", New: "PLUGINS_SYNC_DISABLED"},
	{Old: "PLUGIN_SYNC_REFRESH_INTERVAL", New: "PLUGINS_SYNC_REFRESH_INTERVAL"},
	{Old: "PLUGIN_SYNC_MIN_BACKOFF", New: "PLUGINS_SYNC_MIN_BACKOFF"},
	{Old: "PLUGIN_SYNC_MAX_BACKOFF", New: "PLUGINS_SYNC_MAX_BACKOFF"},
	{Old: "PLUGIN_NODEFS_MAX_INLINE", New: "PLUGINS_NODEFS_MAX_INLINE"},
	{Old: "PLUGIN_NODEFS_PATH_POLICY", New: "PLUGINS_NODEFS_PATH_POLICY"},
	{Old: "PLUGIN_NODEFS_ALLOWED_PATHS", New: "PLUGINS_NODEFS_ALLOWED_PATHS"},
	{Old: "PLUGIN_STORAGE_MAX_KEYS_PER_PLUGIN", New: "PLUGINS_STORAGE_MAX_KEYS_PER_PLUGIN"},
	{Old: "PLUGIN_STORAGE_MAX_VALUE", New: "PLUGINS_STORAGE_MAX_VALUE"},
	{Old: "PLUGIN_STORAGE_MAX_TOTAL", New: "PLUGINS_STORAGE_MAX_TOTAL"},
	{Old: "PLUGIN_CACHE_MAX_VALUE", New: "PLUGINS_CACHE_MAX_VALUE"},
	{Old: "PLUGIN_RATELIMIT_NODECMD_RPS", New: "PLUGINS_RATELIMIT_NODECMD_RPS"},
	{Old: "PLUGIN_RATELIMIT_NODECMD_BURST", New: "PLUGINS_RATELIMIT_NODECMD_BURST"},
	{Old: "PLUGIN_RATELIMIT_SERVERCONTROL_RPS", New: "PLUGINS_RATELIMIT_SERVERCONTROL_RPS"},
	{Old: "PLUGIN_RATELIMIT_SERVERCONTROL_BURST", New: "PLUGINS_RATELIMIT_SERVERCONTROL_BURST"},
	{Old: "PLUGIN_RATELIMIT_NODEFS_RPS", New: "PLUGINS_RATELIMIT_NODEFS_RPS"},
	{Old: "PLUGIN_RATELIMIT_NODEFS_BURST", New: "PLUGINS_RATELIMIT_NODEFS_BURST"},
	{Old: "PLUGIN_RATELIMIT_HTTP_RPS", New: "PLUGINS_RATELIMIT_HTTP_RPS"},
	{Old: "PLUGIN_RATELIMIT_HTTP_BURST", New: "PLUGINS_RATELIMIT_HTTP_BURST"},
	{Old: "PLUGIN_RATELIMIT_RBAC_RPS", New: "PLUGINS_RATELIMIT_RBAC_RPS"},
	{Old: "PLUGIN_RATELIMIT_RBAC_BURST", New: "PLUGINS_RATELIMIT_RBAC_BURST"},
	{Old: "PLUGIN_RATELIMIT_SSH_RPS", New: "PLUGINS_RATELIMIT_SSH_RPS"},
	{Old: "PLUGIN_RATELIMIT_SSH_BURST", New: "PLUGINS_RATELIMIT_SSH_BURST"},
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

		if err := os.Setenv(v.New, oldValue); err != nil {
			slog.Error("failed to apply deprecated environment variable",
				slog.String("deprecated", v.Old),
				slog.String("use", v.New),
				slog.String("error", err.Error()))

			continue
		}

		slog.Warn("environment variable is deprecated and will be removed in a future release",
			slog.String("deprecated", v.Old),
			slog.String("use", v.New))
	}
}
